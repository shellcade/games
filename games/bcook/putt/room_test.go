package main

import (
	"math"
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// newTestRoom builds a started room driven by a kittest.Room (deterministic
// clock + rng), with no players joined yet.
func newTestRoom(t *testing.T, handles ...string) (*room, *kittest.Room) {
	t.Helper()
	players := make([]kit.Player, len(handles))
	for i, h := range handles {
		players[i] = kittest.Player(h)
	}
	tr := kittest.NewRoom(players...)
	rm := newRoom(tr.Cfg, tr.Services())
	rm.OnStart(tr)
	return rm, tr
}

func keyRune(ru rune) kit.Input { return kit.Input{Kind: kit.InputRune, Rune: ru} }
func keyNamed(k kit.Key) kit.Input {
	return kit.Input{Kind: kit.InputKey, Key: k}
}

// settle drives wakes until a rolling ball comes to rest or sinks (bounded).
func runUntilRest(rm *room, tr *kittest.Room, g *golfer, maxWakes int) {
	for i := 0; i < maxWakes && g.state == stateRoll; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
}

func TestStartAndSmokeNoPanic(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	if len(rm.golfers) != 2 {
		t.Fatalf("want 2 golfers, got %d", len(rm.golfers))
	}

	inputs := []kit.Input{
		keyNamed(kit.KeyLeft), keyNamed(kit.KeyRight), keyNamed(kit.KeyUp),
		keyNamed(kit.KeyDown), keyRune(' '),
	}
	players := []kit.Player{a, b}
	for i := 0; i < 600; i++ {
		p := players[i%2]
		rm.OnInput(tr, p, inputs[i%len(inputs)])
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
		// Every ball must stay on the playfield (never round onto HUD rows /
		// off the edge).
		for id, g := range rm.golfers {
			row, col := roundCell(g.y), roundCell(g.x)
			if col < 0 || col >= cols || row < top || row > bottom {
				t.Fatalf("golfer %s off-field at (row %d,col %d) from (%.2f,%.2f)", id, row, col, g.x, g.y)
			}
		}
	}
}

// TestFrictionStopsBall: a putt on the open green eventually comes to rest.
func TestFrictionStopsBall(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]

	g.x, g.y = 40, 11
	g.aim = 0 // due east
	g.notch = 6
	rm.launch(g)
	if g.state != stateRoll {
		t.Fatalf("ball should be rolling after launch, state=%d", g.state)
	}
	runUntilRest(rm, tr, g, 200)
	if g.state == stateRoll {
		t.Fatal("ball never came to rest - friction not bleeding speed")
	}
	if g.vx != 0 || g.vy != 0 {
		t.Fatalf("rested ball has residual velocity (%.3f,%.3f)", g.vx, g.vy)
	}
}

// TestWallBounceReflects: a ball putted hard into the west rail reverses x.
func TestWallBounceReflects(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]

	// Park near the left rail, aim due west into it at speed.
	g.x, g.y = 3, 10
	g.preX, g.preY = g.x, g.y
	g.aim = 0
	g.vx, g.vy = -40, 0
	g.state = stateRoll
	// One sub-stepped frame should reflect vx to positive (bounced east).
	for i := 0; i < 5 && g.vx < 0; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if g.vx <= 0 {
		t.Fatalf("ball did not bounce off the west wall, vx=%.3f", g.vx)
	}
}

// TestWaterPenaltyAndReset: rolling into water adds a stroke and snaps the ball
// back to its pre-shot spot.
func TestWaterPenaltyAndReset(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]

	// Use hole 4 (Water Carry): park just above the pond and putt south into it.
	rm.holeIdx = 3
	h := &holes[rm.holeIdx]
	// Find a water cell to confirm geometry, then sit a couple rows above it.
	wx, wy := -1, -1
	for row := top; row <= bottom && wx < 0; row++ {
		for col := 0; col < cols; col++ {
			if h.at(row, col) == tileWater {
				wx, wy = col, row
				break
			}
		}
	}
	if wx < 0 {
		t.Fatal("hole 4 has no water cell")
	}
	g.x, g.y = float64(wx), float64(wy-3)
	g.preX, g.preY = g.x, g.y
	preX, preY := g.x, g.y
	startStrokes := g.strokes
	// Putt south into the pond at moderate power.
	g.aim = 0
	g.vx, g.vy = 0, 20*aspect
	g.state = stateRoll
	for i := 0; i < 40 && g.state == stateRoll; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if g.strokes != startStrokes+1 {
		t.Fatalf("water splash should cost 1 stroke: before %d after %d", startStrokes, g.strokes)
	}
	if roundCell(g.x) != roundCell(preX) || roundCell(g.y) != roundCell(preY) {
		t.Fatalf("water reset should restore pre-shot spot: got (%.1f,%.1f) want (%.1f,%.1f)", g.x, g.y, preX, preY)
	}
}

