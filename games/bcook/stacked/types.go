package main

import (
	"time"

	kit "github.com/shellcade/kit/v2"
)

// Canvas geometry. The terminal is 80x24; row 0 is the scoreboard/header and
// row 23 is the controls bar. The big well occupies the left of the playfield
// for the viewing player; rival wells render as live miniatures down a right
// side panel.
const (
	cols = kit.Cols // 80
	rows = kit.Rows // 24

	// The well is 10 cells wide and 18 tall — fits the frame with room for a
	// border, the next-piece preview, and the rival panel.
	wellW = 10
	wellH = 18

	// Spawn rows: pieces enter at the top with a couple of hidden buffer rows so
	// rotation near the ceiling has space. A block resting in a buffer row at
	// lock time is a top-out.
	spawnRow = 0 // the spawn anchor row inside the grid
)

// Cell values inside a well grid. 0 is empty; 1..len(pieces) tag which piece
// color filled the cell; garbageCell is the welded junk sent by rivals.
const (
	cellEmpty   = 0
	garbageCell = 99
)

// Gameplay tuning.
const (
	// Gravity: how long a piece waits before dropping one row, by level. Level
	// rises every levelStep cleared rows (solo) and the interval shrinks.
	baseGravity = 800 * time.Millisecond
	minGravity  = 90 * time.Millisecond
	gravityStep = 65 * time.Millisecond // shaved off baseGravity per level
	levelStep   = 8                     // cleared rows per level

	softDropMul = 8 // soft-drop falls this many times faster than gravity

	// Lock delay: once a piece can't fall, it waits this long (resettable by a
	// successful move/rotate) before welding into the stack.
	lockDelay = 450 * time.Millisecond

	// Line-clear flash: cleared rows blink for this long before collapsing.
	clearFlash = 220 * time.Millisecond

	// Incoming-garbage warning: rows arrive this long after the hit lands, with
	// a banner counting down so the target can brace.
	garbageWarn = 1200 * time.Millisecond

	// Solo score-attack garbage timer: a junk row creeps in on this cadence,
	// accelerating as the run goes on (never faster than soloGarbageMin).
	soloGarbageBase = 14 * time.Second
	soloGarbageMin  = 4 * time.Second
	soloGarbageStep = 900 * time.Millisecond // shaved per garbage wave survived
)

// Scoring: clearing N rows at once awards lineScore[N] points (x current
// level + 1). Bigger simultaneous clears pay disproportionately more.
var lineScore = [6]int{0, 100, 300, 600, 1000, 1500}

// attackRows maps a simultaneous clear count to how many garbage rows it fires
// at the tallest rival. A single clear sends nothing; the hook starts at 2.
var attackRows = [6]int{0, 0, 1, 2, 4, 5}

// piece is one shape: a name, a color tag, and its four rotation states. Each
// state is a list of (row, col) cell offsets from the piece anchor. Shapes are
// authored as offsets so rotation is a lookup, not a matrix transform — which
// keeps wall-kick behavior explicit and deterministic.
type piece struct {
	name   string
	color  kit.Color
	glyph  rune
	states [][][2]int // [rotation][cell]{drow, dcol}
}

// active is the piece currently falling in a well.
type active struct {
	kind int // index into pieces
	rot  int // current rotation state
	row  int // anchor row in the grid
	col  int // anchor col in the grid
}

// cells returns the active piece's occupied grid cells at its current pose.
func (a active) cells(ps []piece) [][2]int {
	st := ps[a.kind].states[a.rot%len(ps[a.kind].states)]
	out := make([][2]int, len(st))
	for i, c := range st {
		out[i] = [2]int{a.row + c[0], a.col + c[1]}
	}
	return out
}

// well is one player's playfield plus their run state. Keyed by AccountID so it
// survives room hibernation (connections change across a freeze; accounts
// don't).
type well struct {
	grid [wellH][wellW]int // [row][col]; row 0 is the top

	cur  active // the falling piece
	next int    // index of the next piece to spawn
	bag  []int  // shuffle bag for fair, deterministic piece order

	hasPiece bool // a piece is currently falling

	alive bool
	score int
	lines int // total rows cleared (drives level)
	level int

	nextDrop time.Time // when the active piece next falls under gravity
	lockAt   time.Time // when a grounded piece welds (zero = not grounded)

	// Line-clear animation: rows currently flashing before they collapse.
	clearing   []int
	clearUntil time.Time

	// Incoming garbage: rows queued by a rival, landing after the warning.
	pendingGarbage int
	garbageGap     int       // the open column in the next garbage volley
	garbageAt      time.Time // when the queued garbage lands (zero = none)

	// Solo score-attack garbage timer.
	soloNextGarbage time.Time
	soloWave        int // garbage waves survived (accelerates the timer)

	glyph rune      // owner character glyph, or a palette diamond
	color kit.Color // owner color (character bg, or join-order palette)
	best  int       // all-time best score, seeded from durable KV
}

// height returns the height of the stack: rows from the top of the tallest
// column down to the floor. 0 = empty well, wellH = stacked to the ceiling.
func (w *well) height() int {
	for r := 0; r < wellH; r++ {
		for c := 0; c < wellW; c++ {
			if w.grid[r][c] != cellEmpty {
				return wellH - r
			}
		}
	}
	return 0
}
