package main

import (
	"time"

	kit "github.com/shellcade/kit/v2"

	"alan/chess/engine"
)

// Time control and timing constants.
const (
	mainClock   = 5 * time.Minute  // each side's starting blitz time
	increment   = 3 * time.Second  // Fischer increment added after each move
	waitTimeout = 2 * time.Minute  // how long a lone player waits for an opponent
	resultsDur  = 12 * time.Second // how long the results screen holds before End()
)

// phases. Kept as internal room state for game logic only — the lean ABI has no
// phase surface (the native SetPhase calls are dropped; joinability is host-
// derived from the roster + capacity).
const (
	phWaiting = "waiting"
	phPlaying = "playing"
	phOver    = "over"
)

// noOffer is the drawOffer sentinel meaning "no pending offer". A real offer
// stores the offering colour (White/Black).
const noOffer engine.Color = 0xff

// selection is one player's cursor and pending move-entry state.
type selection struct {
	cursor   engine.Square // the square the cursor sits on
	from     engine.Square // selected origin (NoSquare = nothing selected)
	targets  []engine.Move // legal moves from `from`, for highlighting
	promoSel int           // promotion picker index into promoOrder (Q R B N)
	promoing bool          // promotion picker is open
	promoMv  engine.Move   // the pawn move awaiting promotion choice
}

// promoOrder is the order the promotion picker cycles through.
var promoOrder = [4]engine.PieceType{engine.Queen, engine.Rook, engine.Bishop, engine.Knight}

type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	pos   engine.Position
	phase string
	seats []kit.Player                // join order
	color map[string]engine.Color     // accountID -> White/Black
	byID  map[string]kit.Player       // accountID -> latest Player token (refreshed on join)

	sel     map[string]*selection // accountID -> selection
	lastMv  *engine.Move
	moves   []string       // long-algebraic, for the move list
	history map[string]int // repetition counts (RepetitionKey -> count)

	clock     [2]time.Duration // remaining per colour (index by engine.Color)
	turnStart time.Time        // when the side-to-move's clock started

	drawOffer engine.Color    // colour with a pending draw offer, or noOffer
	resignArm map[string]bool // accountID -> armed (awaiting resign confirm)

	result   kit.Result
	resultOK bool
	outcome  string // human text, e.g. "Checkmate - White wins"

	// Wake-driven deadlines (replacing the native engine timers). Zero == disarmed.
	waitDeadline    time.Time // when a lone player's wait expires (phWaiting)
	resultsDeadline time.Time // when the results screen auto-settles (phOver)

	lastNow time.Time // most recent room clock, for live clock display
	frame   *kit.Frame
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:       cfg,
		svc:       svc,
		color:     map[string]engine.Color{},
		byID:      map[string]kit.Player{},
		sel:       map[string]*selection{},
		history:   map[string]int{},
		resignArm: map[string]bool{},
		drawOffer: noOffer,
		phase:     phWaiting,
		frame:     kit.NewFrame(),
	}
}

func (rm *room) OnStart(r kit.Room) {
	rm.pos = engine.StartPosition()
	rm.lastNow = r.Now()
	// Chess is a navigation screen throughout: arrows and h/j/k/l move the cursor,
	// Enter/Space confirm, and q/Esc back out. The game's own letter commands
	// (r/d/y/n) are read from raw input, so CtxNav is correct in every phase.
	r.SetInputContext(kit.CtxNav)
}

func (rm *room) OnClose(r kit.Room) {}

// --- pairing ---------------------------------------------------------------

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.byID[p.AccountID] = p
	if _, ok := rm.color[p.AccountID]; ok {
		rm.render(r)
		return // already seated (e.g. a rejoin after hibernation)
	}
	rm.seats = append(rm.seats, p)

	switch len(rm.seats) {
	case 1:
		// First arrival: wait for an opponent. Arm the wake-driven wait deadline.
		rm.phase = phWaiting
		rm.waitDeadline = r.Now().Add(waitTimeout)
	case 2:
		// Second arrival: assign colours, start play, arm the clock.
		rm.startGame(r)
	}
	// A third join is impossible (capacity is 2), so extra joins are a no-op
	// rather than corrupting a live game.
	rm.render(r)
}

