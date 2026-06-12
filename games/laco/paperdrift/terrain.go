package main

import "math/rand"

// World geometry. The sky band is worldRows rows tall; physics runs in
// column units (1 column = 1 m) where one row is two columns tall, because
// terminal cells are ~2:1. y grows downward, 0 at the top of the sky.
const (
	worldRows = 21 // viewport rows 1..21 show world rows 0..20
	rowUnits  = 2.0
	worldH    = worldRows * rowUnits

	// difficultyRamp is the distance (columns) over which the course reaches
	// full difficulty: narrower corridors, tighter gate gaps, closer gates.
	difficultyRamp = 2400
)

// terrain is the per-column world strip, generated lazily ahead of the
// leading glider from the room RNG (deterministic under a seeded room).
// Per column: a solid ceiling band, a solid ground band, optionally a gate
// (a wall with a gap) and/or a thermal (rising air).
type terrain struct {
	ceil  []int16 // rows 0..ceil-1 are solid ceiling
	floor []int16 // rows floor..worldRows-1 are solid ground
	gTop  []int16 // gate gap top row; -1 = no gate at this column
	gBot  []int16 // gate gap bottom row (inclusive)
	therm []bool  // updraft column

	// generator state
	curCeil, curFloor int
	nextGate          int
	gateCols          int // columns left of the gate currently being emitted
	gateTop, gateBot  int
	thermX0, thermX1  int
}

// reset clears the strip for a fresh round, reusing slice capacity.
func (t *terrain) reset() {
	t.ceil, t.floor = t.ceil[:0], t.floor[:0]
	t.gTop, t.gBot = t.gTop[:0], t.gBot[:0]
	t.therm = t.therm[:0]
	t.curCeil, t.curFloor = 2, 17
	t.nextGate = 130 // a gentle opening stretch before the first wall
	t.gateCols = 0
	t.thermX0, t.thermX1 = -1, -1 // thermals first appear past the first gate
}

// difficulty ramps 0→1 over the first difficultyRamp columns and holds.
func difficulty(x int) float64 {
	d := float64(x) / difficultyRamp
	if d > 1 {
		return 1
	}
	return d
}

// ensure extends the strip through column upTo.
func (t *terrain) ensure(rng *rand.Rand, upTo int) {
	for x := len(t.floor); x <= upTo; x++ {
		t.genCol(rng, x)
	}
}

func (t *terrain) genCol(rng *rand.Rand, x int) {
	d := difficulty(x)
	open := 17 - int(6*d) // guaranteed open corridor height, 17 → 11 rows

	// Random-walk the ground and ceiling, more restless as difficulty rises.
	// The walk freezes while a gate is being emitted so the gap stays valid
	// across the wall's full thickness.
	if t.gateCols == 0 {
		t.walk(rng, d, open)
	}

	gT, gB := -1, -1
	if x == t.nextGate {
		gh := 8 - int(4*d) // gap height, 8 → 4 rows
		lo := t.curCeil + 1
		hi := t.curFloor - 1 - gh
		if hi < lo {
			hi = lo
		}
		t.gateCols = 2 // walls are two columns thick
		t.gateTop = lo + rng.Intn(hi-lo+1)
		t.gateBot = t.gateTop + gh
		spacing := 48 + rng.Intn(36) - int(14*d)
		t.nextGate = x + spacing
		// Most inter-gate stretches carry a thermal to claw height back.
		if rng.Float64() < 0.7 {
			tx := x + 10 + rng.Intn(maxInt(spacing-26, 1))
			t.thermX0, t.thermX1 = tx, tx+6+rng.Intn(5)
		}
	}
	if t.gateCols > 0 {
		gT, gB = t.gateTop, t.gateBot
		t.gateCols--
	}

	t.ceil = append(t.ceil, int16(t.curCeil))
	t.floor = append(t.floor, int16(t.curFloor))
	t.gTop = append(t.gTop, int16(gT))
	t.gBot = append(t.gBot, int16(gB))
	t.therm = append(t.therm, x >= t.thermX0 && x <= t.thermX1)
}

func (t *terrain) walk(rng *rand.Rand, d float64, open int) {
	if rng.Float64() < 0.20+0.25*d {
		t.curFloor += rng.Intn(3) - 1
	}
	if rng.Float64() < 0.10+0.15*d {
		t.curCeil += rng.Intn(3) - 1
	}
	t.curCeil = clampInt(t.curCeil, 1, 6)
	t.curFloor = clampInt(t.curFloor, 12, worldRows-1)
	for t.curFloor-t.curCeil < open {
		if t.curCeil > 1 {
			t.curCeil--
		} else if t.curFloor < worldRows-1 {
			t.curFloor++
		} else {
			break
		}
	}
}

// solid reports whether world position (column x, row yRow) is inside
// terrain or a gate wall. Columns outside the generated strip are open air
// (physics always ensures ahead of the leader).
func (t *terrain) solid(x int, yRow float64) bool {
	if x < 0 || x >= len(t.floor) {
		return false
	}
	if yRow < float64(t.ceil[x]) || yRow >= float64(t.floor[x]) {
		return true
	}
	if t.gTop[x] >= 0 && (yRow < float64(t.gTop[x]) || yRow >= float64(t.gBot[x])+1) {
		return true
	}
	return false
}

// midY returns the open-corridor midpoint at column x, in column units.
func (t *terrain) midY(x int) float64 {
	if x < 0 || x >= len(t.floor) {
		return worldH / 2
	}
	return (float64(t.ceil[x]) + float64(t.floor[x])) / 2 * rowUnits
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
