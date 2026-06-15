package main

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/keyhold"
)

// room is the live game state. Per-crew state lives in crew, keyed by account
// id (hibernation-safe); the reactor grid, faults, and core are shared.
type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	grid     [interiorRows][cols]cellKind // the reactor floor plan
	stations [][2]int                     // station cells (row,col in grid space)

	crew  map[string]*crewMember // by account id
	names map[string]kit.Player  // account id -> player (handle / character / persist)
	order []string               // join order of account ids (stable roster)

	faults []*fault
	core   float64 // shared integrity, 0..coreMax

	phase     phase
	startedAt time.Time     // when the run began (for survival time)
	survived  time.Duration // final survival time once over
	nextSpawn time.Time     // when the next fault erupts

	now     time.Time
	lastNow time.Time

	// Leaderboard posting. Survival is a shared co-op metric (every boarded crew
	// member shares the run length), so each tracked member is recorded with the
	// same value. The keeper posts live on improvement (Record), on disconnect
	// (FlushLeave, OnLeave), and periodically (FlushAll) so an abandoned-but-
	// ticking reactor keeps recording.
	sk        *kit.ScoreKeeper
	lastFlush time.Time // game-clock time of the last periodic flush

	hold  *keyhold.Tracker // per-key held state, for FIRE hold-to-smother
	frame *kit.Frame       // long-lived render buffer, reused every frame
}

// flushInterval is how much game time elapses between periodic FlushAll posts.
// Gated on r.Now() (no wall clock, no RNG) so a forgotten, still-ticking
// reactor keeps recording survival to the board.
const flushInterval = 10 * time.Second

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	rm := &room{
		cfg:   cfg,
		svc:   svc,
		crew:  map[string]*crewMember{},
		names: map[string]kit.Player{},
		core:  coreMax,
		sk:    kit.NewScoreKeeper(kit.OnImprove),
		hold:  keyhold.New(0),
		frame: kit.NewFrame(),
	}
	rm.buildShip()
	return rm
}

// --- ship construction -------------------------------------------------------

// buildShip lays out the reactor: hull, interior bulkheads with doorways, a
// central core chamber, and the station cells. It is fully deterministic so the
// map is identical for everyone.
func (rm *room) buildShip() {
	// Start every cell as floor, then wall the hull perimeter.
	for r := 0; r < interiorRows; r++ {
		for c := 0; c < cols; c++ {
			if r == 0 || r == interiorRows-1 || c == 0 || c == cols-1 {
				rm.grid[r][c] = cellWall
			} else {
				rm.grid[r][c] = cellFloor
			}
		}
	}

	// Two vertical bulkheads split the deck into left / mid / right thirds, and
	// two horizontal bulkheads split it into top / mid / bottom — nine bays.
	vx := []int{cols / 3, 2 * cols / 3}
	hy := []int{interiorRows / 3, 2 * interiorRows / 3}
	for _, c := range vx {
		for r := 1; r < interiorRows-1; r++ {
			rm.grid[r][c] = cellWall
		}
	}
	for _, r := range hy {
		for c := 1; c < cols-1; c++ {
			rm.grid[r][c] = cellWall
		}
	}

	// Cut doorways so every bay connects to its neighbours (gaps in the
	// bulkheads). Doorways sit a few cells off each intersection.
	door := func(r, c int) {
		if r > 0 && r < interiorRows-1 && c > 0 && c < cols-1 {
			rm.grid[r][c] = cellFloor
		}
	}
	for _, c := range vx {
		for _, r := range []int{hy[0] / 2, (hy[0] + hy[1]) / 2, (hy[1] + interiorRows) / 2} {
			door(r, c)
			door(r+1, c)
		}
	}
	for _, r := range hy {
		for _, c := range []int{vx[0] / 2, (vx[0] + vx[1]) / 2, (vx[1] + cols) / 2} {
			door(r, c)
			door(r, c+1)
		}
	}

	// The core chamber: a small box in the dead-center bay whose interior cells
	// are the glowing core (impassable). Crew work the faults from the
	// surrounding floor and corridor.
	cr0, cr1 := hy[0]+2, hy[1]-2
	cc0, cc1 := vx[0]+5, vx[1]-5
	for r := cr0; r <= cr1; r++ {
		for c := cc0; c <= cc1; c++ {
			rm.grid[r][c] = cellCore
		}
	}

	// Stations: fixed, walkable cells scattered through the bays where faults
	// erupt. Pick a tidy lattice and keep only the points that landed on open
	// floor (so none collide with a wall or the core).
	rm.stations = rm.stations[:0]
	for r := 3; r < interiorRows-1; r += 4 {
		for c := 4; c < cols-1; c += 9 {
			if rm.grid[r][c] == cellFloor {
				rm.grid[r][c] = cellStation
				rm.stations = append(rm.stations, [2]int{r, c})
			}
		}
	}
}