// startGame assigns colours from the seeded RNG and begins the timed game.
func (rm *room) startGame(r kit.Room) {
	rm.waitDeadline = time.Time{}
	a, b := rm.seats[0], rm.seats[1]
	// Reproducible colour assignment: a coin flip from the room's seeded RNG.
	if r.Rand().Intn(2) == 0 {
		rm.color[a.AccountID], rm.color[b.AccountID] = engine.White, engine.Black
	} else {
		rm.color[a.AccountID], rm.color[b.AccountID] = engine.Black, engine.White
	}
	rm.sel[a.AccountID] = newSelection(rm.color[a.AccountID])
	rm.sel[b.AccountID] = newSelection(rm.color[b.AccountID])

	rm.phase = phPlaying
	rm.clock = [2]time.Duration{mainClock, mainClock}
	rm.turnStart = r.Now()
	rm.history[rm.pos.RepetitionKey()]++
}

func newSelection(c engine.Color) *selection {
	// Cursor starts on the player's own back rank e-file (e1 for White, e8 for
	// Black) - a familiar home square.
	rank := 0
	if c == engine.Black {
		rank = 7
	}
	return &selection{cursor: engine.SquareAt(4, rank), from: engine.NoSquare}
}

func (rm *room) endNoOpponent(r kit.Room) {
	if rm.phase != phWaiting {
		return
	}
	// No opponent ever arrived; settle with no winner and dispose.
	rm.phase = phOver
	rm.outcome = "No opponent - room closed"
	rm.resultOK = true
	rm.result = kit.Result{}
	rm.waitDeadline = time.Time{}
	r.End(rm.result)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	delete(rm.byID, p.AccountID)
	switch rm.phase {
	case phPlaying:
		if c, ok := rm.color[p.AccountID]; ok {
			// A participant abandoned the game: the opponent wins.
			winner := rm.otherSeat(p)
			rm.finishGame(r, &outcomeSpec{
				winner:    rm.color[winner.AccountID],
				winnerSet: true,
				loserDNF:  true,
				loser:     c,
				text:      p.DisplayName() + " left - opponent wins",
			})
			return
		}
	case phWaiting:
		rm.waitDeadline = time.Time{}
	case phOver:
		// The game is decided and the ranked result is built, but End() is deferred
		// behind the results deadline. If this leave empties the room, settle now
		// with that result so the host's empty-room handling can't replace it with
		// an unranked one. End() is idempotent.
		if r.Count() == 0 {
			rm.resultsDeadline = time.Time{}
			rm.finish(r)
		}
	}
}

// otherSeat returns the seat that is not p (2-player only).
func (rm *room) otherSeat(p kit.Player) kit.Player {
	for _, s := range rm.seats {
		if s.AccountID != p.AccountID {
			return s
		}
	}
	return p
}

// --- wake: clock, timeouts, render -----------------------------------------

// OnWake is the host heartbeat. It advances every time-driven element against
// CallContext time — the wait timeout, the blitz flag-fall, and the results-hold
// settle — then renders once. The native game used three separate engine surfaces
// for these (After timer, OnTick, OnFrame); the lean ABI folds them all here.
func (rm *room) OnWake(r kit.Room) {
	now := r.Now()
	rm.lastNow = now

	switch rm.phase {
	case phWaiting:
		if !rm.waitDeadline.IsZero() && now.After(rm.waitDeadline) {
			rm.endNoOpponent(r)
		}
	case phPlaying:
		rm.checkFlagFall(r, now)
	case phOver:
		if !rm.resultsDeadline.IsZero() && now.After(rm.resultsDeadline) {
			rm.resultsDeadline = time.Time{}
			rm.finish(r)
		}
	}
	rm.render(r)
}

// liveRemaining returns the side-to-move's remaining clock at time now (the
// stored remainder minus the elapsed since their clock started).
func (rm *room) liveRemaining(side engine.Color, now time.Time) time.Duration {
	rem := rm.clock[side] - now.Sub(rm.turnStart)
	if rem < 0 {
		rem = 0
	}
	return rem
}

// checkFlagFall settles the game if the side to move has run out of time.
func (rm *room) checkFlagFall(r kit.Room, now time.Time) {
	if rm.phase != phPlaying {
		return
	}
	side := rm.pos.Side
	if rm.clock[side]-now.Sub(rm.turnStart) > 0 {
		return // still has time
	}
	// Flag-fall. The side to move loses on time, UNLESS the opponent cannot
	// possibly mate (e.g. a lone king) - then it is a draw.
	rm.clock[side] = 0
	opp := side ^ 1
	if !rm.sideCanMate(opp) {
		rm.finishGame(r, &outcomeSpec{
			draw: true,
			text: "Time - draw (insufficient mating material)",
		})
		return
	}
	rm.finishGame(r, &outcomeSpec{
		winner:    opp,
		winnerSet: true,
		loser:     side,
		loserFlag: true,
		text:      "Time - " + colorName(opp) + " wins",
	})
}

