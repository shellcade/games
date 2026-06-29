package main

import (
	"fmt"
	"strings"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// Felt-table geometry (a casino table): a rounded felt frame with the dealer
// centred up top and up to five player seats along the rail.
const (
	feltTop      = 1
	feltBottom   = 19
	dealerRow    = 4 // dealer card group occupies dealerRow..dealerRow+2
	dealerValRow = 7 // dealer total / verdict, centred just below the cards

	// The seat block sits one row higher than the dealer-only layout used to
	// allow: relocating the rules tagline (it now flanks the dealer) freed rows
	// 8-9, so the block shifts up to open a dedicated backers line at the bottom.
	seatNameRow = 10
	seatCardRow = 11 // seat card group occupies seatCardRow..seatCardRow+2
	seatValRow  = 14
	seatChipRow = 15
	seatPairRow = 16 // Perfect Pairs side-bet line (stake while betting, result once dealt)
	seatBackRow = 17 // backers line: who is backing this seat, with their tiles
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
	stBack   = kit.Style{FG: kit.Yellow}                                           // card back while sliding/flipping
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

	// Dealer, centred near the top of the felt, with the table rules as subtle
	// signage flanking the DEALER label (left + right) rather than a banner
	// across the middle of the felt — keeping the centre clear for the cards.
	f.Text(dealerRow-1, 2, "blackjack pays 3:2", stDim)
	f.TextRight(dealerRow-1, kit.Cols-3, "dealer stands on 17", stDim)
	center(f, dealerRow-1, "D E A L E R", stTitle)
	rm.drawDealer(f)

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
		rm.drawSeat(f, groupLeft+i*slotW, s, v, own, isActive)
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
	// Size and centre the row to the cards actually on the table (dealt cards
	// plus any hit already arriving), and draw only those — so a pending hit's
	// slot never appears before the hole card is turned over and that hit begins
	// to slide in.
	lc := rm.dealerLayoutCount()
	w := cardsWidth(lc)
	col := (kit.Cols - w) / 2
	drawCardsAnim(f, dealerRow, col, rm.dealer[:lc], hide, rm.dealerResolver())
	// The total and the verdict read off only the cards shown face up so far,
	// never the authoritative hand — so the number ticks up and BUST/BLACKJACK
	// appear as each card lands, not the instant the hand is dealt behind the
	// scenes. Centred just below the cards (like each seat's value line) and
	// coloured by outcome so the dealer's result reads at a glance.
	shown := rm.dealer[:rm.dealerShownCount()]
	label, st := fmt.Sprintf("(%d)", shown.total()), stDim
	switch {
	case len(shown) < 2:
		// Only the up card is face up — while the hole is concealed, mid lead-in,
		// or mid-flip — so report what the dealer is showing, not a phantom total.
		label = fmt.Sprintf("shows %d", hand{rm.dealer[0]}.total())
	case shown.isBlackjack():
		label, st = "BLACKJACK", stWin
	case shown.isBust():
		label, st = "BUST", stLose
	}
	center(f, dealerValRow, label, st)
}

func (rm *room) drawSeat(f *kit.Frame, slot int, s *seat, v kit.Player, own, active bool) {
	if s == nil {
		return
	}
	defer rm.drawBackersLine(f, slot, s, v) // who is backing this seat (its own dedicated line)
	nameSt := stCard
	switch {
	case active:
		nameSt = stActive
	case own:
		nameSt = stOwn
	}
	// Seat label: [►►]<character tile> <name>, centred in the slot. The
	// arcade character tile (kit v2.9.0) rides immediately before the name;
	// the 2 extra columns (tile + space) come out of the name budget so the
	// slot never overflows.
	marker, markerW := "", 0
	if active {
		// A doubled, highlighted marker so the active seat is unmissable; the
		// glyph degrades to ">>" on non-UTF-8 sessions (► -> >).
		marker, markerW = "►►", 2
	}
	name := s.p.Handle
	if max := slotW - markerW - 2; len(name) > max {
		name = name[:max]
	}
	labelW := markerW + 2 + len([]rune(name))
	nameCol := f.Text(seatNameRow, slot+(slotW-labelW)/2, marker, nameSt)
	f.Set(seatNameRow, nameCol, kit.CharacterCell(s.p.Character))
	f.Text(seatNameRow, nameCol+2, name, nameSt)

	rm.drawPairsLine(f, slot, s, own)

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

	// A split seat (2+ hands) renders one compact line per hand, stacked down the
	// seat rows, so every hand's cards stay visible and the hand on turn is
	// marked — the 15-col slot cannot fit multiple card boxes side by side, which
	// is why the old layout collapsed later hands to a bare "+". A single hand
	// keeps the full card box below.
	if len(s.hands) >= 2 {
		_, ah := rm.firstUnresolved()
		for hi, h := range s.hands {
			line, st := compactHandLine(h, active && ah == h)
			centerSlot(f, seatCardRow+hi, slot, line, st)
		}
		if rm.phase == phResults && s.result != "" {
			centerSlot(f, seatChipRow, slot, s.result, resultStyle(s.result))
		} else {
			centerSlot(f, seatChipRow, slot, fmt.Sprintf("$%d", s.chips), stDim)
		}
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
	// During results the value line doubles as the ready indicator: a readied
	// seat shows READY where its hand total was, so who's holding up the table
	// reads at a glance.
	valStr, valSt := strings.Join(vals, " "), valueStyle(s)
	if rm.phase == phResults && s.ready {
		valStr, valSt = "READY", stWin
	}
	centerSlot(f, seatValRow, slot, valStr, valSt)
	// The chip line carries the settlement summary during results, drawn instead
	// of the stack (not over it) — a shorter result like "PUSH" centred over the
	// wider "$1000" would otherwise leave a stray digit peeking out beside it.
	if rm.phase == phResults && s.result != "" {
		centerSlot(f, seatChipRow, slot, s.result, resultStyle(s.result))
	} else {
		centerSlot(f, seatChipRow, slot, fmt.Sprintf("$%d", s.chips), stDim)
	}
}

// pairsMult maps a Perfect Pairs result kind to its payout multiplier (X:1),
// for the result label; 0 for no pair.
func pairsMult(kind string) int {
	switch kind {
	case "perfect":
		return 25
	case "colored":
		return 12
	case "mixed":
		return 6
	}
	return 0
}

// drawPairsLine renders the seat's Perfect Pairs side-bet line. While betting it
// sits directly beneath that seat's main bet (seatCardRow+2), so each seat's
// bet+pairs read as one contiguous block and whose side bet is whose is never
// ambiguous; it shows only once placed, or for the seat's owner, matching how
// the main bet stays private until placed. Once the cards are dealt it moves to
// seatPairRow below the seat's hand, showing the win label (e.g. "COLORED 12:1")
// or a quiet "pairs lost".
func (rm *room) drawPairsLine(f *kit.Frame, slot int, s *seat, own bool) {
	ch := kit.CharacterCell(s.p.Character) // the placing player's face, beside their side bet
	switch {
	case rm.phase == phBetting:
		if s.pairsBet > 0 && (s.placed || own) {
			centerSlotChar(f, seatCardRow+2, slot, ch, fmt.Sprintf("+pairs %d", s.pairsBet), stOwn)
		}
	case s.pairsKind != "":
		centerSlotChar(f, seatPairRow, slot, ch, fmt.Sprintf("%s %d:1", strings.ToUpper(s.pairsKind), pairsMult(s.pairsKind)), stWin)
	case s.pairsBet > 0:
		centerSlotChar(f, seatPairRow, slot, ch, "pairs lost", stDim)
	}
}

// drawBackersLine renders, on a seat's dedicated backers row, a token per player
// backing it: their character tile then a compact stake (betting: "25p10") or
// net (results: "+25"). The viewer's own back is shown even at zero while they
// are focused on this seat (so "you're editing this" reads) and highlighted.
// Tokens are drawn left-to-right and truncated with an ellipsis past the slot.
func (rm *room) drawBackersLine(f *kit.Frame, slot int, target *seat, v kit.Player) {
	col, limit := slot, slot+slotW
	for _, id := range rm.order {
		s := rm.seats[id]
		if s == nil || s.p.AccountID == target.p.AccountID {
			continue
		}
		b := s.backs[target.p.AccountID]
		focused := s.p.AccountID == v.AccountID && s.focus == target.p.AccountID
		if (b == nil || (b.behind == 0 && b.pairs == 0)) && !focused {
			continue
		}
		if b == nil {
			b = &backBet{}
		}
		text, st := backerToken(rm.phase, b), stDim
		if focused {
			st = stActive
		}
		if col+1+len([]rune(text)) > limit { // no room for tile + token
			if col < limit {
				f.SetRune(seatBackRow, col, '…', stDim)
			}
			return
		}
		f.Set(seatBackRow, col, kit.CharacterCell(s.p.Character))
		col = f.Text(seatBackRow, col+1, text, st) + 1 // tile, token, then a gap
	}
}

// backerToken is one backer's compact cell content on the backers line: the
// stakes while betting (e.g. "25p10", "p10"), else the round net (e.g. "+25").
func backerToken(phase string, b *backBet) string {
	if phase == phBetting {
		s := ""
		if b.behind > 0 {
			s += fmt.Sprintf("%d", b.behind)
		}
		if b.pairs > 0 {
			s += fmt.Sprintf("p%d", b.pairs)
		}
		return s
	}
	return fmt.Sprintf("%+d", (b.behindWin-b.behind)+(b.pairsWin-b.pairs))
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
		// When the viewer is focused on another seat, the bar spells out their
		// back on that seat (the nav axes are editing it) — drawn in parts so the
		// backed player's character tile rides before their name.
		if t := rm.seats[s.focus]; s.focus != "" && t != nil {
			b := s.backOn(s.focus)
			post := fmt.Sprintf("%s  Up/Down behind %d  P/B pairs %d  Left/Right seat", t.p.Handle, b.behind, b.pairs)
			centerWithChar(f, actionRow, "BACKING ", kit.CharacterCell(t.p.Character), post, stPrompt)
			return
		}
		if !s.placed {
			// Prominent, highlighted call to bet for a viewer who hasn't yet.
			// ASCII-only so it reads identically on non-UTF-8 sessions.
			msg, st = "BET: Up/Down stake  P/B pairs  Left/Right back a seat  SPACE bet", stPrompt
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
		switch n := rm.insuranceUndecidedCount(); {
		case s.placed && !s.insuranceDecided:
			msg = "Dealer shows an Ace - Insurance?   [Y]es   [N]o"
		case n > 0:
			noun := "player"
			if n != 1 {
				noun = "players"
			}
			msg, st = fmt.Sprintf("waiting on %d %s for insurance - %s", n, noun, clock(rm.remaining())), stDim
		default:
			msg, st = "resolving insurance...", stDim
		}
	case phTurns:
		switch {
		case active != nil && active.p.AccountID == v.AccountID:
			_, h := rm.firstUnresolved()
			// Highlighted YOUR TURN so the viewer can't miss that it's on them.
			msg, st = "YOUR TURN - "+legalActions(s, h), stPrompt
		case active != nil:
			// The active player's character tile rides immediately before
			// their name, so the wait line is drawn in parts (text–tile–text)
			// rather than through the centered-string path.
			centerWithChar(f, actionRow, "waiting on ", kit.CharacterCell(active.p.Character), active.p.Handle+"...", stDim)
			return
		default:
			// No hand left on turn: every player has resolved and the dealer is
			// turning its hole card and drawing. Name the moment so the slow
			// reveal reads as the dealer acting rather than a frozen table.
			msg, st = "dealer plays...", stDim
		}
	case phResults:
		switch n := rm.unreadyCount(); {
		case !s.ready:
			// Prominent call to ready up for the viewer who hasn't yet.
			msg, st = "round over - SPACE to ready up for the next hand", stPrompt
		case n > 0:
			noun := "player"
			if n != 1 {
				noun = "players"
			}
			msg, st = fmt.Sprintf("ready - waiting on %d %s (next hand in %s)", n, noun, clock(rm.remaining())), stDim
		default:
			msg, st = "all ready - next hand starting...", stPhase
		}
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

// dealerCardFaceUp reports whether dealer card i currently shows its face at the
// latest composed instant: the concealed hole card never does, a settled (or
// unscheduled) card always does, and an animating card only once its slide has
// landed and its reveal flip has turned far enough to expose the face. It reads
// only the recorded schedule and the frozen clock.
func (rm *room) dealerCardFaceUp(i int) bool {
	if rm.dealerHole && i == 1 {
		return false // hole card still concealed
	}
	a, ok := rm.animFor(animDealer, kit.Player{}, 0, i)
	if !ok {
		return true // settled or no animation -> face up
	}
	if a.slideProgress(rm.lastNow) < 1 {
		return false // still gliding in
	}
	frame, _ := a.flipFrame(rm.lastNow)
	return frame == 2 // face exposed only on the final flip frame
}

// dealerLayoutCount is how many dealer cards occupy the table right now: the two
// dealt cards plus any hit that has actually begun sliding in. A hit that is
// scheduled but has not yet started arriving is excluded, so the row is only as
// wide as the cards on show and the next hit's arrival is announced by its slide
// — never by a slot reserved before the hole card has even been turned over.
func (rm *room) dealerLayoutCount() int {
	n := 0
	for i := range rm.dealer {
		if i >= 2 {
			if a, ok := rm.animFor(animDealer, kit.Player{}, 0, i); ok && rm.lastNow.Before(a.slideStart) {
				break // this hit has not begun arriving yet, nor have any after it
			}
		}
		n++
	}
	return n
}

// dealerShownCount is how many leading dealer cards are face up right now. The
// dealer reveals strictly in order — up card, then the hole flip, then each hit
// in turn — so the shown set is always this contiguous prefix, and the displayed
// total/verdict slice off it without allocating.
func (rm *room) dealerShownCount() int {
	n := 0
	for i := range rm.dealer {
		if !rm.dealerCardFaceUp(i) {
			break
		}
		n++
	}
	return n
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

// centerWithChar centres "<pre><character tile> <post>" on a row: the styled
// tile cell (width 1, kit v2.9.0) plus its trailing space sit between the two
// text parts, so a player name in post carries its character right before it.
func centerWithChar(f *kit.Frame, row int, pre string, ch kit.Cell, post string, st kit.Style) {
	w := len([]rune(pre)) + 2 + len([]rune(post))
	col := (kit.Cols - w) / 2
	if col < 0 {
		col = 0
	}
	col = f.Text(row, col, pre, st)
	f.Set(row, col, ch)
	f.Text(row, col+2, post, st)
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

// centerSlotChar centres "<character tile> <text>" within a slotW-wide column:
// the styled character cell (width 1) plus a space precede the text, tying the
// line to a specific player by face. The text is clamped so the tile + text
// never overflow the slot.
func centerSlotChar(f *kit.Frame, row, slot int, ch kit.Cell, text string, st kit.Style) {
	tr := []rune(text)
	if len(tr) > slotW-2 {
		tr = tr[:slotW-2]
	}
	w := 2 + len(tr)
	col := slot + (slotW-w)/2
	f.Set(row, col, ch)
	f.Text(row, col+2, string(tr), st)
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

// compactHandLine formats one split hand as a single slot-wide line —
// "<marker><cards> <total>", e.g. "►8♠3♥ 11" — for the stacked split layout.
// The on-turn hand carries a ► marker (degrades to > on non-UTF-8) and the
// active style; a busted hand reads red. The card tokens are truncated with an
// ellipsis if a long (hit-heavy) hand would otherwise overflow the slot, so the
// total always stays visible.
func compactHandLine(h *phand, active bool) (string, kit.Style) {
	var cards strings.Builder
	for _, c := range h.cards {
		cards.WriteString(c.r.boxLabel())
		cards.WriteRune(c.s.pip())
	}
	total := valueLabel(h.cards)
	marker, st := " ", stCard
	if h.cards.isBust() {
		st = stLose
	}
	if active {
		marker, st = "►", stActive
	}
	budget := slotW - len([]rune(marker)) - 1 - len([]rune(total)) // marker + space + total
	if budget < 1 {
		budget = 1
	}
	cr := []rune(cards.String())
	if len(cr) > budget {
		cr = append(cr[:budget-1:budget-1], '…')
	}
	return marker + string(cr) + " " + total, st
}

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