// walkable reports whether a crew body may occupy interior cell (r,c). Floor
// and station cells are walkable; walls and the core are not.
func (rm *room) walkable(r, c int) bool {
	if r < 0 || r >= interiorRows || c < 0 || c >= cols {
		return false
	}
	k := rm.grid[r][c]
	return k == cellFloor || k == cellStation
}

// --- lifecycle ---------------------------------------------------------------

func (rm *room) OnStart(r kit.Room) {
	r.SetInputContext(kit.CtxNav)
	rm.now = r.Now()
	rm.startedAt = rm.now
	rm.lastFlush = rm.now
	rm.scheduleNextSpawn()
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	rm.names[p.AccountID] = p
	m, existed := rm.crew[p.AccountID]
	if !existed {
		m = &crewMember{}
		rm.crew[p.AccountID] = m
		rm.order = append(rm.order, p.AccountID)
		m.row, m.col = rm.spawnCell(r)
		m.best = rm.loadBest(r, p)
	}
	m.joined = true
	// The crew member's character IS their body: their glyph is the figure and
	// their character's BACKGROUND colour becomes their crew colour everywhere
	// it shows (body + roster). A zero Character (a host that doesn't declare
	// the feature, or test doubles) reverts to '☺' and the join-order palette.
	if c := p.Character; c.Glyph != "" {
		for _, ru := range c.Glyph {
			m.glyph = ru
			break
		}
		m.color = kit.RGB(c.BgR, c.BgG, c.BgB)
	} else {
		m.glyph = '☺'
		for i, id := range rm.order {
			if id == p.AccountID {
				m.color = palette[i%len(palette)]
				break
			}
		}
	}
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	if m := rm.crew[p.AccountID]; m != nil {
		m.joined = false
	}
	// Post the leaving crew member's current survival so a mid-run disconnect
	// (or a host crash before OnClose) still reaches the board.
	rm.sk.FlushLeave(r, p, kit.StatusDNF)
	rm.render(r)
}

func (rm *room) OnClose(r kit.Room) {
	rm.now = r.Now()
	rm.recordResults(r)
}

// activeCrew counts crew currently in the room — the basis for spawn scaling
// and two-person fault eligibility (mirrors voidrunners keying off live ships).
func (rm *room) activeCrew() int {
	n := 0
	for _, m := range rm.crew {
		if m.joined {
			n++
		}
	}
	if n < 1 {
		n = 1
	}
	return n
}

// --- input -------------------------------------------------------------------

// OnInput moves a crew member or applies a fix interaction. A terminal has no
// key-up, so movement is one cell per press; FIRE/BREACH holds are derived from
// auto-repeat via the keyhold tracker and resolved each wake.
func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	rm.now = r.Now()
	rm.hold.Observe(in, rm.now)
	if rm.phase != phaseRunning {
		rm.render(r)
		return
	}
	m := rm.crew[p.AccountID]
	if m == nil || !m.joined {
		return
	}

	// A jammed valve consumes raw printable keys: if the crew member stands on
	// a valve fault, the next expected key in its sequence advances it (a wrong
	// key resets). This is checked before navigation so the WASD-free sequence
	// keys (which may also be hjkl) drive the valve, not the legs.
	if in.Kind == kit.InputRune {
		if f := rm.faultAt(m.row, m.col); f != nil && f.kind == faultValve {
			rm.tapValve(r, f, in.Rune)
			rm.render(r)
			return
		}
	}

	switch kit.Resolve(in, kit.CtxNav) {
	case kit.ActUp:
		rm.move(m, -1, 0)
	case kit.ActDown:
		rm.move(m, 1, 0)
	case kit.ActLeft:
		rm.move(m, 0, -1)
	case kit.ActRight:
		rm.move(m, 0, 1)
	case kit.ActConfirm:
		// Space on a leak mashes it; on/adjacent a fire the hold tracker (set
		// above via Observe) handles smothering each wake.
		if f := rm.faultAt(m.row, m.col); f != nil && f.kind == faultLeak {
			rm.mashLeak(r, f)
		}
	}
	rm.render(r)
}