// sideCanMate reports whether colour c has, on its own (alongside the bare
// kings), enough material to force checkmate. Used for the flag-fall draw rule:
// if the side that did NOT flag cannot mate, the timeout is a draw.
func (rm *room) sideCanMate(c engine.Color) bool {
	var knights, bishops int
	for s := engine.Square(0); s < 64; s++ {
		pc := rm.pos.Board[s]
		if pc.Type == engine.Empty || pc.Color != c {
			continue
		}
		switch pc.Type {
		case engine.King:
		case engine.Knight:
			knights++
		case engine.Bishop:
			bishops++
		default: // pawn, rook, queen - always sufficient
			return true
		}
	}
	// A lone king or a single minor piece cannot force mate.
	return knights+bishops >= 2
}

// --- moves -----------------------------------------------------------------

// controller returns the player who controls the side to move.
func (rm *room) controller() (kit.Player, bool) {
	for _, s := range rm.seats {
		if rm.color[s.AccountID] == rm.pos.Side {
			return s, true
		}
	}
	return kit.Player{}, false
}

// makeMove validates and applies a move, updates clocks/history/last-move,
// clears selection + draw offer, and evaluates end conditions.
func (rm *room) makeMove(r kit.Room, m engine.Move) bool {
	legal := false
	for _, lm := range engine.LegalMoves(rm.pos) {
		if lm == m {
			legal = true
			break
		}
	}
	if !legal {
		return false
	}

	mover := rm.pos.Side
	now := r.Now()

	rm.moves = append(rm.moves, rm.pos.Long(m))
	rm.pos = engine.Apply(rm.pos, m)
	mv := m
	rm.lastMv = &mv
	rm.history[rm.pos.RepetitionKey()]++

	// Clear every player's selection (positions changed) and any draw offer.
	for id := range rm.sel {
		rm.sel[id] = newSelectionFollow(rm.sel[id], m.To)
	}
	rm.drawOffer = noOffer
	for id := range rm.resignArm {
		rm.resignArm[id] = false
	}

	// Clock: deduct elapsed, add the increment, switch the running clock.
	elapsed := now.Sub(rm.turnStart)
	rm.clock[mover] -= elapsed
	if rm.clock[mover] < 0 {
		rm.clock[mover] = 0
	}
	rm.clock[mover] += increment
	rm.turnStart = now

	rm.evaluateEnd(r)
	return true
}

// newSelectionFollow builds a fresh (unselected) selection, keeping the cursor
// where it was so play feels continuous.
func newSelectionFollow(prev *selection, to engine.Square) *selection {
	cur := to
	if prev != nil {
		cur = prev.cursor
	}
	return &selection{cursor: cur, from: engine.NoSquare}
}

// evaluateEnd checks the terminal conditions in order and finishes if reached.
func (rm *room) evaluateEnd(r kit.Room) {
	side := rm.pos.Side
	if !engine.HasLegalMove(rm.pos) {
		if engine.InCheck(rm.pos, side) {
			// Checkmate: the side to move is mated; the other side wins.
			winner := side ^ 1
			rm.finishGame(r, &outcomeSpec{
				winner:    winner,
				winnerSet: true,
				loser:     side,
				text:      "Checkmate - " + colorName(winner) + " wins",
			})
			return
		}
		rm.finishGame(r, &outcomeSpec{draw: true, text: "Stalemate - draw"})
		return
	}
	if engine.InsufficientMaterial(rm.pos) {
		rm.finishGame(r, &outcomeSpec{draw: true, text: "Draw - insufficient material"})
		return
	}
	if rm.pos.HalfMove >= 100 {
		rm.finishGame(r, &outcomeSpec{draw: true, text: "Draw - fifty-move rule"})
		return
	}
	if rm.history[rm.pos.RepetitionKey()] >= 3 {
		rm.finishGame(r, &outcomeSpec{draw: true, text: "Draw - threefold repetition"})
		return
	}
}

// --- ending ----------------------------------------------------------------