// TestHoleOutDetection: a slow ball arriving at the cup sinks.
func TestHoleOutDetection(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]
	h := &holes[rm.holeIdx]

	// Place the ball just short of the cup, creeping toward it.
	g.x, g.y = float64(h.cupX)-2, float64(h.cupY)
	g.vx, g.vy = 8, 0
	g.state = stateRoll
	for i := 0; i < 20 && g.state == stateRoll; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if g.state != stateSunk {
		t.Fatalf("ball should have sunk at the cup, state=%d at (%.1f,%.1f)", g.state, g.x, g.y)
	}
}

// TestStrokeCapConcedesHole: a golfer who never sinks is conceded at par+cap and
// the round advances.
func TestStrokeCapConcedesHole(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]
	h := &holes[rm.holeIdx]
	cap := h.par + strokeCapOverPar

	g.strokes = cap
	g.state = stateAim // at rest, over the cap
	rm.checkHoleComplete(tr)

	if len(g.scores) != 1 {
		t.Fatalf("conceded hole should record a score, got %d entries", len(g.scores))
	}
	if g.scores[0] != cap {
		t.Fatalf("conceded score should be the cap %d, got %d", cap, g.scores[0])
	}
	if rm.phase != phaseScorecard {
		t.Fatalf("hole should transition to scorecard, phase=%d", rm.phase)
	}
}

// TestScorecardTotals: scores accumulate across holes into the round total.
func TestScorecardTotals(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]
	g.scores = []int{2, 3, 4}
	if g.total() != 9 {
		t.Fatalf("total = %d, want 9", g.total())
	}
	_ = tr
}

// TestFullRoundSettlesWithLeaderboard: sinking every hole drives the round to a
// final result with each golfer's total submitted, lowest-first.
func TestFullRoundSettlesWithLeaderboard(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	for hole := 0; hole < len(holes); hole++ {
		ga, gb := rm.golfers[a.AccountID], rm.golfers[b.AccountID]
		// Alice sinks in 2 (better), Bob in 3.
		ga.strokes, ga.state = 2, stateSunk
		gb.strokes, gb.state = 3, stateSunk
		rm.checkHoleComplete(tr)
		// Advance the intermission timer to roll to the next hole / settle.
		tr.Advance(scorecardDwell + finalDwell + time.Second)
		rm.OnWake(tr)
	}

	if tr.Ended == nil {
		t.Fatal("round never settled - End was not called after hole 9")
	}
	rk := tr.Ended.Rankings
	if len(rk) != 2 {
		t.Fatalf("want 2 rankings, got %d", len(rk))
	}
	if rk[0].Rank != 1 || rk[0].Metric != 18 {
		t.Fatalf("winner should be rank 1 with 18 strokes (2*9), got rank %d metric %d", rk[0].Rank, rk[0].Metric)
	}
	if rk[0].Metric > rk[1].Metric {
		t.Fatalf("rankings not ascending by strokes: %d then %d", rk[0].Metric, rk[1].Metric)
	}
}