func (rm *room) move(m *crewMember, dr, dc int) {
	if rm.walkable(m.row+dr, m.col+dc) {
		m.row += dr
		m.col += dc
	}
}

// --- fix interactions --------------------------------------------------------

// faultAt returns the active fault on cell (r,c), or nil.
func (rm *room) faultAt(r, c int) *fault {
	for _, f := range rm.faults {
		if f.row == r && f.col == c {
			return f
		}
	}
	return nil
}

func (rm *room) mashLeak(r kit.Room, f *fault) {
	f.mashes++
	f.progress = float64(f.mashes) / float64(leakMashes)
	if f.mashes >= leakMashes {
		rm.completeFault(r, f)
	}
}

// tapValve advances the valve sequence if ru is the next expected key; any
// other key resets progress to the start.
func (rm *room) tapValve(r kit.Room, f *fault, ru rune) {
	if f.seqAt < len(f.seq) && ru == f.seq[f.seqAt] {
		f.seqAt++
		f.progress = float64(f.seqAt) / float64(len(f.seq))
		if f.seqAt >= len(f.seq) {
			rm.completeFault(r, f)
		}
		return
	}
	// Wrong key: jam tightens again.
	f.seqAt = 0
	f.progress = 0
}

// completeFault removes a fixed fault and credits whoever is standing on it.
func (rm *room) completeFault(r kit.Room, done *fault) {
	for i, f := range rm.faults {
		if f != done {
			continue
		}
		rm.faults = append(rm.faults[:i], rm.faults[i+1:]...)
		break
	}
	// Credit the fix to every crew member working the cell (both, for a breach).
	for _, m := range rm.crew {
		if !m.joined {
			continue
		}
		if rm.adjacentToFault(m, done) {
			m.fixes++
		}
	}
}

// adjacentToFault reports whether m is positioned to work fault f: on the cell
// for leak/valve/breach, on or orthogonally adjacent for a fire.
func (rm *room) adjacentToFault(m *crewMember, f *fault) bool {
	dr := m.row - f.row
	dc := m.col - f.col
	if f.kind == faultFire {
		return (dr == 0 && dc == 0) ||
			(dr == 0 && (dc == 1 || dc == -1)) ||
			(dc == 0 && (dr == 1 || dr == -1))
	}
	return dr == 0 && dc == 0
}

// --- heartbeat ---------------------------------------------------------------

// OnWake is the ~20Hz heartbeat: advance fixes that depend on continuous time
// (fire holds, breach holds, fire regrowth), drain the core by every active
// fault, erupt new faults on schedule, and end the run when the core dies.
func (rm *room) OnWake(r kit.Room) {
	rm.now = r.Now()
	dt := rm.step()
	if rm.phase != phaseRunning {
		rm.render(r)
		return
	}

	rm.resolveHolds(r, dt)
	rm.drainCore(dt)
	rm.maybeSpawn(r)

	if rm.core <= 0 {
		rm.core = 0
		rm.phase = phaseOver
		rm.survived = rm.now.Sub(rm.startedAt)
		rm.postSurvival(r)
		rm.recordResults(r)
		rm.render(r)
		return
	}

	rm.postSurvival(r)
	rm.maybeFlush(r)
	rm.render(r)
}

// postSurvival records the current shared survival time for every boarded crew
// member. OnImprove means a Post only reaches the board when the whole-second
// metric advances, so the steady state is one post per crew per survived
// second — the live feed for the leaderboard.
func (rm *room) postSurvival(r kit.Room) {
	secs := int(rm.survivedSeconds())
	for _, id := range rm.order {
		m := rm.crew[id]
		if m == nil || !m.joined {
			continue
		}
		if p, ok := rm.names[id]; ok {
			rm.sk.Record(r, p, secs)
		}
	}
}

