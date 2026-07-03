package main

import (
	"errors"
	"fmt"

	kit "github.com/shellcade/kit/v2"
)

// Price tiers, left-to-right along the counter.
var standPrices = []int{1, 2, 5, 10}

// maxPayoutMultiplier is the declared casino payout ceiling (Meta.
// MaxPayoutMultiplier). The dearest headline jackpot is Cash Explosion's
// 300,000 gross on a $10 ticket = 30,000x, and every other ticket's top ratio
// is smaller (mega $5 24,000x, double $2 15,000x, tinnie $1 12,000x), so no
// honest jackpot is ever clamped.
const maxPayoutMultiplier = 30000

// Patron states (the per-player state machine; see SPEC §3).
const (
	stateCounter = iota // browsing the four stands
	stateStand          // browsing one stand's tickets
	stateCard           // scratching a bought card
	stateResult         // a resolved card; buy again or leave
	stateBust           // out of credits; rebuy beat
)

// patron is one player's view within the shared shop. The platform owns every
// balance now (svc.Credits); the fields below are display/leaderboard state
// only.
type patron struct {
	p          kit.Player
	balance    int64 // cached account-wide credits, refreshed per credits op / render
	postedPeak int64 // highest balance posted to the leaderboard
	state      int
	standIdx   int // 0..3 → standPrices
	ticketIdx  int // index within the current stand's tickets
	card       Card
	lastWin    int
	staked     bool   // an open Wager is awaiting Settle (leak guard)
	notice     string // transient one-line message (e.g. a refused rebuy)
	oos        bool   // the host has no economy / it is switched off
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
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:     cfg,
		svc:     svc,
		frame:   kit.NewFrame(),
		patrons: map[string]*patron{},
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
	pt := &patron{p: p, state: stateCounter}
	rm.patrons[p.AccountID] = pt
	rm.order = append(rm.order, p.AccountID)
	rm.refreshBalance(pt)
	pt.postedPeak = pt.balance
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	pt := rm.patrons[p.AccountID]
	if pt == nil {
		return
	}
	// A player who walks out mid-scratch has a Wager still open; book it as a
	// loss so the escrow never leaks (rule 3).
	rm.abandonStake(pt)
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
		// Walking away from an unscratched (or half-scratched) card abandons a
		// live Wager: settle it as a loss before dropping the card (rule 3).
		rm.abandonStake(pt)
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
		// The stake was already settled by maybeSettle on the way into
		// stateResult; abandonStake is a no-op here (staked == false) and only
		// guards against any future path that reaches result unsettled.
		rm.abandonStake(pt)
		pt.card = nil
		pt.state = stateStand
	}
}

// refreshBalance caches the player's account-wide credits for the HUD. It is
// the single read point (called after each credits op, and on join) so the hot
// render path never touches the host. A nil/disabled economy flips the patron
// to the out-of-service view instead of trapping.
func (rm *room) refreshBalance(pt *patron) {
	cr := rm.svc.Credits
	if cr == nil {
		pt.oos = true
		return
	}
	bal, err := cr.Balance(pt.p)
	if err != nil {
		if errors.Is(err, kit.ErrEconomyDisabled) {
			pt.oos = true
		}
		return
	}
	pt.oos = false
	pt.balance = bal
}

// abandonStake settles any open Wager as a loss (rule 3): a card left
// unresolved on leave/quit would otherwise leak its escrow. It is idempotent —
// once the stake is closed, staked is false and this is a no-op.
func (rm *room) abandonStake(pt *patron) {
	if !pt.staked {
		return
	}
	pt.staked = false
	if cr := rm.svc.Credits; cr != nil {
		_ = cr.Settle(pt.p, 0)
	}
	rm.refreshBalance(pt)
}

// buy opens ONE Wager for the ticket price and deals a fresh card. If the host
// refuses the stake for lack of funds, it routes to the rebuy beat and does NOT
// open a card (rule 1: exactly one open stake per ticket).
func (rm *room) buy(r kit.Room, pt *patron, t *Ticket) {
	pt.notice = ""
	cr := rm.svc.Credits
	if cr == nil {
		pt.oos = true
		return
	}
	if err := cr.Wager(pt.p, int64(t.Price)); err != nil {
		switch {
		case errors.Is(err, kit.ErrInsufficientCredits):
			rm.rebuy(pt)
		case errors.Is(err, kit.ErrEconomyDisabled):
			pt.oos = true
		default:
			// denied/temporarily-unavailable: the bet did not happen; stay put.
			pt.notice = "the till's jammed - try again in a sec"
		}
		return
	}
	pt.staked = true
	pt.card = buildCard(t, r.Rand())
	pt.lastWin = 0
	pt.state = stateCard
	rm.refreshBalance(pt)
}

