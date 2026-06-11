package main

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// palette assigns each well a distinct bright color by join order, used when a
// player has no arcade character.
var palette = []kit.Color{
	kit.RGB(0x4f, 0xd6, 0xff), // cyan
	kit.RGB(0xff, 0x8a, 0x4f), // orange
	kit.RGB(0x7d, 0xff, 0x6b), // green
	kit.RGB(0xff, 0x6b, 0xc7), // pink
	kit.RGB(0xb9, 0x8a, 0xff), // purple
	kit.RGB(0xff, 0xe1, 0x55), // yellow
}

// room is the live game state. Per-player wells live keyed by account id
// (hibernation-safe); everything else is plain room scalars.
type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	wells map[string]*well      // by account id (hibernation-safe)
	names map[string]kit.Player // account id -> player (handle/persist)
	order []string              // join order of account ids (stable layout)

	matchOver bool   // pvp: a winner has been decided
	winner    string // account id of the winner ("" = none/solo)

	now     time.Time
	lastNow time.Time

	frame *kit.Frame // long-lived render buffer, reused every frame (Send copies)
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:   cfg,
		svc:   svc,
		wells: map[string]*well{},
		names: map[string]kit.Player{},
		frame: kit.NewFrame(),
	}
}

// solo reports whether this is a single-well score-attack room (1 player). PvP
// rules (attacks, elimination, a winner) kick in once a second well joins.
func (rm *room) solo() bool { return len(rm.wells) < 2 }

// --- lifecycle ---------------------------------------------------------------

func (rm *room) OnStart(r kit.Room) {
	r.SetInputContext(kit.CtxNav)
	rm.now = r.Now()
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	rm.names[p.AccountID] = p
	if _, ok := rm.wells[p.AccountID]; !ok {
		w := &well{
			glyph: '◆',
			color: palette[len(rm.order)%len(palette)],
			best:  rm.loadBest(r, p),
		}
		rm.wells[p.AccountID] = w
		rm.order = append(rm.order, p.AccountID)
		rm.startWell(r, w)
	}
	// Each well's accent is its owner's join-order palette color and a '◆' tag.
	// If a player carries an arcade character, its glyph heads the well and its
	// background color becomes the accent instead; a zero Character (the default)
	// keeps the palette look. Re-applied on every join so a reconnecting player
	// carries their current look.
	w := rm.wells[p.AccountID]
	if c := p.Character; c.Glyph != "" {
		for _, ru := range c.Glyph {
			w.glyph = ru
			break
		}
		w.color = kit.RGB(c.BgR, c.BgG, c.BgB)
	} else {
		w.glyph = '◆'
		for i, id := range rm.order {
			if id == p.AccountID {
				w.color = palette[i%len(palette)]
				break
			}
		}
	}
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	if w := rm.wells[p.AccountID]; w != nil {
		rm.bankScore(r, p, w.score)
		delete(rm.wells, p.AccountID)
	}
	delete(rm.names, p.AccountID)
	for i, id := range rm.order {
		if id == p.AccountID {
			rm.order = append(rm.order[:i], rm.order[i+1:]...)
			break
		}
	}
	rm.checkWinner(r)
	rm.render(r)
}

func (rm *room) OnClose(r kit.Room) {
	for id, w := range rm.wells {
		if p, ok := rm.names[id]; ok {
			rm.bankScore(r, p, w.score)
		}
	}
}

// --- input -------------------------------------------------------------------

// OnInput applies one control action. Terminals have no key-up, so soft-drop is
// modeled as a one-row nudge per press (and the held repeat the terminal sends
// keeps it falling); hard-drop slams to the floor and locks.
func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	rm.now = r.Now()
	w := rm.wells[p.AccountID]
	if w == nil || !w.alive || !w.hasPiece || len(w.clearing) > 0 {
		return
	}
	switch kit.Resolve(in, kit.CtxNav) {
	case kit.ActLeft:
		rm.tryMove(w, 0, -1)
	case kit.ActRight:
		rm.tryMove(w, 0, +1)
	case kit.ActDown:
		rm.softStep(w)
	case kit.ActUp:
		rm.tryRotate(w, +1)
	case kit.ActConfirm:
		rm.hardDrop(r, w)
	}
	// Rune fallbacks for rotate (z/x) regardless of nav mapping.
	if in.Kind == kit.InputRune {
		switch in.Rune {
		case 'z', 'Z':
			rm.tryRotate(w, -1)
		case 'x', 'X':
			rm.tryRotate(w, +1)
		}
	}
	rm.render(r)
}