// maybeFlush re-posts every tracked crew member's survival on a throttled
// game-clock interval, so a reactor left ticking with nobody improving the
// metric (or with no improvement since the last post) still keeps recording.
func (rm *room) maybeFlush(r kit.Room) {
	if rm.now.Sub(rm.lastFlush) < flushInterval {
		return
	}
	rm.lastFlush = rm.now
	rm.sk.FlushAll(r, kit.StatusFinished)
}

// step returns seconds since the last wake, clamped so a pause / hibernation
// can't drain the core in one giant tick.
func (rm *room) step() float64 {
	dt := 0.05
	if !rm.lastNow.IsZero() {
		if d := rm.now.Sub(rm.lastNow).Seconds(); d > 0 {
			dt = math.Min(d, 0.2)
		}
	}
	rm.lastNow = rm.now
	return dt
}

// resolveHolds advances the time-based fixes — FIRE (held space, on or
// adjacent) and BREACH (two crew on the cell) — and regrows fires nobody is
// holding. Mash/valve fixes happen in OnInput, not here.
func (rm *room) resolveHolds(r kit.Room, dt float64) {
	// Tally how many qualifying crew are working each fire/breach this tick.
	for _, f := range rm.faults {
		f.holders = 0
	}
	for _, m := range rm.crew {
		if !m.joined {
			continue
		}
		for _, f := range rm.faults {
			if f.kind != faultFire && f.kind != faultBreach {
				continue
			}
			if !rm.adjacentToFault(m, f) {
				continue
			}
			if f.kind == faultFire && !rm.hold.HeldRune(' ', rm.now) {
				continue
			}
			f.holders++
		}
	}

	done := rm.faults[:0:0]
	for _, f := range rm.faults {
		switch f.kind {
		case faultFire:
			if f.holders > 0 {
				f.progress += dt / fireHoldSecs
			} else {
				f.progress -= dt * fireRegrow / fireHoldSecs
			}
			f.progress = clampf(f.progress, 0, 1)
		case faultBreach:
			if f.holders >= 2 {
				f.progress += dt / breachHoldSecs
			} else {
				// Breach needs both bodies; it bleeds back without the pair.
				f.progress -= dt * fireRegrow / breachHoldSecs
			}
			f.progress = clampf(f.progress, 0, 1)
		}
		if f.progress >= 1 {
			done = append(done, f)
		}
	}
	for _, f := range done {
		rm.completeFault(r, f)
	}
}

// drainCore subtracts the integrity eaten by every active fault this tick.
func (rm *room) drainCore(dt float64) {
	var drain float64
	for _, f := range rm.faults {
		base := 0.0
		switch f.kind {
		case faultLeak:
			base = leakDrain
		case faultFire:
			base = fireDrain
		case faultValve:
			base = valveDrain
		case faultBreach:
			base = breachDrain
		}
		age := rm.now.Sub(f.born).Seconds()
		drain += base + age*ageDrainMul
	}
	rm.core = clampf(rm.core-drain*dt, 0, coreMax)
}

// --- spawning ----------------------------------------------------------------

// maybeSpawn erupts a new fault when the schedule says so (and there is room),
// then reschedules the next eruption.
func (rm *room) maybeSpawn(r kit.Room) {
	if !rm.now.After(rm.nextSpawn) {
		return
	}
	if len(rm.faults) < maxFaults {
		rm.spawnFault(r)
	}
	rm.scheduleNextSpawn()
}

// spawnInterval is the gap before the next fault: it shrinks as the run goes on
// (panic ramps), and is stretched sub-linearly by crew size so a bigger crew
// faces fewer faults per person. Mirrors voidrunners' craterTarget() solo-vs-
// multi adaptation: solo gets the gentlest cadence; crew scales the load up,
// but only by sqrt(crew), so recruiting friends genuinely eases the shift.
func (rm *room) spawnInterval() time.Duration {
	elapsed := rm.now.Sub(rm.startedAt).Seconds()
	// Ramp: the base interval decays toward the floor with elapsed time.
	ramp := spawnBase * math.Exp(-elapsed/(spawnRampSecs*6))
	if ramp < spawnFloor {
		ramp = spawnFloor
	}
	// Sub-linear crew scaling: divide the per-spawn gap by sqrt(crew) so total
	// load grows slower than crew size (load/crew falls as crew rises).
	crew := float64(rm.activeCrew())
	gap := ramp / math.Sqrt(crew)
	if gap < spawnFloor/2 {
		gap = spawnFloor / 2
	}
	return time.Duration(gap * float64(time.Second))
}