// outcomeSpec describes a terminal result to build into a kit.Result.
type outcomeSpec struct {
	draw      bool
	winner    engine.Color
	winnerSet bool
	loser     engine.Color
	loserDNF  bool // the losing player abandoned (StatusDNF)
	loserFlag bool // the losing player flagged on time (StatusFlagged)
	text      string
}

func (rm *room) finishGame(r kit.Room, spec *outcomeSpec) {
	if rm.phase == phOver {
		return
	}
	rm.phase = phOver
	rm.outcome = spec.text
	rm.result = rm.buildResult(spec)
	rm.resultOK = true

	// Hold the results screen, then auto-settle on the wake that passes the
	// deadline (was: an After timer).
	rm.resultsDeadline = r.Now().Add(resultsDur)
}

// buildResult turns an outcome into a ranked kit.Result over the seated players.
func (rm *room) buildResult(spec *outcomeSpec) kit.Result {
	var res kit.Result
	for _, p := range rm.seats {
		c := rm.color[p.AccountID]
		pr := kit.PlayerResult{Player: p, Status: kit.StatusFinished}
		switch {
		case spec.draw:
			pr.Rank = 1 // a draw is not a win: Metric stays 0 for both
		case spec.winnerSet && c == spec.winner:
			pr.Rank = 1
			pr.Metric = 1 // one win for the leaderboard tally
		default: // loser (Metric stays 0)
			pr.Rank = 2
			if spec.loserDNF && c == spec.loser {
				pr.Status = kit.StatusDNF
			} else if spec.loserFlag && c == spec.loser {
				pr.Status = kit.StatusFlagged
			}
		}
		res.Rankings = append(res.Rankings, pr)
	}
	return res
}

func (rm *room) finish(r kit.Room) {
	if rm.resultOK {
		r.End(rm.result)
	} else {
		r.End(kit.Result{})
	}
}

// colorName returns the display name of a colour.
func colorName(c engine.Color) string {
	if c == engine.White {
		return "White"
	}
	return "Black"
}

// --- input -----------------------------------------------------------------

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	switch rm.phase {
	case phWaiting:
		rm.render(r)
		return // just watch the waiting screen
	case phOver:
		if in.Kind == kit.InputKey && in.Key == kit.KeyEnter {
			rm.resultsDeadline = time.Time{}
			rm.finish(r)
		}
		rm.render(r)
		return
	case phPlaying:
		rm.handlePlayInput(r, p, in)
		rm.render(r)
	}
}

// orientationColor returns the colour whose perspective player p views the board
// from.
func (rm *room) orientationColor(p kit.Player) engine.Color {
	if c, ok := rm.color[p.AccountID]; ok {
		return c
	}
	return engine.White // spectators (none in v1) see White's view
}

func (rm *room) handlePlayInput(r kit.Room, p kit.Player, in kit.Input) {
	sel := rm.sel[p.AccountID]
	if sel == nil {
		return // not a seated participant
	}

	// Whose turn is it, and is this player the controller?
	ctrl, ok := rm.controller()
	isController := ok && ctrl.AccountID == p.AccountID

	// Resign confirmation is modal: once armed, the next key either confirms (a
	// second 'r' or 'y') or cancels. ANY other key disarms — so clear the arm up
	// front and let the key fall through to be handled normally.
	if rm.resignArm[p.AccountID] {
		rm.resignArm[p.AccountID] = false
		if in.Kind == kit.InputRune && (in.Rune == 'r' || in.Rune == 'R' || in.Rune == 'y' || in.Rune == 'Y') {
			rm.doResign(r, p)
			return
		}
	}

	// Backspace cancels selection / exits the promotion picker, in any state.
	if in.Kind == kit.InputKey && in.Key == kit.KeyBackspace {
		rm.cancelSelection(sel)
		return
	}

	// Promotion picker open: ←/→ or h/l cycle the piece, Enter/Space confirms.
	if sel.promoing {
		switch {
		case promoCycleLeft(in):
			sel.promoSel = (sel.promoSel + len(promoOrder) - 1) % len(promoOrder)
		case promoCycleRight(in):
			sel.promoSel = (sel.promoSel + 1) % len(promoOrder)
		case isConfirm(in) && isController:
			rm.confirm(r, p, sel)
		}
		return
	}

	// Cursor movement (always allowed for a seated player, even off-turn).
	if d, ok := arrowDelta(in); ok {
		rm.moveCursor(sel, p, d[0], d[1])
		return
	}

	switch {
	case isConfirm(in):
		if isController {
			rm.confirm(r, p, sel)
		}
	case in.Kind == kit.InputRune && (in.Rune == 'r' || in.Rune == 'R'):
		rm.handleResign(r, p)
	case in.Kind == kit.InputRune && (in.Rune == 'd' || in.Rune == 'D'):
		rm.handleDrawOffer(r, p)
	case in.Kind == kit.InputRune && (in.Rune == 'y' || in.Rune == 'Y'):
		rm.handleYes(r, p)
	case in.Kind == kit.InputRune && (in.Rune == 'n' || in.Rune == 'N'):
		rm.handleNo(r, p)
	}
}