// TestHolesFitCanvas: every hole's geometry stays within the course rows, has a
// tee and a cup on fairway, and is fully bordered (no escape).
func TestHolesFitCanvas(t *testing.T) {
	for i := range holes {
		h := &holes[i]
		// Tee and cup inside the course bounds.
		if h.teeY < top || h.teeY > bottom || h.teeX < 0 || h.teeX >= cols {
			t.Errorf("hole %d tee off-canvas at (%.0f,%.0f)", i+1, h.teeX, h.teeY)
		}
		if h.cupY < top || h.cupY > bottom || h.cupX < 0 || h.cupX >= cols {
			t.Errorf("hole %d cup off-canvas at (%d,%d)", i+1, h.cupX, h.cupY)
		}
		// Tee and cup must be on fairway, not buried in a wall/hazard.
		if h.at(h.cupY, h.cupX) != tileFairway {
			t.Errorf("hole %d cup is not on fairway", i+1)
		}
		if h.at(int(h.teeY), int(h.teeX)) != tileFairway {
			t.Errorf("hole %d tee is not on fairway", i+1)
		}
		// Border: the entire outer ring must be wall.
		for col := 0; col < cols; col++ {
			if h.at(top, col) != tileWall || h.at(bottom, col) != tileWall {
				t.Errorf("hole %d top/bottom border leaks at col %d", i+1, col)
				break
			}
		}
		for row := top; row <= bottom; row++ {
			if h.at(row, 0) != tileWall || h.at(row, cols-1) != tileWall {
				t.Errorf("hole %d left/right border leaks at row %d", i+1, row)
				break
			}
		}
		// Par should be sane.
		if h.par < 2 || h.par > 6 {
			t.Errorf("hole %d par %d out of range", i+1, h.par)
		}
	}
}

// TestWindmillBlocksAndSpins: the windmill holes have a hub and the arm angle
// advances over time.
func TestWindmillBlocksAndSpins(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	rm.holeIdx = 6 // Windmill
	h := &holes[rm.holeIdx]
	if h.windmill == nil {
		t.Fatal("hole 7 should have a windmill")
	}
	start := rm.hub
	for i := 0; i < 10; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if rm.hub == start {
		t.Fatal("windmill arm did not rotate")
	}
	// The hub cell itself must block.
	if !rm.solidAt(h, h.windmill.hubY, h.windmill.hubX) {
		t.Fatal("windmill hub should be solid")
	}
}

// TestPowerDialStepsAndClamps: Up/Down step the notch one at a time, clamped
// to [1, powerNotches], and a fresh golfer starts at the mid-range default.
func TestPowerDialStepsAndClamps(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]

	if g.notch != defaultNotch {
		t.Fatalf("fresh golfer notch = %d, want default %d", g.notch, defaultNotch)
	}
	rm.OnInput(tr, a, keyNamed(kit.KeyUp))
	if g.notch != defaultNotch+1 {
		t.Fatalf("Up should step one notch: got %d, want %d", g.notch, defaultNotch+1)
	}
	rm.OnInput(tr, a, keyNamed(kit.KeyDown))
	rm.OnInput(tr, a, keyNamed(kit.KeyDown))
	if g.notch != defaultNotch-1 {
		t.Fatalf("Down should step one notch: got %d, want %d", g.notch, defaultNotch-1)
	}
	// Clamp at the top…
	for i := 0; i < powerNotches*2; i++ {
		rm.OnInput(tr, a, keyNamed(kit.KeyUp))
	}
	if g.notch != powerNotches {
		t.Fatalf("dial should clamp at %d, got %d", powerNotches, g.notch)
	}
	// …and at the bottom.
	for i := 0; i < powerNotches*2; i++ {
		rm.OnInput(tr, a, keyNamed(kit.KeyDown))
	}
	if g.notch != 1 {
		t.Fatalf("dial should clamp at 1, got %d", g.notch)
	}
}

// TestSpacePuttsImmediatelyAtDialedPower: space fires the putt on the spot (no
// charge phase) with launch speed exactly matching the dialed notch.
func TestSpacePuttsImmediatelyAtDialedPower(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]
	g.x, g.y = 40, 11
	g.aim = 0 // due east
	g.notch = 7

	rm.OnInput(tr, a, keyRune(' '))
	if g.state != stateRoll {
		t.Fatalf("space should putt immediately, state=%d", g.state)
	}
	if g.strokes != 1 {
		t.Fatalf("a putt should count 1 stroke, got %d", g.strokes)
	}
	want := notchSpeed(7)
	if math.Abs(g.vx-want) > 1e-9 || g.vy != 0 {
		t.Fatalf("launch velocity (%.2f,%.2f), want (%.2f,0) for notch 7", g.vx, g.vy, want)
	}
	// While rolling, space does nothing (no double-hit).
	rm.OnInput(tr, a, keyRune(' '))
	if g.strokes != 1 {
		t.Fatalf("space mid-roll must not putt again, strokes=%d", g.strokes)
	}
}