// tryMove shifts the active piece by (drow, dcol) if the destination is clear,
// resetting the lock timer on a successful horizontal nudge.
func (rm *room) tryMove(w *well, drow, dcol int) bool {
	moved := w.cur
	moved.row += drow
	moved.col += dcol
	if rm.collides(w, moved) {
		return false
	}
	w.cur = moved
	if dcol != 0 && !w.lockAt.IsZero() {
		// A successful slide re-arms lock delay if still grounded.
		if rm.grounded(w) {
			w.lockAt = rm.now.Add(lockDelay)
		} else {
			w.lockAt = time.Time{}
		}
	}
	return true
}

// tryRotate rotates the piece by dir (+1 cw, -1 ccw) with simple wall kicks: if
// the rotated pose collides, try nudging it left/right/up a cell or two before
// giving up. Kicks are tried in a fixed order, so behavior is deterministic.
var kickOffsets = [][2]int{{0, 0}, {0, -1}, {0, 1}, {0, -2}, {0, 2}, {-1, 0}}

func (rm *room) tryRotate(w *well, dir int) bool {
	n := len(pieces[w.cur.kind].states)
	rot := ((w.cur.rot+dir)%n + n) % n
	for _, k := range kickOffsets {
		cand := w.cur
		cand.rot = rot
		cand.row += k[0]
		cand.col += k[1]
		if !rm.collides(w, cand) {
			w.cur = cand
			if !w.lockAt.IsZero() {
				if rm.grounded(w) {
					w.lockAt = rm.now.Add(lockDelay)
				} else {
					w.lockAt = time.Time{}
				}
			}
			return true
		}
	}
	return false
}

// softStep drops the piece one row if it can; otherwise it grounds. Soft-drop
// awards a tiny point per row to reward active play.
func (rm *room) softStep(w *well) {
	if rm.tryMove(w, 1, 0) {
		w.score++
		w.nextDrop = rm.now.Add(rm.gravity(w))
		return
	}
	rm.ground(w)
}

// hardDrop slams the piece to the bottom and locks it immediately.
func (rm *room) hardDrop(r kit.Room, w *well) {
	for rm.tryMove(w, 1, 0) {
		w.score += 2
	}
	rm.lockPiece(r, w)
}

// --- collision / grounding ---------------------------------------------------

// collides reports whether the given pose overlaps a wall, the floor, or a
// filled cell.
func (rm *room) collides(w *well, a active) bool {
	for _, c := range a.cells(pieces) {
		row, col := c[0], c[1]
		if col < 0 || col >= wellW || row >= wellH {
			return true
		}
		if row < 0 {
			continue // above the ceiling is allowed mid-spawn
		}
		if w.grid[row][col] != cellEmpty {
			return true
		}
	}
	return false
}

// grounded reports whether the active piece is resting (can't fall one row).
func (rm *room) grounded(w *well) bool {
	moved := w.cur
	moved.row++
	return rm.collides(w, moved)
}

// ground marks a grounded piece for lock after lockDelay (idempotent).
func (rm *room) ground(w *well) {
	if w.lockAt.IsZero() {
		w.lockAt = rm.now.Add(lockDelay)
	}
}

// --- heartbeat ---------------------------------------------------------------

// OnWake is the heartbeat: advance gravity, resolve locks and line-clear
// animations, deliver queued/solo garbage, and re-render every view.
func (rm *room) OnWake(r kit.Room) {
	rm.now = r.Now()
	rm.lastNow = rm.now

	for id := range rm.wells {
		rm.stepWell(r, rm.wells[id])
	}
	rm.checkWinner(r)
	rm.render(r)
}

// stepWell advances one well by one heartbeat.
func (rm *room) stepWell(r kit.Room, w *well) {
	if !w.alive {
		return
	}

	// Finish a line-clear animation: collapse the flashed rows, then spawn.
	if len(w.clearing) > 0 {
		if rm.now.Before(w.clearUntil) {
			return
		}
		rm.collapseRows(r, w)
		return
	}

	// Deliver queued rival garbage once the warning elapses.
	if !w.garbageAt.IsZero() && !rm.now.Before(w.garbageAt) {
		rm.applyGarbage(r, w, w.pendingGarbage, w.garbageGap)
		w.pendingGarbage = 0
		w.garbageAt = time.Time{}
		if !w.alive {
			return
		}
	}

	// Solo score-attack: junk creeps in on an accelerating timer.
	if rm.solo() && !w.soloNextGarbage.IsZero() && !rm.now.Before(w.soloNextGarbage) {
		gap := r.Rand().Intn(wellW)
		rm.applyGarbage(r, w, 1, gap)
		w.soloWave++
		w.soloNextGarbage = rm.now.Add(rm.soloGarbageInterval(w))
		if !w.alive {
			return
		}
	}

	if !w.hasPiece {
		return
	}

	// Gravity / lock.
	if !w.lockAt.IsZero() {
		if !rm.grounded(w) {
			w.lockAt = time.Time{} // slid off a ledge: keep falling
		} else if !rm.now.Before(w.lockAt) {
			rm.lockPiece(r, w)
			return
		}
	}
	if w.lockAt.IsZero() && !rm.now.Before(w.nextDrop) {
		if rm.tryMove(w, 1, 0) {
			w.nextDrop = rm.now.Add(rm.gravity(w))
		} else {
			rm.ground(w)
		}
	}
}

