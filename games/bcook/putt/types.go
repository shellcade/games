package main

import (
	"math"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// Canvas geometry. The canvas is 80x24; row 0 is the HUD (hole/par/strokes +
// standings) and row 23 is the controls/status bar, leaving rows 1..22 for the
// course. Terminal cells are about twice as tall as they are wide, so vertical
// motion is scaled by `aspect` — a diagonal putt looks diagonal on screen and
// the ball doesn't crawl vertically.
const (
	cols    = kit.Cols // 80
	rows    = kit.Rows // 24
	top     = 1        // first course row
	bottom  = 22       // last course row (inclusive)
	courseH = bottom - top + 1
	aspect  = 0.5 // vertical cells per horizontal cell of "real" distance
)

// Putt physics (units are horizontal cells / second unless noted). Power is a
// notched dial: Up/Down step it, the setting persists between shots, and Space
// putts immediately at the dialed notch (no hold-to-charge — over SSH, latency
// jitter made a release-timed charge unplayable). Friction bleeds the launch
// off until the ball settles. Sand multiplies friction; water is a hazard, not
// a surface.
const (
	powerNotches = 10           // dial steps; notch 1 = feather, notch 10 = full send
	defaultNotch = 5            // a fresh golfer's dial starts mid-range
	minLaunch    = 8.0          // launch speed at notch 1 (approaching zero power)
	maxLaunch    = 200.0        // launch speed at full dial (curve is quadratic)
	rollFric     = 0.06         // fraction of velocity retained per second on the green
	sandFric     = 0.002        // much heavier drag in the sand
	stopSpeed    = 3.0          // below this the ball is considered at rest
	wallDamp     = 0.74         // speed retained after a wall bounce
	trailSpeed   = 40.0         // ball draws a motion trail above this speed
	aimStepRad   = math.Pi / 16 // radians the aim turns per arrow press
)

// notchSpeed maps a dial notch (1..powerNotches) to a launch speed. The curve
// is quadratic so the low notches stay genuinely soft for delicate finishes
// while the top of the dial still drives most of the way across the course.
func notchSpeed(notch int) float64 {
	if notch < 1 {
		notch = 1
	}
	if notch > powerNotches {
		notch = powerNotches
	}
	frac := float64(notch) / powerNotches
	return minLaunch + (maxLaunch-minLaunch)*frac*frac
}

// Round structure.
const (
	strokeCapOverPar = 4               // a hole ends for you at par + this many strokes
	scorecardDwell   = 3 * time.Second // how long the per-hole scorecard lingers
	finalDwell       = 8 * time.Second // final scorecard before the room settles
)

// phase is the room's high-level state.
type phase uint8

const (
	phasePlay      phase = iota // a hole is in progress
	phaseScorecard              // between-holes intermission
	phaseFinal                  // final scorecard after hole 9
)

// ballState is where a golfer's ball is in the shot cycle.
type ballState uint8

const (
	stateAim  ballState = iota // resting: aim and dial, ready to putt
	stateRoll                  // ball in motion
	stateSunk                  // holed out this hole
)

// palette assigns each golfer a distinct bright color by join order.
var palette = []kit.Color{
	kit.RGB(0x4f, 0xd6, 0xff), // cyan
	kit.RGB(0xff, 0x8a, 0x4f), // orange
	kit.RGB(0x7d, 0xff, 0x6b), // green
	kit.RGB(0xff, 0x6b, 0xc7), // pink
	kit.RGB(0xb9, 0x8a, 0xff), // purple
	kit.RGB(0xff, 0xe1, 0x55), // yellow
}

var (
	fairwayColor = kit.RGB(0x2f, 0x7d, 0x3f) // mown green
	wallColor    = kit.RGB(0xc9, 0xb8, 0x8a) // wooden rails
	sandColor    = kit.RGB(0xe6, 0xcf, 0x8a) // bunker sand
	waterColor   = kit.RGB(0x3d, 0x8f, 0xd6) // pond
	cupColor     = kit.RGB(0xff, 0xf2, 0x66) // flag/cup
	ghostColor   = kit.Gray(0x6a)            // rivals' dim ghost balls
)

// golfer is one player's per-round state. Keyed by Player.AccountID so it
// survives room hibernation (connections change across a freeze; accounts
// don't).
type golfer struct {
	// Live ball on the current hole.
	x, y    float64
	vx, vy  float64
	state   ballState
	aim     float64 // aim heading in radians (0 = east, clockwise, y-down)
	notch   int     // power dial setting (1..powerNotches); persists between shots
	preX    float64 // pre-shot position (water reset target)
	preY    float64
	prevX   float64 // last-frame position, for the motion trail
	prevY   float64
	strokes int // strokes on the current hole
	holeIdx int // which hole's scores are recorded (for safety)
	sunkAt  time.Time

	// Round totals.
	scores []int // strokes per completed hole (len grows as holes finish)

	glyph rune      // ball glyph: the golfer's character, or '●' when they have none
	color kit.Color // golfer colour: the character's BG colour, or a palette pick
}

// total returns the golfer's cumulative strokes across completed holes.
func (g *golfer) total() int {
	t := 0
	for _, s := range g.scores {
		t += s
	}
	return t
}

// --- aspect-aware helpers ----------------------------------------------------

// roundCell rounds a float coordinate to the nearest integer cell.
func roundCell(v float64) int { return int(math.Floor(v + 0.5)) }

// dist2 is the squared, aspect-corrected distance between two course points
// (vertical deltas are converted to horizontal-cell units so a hole-out radius
// is visually circular rather than squashed).
func dist2(ax, ay, bx, by float64) float64 {
	dx := ax - bx
	dy := (ay - by) / aspect
	return dx*dx + dy*dy
}
