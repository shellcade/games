package main

import (
	"fmt"
	"strings"
	"time"

	kit "github.com/shellcade/kit"
)

// Felt-table geometry (a casino table): a rounded felt frame with the dealer
// centred up top and up to five player seats along the rail.
const (
	feltTop    = 1
	feltBottom = 19
	dealerRow  = 4 // dealer card group occupies dealerRow..dealerRow+2
	taglineRow = 8

	seatNameRow = 11
	seatCardRow = 12 // seat card group occupies seatCardRow..seatCardRow+2
	seatValRow  = 15
	seatChipRow = 16
	actionRow   = 18
	slotW       = 15
	maxSeats    = 5

	// slideEdgeCol is the right-felt-edge column a dealt card slides in from.
	slideEdgeCol = kit.Cols - 6
)

var (
	stTitle  = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stDim    = kit.Style{FG: kit.DimGray}
	stPhase  = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stFelt   = kit.Style{FG: kit.Green}
	stOwn    = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	stActive = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stCard   = kit.Style{FG: kit.White}
	stRed    = kit.Style{FG: kit.Red, Attr: kit.AttrBold}
	stWin    = kit.Style{FG: kit.Green, Attr: kit.AttrBold}
	stLose   = kit.Style{FG: kit.Red, Attr: kit.AttrBold}
	stBack   = kit.Style{FG: kit.Yellow}                                       // card back while sliding/flipping
	stPrompt = kit.Style{FG: kit.RGB(0, 0, 0), BG: kit.Yellow, Attr: kit.AttrBold} // bright wait-state prompt
)

// render composes and sends a per-viewer frame to every member, reusing one
// long-lived frame (Send copies immediately, so the steady state is
// allocation-free).
func (rm *room) render(r kit.Room) {
	rm.lastNow = r.Now()
	for _, v := range r.Members() {
		rm.frame.Clear()
		rm.compose(rm.frame, v)
		r.Send(v, rm.frame)
	}
}

func (rm *room) compose(f *kit.Frame, v kit.Player) {
	active, _ := rm.firstUnresolved()

	// Title + phase/time above the felt.
	f.Text(0, 1, "♠♥♦♣ BLACKJACK", stTitle)
	phase := rm.phase
	if secs := rm.remaining(); secs > 0 {
		phase = fmt.Sprintf("%s · %s", rm.phase, clock(secs))
	}
	f.TextRight(0, kit.Cols-1, phase, stPhase)

	drawFelt(f, feltTop, feltBottom)
	center(f, feltTop, " B L A C K J A C K ", stFelt)

	// Dealer, centred near the top of the felt.
	center(f, dealerRow-1, "D E A L E R", stTitle)
	rm.drawDealer(f)

	center(f, taglineRow, "~ blackjack pays 3:2  ·  dealer stands on 17 ~", stFelt)

	// Seats along the rail, centred as a group.
	n := len(rm.order)
	if n > maxSeats {
		n = maxSeats
	}
	groupLeft := (kit.Cols - n*slotW) / 2
	for i := 0; i < n; i++ {
		s := rm.seats[rm.order[i]]
		if s == nil {
			continue
		}
		own := s.p.AccountID == v.AccountID
		isActive := active != nil && active.p.AccountID == s.p.AccountID
		rm.drawSeat(f, groupLeft+i*slotW, s, own, isActive)
	}

	rm.drawActionBar(f, v, active)

	f.Text(kit.Rows-1, 1, "Esc leave", stDim)
	if s := rm.seats[v.AccountID]; s != nil {
		f.TextRight(kit.Rows-1, kit.Cols-1, fmt.Sprintf("chips %d   HI %d", s.chips, s.highScore), stDim)
	}
}

func (rm *room) drawDealer(f *kit.Frame) {
	if len(rm.dealer) == 0 {
		center(f, dealerRow+1, "(waiting for bets)", stDim)
		return
	}
	hide := -1
	if rm.dealerHole {
		hide = 1
	}
	w := cardsWidth(len(rm.dealer))
	col := (kit.Cols - w) / 2
	drawCardsAnim(f, dealerRow, col, rm.dealer, hide, rm.dealerResolver())
	show := rm.dealer.total()
	label := fmt.Sprintf("(%d)", show)
	if rm.dealerHole {
		label = fmt.Sprintf("shows %d", hand{rm.dealer[0]}.total())
	} else if rm.dealer.isBlackjack() {
		label = "BLACKJACK"
	} else if rm.dealer.isBust() {
		label = "BUST"
	}
	f.Text(dealerRow+1, col+w+2, label, stDim)
}