// gravity is the current drop interval for a well, shrinking with level.
func (rm *room) gravity(w *well) time.Duration {
	g := baseGravity - time.Duration(w.level)*gravityStep
	if g < minGravity {
		g = minGravity
	}
	return g
}

// soloGarbageInterval is the score-attack junk cadence, accelerating per wave.
func (rm *room) soloGarbageInterval(w *well) time.Duration {
	g := soloGarbageBase - time.Duration(w.soloWave)*soloGarbageStep
	if g < soloGarbageMin {
		g = soloGarbageMin
	}
	return g
}

// --- piece lifecycle ---------------------------------------------------------

// startWell initializes a well at the start of a run and spawns the first piece.
func (rm *room) startWell(r kit.Room, w *well) {
	for r0 := 0; r0 < wellH; r0++ {
		for c := 0; c < wellW; c++ {
			w.grid[r0][c] = cellEmpty
		}
	}
	w.alive = true
	w.score = 0
	w.lines = 0
	w.level = 0
	w.clearing = nil
	w.pendingGarbage = 0
	w.garbageAt = time.Time{}
	w.soloWave = 0
	w.bag = nil
	w.next = w.drawPiece(r.Rand())
	if rm.solo() {
		w.soloNextGarbage = rm.now.Add(rm.soloGarbageInterval(w))
	}
	rm.spawnPiece(r, w)
}

// spawnPiece pops the next piece into play, centered at the top. If it collides
// on entry, the well has topped out.
func (rm *room) spawnPiece(r kit.Room, w *well) {
	w.cur = active{
		kind: w.next,
		rot:  0,
		row:  spawnRow,
		col:  (wellW - 4) / 2,
	}
	w.next = w.drawPiece(r.Rand())
	w.hasPiece = true
	w.lockAt = time.Time{}
	w.nextDrop = rm.now.Add(rm.gravity(w))
	if rm.collides(w, w.cur) {
		rm.topOut(r, w)
	}
}

// lockPiece welds the active piece into the grid, then scores any full rows or
// (if none) spawns the next piece.
func (rm *room) lockPiece(r kit.Room, w *well) {
	tag := w.cur.kind + 1
	for _, c := range w.cur.cells(pieces) {
		if c[0] < 0 {
			// A cell locked above the ceiling means the stack reached the top.
			rm.topOut(r, w)
			return
		}
		w.grid[c[0]][c[1]] = tag
	}
	w.hasPiece = false
	w.lockAt = time.Time{}

	full := rm.fullRows(w)
	if len(full) == 0 {
		rm.spawnPiece(r, w)
		return
	}
	// Flash the full rows; collapseRows finishes the clear after clearFlash.
	w.clearing = full
	w.clearUntil = rm.now.Add(clearFlash)
}

// fullRows returns the indices of completely filled rows, top-to-bottom.
func (rm *room) fullRows(w *well) []int {
	var out []int
	for r0 := 0; r0 < wellH; r0++ {
		full := true
		for c := 0; c < wellW; c++ {
			if w.grid[r0][c] == cellEmpty {
				full = false
				break
			}
		}
		if full {
			out = append(out, r0)
		}
	}
	return out
}

// collapseRows removes the flashed rows, scores them, fires any attack, and
// spawns the next piece.
func (rm *room) collapseRows(r kit.Room, w *well) {
	n := len(w.clearing)
	rm.removeRows(w, w.clearing)
	w.clearing = nil

	if n >= 1 && n < len(lineScore) {
		w.lines += n
		w.score += lineScore[n] * (w.level + 1)
		w.level = w.lines / levelStep
	}

	// Attack: a multi-row clear dumps garbage on the tallest rival.
	if !rm.solo() && n >= 2 && n < len(attackRows) {
		rm.sendGarbage(r, w, attackRows[n])
	}

	rm.spawnPiece(r, w)
}

// removeRows deletes the given grid rows and drops everything above down.
func (rm *room) removeRows(w *well, victims []int) {
	gone := map[int]bool{}
	for _, v := range victims {
		gone[v] = true
	}
	var grid [wellH][wellW]int
	dst := wellH - 1
	for r0 := wellH - 1; r0 >= 0; r0-- {
		if gone[r0] {
			continue
		}
		grid[dst] = w.grid[r0]
		dst--
	}
	// Rows above dst stay empty (zero value).
	w.grid = grid
}

// --- garbage / attacks -------------------------------------------------------

