package main

import (
	"fmt"

	kit "github.com/shellcade/kit/v2"
)

// Price tiers, left-to-right along the counter.
var standPrices = []int{1, 2, 5, 10}

// Patron states (the per-player state machine; see SPEC §3).
const (
	stateCounter = iota // browsing the four stands
	stateStand          // browsing one stand's tickets
	stateCard           // scratching a bought card
	stateResult         // a resolved card; buy again or leave
	stateBust           // out of credits; rebuy beat
)

// patron is one player's view & wallet within the shared shop.
type patron struct {
	p          kit.Player
	balance    int
	peak       int
	postedPeak int
	state      int
	standIdx   int // 0..3 → standPrices
	ticketIdx  int // index within the current stand's tickets
	card       Card
	lastWin    int
}

// room is the shared newsagent floor.
type room struct {
	kit.Base
	cfg     kit.RoomConfig
	svc     kit.Services
	frame   *Frame
	patrons map[string]*patron
	order   []string
	ticker  []string // recent big wins, newest last

	// sk standardises the durable-wallet KV writes (PersistWallet), replacing
	// the duplicated persistWallet helper. The leaderboard Post stays
	// hand-rolled below because postedPeak is seeded from the durable peak at
	// join — so a returning player only posts on a NEW personal best, which
	// ScoreKeeper.Record (always posts the first observed value) would not
	// preserve.
	sk *kit.ScoreKeeper
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:     cfg,
		svc:     svc,
		frame:   kit.NewFrame(),
		patrons: map[string]*patron{},
		sk:      kit.NewScoreKeeper(kit.OnImprove),
	}
}

func (rm *room) OnStart(r kit.Room) {
	r.SetInputContext(kit.CtxNav)
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	if _, ok := rm.patrons[p.AccountID]; ok {
		rm.patrons[p.AccountID].p = p
		rm.render(r)
		return
	}
	bal, peak := seedWallet(r, p)
	rm.patrons[p.AccountID] = &patron{p: p, balance: bal, peak: peak, postedPeak: peak, state: stateCounter}
	rm.order = append(rm.order, p.AccountID)
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	pt := rm.patrons[p.AccountID]
	if pt == nil {
		return
	}
	rm.persistWallet(r, p, pt.balance, pt.peak)
	delete(rm.patrons, p.AccountID)
	for i, id := range rm.order {
		if id == p.AccountID {
			rm.order = append(rm.order[:i], rm.order[i+1:]...)
			break
		}
	}
	rm.render(r)
}

func (rm *room) OnWake(r kit.Room) { rm.render(r) }

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	pt := rm.patrons[p.AccountID]
	if pt == nil {
		return
	}
	switch pt.state {
	case stateCounter:
		rm.inputCounter(r, pt, in)
	case stateStand:
		rm.inputStand(r, pt, in)
	case stateCard:
		rm.inputCard(r, pt, in)
	case stateResult:
		rm.inputResult(r, pt, in)
	case stateBust:
		if kit.Resolve(in, kit.CtxNav) == kit.ActConfirm {
			pt.state = stateCounter
		}
	}
	rm.render(r)
}

func (rm *room) inputCounter(r kit.Room, pt *patron, in kit.Input) {
	switch kit.Resolve(in, kit.CtxNav) {
	case kit.ActLeft:
		if pt.standIdx > 0 {
			pt.standIdx--
		}
	case kit.ActRight:
		if pt.standIdx < len(standPrices)-1 {
			pt.standIdx++
		}
	case kit.ActConfirm:
		pt.ticketIdx = 0
		pt.state = stateStand
	}
}

func (rm *room) inputStand(r kit.Room, pt *patron, in kit.Input) {
	list := ticketsAtPrice(standPrices[pt.standIdx])
	switch kit.Resolve(in, kit.CtxNav) {
	case kit.ActUp:
		if pt.ticketIdx > 0 {
			pt.ticketIdx--
		}
	case kit.ActDown:
		if pt.ticketIdx < len(list)-1 {
			pt.ticketIdx++
		}
	case kit.ActBack:
		pt.state = stateCounter
	case kit.ActConfirm:
		rm.buy(r, pt, list[pt.ticketIdx])
	}
}