// TestPowerDialPersistsAcrossShotsAndHoles: the dial keeps its setting through
// a putt-and-settle and through a tee reset — the scroll-wheel feel.
func TestPowerDialPersistsAcrossShotsAndHoles(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]
	g.x, g.y = 40, 11
	g.aim = 0
	g.notch = 3

	rm.OnInput(tr, a, keyRune(' '))
	runUntilRest(rm, tr, g, 200)
	if g.notch != 3 {
		t.Fatalf("dial changed across a shot: %d, want 3", g.notch)
	}
	rm.holeIdx = 1
	rm.placeAtTee(g)
	if g.notch != 3 {
		t.Fatalf("dial reset by the next tee: %d, want 3", g.notch)
	}
}

// TestNotchSpeedMapping: the dial maps monotonically from a feather at notch 1
// to the full launch speed at the top, clamping out-of-range notches.
func TestNotchSpeedMapping(t *testing.T) {
	if got := notchSpeed(powerNotches); got != maxLaunch {
		t.Fatalf("top notch speed = %.2f, want maxLaunch %.2f", got, maxLaunch)
	}
	lo := notchSpeed(1)
	if lo <= minLaunch || lo > minLaunch+(maxLaunch-minLaunch)*0.05 {
		t.Fatalf("notch 1 speed = %.2f, want a feather just above minLaunch %.2f", lo, minLaunch)
	}
	for n := 2; n <= powerNotches; n++ {
		if notchSpeed(n) <= notchSpeed(n-1) {
			t.Fatalf("notch speeds not strictly increasing at %d", n)
		}
	}
	if notchSpeed(0) != notchSpeed(1) || notchSpeed(powerNotches+5) != notchSpeed(powerNotches) {
		t.Fatal("out-of-range notches should clamp to the dial ends")
	}
}

func TestComposeRendersFrame(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	f := kit.NewFrame()
	rm.composeFor(f, a)
	if len(f.Cells) != kit.Rows || len(f.Cells[0]) != kit.Cols {
		t.Fatal("frame is not 24x80")
	}
}

// TestScorecardCharacterRendersBesideName: each golfer's character tile lands
// right before their name on the scorecard panel.
func TestScorecardCharacterRendersBesideName(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	a.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]
	g.scores = []int{2}
	rm.phase = phaseScorecard

	if got := g.color; got != kit.RGB(0x2D, 0x1B, 0x4E) {
		t.Fatalf("ball colour = %v, want character bg", got)
	}

	f := kit.NewFrame()
	rm.composeFor(f, a)
	found := false
	for r := 0; r < kit.Rows && !found; r++ {
		for c := 0; c+1 < kit.Cols; c++ {
			if f.Cells[r][c] == kit.CharacterCell(a.Character) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("character tile not found on the scorecard")
	}
}

// TestZeroCharacterUsesPalette: a golfer with no character keeps the '●' ball
// and the join-order palette colour.
func TestZeroCharacterUsesPalette(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0] // kittest player: zero Character
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]
	if g.glyph != '●' {
		t.Fatalf("ball glyph = %q, want '●'", g.glyph)
	}
	if g.color != palette[0] {
		t.Fatalf("ball colour = %v, want palette[0] %v", g.color, palette[0])
	}
}

// TestRenderReusesFrame: render keeps using the one long-lived buffer.
func TestRenderReusesFrame(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	before := rm.frame
	rm.render(tr)
	rm.render(tr)
	if rm.frame != before {
		t.Fatal("render replaced rm.frame - it must reuse the single long-lived buffer")
	}
}

// TestSteadyStateWakeAllocs guards against per-tick framing growth under the
// production -gc=leaking collector (every byte allocated is permanent).
func TestSteadyStateWakeAllocs(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob", "cleo")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	// Put a couple of balls in motion so the busy paths are live.
	for _, p := range tr.Players {
		g := rm.golfers[p.AccountID]
		g.vx, g.vy = 20, 5
		g.state = stateRoll
	}
	for i := 0; i < 10; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	allocs := testing.AllocsPerRun(50, func() {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	})
	t.Logf("3-player wake allocs/op: %.1f", allocs)
	if allocs > 40 {
		t.Fatalf("3-player wake allocates %.1f/op - did render() stop reusing rm.frame?", allocs)
	}
}
