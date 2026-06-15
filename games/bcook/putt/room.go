package main

import (
	"math"
	"sort"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// room is the live game state. Per-golfer state lives in golfers, keyed by
// account id (hibernation-safe); the rest is the round bookkeeping advanced on
// each wake.
type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	golfers map[string]*golfer    // by account id
	names   map[string]kit.Player // account id -> player (for handle/character)
	order   []string              // join order of account ids (stable scorecard)

	holeIdx int   // current hole index (0..8)
	phase   phase // play / scorecard / final

	hub float64 // windmill arm angle (radians), advanced each wake

	phaseUntil time.Time // when a scorecard/final intermission ends
	now        time.Time
	lastNow    time.Time

	frame *kit.Frame // long-lived render buffer, reused every frame
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:     cfg,
		svc:     svc,
		golfers: map[string]*golfer{},
		names:   map[string]kit.Player{},
		frame:   kit.NewFrame(),
	}
}

// --- lifecycle ---------------------------------------------------------------

func (rm *room) OnStart(r kit.Room) {
	r.SetInputContext(kit.CtxNav)
	rm.now = r.Now()
	rm.holeIdx = 0
	rm.phase = phasePlay
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	rm.names[p.AccountID] = p
	if _, ok := rm.golfers[p.AccountID]; !ok {
		g := &golfer{glyph: '●', color: palette[len(rm.order)%len(palette)], notch: defaultNotch}
		rm.golfers[p.AccountID] = g
		rm.order = append(rm.order, p.AccountID)
		// Backfill par for holes already finished by everyone else, so a late
		// joiner's total stays comparable rather than artificially low.
		for h := 0; h < rm.holeIdx; h++ {
			g.scores = append(g.scores, holes[h].par)
		}
		rm.placeAtTee(g)
	}
	// The golfer's character colours their ball + scorecard row. A zero
	// Character (a host that doesn't declare the feature, or a test double)
	// reverts to the '●' ball and the join-order palette colour.
	g := rm.golfers[p.AccountID]
	if c := p.Character; c.Glyph != "" {
		for _, ru := range c.Glyph {
			g.glyph = ru
			break
		}
		g.color = kit.RGB(c.BgR, c.BgG, c.BgB)
	} else {
		g.glyph = '●'
		for i, id := range rm.order {
			if id == p.AccountID {
				g.color = palette[i%len(palette)]
				break
			}
		}
	}
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	// A golfer who quits mid-round still posts a leaderboard result so their
	// progress isn't silently lost. The board is LOWER-better and the reader
	// ranks a DNF row exactly like a finished one, so we must NOT post the raw
	// partial total (a 2-hole quitter with 6 strokes would unfairly top the
	// board). Instead post a fair full-round ESTIMATE: actual strokes on the
	// COMPLETED holes plus par for every hole not yet completed.
	//
	// "Completed" means committed to g.scores — that's exactly the holes the
	// final 9-hole total (g.total) counts. A hole in progress (g.strokes on the
	// current hole) is deliberately treated as "remaining" and filled with par,
	// matching the existing total computation.
	if g, ok := rm.golfers[p.AccountID]; ok {
		est := g.total()
		for h := len(g.scores); h < len(holes); h++ {
			est += holes[h].par
		}
		r.Post(kit.Result{Rankings: []kit.PlayerResult{{
			Player: p,
			Metric: est,
			Status: kit.StatusDNF,
		}}})
	}
	delete(rm.golfers, p.AccountID)
	delete(rm.names, p.AccountID)
	for i, id := range rm.order {
		if id == p.AccountID {
			rm.order = append(rm.order[:i], rm.order[i+1:]...)
			break
		}
	}
	rm.render(r)
}

func (rm *room) OnClose(r kit.Room) {}

// --- input -------------------------------------------------------------------

// OnInput handles aiming, the power dial, and the putt. Left/Right rotate the
// aim indicator; Up/Down step the notched power dial (the setting persists
// between shots — dial it once and it stays, like a scroll wheel); Space putts
// immediately at the dialed power. No hold-to-charge: over SSH, latency jitter
// moves a release point, but a notch you dialed is exactly the notch you get.
func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	rm.now = r.Now()
	g := rm.golfers[p.AccountID]
	if g == nil || rm.phase != phasePlay {
		return
	}
	switch kit.Resolve(in, kit.CtxNav) {
	case kit.ActLeft:
		if g.state == stateAim {
			g.aim -= aimStepRad
		}
	case kit.ActRight:
		if g.state == stateAim {
			g.aim += aimStepRad
		}
	case kit.ActUp:
		// Dial a notch of power up. Many terminals translate mouse scroll into
		// arrow up/down, so the dial doubles as a literal scroll wheel.
		if g.notch < powerNotches {
			g.notch++
		}
	case kit.ActDown:
		if g.notch > 1 {
			g.notch--
		}
	case kit.ActConfirm:
		// Space: putt right now at the dialed power.
		if g.state == stateAim {
			rm.launch(g)
		}
	}
	rm.render(r)
}