// sendGarbage queues `rows` garbage rows at the rival with the tallest stack
// (the leader always wears the target). Ties break by join order for
// determinism. The volley shares one open gap column.
func (rm *room) sendGarbage(r kit.Room, from *well, rows int) {
	if rows <= 0 {
		return
	}
	target := rm.tallestRival(from)
	if target == nil {
		return
	}
	gap := r.Rand().Intn(wellW)
	target.pendingGarbage += rows
	target.garbageGap = gap
	target.garbageAt = rm.now.Add(garbageWarn)
}

// tallestRival returns the living rival well with the greatest stack height,
// excluding `from`. Nil if there is no eligible rival.
func (rm *room) tallestRival(from *well) *well {
	var best *well
	bestH := -1
	for _, id := range rm.order { // join order => deterministic tie-break
		w := rm.wells[id]
		if w == nil || w == from || !w.alive {
			continue
		}
		if h := w.height(); h > bestH {
			bestH = h
			best = w
		}
	}
	return best
}

// applyGarbage pushes `rows` junk rows up from the bottom of a well, each a full
// row of garbage cells with a single open column at `gap`. Anything shoved above
// the ceiling tops the well out.
func (rm *room) applyGarbage(r kit.Room, w *well, rows, gap int) {
	if rows <= 0 {
		return
	}
	if gap < 0 || gap >= wellW {
		gap = 0
	}
	for i := 0; i < rows; i++ {
		// Detect overflow: a filled cell in the top row would be pushed off.
		for c := 0; c < wellW; c++ {
			if w.grid[0][c] != cellEmpty {
				rm.topOut(r, w)
				return
			}
		}
		// Shift every row up one.
		for r0 := 0; r0 < wellH-1; r0++ {
			w.grid[r0] = w.grid[r0+1]
		}
		var junk [wellW]int
		for c := 0; c < wellW; c++ {
			if c == gap {
				junk[c] = cellEmpty
			} else {
				junk[c] = garbageCell
			}
		}
		w.grid[wellH-1] = junk
	}
	// If the active piece now overlaps the raised stack, nudge it up; if it
	// can't fit, the well tops out.
	if w.hasPiece && rm.collides(w, w.cur) {
		for k := 0; k < rows+2; k++ {
			up := w.cur
			up.row -= k
			if !rm.collides(w, up) {
				w.cur = up
				return
			}
		}
		rm.topOut(r, w)
	}
}

// --- elimination / winner ----------------------------------------------------

// topOut ends a well's run: in solo it banks the score and stops; in pvp it
// eliminates the player and may decide the match.
func (rm *room) topOut(r kit.Room, w *well) {
	if !w.alive {
		return
	}
	w.alive = false
	w.hasPiece = false
	w.clearing = nil
	if w.score > w.best {
		w.best = w.score
	}
	// Bank the run to the leaderboard.
	if p, ok := rm.names[rm.idOf(w)]; ok {
		rm.bankScore(r, p, w.score)
	}
}

// idOf finds the account id for a well (linear scan; tiny maps).
func (rm *room) idOf(w *well) string {
	for id, ww := range rm.wells {
		if ww == w {
			return id
		}
	}
	return ""
}

// checkWinner decides a pvp match once at most one well remains alive. Solo
// rooms never declare a winner (the run just ends on top-out).
func (rm *room) checkWinner(r kit.Room) {
	if rm.matchOver || rm.solo() {
		return
	}
	alive := 0
	var last string
	for _, id := range rm.order {
		if w := rm.wells[id]; w != nil && w.alive {
			alive++
			last = id
		}
	}
	if alive <= 1 {
		rm.matchOver = true
		rm.winner = last
	}
}

// --- durable best score + leaderboard ----------------------------------------

// bankScore records a run to the arcade leaderboard (best-aggregation board)
// and persists the durable best for the well header readout.
func (rm *room) bankScore(r kit.Room, p kit.Player, score int) {
	if score <= 0 {
		return
	}
	r.Post(kit.Result{Rankings: []kit.PlayerResult{{
		Player: p, Metric: score, Status: kit.StatusFinished,
	}}})
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return
	}
	_ = acct.Store().Set(context.Background(), "best", []byte(strconv.Itoa(score)), kit.MergeMax)
}

func (rm *room) loadBest(r kit.Room, p kit.Player) int {
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return 0
	}
	v, ok, err := acct.Store().Get(context.Background(), "best")
	if err != nil || !ok {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(v)))
	if err != nil {
		return 0
	}
	return n
}

// rankedOrder returns account ids sorted by score (desc) for the scoreboard.
func (rm *room) rankedOrder() []string {
	ids := make([]string, len(rm.order))
	copy(ids, rm.order)
	sort.SliceStable(ids, func(i, j int) bool {
		return rm.wells[ids[i]].score > rm.wells[ids[j]].score
	})
	return ids
}