// confirm handles Enter/Space during play for the side-to-move's controller.
func (rm *room) confirm(r kit.Room, p kit.Player, sel *selection) {
	// Promotion picker open: confirm the chosen piece and make the move.
	if sel.promoing {
		m := sel.promoMv
		m.Promo = promoOrder[sel.promoSel]
		rm.makeMove(r, m)
		return
	}

	cur := sel.cursor
	side := rm.pos.Side

	if sel.from == engine.NoSquare {
		// No selection: select a friendly piece of the side to move.
		pc := rm.pos.Board[cur]
		if pc.Type != engine.Empty && pc.Color == side {
			rm.selectSquare(sel, cur)
		}
		return
	}

	// A piece is selected.
	if cur == sel.from {
		rm.cancelSelection(sel) // re-pressing the origin deselects
		return
	}
	// Cursor on another friendly piece → re-select it.
	pc := rm.pos.Board[cur]
	if pc.Type != engine.Empty && pc.Color == side {
		rm.selectSquare(sel, cur)
		return
	}
	// Cursor on a legal target?
	for _, m := range sel.targets {
		if m.To == cur {
			if m.Promo != engine.Empty {
				// Pawn reaching the last rank: open the promotion picker.
				sel.promoing = true
				sel.promoSel = 0 // default Queen
				sel.promoMv = engine.Move{From: m.From, To: m.To}
				return
			}
			rm.makeMove(r, m)
			return
		}
	}
	// Otherwise ignore (keep the selection).
}

// selectSquare selects the piece on sq if it has at least one legal move,
// computing its legal targets. A piece with no legal move is NOT picked up.
func (rm *room) selectSquare(sel *selection, sq engine.Square) {
	var targets []engine.Move
	for _, m := range engine.LegalMoves(rm.pos) {
		if m.From == sq {
			targets = append(targets, m)
		}
	}
	if len(targets) == 0 {
		return // immovable piece — leave the cursor free to pick another
	}
	sel.from = sq
	sel.promoing = false
	sel.targets = targets
}

func (rm *room) cancelSelection(sel *selection) {
	sel.from = engine.NoSquare
	sel.targets = sel.targets[:0]
	sel.promoing = false
}

// moveCursor moves the cursor in the on-screen direction (df,dr in board
// coordinates, white-perspective: dr=+1 is "up"). While a piece is selected the
// cursor is constrained to that piece's legal destinations; otherwise it
// free-roams one square at a time.
func (rm *room) moveCursor(sel *selection, p kit.Player, df, dr int) {
	orient := rm.orientationColor(p)
	if sel.from != engine.NoSquare {
		rm.snapCursorToTarget(sel, orient, df, dr)
		return
	}
	if orient == engine.Black {
		// Black views the board flipped, so on-screen up/down/left/right invert.
		df, dr = -df, -dr
	}
	f := sel.cursor.File() + df
	rk := sel.cursor.Rank() + dr
	if f < 0 || f > 7 || rk < 0 || rk > 7 {
		return
	}
	sel.cursor = engine.SquareAt(f, rk)
}