// --- heartbeat ---------------------------------------------------------------

// OnWake is the ~20 Hz heartbeat: advance the windmill, integrate the rolling
// balls, resolve hole-outs and the stroke cap, drive the scorecard timers, and
// render every view.
func (rm *room) OnWake(r kit.Room) {
	rm.now = r.Now()
	dt := rm.step()

	switch rm.phase {
	case phasePlay:
		rm.advanceWindmill(dt)
		rm.advanceGolfers(r, dt)
		rm.checkHoleComplete(r)
	case phaseScorecard:
		if !rm.now.Before(rm.phaseUntil) {
			rm.nextHole(r)
		}
	case phaseFinal:
		if !rm.now.Before(rm.phaseUntil) && !r.Settled() {
			rm.settle(r)
		}
	}
	rm.render(r)
}

// step returns seconds elapsed since the last wake, clamped so a pause or
// hibernation can't teleport a ball across the course.
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

func (rm *room) advanceWindmill(dt float64) {
	h := &holes[rm.holeIdx]
	if h.windmill == nil {
		return
	}
	rm.hub += h.windmill.rate * dt
	if rm.hub > 2*math.Pi {
		rm.hub -= 2 * math.Pi
	}
}

// --- per-golfer physics ------------------------------------------------------

func (rm *room) advanceGolfers(r kit.Room, dt float64) {
	for _, g := range rm.golfers {
		if g.state == stateRoll {
			rm.advanceRoll(r, g, dt)
		}
	}
}

// launch fires a putt at the dialed notch and counts the stroke. The dial is
// left where it is — the next shot reuses the setting until it's re-dialed.
func (rm *room) launch(g *golfer) {
	speed := notchSpeed(g.notch)
	g.preX, g.preY = g.x, g.y
	g.vx = math.Cos(g.aim) * speed
	g.vy = math.Sin(g.aim) * speed * aspect
	g.state = stateRoll
	g.strokes++
}

// advanceRoll integrates one rolling ball: friction by surface, sub-stepped
// motion with wall bounces and the windmill, then hazard and rest checks.
func (rm *room) advanceRoll(r kit.Room, g *golfer, dt float64) {
	h := &holes[rm.holeIdx]
	g.prevX, g.prevY = g.x, g.y

	// Friction depends on the surface under the ball.
	fric := rollFric
	if h.at(roundCell(g.y), roundCell(g.x)) == tileSand {
		fric = sandFric
	}
	drag := math.Pow(fric, dt)
	g.vx *= drag
	g.vy *= drag

	// Sub-step so a fast ball can't tunnel through a one-cell wall.
	speed := math.Hypot(g.vx, g.vy/aspect)
	steps := int(speed*dt) + 1
	if steps > 8 {
		steps = 8
	}
	sdt := dt / float64(steps)
	for i := 0; i < steps; i++ {
		if rm.moveSub(r, h, g, sdt) {
			return // sunk or reset mid-step
		}
	}

	// Come to rest on the green / sand.
	if math.Hypot(g.vx, g.vy/aspect) < stopSpeed {
		g.vx, g.vy = 0, 0
		g.state = stateAim
	}
}

// moveSub advances the ball one sub-step, reflecting off walls/windmill and
// resolving water/cup. It returns true when the ball was sunk or water-reset
// (the caller should stop integrating this frame).
func (rm *room) moveSub(r kit.Room, h *hole, g *golfer, sdt float64) bool {
	nx := g.x + g.vx*sdt
	ny := g.y + g.vy*sdt

	// Resolve walls per-axis so the ball slides along a flush rail instead of
	// sticking, and the windmill arm bats like a wall.
	if rm.solidAt(h, roundCell(ny), roundCell(nx)) {
		// Try X-only and Y-only moves to pick the reflection axis.
		blockX := rm.solidAt(h, roundCell(g.y), roundCell(nx))
		blockY := rm.solidAt(h, roundCell(ny), roundCell(g.x))
		if blockX {
			g.vx = -g.vx * wallDamp
			nx = g.x
		}
		if blockY {
			g.vy = -g.vy * wallDamp
			ny = g.y
		}
		if !blockX && !blockY {
			// Diagonal corner: reflect both.
			g.vx = -g.vx * wallDamp
			g.vy = -g.vy * wallDamp
			nx, ny = g.x, g.y
		}
	}
	g.x, g.y = nx, ny

	// Cup: a near miss at low speed drops; a screamer can lip out (handled by
	// the speed gate, which makes hard putts skip over).
	if dist2(g.x, g.y, float64(h.cupX), float64(h.cupY)) <= cupRadius*cupRadius &&
		math.Hypot(g.vx, g.vy/aspect) < cupCatch {
		rm.sink(g)
		return true
	}

	// Water: splash. One-stroke penalty and reset to the pre-shot spot.
	if h.at(roundCell(g.y), roundCell(g.x)) == tileWater {
		g.strokes++
		g.x, g.y = g.preX, g.preY
		g.vx, g.vy = 0, 0
		g.state = stateAim
		return true
	}
	return false
}