func (rm *room) drawSeat(f *kit.Frame, slot int, s *seat, own, active bool) {
	if s == nil {
		return
	}
	nameSt := stCard
	switch {
	case active:
		nameSt = stActive
	case own:
		nameSt = stOwn
	}
	name := s.p.Handle
	if len(name) > slotW-2 {
		name = name[:slotW-2]
	}
	if active {
		// A doubled, highlighted marker so the active seat is unmissable; the
		// glyph degrades to ">>" on non-UTF-8 sessions (► -> >).
		name = "►►" + name
	}
	centerSlot(f, seatNameRow, slot, name, nameSt)

	if rm.phase == phBetting {
		status := "--"
		if s.placed {
			status = fmt.Sprintf("bet %d", s.bet)
		} else if own {
			status = fmt.Sprintf("bet %d?", s.bet)
		}
		centerSlot(f, seatCardRow+1, slot, status, stDim)
		centerSlot(f, seatChipRow, slot, fmt.Sprintf("$%d", s.chips), stDim)
		return
	}
	// A seat with no hand sat the round out (it never placed a bet). This keys off
	// the hand, not s.placed: settle() clears s.placed on every seat that played,
	// so a busted/played seat keeps its cards on the felt through results while a
	// genuine sat-out seat (never dealt in) says why rather than blanking.
	if len(s.hands) == 0 {
		centerSlot(f, seatCardRow+1, slot, "no bet", stDim)
		centerSlot(f, seatValRow, slot, "sat out", stDim)
		centerSlot(f, seatChipRow, slot, fmt.Sprintf("$%d", s.chips), stDim)
		return
	}

	// Draw the seat's hand(s) as joined card groups, left to right within the slot.
	col := slot + 1
	limit := slot + slotW
	var vals []string
	for hi, h := range s.hands {
		w := cardsWidth(len(h.cards))
		if col+w > limit && hi > 0 {
			vals = append(vals, "+")
			break
		}
		drawCardsAnim(f, seatCardRow, col, h.cards, -1, rm.seatResolver(s.p, hi, h))
		col += w + 1
		vals = append(vals, valueLabel(h.cards))
	}
	centerSlot(f, seatValRow, slot, strings.Join(vals, " "), valueStyle(s))
	centerSlot(f, seatChipRow, slot, fmt.Sprintf("$%d", s.chips), stDim)
	if rm.phase == phResults && s.result != "" {
		centerSlot(f, seatChipRow, slot, s.result, resultStyle(s.result))
	}
}

func (rm *room) drawActionBar(f *kit.Frame, v kit.Player, active *seat) {
	s := rm.seats[v.AccountID]
	if s == nil {
		return
	}
	var msg string
	st := stActive
	switch rm.phase {
	case phBetting:
		if !s.placed {
			// Prominent, highlighted call to bet for a viewer who hasn't yet.
			// ASCII-only so it reads identically on non-UTF-8 sessions.
			msg, st = "PLACE YOUR BET - Up/Down stake  SPACE bet", stPrompt
		} else if n := rm.unplacedCount(); n > 0 {
			noun := "player"
			if n != 1 {
				noun = "players"
			}
			msg, st = fmt.Sprintf("waiting on %d %s to bet - deals in %s", n, noun, clock(rm.remaining())), stDim
		} else {
			msg, st = "all bets in - dealing...", stPhase
		}
	case phInsurance:
		if s.placed && !s.insuranceDecided {
			msg = "Dealer shows an Ace - Insurance?   [Y]es   [N]o"
		} else {
			msg, st = "waiting for insurance...", stDim
		}
	case phTurns:
		switch {
		case active != nil && active.p.AccountID == v.AccountID:
			_, h := rm.firstUnresolved()
			// Highlighted YOUR TURN so the viewer can't miss that it's on them.
			msg, st = "YOUR TURN - "+legalActions(s, h), stPrompt
		case active != nil:
			msg, st = fmt.Sprintf("waiting on %s...", active.p.Handle), stDim
		}
	case phResults:
		msg, st = "round over - next hand shortly", stDim
	}
	if msg != "" {
		center(f, actionRow, msg, st)
	}
}

// unplacedCount is how many seated players have not yet placed a bet.
func (rm *room) unplacedCount() int {
	n := 0
	for _, s := range rm.seats {
		if !s.placed {
			n++
		}
	}
	return n
}

// legalActions lists the action prompts available for hand h.
func legalActions(s *seat, h *phand) string {
	if h == nil {
		return ""
	}
	parts := []string{"[H]it", "[S]tand"}
	first := len(h.cards) == 2 && !h.doubled
	if first && s.chips >= h.bet {
		parts = append(parts, "[D]ouble")
	}
	if first && h.cards[0].r == h.cards[1].r && s.chips >= h.bet && len(s.hands) < maxHands {
		parts = append(parts, "[P]split")
	}
	if first && len(s.hands) == 1 {
		parts = append(parts, "[R]surrender")
	}
	return strings.Join(parts, "  ")
}

// --- animation-aware card drawing ------------------------------------------