func (rm *room) inputCard(r kit.Room, pt *patron, in kit.Input) {
	if in.Kind == kit.InputRune {
		switch in.Rune {
		case ' ':
			pt.card.Scratch()
			rm.maybeSettle(r, pt)
			return
		case 'a', 'A':
			pt.card.ScratchAll()
			rm.maybeSettle(r, pt)
			return
		}
	}
	switch kit.Resolve(in, kit.CtxNav) {
	case kit.ActUp:
		pt.card.Move(0, -1)
	case kit.ActDown:
		pt.card.Move(0, 1)
	case kit.ActLeft:
		pt.card.Move(-1, 0)
	case kit.ActRight:
		pt.card.Move(1, 0)
	case kit.ActBack:
		pt.card = nil
		pt.state = stateStand
	}
}

func (rm *room) inputResult(r kit.Room, pt *patron, in kit.Input) {
	switch kit.Resolve(in, kit.CtxNav) {
	case kit.ActConfirm:
		list := ticketsAtPrice(standPrices[pt.standIdx])
		rm.buy(r, pt, list[pt.ticketIdx])
	case kit.ActBack:
		pt.card = nil
		pt.state = stateStand
	}
}

// buy deducts the ticket price and opens a fresh card. If the patron can't
// afford even a $1 ticket, it triggers the rebuy beat instead.
func (rm *room) buy(r kit.Room, pt *patron, t *Ticket) {
	if pt.balance < t.Price {
		if pt.balance < standPrices[0] {
			pt.balance = rebuyAmount
			pt.state = stateBust
		}
		return
	}
	pt.balance -= t.Price
	pt.card = buildCard(t, r.Rand())
	pt.lastWin = 0
	pt.state = stateCard
}

// maybeSettle credits the wallet and advances to the result once the card has
// resolved.
func (rm *room) maybeSettle(r kit.Room, pt *patron) {
	if pt.card == nil || !pt.card.Resolved() {
		return
	}
	win := pt.card.Win()
	pt.lastWin = win
	pt.balance += win
	if pt.balance > pt.peak {
		pt.peak = pt.balance
	}
	if win > 0 {
		t := ticketsAtPrice(standPrices[pt.standIdx])[pt.ticketIdx]
		if isBigWin(win, t.Price) {
			rm.pushTicker(fmt.Sprintf("%s scored %s on %s!", pt.p.Handle, commaInt(win), t.Name))
		}
	}
	if pt.peak > pt.postedPeak {
		pt.postedPeak = pt.peak
		r.Post(kit.Result{Rankings: []kit.PlayerResult{{
			Player: pt.p, Metric: pt.peak, Status: kit.StatusFinished,
		}}})
	}
	pt.state = stateResult
}

// isBigWin reports whether a win clears the room-wide announce threshold.
func isBigWin(win, price int) bool { return win >= 500 && win >= 50*price }

func (rm *room) pushTicker(msg string) {
	rm.ticker = append(rm.ticker, msg)
	if len(rm.ticker) > 5 {
		rm.ticker = rm.ticker[len(rm.ticker)-5:]
	}
}

func (rm *room) tickerLine() string {
	if len(rm.ticker) == 0 {
		return ""
	}
	out := ""
	for i, m := range rm.ticker {
		if i > 0 {
			out += " · "
		}
		out += m
	}
	if len([]rune(out)) > kit.Cols-6 {
		out = string([]rune(out)[:kit.Cols-6])
	}
	return out
}

// ticketsAtPrice returns pointers to the catalog tickets at the given price,
// in catalog order.
func ticketsAtPrice(price int) []*Ticket {
	var out []*Ticket
	for i := range tickets {
		if tickets[i].Price == price {
			out = append(out, &tickets[i])
		}
	}
	return out
}

// --- rendering ---------------------------------------------------------------

func (rm *room) render(r kit.Room) {
	for _, p := range r.Members() {
		pt := rm.patrons[p.AccountID]
		if pt == nil {
			continue
		}
		rm.frame.Clear()
		rm.compose(rm.frame, pt)
		r.Send(p, rm.frame)
	}
}

func (rm *room) compose(f *Frame, pt *patron) {
	switch pt.state {
	case stateCounter:
		rm.drawCounter(f, pt)
	case stateStand:
		rm.drawStand(f, pt)
	case stateCard, stateResult:
		rm.drawCard(f, pt)
	case stateBust:
		rm.drawBust(f, pt)
	}
}