// solidAt reports whether a cell stops the ball: a wall tile, or a live
// windmill arm cell on this hole.
func (rm *room) solidAt(h *hole, row, col int) bool {
	if h.at(row, col) == tileWall {
		return true
	}
	return rm.windmillBlocks(h, row, col)
}

func (rm *room) sink(g *golfer) {
	g.vx, g.vy = 0, 0
	g.state = stateSunk
	g.sunkAt = rm.now
}

// --- windmill ----------------------------------------------------------------

// windmillCells yields the solid arm cells for the current hole's windmill at
// the current angle. Used both for collision and rendering.
func (rm *room) windmillBlocks(h *hole, row, col int) bool {
	wm := h.windmill
	if wm == nil {
		return false
	}
	for a := 0; a < wm.arms; a++ {
		ang := rm.hub + float64(a)*2*math.Pi/float64(wm.arms)
		dx, dy := math.Cos(ang), math.Sin(ang)*aspect
		for l := 1; l <= wm.length; l++ {
			cx := roundCell(float64(wm.hubX) + dx*float64(l))
			cy := roundCell(float64(wm.hubY) + dy*float64(l))
			if cx == col && cy == row {
				return true
			}
		}
	}
	// The hub itself is solid too.
	return col == wm.hubX && row == wm.hubY
}

// --- hole flow ---------------------------------------------------------------

const (
	cupRadius = 1.1  // hole-out radius (horizontal cells)
	cupCatch  = 16.0 // max speed at which the ball drops instead of lipping out
)

func (rm *room) placeAtTee(g *golfer) {
	h := &holes[rm.holeIdx]
	g.x, g.y = h.teeX, h.teeY
	g.prevX, g.prevY = g.x, g.y
	g.preX, g.preY = g.x, g.y
	g.vx, g.vy = 0, 0
	g.state = stateAim
	g.aim = aimToCup(g, h)
	// The power dial is deliberately NOT reset: it persists between shots and
	// holes (the scroll-wheel feel — you dial it, it stays).
	g.strokes = 0
	g.holeIdx = rm.holeIdx
}

// aimToCup points a fresh ball roughly at the flag so a new hole starts sane.
func aimToCup(g *golfer, h *hole) float64 {
	return math.Atan2((float64(h.cupY)-g.y)*1, float64(h.cupX)-g.x)
}

// checkHoleComplete records finished golfers (sunk, or capped at par+4) and, if
// every active golfer is done, freezes their hole score and shows the scorecard.
func (rm *room) checkHoleComplete(r kit.Room) {
	if len(rm.golfers) == 0 {
		return
	}
	h := &holes[rm.holeIdx]
	cap := h.par + strokeCapOverPar
	allDone := true
	for _, g := range rm.golfers {
		if g.state == stateSunk {
			continue
		}
		// A non-sunk ball that's at rest and over the cap is conceded.
		if g.state == stateAim && g.strokes >= cap {
			continue
		}
		allDone = false
	}
	if !allDone {
		return
	}
	// Freeze each golfer's score for this hole (sunk = strokes; conceded = cap).
	for _, g := range rm.golfers {
		score := g.strokes
		if g.state != stateSunk && score < cap {
			score = cap
		}
		// Only append if this golfer hasn't been scored for this hole yet.
		if len(g.scores) == rm.holeIdx {
			g.scores = append(g.scores, score)
		}
	}
	if rm.holeIdx >= len(holes)-1 {
		rm.phase = phaseFinal
		rm.phaseUntil = rm.now.Add(finalDwell)
	} else {
		rm.phase = phaseScorecard
		rm.phaseUntil = rm.now.Add(scorecardDwell)
	}
}

func (rm *room) nextHole(r kit.Room) {
	rm.holeIdx++
	rm.phase = phasePlay
	rm.hub = 0
	for _, g := range rm.golfers {
		rm.placeAtTee(g)
	}
}

// settle ends the round and submits each golfer's 9-hole total to the
// leaderboard (lower is better), ranked ascending.
func (rm *room) settle(r kit.Room) {
	type entry struct {
		p     kit.Player
		total int
	}
	var es []entry
	for _, id := range rm.order {
		g := rm.golfers[id]
		p, ok := rm.names[id]
		if g == nil || !ok {
			continue
		}
		es = append(es, entry{p: p, total: g.total()})
	}
	sort.SliceStable(es, func(i, j int) bool { return es[i].total < es[j].total })

	rankings := make([]kit.PlayerResult, 0, len(es))
	for i, e := range es {
		rankings = append(rankings, kit.PlayerResult{
			Player: e.p,
			Metric: e.total,
			Rank:   i + 1,
			Status: kit.StatusFinished,
		})
	}
	r.End(kit.Result{Rankings: rankings})
}

// --- render entry point ------------------------------------------------------

func (rm *room) render(r kit.Room) {
	for _, p := range r.Members() {
		rm.frame.Clear()
		rm.composeFor(rm.frame, p)
		r.Send(p, rm.frame)
	}
}
