package main

import (
	"strings"
	"time"

	kit "github.com/shellcade/kit/v2"
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
	if secs := rm.remaining(); secs > 0 {
		// "<phase> · <clock>" right-anchored: compute width, then write l-to-r.
		w := runeLen(rm.phase) + runeLen(" · ") + clockWidth(secs)
		c := f.Text(0, kit.Cols-1-w+1, rm.phase, stPhase)
		c = f.Text(0, c, " · ", stPhase)
		putClock(f, 0, c, secs, stPhase)
	} else {
		f.TextRight(0, kit.Cols-1, rm.phase, stPhase)
	}

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
		// "chips %d   HI %d" right-anchored ending at Cols-1.
		w := runeLen("chips ") + intWidth(s.chips) + runeLen("   HI ") + intWidth(s.highScore)
		c := f.Text(kit.Rows-1, kit.Cols-1-w+1, "chips ", stDim)
		c = putInt(f, kit.Rows-1, c, s.chips, stDim)
		c = f.Text(kit.Rows-1, c, "   HI ", stDim)
		putInt(f, kit.Rows-1, c, s.highScore, stDim)
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
	lc := col + w + 2
	if rm.dealerHole {
		// "shows %d" — the dealer up card's total only.
		c := f.Text(dealerRow+1, lc, "shows ", stDim)
		putInt(f, dealerRow+1, c, hand{rm.dealer[0]}.total(), stDim)
	} else if rm.dealer.isBlackjack() {
		f.Text(dealerRow+1, lc, "BLACKJACK", stDim)
	} else if rm.dealer.isBust() {
		f.Text(dealerRow+1, lc, "BUST", stDim)
	} else {
		// "(%d)" — the dealer's full total.
		c := f.Text(dealerRow+1, lc, "(", stDim)
		c = putInt(f, dealerRow+1, c, rm.dealer.total(), stDim)
		f.Text(dealerRow+1, c, ")", stDim)
	}
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
		// glyph degrades to ">>" on non-UTF-8 sessions (► -> >). Written as a
		// "►►"+name composite centred in the slot, without the allocating concat.
		w := runeLen("►►") + runeLen(name)
		c := f.Text(seatNameRow, slotStart(slot, w), "►►", nameSt)
		f.Text(seatNameRow, c, name, nameSt)
	} else {
		centerSlot(f, seatNameRow, slot, name, nameSt)
	}

	if rm.phase == phBetting {
		if s.placed {
			// "bet %d" centred in the slot.
			w := runeLen("bet ") + intWidth(s.bet)
			c := f.Text(seatCardRow+1, slotStart(slot, w), "bet ", stDim)
			putInt(f, seatCardRow+1, c, s.bet, stDim)
		} else if own {
			// "bet %d?" centred in the slot.
			w := runeLen("bet ") + intWidth(s.bet) + 1
			c := f.Text(seatCardRow+1, slotStart(slot, w), "bet ", stDim)
			c = putInt(f, seatCardRow+1, c, s.bet, stDim)
			f.Text(seatCardRow+1, c, "?", stDim)
		} else {
			centerSlot(f, seatCardRow+1, slot, "--", stDim)
		}
		putChips(f, seatChipRow, slot, s.chips, stDim)
		return
	}
	// A seat with no hand sat the round out (it never placed a bet). This keys off
	// the hand, not s.placed: settle() clears s.placed on every seat that played,
	// so a busted/played seat keeps its cards on the felt through results while a
	// genuine sat-out seat (never dealt in) says why rather than blanking.
	if len(s.hands) == 0 {
		centerSlot(f, seatCardRow+1, slot, "no bet", stDim)
		centerSlot(f, seatValRow, slot, "sat out", stDim)
		putChips(f, seatChipRow, slot, s.chips, stDim)
		return
	}

	// Draw the seat's hand(s) as joined card groups, left to right within the
	// slot. The drawn count is the prefix of hands that fit; an overflowing tail
	// shows a "+" marker. We mirror the value-list join (labels separated by
	// single spaces) without allocating: compute its width, then write directly.
	col := slot + 1
	limit := slot + slotW
	drawn := 0 // hands actually drawn (card groups rendered)
	overflow := false
	for hi, h := range s.hands {
		w := cardsWidth(len(h.cards))
		if col+w > limit && hi > 0 {
			overflow = true
			break
		}
		drawCardsAnim(f, seatCardRow, col, h.cards, -1, rm.seatResolver(s.p, hi, h))
		col += w + 1
		drawn++
	}
	// Joined value-label width: each drawn hand's label, plus a "+" if overflowed,
	// separated by single spaces (matching strings.Join(vals, " ")).
	valW := 0
	for i := 0; i < drawn; i++ {
		if i > 0 {
			valW++ // separating space
		}
		valW += valueLabelWidth(s.hands[i].cards)
	}
	if overflow {
		if drawn > 0 {
			valW++ // separating space
		}
		valW++ // "+"
	}
	// Write the joined labels centred within the slot.
	valSt := valueStyle(s)
	vc := slotStart(slot, valW)
	for i := 0; i < drawn; i++ {
		if i > 0 {
			f.SetRune(seatValRow, vc, ' ', valSt)
			vc++
		}
		vc = putValueLabel(f, seatValRow, vc, s.hands[i].cards, valSt)
	}
	if overflow {
		if drawn > 0 {
			f.SetRune(seatValRow, vc, ' ', valSt)
			vc++
		}
		f.SetRune(seatValRow, vc, '+', valSt)
		vc++
	}
	putChips(f, seatChipRow, slot, s.chips, stDim)
	if rm.phase == phResults && s.result != "" {
		centerSlot(f, seatChipRow, slot, s.result, resultStyle(s.result))
	}
}