func (rm *room) drawCounter(f *Frame, pt *patron) {
	drawChrome(f, "THE CORNER NEWSAGENT", pt.balance, rm.tickerLine(),
		"◂ ▸ choose a stand     [ENTER] step up to it     [q] leave the shop")
	f.Text(3, 3, "★ INSTANT SCRATCH-ITS ★", stTitle)
	for i, price := range standPrices {
		x := 3 + i*18
		st := stDim
		if i == pt.standIdx {
			st = stSel
		}
		box(f, 6, x, 11, x+13, st)
		f.Text(6, x+2, fmt.Sprintf(" $%d ", price), stPrice)
		f.Text(8, x+2, "▒▒▒  ▒▒▒", stLatex)
		f.Text(9, x+2, "▒▒▒  ▒▒▒", stLatex)
		f.Text(10, x+2, "4 tickets", stDim)
	}
}

func (rm *room) drawStand(f *Frame, pt *patron) {
	price := standPrices[pt.standIdx]
	drawChrome(f, fmt.Sprintf("$%d STAND · pick a ticket", price), pt.balance, rm.tickerLine(),
		fmt.Sprintf("▲ ▼ choose ticket     [ENTER] buy $%d     [q] back to counter", price))
	list := ticketsAtPrice(price)
	for i, t := range list {
		row := 4 + i*2
		marker := "  "
		st := stReveal
		if i == pt.ticketIdx {
			marker = "▸ "
			st = stSel
		}
		f.Text(row, 3, marker+t.Name, st)
		f.Text(row, 24, mechanicBlurb(t.Mechanic), stDim)
		f.TextRight(row, kit.Cols-3, fmt.Sprintf("top %s", commaInt(topPrize(t))), stDim)
	}
}

func (rm *room) drawCard(f *Frame, pt *patron) {
	title := pt.card.Title()
	hint := "←↑↓→ move coin    [SPACE] scratch    [a] scratch all    [q] leave it"
	if pt.state == stateResult {
		if pt.lastWin > 0 {
			title += " · ★ WINNER ★"
		} else {
			title += " · no win"
		}
		hint = "[ENTER] buy another     [q] back to the stand"
	}
	drawChrome(f, title, pt.balance, rm.tickerLine(), hint)
	pt.card.Render(f, 3)
	if pt.state == stateResult {
		if pt.lastWin > 0 {
			f.Text(19, 3, fmt.Sprintf("✦ ✦ ✦   WON %s CREDITS   ✦ ✦ ✦", commaInt(pt.lastWin)), stWin)
		} else {
			f.Text(19, 3, "no win - better luck on the next one", stDim)
		}
	}
}

func (rm *room) drawBust(f *Frame, pt *patron) {
	drawChrome(f, "THE CORNER NEWSAGENT", pt.balance, rm.tickerLine(),
		"[ENTER] back to the counter - have another crack")
	box(f, 7, 18, 15, 61, stBust)
	f.Text(9, 24, "OUT OF CREDITS", stBust)
	f.Text(11, 24, "the newsagent slides you a fresh twenty.", stDim)
	f.Text(13, 24, fmt.Sprintf("✦  + %s CREDITS  ✦", commaInt(rebuyAmount)), stWin)
}

// mechanicBlurb is the one-line stand description per mechanic.
func mechanicBlurb(m MechanicKind) string {
	switch m {
	case MechMatch3:
		return "match three equal amounts"
	case MechKeyNum:
		return "match the winning numbers"
	case MechMult:
		return "find a prize, then multiply"
	case MechFind:
		return "find three symbols"
	case MechLines:
		return "three in a line"
	case MechCrossword:
		return "complete the words"
	case MechBingo:
		return "mark a bingo line"
	case MechShowdown:
		return "beat the house"
	case MechTriple:
		return "spell the bonus words"
	}
	return ""
}

// topPrize returns a ticket's largest prize (the headline jackpot).
func topPrize(t *Ticket) int {
	top := 0
	for _, row := range t.Prizes {
		if row.Credits > top {
			top = row.Credits
		}
	}
	return top
}