func (rm *room) scheduleNextSpawn() {
	rm.nextSpawn = rm.now.Add(rm.spawnInterval())
}

// spawnFault erupts one fault at a random free station. With 2+ crew it may be
// a two-person BREACH; with a lone engineer it never is.
func (rm *room) spawnFault(r kit.Room) {
	rng := r.Rand()
	cell, ok := rm.freeStation(rng)
	if !ok {
		return
	}
	kind := rm.pickFaultKind(rng)
	f := &fault{kind: kind, row: cell[0], col: cell[1], born: rm.now}
	if kind == faultValve {
		f.seq = rm.makeValveSeq(rng)
	}
	rm.faults = append(rm.faults, f)
}

// pickFaultKind chooses a fault type. BREACH is only ever offered with 2+
// active crew (the hard rule: never spawn a two-person fault solo), and only
// occasionally even then.
func (rm *room) pickFaultKind(rng interface {
	Intn(int) int
	Float64() float64
}) faultKind {
	if rm.activeCrew() >= 2 && rng.Float64() < 0.2 {
		return faultBreach
	}
	switch rng.Intn(3) {
	case 0:
		return faultLeak
	case 1:
		return faultFire
	default:
		return faultValve
	}
}

// valveKeys are the printable keys a jammed valve may ask for (lowercase
// letters that are NOT movement aliases h/j/k/l, and not q which leaves).
var valveKeys = []rune("abcdefginoprstuvwxyz")

func (rm *room) makeValveSeq(rng interface{ Intn(int) int }) []rune {
	n := valveMinKeys + rng.Intn(valveMaxKeys-valveMinKeys+1)
	seq := make([]rune, n)
	for i := range seq {
		seq[i] = valveKeys[rng.Intn(len(valveKeys))]
	}
	return seq
}

// freeStation returns a random station cell that currently has no fault on it.
func (rm *room) freeStation(rng interface{ Intn(int) int }) ([2]int, bool) {
	if len(rm.stations) == 0 {
		return [2]int{}, false
	}
	start := rng.Intn(len(rm.stations))
	for i := 0; i < len(rm.stations); i++ {
		st := rm.stations[(start+i)%len(rm.stations)]
		if rm.faultAt(st[0], st[1]) == nil {
			return st, true
		}
	}
	return [2]int{}, false
}

// spawnCell picks a walkable starting cell for a fresh crew member, away from
// the core, deterministically chosen from the open floor.
func (rm *room) spawnCell(r kit.Room) (int, int) {
	rng := r.Rand()
	for try := 0; try < 40; try++ {
		row := 1 + rng.Intn(interiorRows-2)
		col := 1 + rng.Intn(cols-2)
		if rm.grid[row][col] == cellFloor {
			return row, col
		}
	}
	return 1, 1
}

// --- durable best survival ---------------------------------------------------

// recordResults persists each crew member's best survival time (the shared run
// length) to durable KV so the arcade leaderboard can read it.
func (rm *room) recordResults(r kit.Room) {
	secs := int(rm.survivedSeconds())
	for id := range rm.crew {
		p, ok := rm.names[id]
		if !ok {
			continue
		}
		acct := r.Services().Accounts.For(p)
		if acct == nil {
			continue
		}
		_ = acct.Store().Set(context.Background(), "best",
			[]byte(strconv.Itoa(secs)), kit.MergeMax)
	}
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

// survivedSeconds is the run length: live elapsed while running, frozen once
// the core dies.
func (rm *room) survivedSeconds() float64 {
	if rm.phase == phaseOver {
		return rm.survived.Seconds()
	}
	return rm.now.Sub(rm.startedAt).Seconds()
}