// animFor returns the active animation for the addressed card, if the schedule
// holds one that has not yet settled at the latest composed instant. It reads
// only the recorded schedule and the frozen clock — never the RNG.
func (rm *room) animFor(kind animKind, p kit.Player, handIdx, cardIdx int) (cardAnim, bool) {
	if len(rm.sched) == 0 || rm.lastNow.IsZero() {
		return cardAnim{}, false
	}
	for _, a := range rm.sched {
		if a.kind != kind || a.handIdx != handIdx || a.cardIdx != cardIdx {
			continue
		}
		if kind == animSeat && a.player.AccountID != p.AccountID {
			continue
		}
		if a.settled(rm.lastNow) {
			return cardAnim{}, false
		}
		return a, true
	}
	return cardAnim{}, false
}

// faceFromAnim turns a card's recorded animation into this frame's draw aspect.
func (rm *room) faceFromAnim(a cardAnim) cardFace {
	now := rm.lastNow
	if p := a.slideProgress(now); p < 1 {
		return cardFace{sliding: true, slideFrac: p}
	}
	frame, flipping := a.flipFrame(now)
	return cardFace{flip: frame, flipping: flipping}
}

// seatResolver returns a per-card animation resolver for one of a seat's hands.
// It also highlights the busting (final) card of a busted hand through results
// so a bust stays visible and explained rather than the seat blanking.
func (rm *room) seatResolver(p kit.Player, handIdx int, h *phand) func(i int) cardFace {
	bustIdx := -1
	if h.cards.isBust() {
		bustIdx = len(h.cards) - 1
	}
	return func(i int) cardFace {
		var face cardFace
		if a, ok := rm.animFor(animSeat, p, handIdx, i); ok {
			face = rm.faceFromAnim(a)
		}
		if i == bustIdx && !face.sliding {
			face.highlight = stLose
			face.hasHL = true
		}
		return face
	}
}

// dealerResolver returns the dealer row's per-card animation resolver.
func (rm *room) dealerResolver() func(i int) cardFace {
	bustIdx := -1
	if !rm.dealerHole && rm.dealer.isBust() {
		bustIdx = len(rm.dealer) - 1
	}
	return func(i int) cardFace {
		var face cardFace
		if a, ok := rm.animFor(animDealer, kit.Player{}, 0, i); ok {
			face = rm.faceFromAnim(a)
		}
		if i == bustIdx && !face.sliding {
			face.highlight = stLose
			face.hasHL = true
		}
		return face
	}
}

// drawCardBack renders a face-down card box (4 wide) at (row, col). It degrades
// to ASCII on non-UTF-8 sessions via the renderer's box-drawing fallback.
func drawCardBack(f *kit.Frame, row, col int) {
	f.SetRune(row, col, '┌', stBack)
	f.SetRune(row, col+1, '─', stBack)
	f.SetRune(row, col+2, '─', stBack)
	f.SetRune(row, col+3, '┐', stBack)
	f.SetRune(row+1, col, '│', stBack)
	f.SetRune(row+1, col+1, '●', stBack)
	f.SetRune(row+1, col+2, '●', stBack)
	f.SetRune(row+1, col+3, '│', stBack)
	f.SetRune(row+2, col, '└', stBack)
	f.SetRune(row+2, col+1, '─', stBack)
	f.SetRune(row+2, col+2, '─', stBack)
	f.SetRune(row+2, col+3, '┘', stBack)
}

// --- drawing helpers -------------------------------------------------------

func cardsWidth(n int) int {
	if n == 0 {
		return 3
	}
	return 3*n + 1
}

// faceFor is the per-card draw decision returned by an animation resolver: how a
// card should render this frame given the schedule and the frozen clock.
type cardFace struct {
	sliding   bool      // card is gliding in; render it as a floating back box
	slideFrac float64   // [0,1] slide progress (only when sliding)
	flip      int       // 0=back,1=edge,2=face (only meaningful when !sliding)
	flipping  bool      // mid-flip back/edge in place
	hasHL     bool      // highlight overrides the face style (e.g. the busting card)
	highlight kit.Style // the override style (valid when hasHL)
}