// maybeSettle closes the ticket's single open stake with the GROSS payout once
// the card resolves, then advances to the result. Win() is already the
// stake-inclusive gross drawn at purchase, so it is Settled as-is (clamped to
// the declared ceiling for display honesty — no honest jackpot reaches it).
func (rm *room) maybeSettle(r kit.Room, pt *patron) {
	if pt.card == nil || !pt.card.Resolved() || !pt.staked {
		return
	}
	t := ticketsAtPrice(standPrices[pt.standIdx])[pt.ticketIdx]
	gross := int64(pt.card.Win())
	if lim := int64(t.Price) * maxPayoutMultiplier; gross > lim {
		gross = lim
	}
	cr := rm.svc.Credits
	if cr == nil {
		pt.oos = true
		return
	}
	if err := cr.Settle(pt.p, gross); err != nil {
		if errors.Is(err, kit.ErrEconomyDisabled) {
			pt.oos = true
		}
		// A denied/unavailable Settle leaves the stake open; keep staked=true so
		// a later abandon/leave still books it. Do not advance.
		return
	}
	pt.staked = false
	pt.lastWin = int(gross)
	rm.refreshBalance(pt)
	if gross > 0 && isBigWin(int(gross), t.Price) {
		rm.pushTicker(fmt.Sprintf("%s scored %s on %s!", pt.p.Handle, commaInt(int(gross)), t.Name))
	}
	// Leaderboard: peak account-wide credits, posted only on a new high.
	if pt.balance > pt.postedPeak {
		pt.postedPeak = pt.balance
		r.Post(kit.Result{Rankings: []kit.PlayerResult{{
			Player: pt.p, Metric: int(pt.balance), Status: kit.StatusFinished,
		}}})
	}
	pt.state = stateResult
}

// rebuy triggers the platform broke-relief Buyback and only enters the bust
// celebration on success. A refusal (still solvent, or the daily limit reached)
// surfaces as a notice — never retried (rule 5).
func (rm *room) rebuy(pt *patron) {
	cr := rm.svc.Credits
	if cr == nil {
		pt.oos = true
		return
	}
	bal, err := cr.Buyback(pt.p)
	if err != nil {
		if errors.Is(err, kit.ErrEconomyDisabled) {
			pt.oos = true
			return
		}
		// ErrInsufficientCredits: render it, do not retry.
		pt.balance = bal
		pt.notice = "no rebuy available right now"
		return
	}
	pt.balance = bal
	pt.state = stateBust
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
	if pt.oos {
		rm.drawOOS(f)
		return
	}
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
	if pt.notice != "" {
		f.Text(20, 3, "⚠ "+pt.notice, stBust)
	}
}

// drawOOS is the out-of-service screen shown when the host has no credits
// economy (svc.Credits == nil) or it is switched off (ErrEconomyDisabled).
func (rm *room) drawOOS(f *Frame) {
	f.Text(0, 1, "THE CORNER NEWSAGENT", stTitle)
	ruleRow(f, 1)
	box(f, 7, 18, 15, 61, stBust)
	f.Text(9, 24, "SHOP CLOSED", stBust)
	f.Text(11, 24, "the credits till is offline right now.", stDim)
	f.Text(13, 24, "pop back in a little while.", stDim)
	f.Text(23, 1, "[q] leave the shop", stHint)
}

func (rm *room) drawCounter(f *Frame, pt *patron) {
	drawChrome(f, "THE CORNER NEWSAGENT", int(pt.balance), rm.tickerLine(),
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
	drawChrome(f, fmt.Sprintf("$%d STAND · pick a ticket", price), int(pt.balance), rm.tickerLine(),
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
	drawChrome(f, title, int(pt.balance), rm.tickerLine(), hint)
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
	drawChrome(f, "THE CORNER NEWSAGENT", int(pt.balance), rm.tickerLine(),
		"[ENTER] back to the counter - have another crack")
	box(f, 7, 18, 15, 61, stBust)
	f.Text(9, 24, "OUT OF CREDITS", stBust)
	f.Text(11, 24, "the newsagent slides you a rebuy on the house.", stDim)
	f.Text(13, 24, fmt.Sprintf("✦  BACK TO %s CREDITS  ✦", commaInt(int(pt.balance))), stWin)
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