func (rm *room) drawActionBar(f *kit.Frame, v kit.Player, active *seat) {
	s := rm.seats[v.AccountID]
	if s == nil {
		return
	}
	switch rm.phase {
	case phBetting:
		if !s.placed {
			// Prominent, highlighted call to bet for a viewer who hasn't yet.
			// ASCII-only so it reads identically on non-UTF-8 sessions.
			center(f, actionRow, "PLACE YOUR BET - Up/Down stake  SPACE bet", stPrompt)
		} else if n := rm.unplacedCount(); n > 0 {
			// "waiting on %d %s to bet - deals in %s" centred, written without
			// allocating: compute the width, then write the pieces directly.
			noun := "player"
			if n != 1 {
				noun = "players"
			}
			secs := rm.remaining()
			w := runeLen("waiting on ") + intWidth(n) + 1 + runeLen(noun) +
				runeLen(" to bet - deals in ") + clockWidth(secs)
			c := (kit.Cols - w) / 2
			c = f.Text(actionRow, c, "waiting on ", stDim)
			c = putInt(f, actionRow, c, n, stDim)
			f.SetRune(actionRow, c, ' ', stDim)
			c++
			c = f.Text(actionRow, c, noun, stDim)
			c = f.Text(actionRow, c, " to bet - deals in ", stDim)
			putClock(f, actionRow, c, secs, stDim)
		} else {
			center(f, actionRow, "all bets in - dealing...", stPhase)
		}
	case phInsurance:
		if s.placed && !s.insuranceDecided {
			center(f, actionRow, "Dealer shows an Ace - Insurance?   [Y]es   [N]o", stActive)
		} else {
			center(f, actionRow, "waiting for insurance...", stDim)
		}
	case phTurns:
		switch {
		case active != nil && active.p.AccountID == v.AccountID:
			_, h := rm.firstUnresolved()
			// Highlighted "YOUR TURN - " + the legal-action prompt, centred and
			// written piece-by-piece (the actions joined by "  ") with no alloc.
			w := runeLen("YOUR TURN - ") + legalActionsWidth(s, h)
			c := (kit.Cols - w) / 2
			c = f.Text(actionRow, c, "YOUR TURN - ", stPrompt)
			putLegalActions(f, actionRow, c, s, h, stPrompt)
		case active != nil:
			// "waiting on %s..." centred, written directly (literal, handle, "...").
			w := runeLen("waiting on ") + runeLen(active.p.Handle) + runeLen("...")
			c := (kit.Cols - w) / 2
			c = f.Text(actionRow, c, "waiting on ", stDim)
			c = f.Text(actionRow, c, active.p.Handle, stDim)
			f.Text(actionRow, c, "...", stDim)
		}
	case phResults:
		center(f, actionRow, "round over - next hand shortly", stDim)
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

// eachLegalAction calls fn for each action prompt available for hand h, in the
// same order legalActions used to list them. Shared by legalActionsWidth and
// putLegalActions so the width and the writes can never diverge.
func eachLegalAction(s *seat, h *phand, fn func(label string)) {
	if h == nil {
		return
	}
	fn("[H]it")
	fn("[S]tand")
	first := len(h.cards) == 2 && !h.doubled
	if first && s.chips >= h.bet {
		fn("[D]ouble")
	}
	if first && h.cards[0].r == h.cards[1].r && s.chips >= h.bet && len(s.hands) < maxHands {
		fn("[P]split")
	}
	if first && len(s.hands) == 1 {
		fn("[R]surrender")
	}
}

// legalActionsWidth returns the column width of the action prompts joined by
// two-space separators (matching the old strings.Join(parts, "  ")).
func legalActionsWidth(s *seat, h *phand) int {
	w, n := 0, 0
	eachLegalAction(s, h, func(label string) {
		if n > 0 {
			w += 2 // "  " separator
		}
		w += runeLen(label)
		n++
	})
	return w
}

// putLegalActions writes the action prompts (joined by "  ") at (row, col) and
// returns the next column, allocating nothing.
func putLegalActions(f *kit.Frame, row, col int, s *seat, h *phand, st kit.Style) int {
	n := 0
	eachLegalAction(s, h, func(label string) {
		if n > 0 {
			col = f.Text(row, col, "  ", st)
		}
		col = f.Text(row, col, label, st)
		n++
	})
	return col
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

// cardResolver carries the per-card draw decision inputs as a plain value so
// drawCardsAnim can consult it WITHOUT a heap-allocating closure (a func literal
// capturing rm/p/bustIdx escapes to the heap, which leaks under -gc=leaking).
// active=false means the static layout (no animation, no bust highlight).
type cardResolver struct {
	rm      *room
	active  bool
	kind    animKind
	player  kit.Player
	handIdx int
	bustIdx int
}

// face computes the draw aspect for card i, mirroring the old resolver closures.
func (cr cardResolver) face(i int) cardFace {
	var face cardFace
	if !cr.active {
		return face
	}
	if a, ok := cr.rm.animFor(cr.kind, cr.player, cr.handIdx, i); ok {
		face = cr.rm.faceFromAnim(a)
	}
	if i == cr.bustIdx && !face.sliding {
		face.highlight = stLose
		face.hasHL = true
	}
	return face
}

// seatResolver returns a per-card animation resolver for one of a seat's hands.
// It also highlights the busting (final) card of a busted hand through results
// so a bust stays visible and explained rather than the seat blanking.
func (rm *room) seatResolver(p kit.Player, handIdx int, h *phand) cardResolver {
	bustIdx := -1
	if h.cards.isBust() {
		bustIdx = len(h.cards) - 1
	}
	return cardResolver{rm: rm, active: true, kind: animSeat, player: p, handIdx: handIdx, bustIdx: bustIdx}
}

// dealerResolver returns the dealer row's per-card animation resolver.
func (rm *room) dealerResolver() cardResolver {
	bustIdx := -1
	if !rm.dealerHole && rm.dealer.isBust() {
		bustIdx = len(rm.dealer) - 1
	}
	return cardResolver{rm: rm, active: true, kind: animDealer, handIdx: 0, bustIdx: bustIdx}
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
func drawCardsAnim(f *kit.Frame, row, col int, cards hand, hideIdx int, resolve cardResolver) {
	if len(cards) == 0 {
		f.Text(row+1, col, "( )", stDim)
		return
	}
	// The group spans only the contiguous prefix of cards that have arrived
	// (slide complete). A still-sliding tail card floats in separately so the
	// joined box never has to render a gap.
	arrived := len(cards)
	if resolve.active {
		for i := range cards {
			if resolve.face(i).sliding {
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
		if resolve.active {
			face = resolve.face(i)
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
	if resolve.active {
		for i := arrived; i < len(cards); i++ {
			face := resolve.face(i)
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

// centerSlot centres s within a slotW-wide column starting at slot. It is
// alloc-free: it counts runes, and when s overflows the slot it writes only the
// first slotW runes (at offset 0) by iterating runes and stopping at slotW,
// rather than slicing-then-converting.
func centerSlot(f *kit.Frame, row, slot int, s string, st kit.Style) {
	n := runeLen(s)
	if n > slotW {
		col := slot
		i := 0
		for _, r := range s {
			if i >= slotW {
				break
			}
			f.SetRune(row, col, r, st)
			col++
			i++
		}
		return
	}
	f.Text(row, slot+(slotW-n)/2, s, st)
}

// slotStart returns the starting column to centre a w-wide composite within a
// slotW column starting at slot, clamping a w>slotW composite to offset 0 so it
// renders left-aligned exactly as centerSlot truncates an over-long string.
func slotStart(slot, w int) int {
	if w > slotW {
		return slot
	}
	return slot + (slotW-w)/2
}

// putChips writes "$%d" centred in a slotW column starting at slot, allocating
// nothing (the alloc-free form of centerSlot(f, row, slot, fmt.Sprintf("$%d",
// chips), st)).
func putChips(f *kit.Frame, row, slot, chips int, st kit.Style) {
	w := 1 + intWidth(chips) // '$' + digits
	c := f.Text(row, slotStart(slot, w), "$", st)
	putInt(f, row, c, chips, st)
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

// valueLabelWidth returns the column width valueLabel would render for hand h,
// matching putValueLabel exactly (no allocation).
func valueLabelWidth(h hand) int {
	if h.isBlackjack() {
		return 2 // "BJ"
	}
	total, soft := h.value()
	if total > 21 {
		return 4 // "BUST"
	}
	w := intWidth(total)
	if soft {
		w++ // leading 's'
	}
	return w
}

// putValueLabel writes hand h's value label at (row, col) and returns the next
// column, allocating nothing. It reproduces valueLabel's glyphs exactly:
// "BJ" / "BUST" / "s%d" (soft) / "%d".
func putValueLabel(f *kit.Frame, row, col int, h hand, st kit.Style) int {
	if h.isBlackjack() {
		return f.Text(row, col, "BJ", st)
	}
	total, soft := h.value()
	if total > 21 {
		return f.Text(row, col, "BUST", st)
	}
	if soft {
		f.SetRune(row, col, 's', st)
		col++
	}
	return putInt(f, row, col, total, st)
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