// drawCardsAnim renders the joined card group, consulting resolve(i) for each
// card's animation aspect. resolve may be nil (the static layout). hideIdx
// conceals a card as "??" (the dealer hole card).
func drawCardsAnim(f *kit.Frame, row, col int, cards hand, hideIdx int, resolve func(i int) cardFace) {
	if len(cards) == 0 {
		f.Text(row+1, col, "( )", stDim)
		return
	}
	// The group spans only the contiguous prefix of cards that have arrived
	// (slide complete). A still-sliding tail card floats in separately so the
	// joined box never has to render a gap.
	arrived := len(cards)
	if resolve != nil {
		for i := range cards {
			if resolve(i).sliding {
				arrived = i
				break
			}
		}
	}
	if arrived > 0 {
		f.SetRune(row, col, '┌', stFelt)
		f.SetRune(row+1, col, '│', stFelt)
		f.SetRune(row+2, col, '└', stFelt)
	}
	c := col + 1
	for i := 0; i < arrived; i++ {
		cd := cards[i]
		f.SetRune(row, c, '─', stFelt)
		f.SetRune(row, c+1, '─', stFelt)
		f.SetRune(row+2, c, '─', stFelt)
		f.SetRune(row+2, c+1, '─', stFelt)
		var face cardFace
		if resolve != nil {
			face = resolve(i)
		}
		switch {
		case i == hideIdx:
			f.SetRune(row+1, c, '?', stDim)
			f.SetRune(row+1, c+1, '?', stDim)
		case face.flipping && face.flip == 0:
			f.SetRune(row+1, c, '●', stBack)
			f.SetRune(row+1, c+1, '●', stBack)
		case face.flipping && face.flip == 1:
			f.SetRune(row+1, c, ' ', stBack)
			f.SetRune(row+1, c+1, '│', stBack)
		default:
			st := stCard
			if face.hasHL {
				st = face.highlight
			}
			f.Text(row+1, c, cd.r.boxLabel(), st)
			pipSt := st
			if !face.hasHL && cd.s.red() {
				pipSt = stRed
			}
			f.SetRune(row+1, c+1, cd.s.pip(), pipSt)
		}
		c += 2
		top, mid, bot := '┬', '│', '┴'
		if i == arrived-1 {
			top, mid, bot = '┐', '│', '┘'
		}
		f.SetRune(row, c, top, stFelt)
		f.SetRune(row+1, c, mid, stFelt)
		f.SetRune(row+2, c, bot, stFelt)
		c++
	}
	// Float any still-sliding cards in from the right felt edge toward the slot
	// they will occupy once the group grows to include them.
	if resolve != nil {
		for i := arrived; i < len(cards); i++ {
			face := resolve(i)
			if !face.sliding {
				continue
			}
			// Settled left-border column of card i within the eventual group.
			target := col + 1 + i*3 - 1
			cur := lerpCol(slideEdgeCol, target, face.slideFrac)
			drawCardBack(f, row, cur)
		}
	}
}

// lerpCol linearly interpolates a column between from and to at fraction t.
func lerpCol(from, to int, t float64) int {
	return from + int(float64(to-from)*t+0.5)
}

func drawFelt(f *kit.Frame, top, bot int) {
	f.SetRune(top, 0, '╭', stFelt)
	f.SetRune(top, kit.Cols-1, '╮', stFelt)
	f.SetRune(bot, 0, '╰', stFelt)
	f.SetRune(bot, kit.Cols-1, '╯', stFelt)
	for c := 1; c < kit.Cols-1; c++ {
		f.SetRune(top, c, '─', stFelt)
		f.SetRune(bot, c, '─', stFelt)
	}
	for r := top + 1; r < bot; r++ {
		f.SetRune(r, 0, '│', stFelt)
		f.SetRune(r, kit.Cols-1, '│', stFelt)
	}
}

func center(f *kit.Frame, row int, s string, st kit.Style) {
	f.Text(row, (kit.Cols-len([]rune(s)))/2, s, st)
}

// centerSlot centres s within a slotW-wide column starting at slot.
func centerSlot(f *kit.Frame, row, slot int, s string, st kit.Style) {
	n := len([]rune(s))
	if n > slotW {
		s = string([]rune(s)[:slotW])
		n = slotW
	}
	f.Text(row, slot+(slotW-n)/2, s, st)
}

func (rm *room) remaining() int {
	if rm.deadline.IsZero() || rm.lastNow.IsZero() {
		return 0
	}
	d := rm.deadline.Sub(rm.lastNow)
	if d <= 0 {
		return 0
	}
	return int((d + time.Second - 1) / time.Second)
}

func clock(secs int) string { return fmt.Sprintf("0:%02d", secs) }

func valueLabel(h hand) string {
	if h.isBlackjack() {
		return "BJ"
	}
	total, soft := h.value()
	if total > 21 {
		return "BUST"
	}
	if soft {
		return fmt.Sprintf("s%d", total)
	}
	return fmt.Sprintf("%d", total)
}

func valueStyle(s *seat) kit.Style {
	for _, h := range s.hands {
		if h.cards.isBust() {
			return stLose
		}
		if h.cards.isBlackjack() {
			return stWin
		}
	}
	return stCard
}

func resultStyle(result string) kit.Style {
	switch {
	case strings.HasPrefix(result, "WIN"):
		return stWin
	case strings.HasPrefix(result, "LOSE"), strings.HasPrefix(result, "BUST"):
		return stLose
	default:
		return stDim
	}
}