// snapCursorToTarget moves the cursor to the nearest legal-destination square in
// the pressed on-screen direction.
func (rm *room) snapCursorToTarget(sel *selection, orient engine.Color, df, dr int) {
	if len(sel.targets) == 0 {
		return
	}
	// Screen direction from the key: dr=+1 (Up)->row up, dr=-1 (Down)->row down,
	// df=-1 (Left)->col left, df=+1 (Right)->col right.
	var drow, dcol int
	switch {
	case dr == 1:
		drow = -1
	case dr == -1:
		drow = 1
	case df == -1:
		dcol = -1
	case df == 1:
		dcol = 1
	}
	cr, cc := screenSquare(sel.cursor, orient)
	best := engine.NoSquare
	bestPrimary, bestSecondary := 1<<30, 1<<30
	seen := map[engine.Square]bool{}
	for _, m := range sel.targets {
		if seen[m.To] {
			continue
		}
		seen[m.To] = true
		tr, tc := screenSquare(m.To, orient)
		var primary, secondary int
		if drow != 0 {
			if (tr-cr)*drow <= 0 {
				continue // not in the vertical direction pressed
			}
			primary, secondary = absInt(tr-cr), absInt(tc-cc)
		} else {
			if (tc-cc)*dcol <= 0 {
				continue // not in the horizontal direction pressed
			}
			primary, secondary = absInt(tc-cc), absInt(tr-cr)
		}
		if primary < bestPrimary || (primary == bestPrimary && secondary < bestSecondary) {
			best, bestPrimary, bestSecondary = m.To, primary, secondary
		}
	}
	if best != engine.NoSquare {
		sel.cursor = best
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// handleResign arms a resignation. The actual resignation is completed by the
// modal confirm in handlePlayInput on a second 'r' or 'y' (which calls doResign).
func (rm *room) handleResign(r kit.Room, p kit.Player) {
	if _, ok := rm.color[p.AccountID]; !ok {
		return
	}
	rm.resignArm[p.AccountID] = true
}

// doResign ends the game with the resigning player's opponent as the winner.
func (rm *room) doResign(r kit.Room, p kit.Player) {
	c, ok := rm.color[p.AccountID]
	if !ok {
		return
	}
	winner := c ^ 1
	rm.finishGame(r, &outcomeSpec{
		winner:    winner,
		winnerSet: true,
		loser:     c,
		text:      colorName(c) + " resigns - " + colorName(winner) + " wins",
	})
}

// handleDrawOffer offers or withdraws a draw.
func (rm *room) handleDrawOffer(r kit.Room, p kit.Player) {
	c, ok := rm.color[p.AccountID]
	if !ok {
		return
	}
	if rm.drawOffer == c {
		rm.drawOffer = noOffer // withdraw your own offer
		return
	}
	rm.drawOffer = c
}

// handleYes accepts a pending draw offer that came from the opponent.
func (rm *room) handleYes(r kit.Room, p kit.Player) {
	c, ok := rm.color[p.AccountID]
	if !ok {
		return
	}
	if rm.drawOffer != noOffer && rm.drawOffer != c {
		rm.finishGame(r, &outcomeSpec{draw: true, text: "Draw - by agreement"})
	}
}

// handleNo declines a pending draw offer addressed to this player.
func (rm *room) handleNo(r kit.Room, p kit.Player) {
	c, ok := rm.color[p.AccountID]
	if !ok {
		return
	}
	if rm.drawOffer != noOffer && rm.drawOffer != c {
		rm.drawOffer = noOffer
	}
}

// --- input helpers ---------------------------------------------------------

// arrowDelta maps an arrow key or hjkl to an on-screen (file, rank) step where
// "up" is +rank on screen (toward the far edge from the viewer).
func arrowDelta(in kit.Input) ([2]int, bool) {
	if in.Kind == kit.InputKey {
		switch in.Key {
		case kit.KeyUp:
			return [2]int{0, 1}, true
		case kit.KeyDown:
			return [2]int{0, -1}, true
		case kit.KeyLeft:
			return [2]int{-1, 0}, true
		case kit.KeyRight:
			return [2]int{1, 0}, true
		}
	}
	if in.Kind == kit.InputRune {
		switch in.Rune {
		case 'k':
			return [2]int{0, 1}, true
		case 'j':
			return [2]int{0, -1}, true
		case 'h':
			return [2]int{-1, 0}, true
		case 'l':
			return [2]int{1, 0}, true
		}
	}
	return [2]int{}, false
}

func isConfirm(in kit.Input) bool {
	return (in.Kind == kit.InputKey && in.Key == kit.KeyEnter) ||
		(in.Kind == kit.InputRune && in.Rune == ' ')
}

// promoCycleLeft reports a "previous piece" input in the promotion picker.
func promoCycleLeft(in kit.Input) bool {
	return (in.Kind == kit.InputKey && in.Key == kit.KeyLeft) ||
		(in.Kind == kit.InputRune && in.Rune == 'h')
}

// promoCycleRight reports a "next piece" input in the promotion picker.
func promoCycleRight(in kit.Input) bool {
	return (in.Kind == kit.InputKey && in.Key == kit.KeyRight) ||
		(in.Kind == kit.InputRune && in.Rune == 'l')
}
