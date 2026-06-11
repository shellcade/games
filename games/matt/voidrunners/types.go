package main

import (
	"math"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// Playfield geometry. The canvas is 80x24; row 0 is the scoreboard and row 23
// is the controls/status bar, leaving rows 1..22 for the arena. Both axes wrap
// (the arena is a torus). Because terminal cells are about twice as tall as
// they are wide, vertical motion is scaled by `aspect` so a diagonal heading
// looks diagonal on screen and collisions stay round.
const (
	cols   = kit.Cols // 80
	top    = 1        // first arena row
	bottom = 22       // last arena row (inclusive)
	playH  = bottom - top + 1
	aspect = 0.5 // vertical cells per horizontal cell of "real" distance
)

// Flight model (units are horizontal cells / second unless noted). Steering is
// directional: an arrow points the ship that way and thrusts; pressing a second
// perpendicular arrow within chordWindow heads the diagonal between them. There
// is no brake key — tap the direction opposite your drift to slow down.
const (
	thrustDV     = 2.6 // velocity added per direction press
	maxSpeed     = 19.0
	dragPerSec   = 0.55 // gentle space drag so drift eventually settles
	bulletSpeed  = 46.0
	bulletLife   = 750 * time.Millisecond // range = bulletSpeed * bulletLife (~34 cells)
	fireCooldown = 260 * time.Millisecond
	chordWindow  = 250 * time.Millisecond // two arrows within this window -> diagonal
	shipHit      = 1.5                    // ship hit radius (horizontal cells) — covers the 2-cell craft
)

// Cardinal directions, indexed N,E,S,W. dirHeading is the heading in radians
// (0 = east, clockwise, y-down); dirVec is the unit travel vector.
const (
	dirN = iota
	dirE
	dirS
	dirW
)

var (
	dirHeading = [4]float64{-math.Pi / 2, 0, math.Pi / 2, math.Pi}
	dirVec     = [4][2]float64{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
)

// perpendicular reports whether two cardinal indices are at right angles (a
// valid diagonal pairing); opposite or equal cardinals are not.
func perpendicular(a, b int) bool { return (a % 2) != (b % 2) }

// Arena rules.
const (
	soloCraters  = 7               // craters kept floating in a 1-player arena
	pvpCraters   = 1               // craters once it's a 2+ player dogfight
	respawnDelay = 2 * time.Second // dead -> respawn wait
	invulnDur    = 2 * time.Second // post-respawn safety
	explodeDur   = 650 * time.Millisecond
	killPlayer   = 5 // kill credit for downing a rival pilot
	killCrater   = 1 // kill credit per crater fragment destroyed
)

// vec is a simple 2D float position/velocity in cell space.

// ship is one pilot's craft. State is keyed by Player.AccountID so it survives
// room hibernation (connections change across a freeze; accounts don't).
type ship struct {
	x, y        float64
	vx, vy      float64
	heading     float64 // radians, 0 = east, angle increases clockwise (y-down)
	alive       bool
	respawnAt   time.Time
	invulnUntil time.Time
	lastShot    time.Time
	lastDir     int       // cardinal index of the last direction press (-1 = none)
	lastDirAt   time.Time // when it was pressed (for the diagonal chord window)
	kills       int
	deaths      int
	best        int       // all-time best kills (seeded from durable KV)
	glyph       rune      // hull glyph: the pilot's character, or '◆' when they have none
	color       kit.Color // ship colour: the character's BG colour, or a palette pick
}

// bullet is a single shot. owner is the firing pilot's account id.
type bullet struct {
	x, y   float64
	vx, vy float64
	dieAt  time.Time
	owner  string
	color  kit.Color
}

// crater is a floating rock. size 3 = large, 2 = medium, 1 = small; shooting a
// large or medium one breaks it into two smaller fragments (classic asteroids).
type crater struct {
	x, y   float64
	vx, vy float64
	size   int
}

// explosion is a short expanding-ring effect at a point.
type explosion struct {
	x, y  float64
	start time.Time
	color kit.Color
}

// star is a static background speck for ambiance.
type star struct {
	x, y   int
	bright bool
}

func craterRadius(size int) float64 { return float64(size) }

// --- toroidal helpers --------------------------------------------------------

// wrapX/wrapY keep a position on the torus. The domains are centered on cells
// — [-0.5, cols-0.5) and [top-0.5, bottom+0.5) — so rounding always lands on a
// valid arena cell (no entity ever rounds onto the HUD rows or off the right
// edge before wrapping).
func wrapX(x float64) float64 {
	x = math.Mod(x+0.5, cols)
	if x < 0 {
		x += cols
	}
	return x - 0.5
}

func wrapY(y float64) float64 {
	y = math.Mod(y-(top-0.5), playH)
	if y < 0 {
		y += playH
	}
	return y + (top - 0.5)
}

// toroidalDelta returns the shortest signed distance from b to a on a ring.
func toroidalDelta(a, b, size float64) float64 {
	d := math.Mod(a-b, size)
	if d > size/2 {
		d -= size
	} else if d < -size/2 {
		d += size
	}
	return d
}

// dist2 is the squared, aspect-corrected distance between two arena points
// (vertical deltas are converted to horizontal-cell units so collisions are
// visually circular rather than squashed).
func dist2(ax, ay, bx, by float64) float64 {
	dx := toroidalDelta(ax, bx, cols)
	dy := toroidalDelta(ay, by, playH) / aspect
	return dx*dx + dy*dy
}

// roundCell rounds a float coordinate to the nearest integer cell.
func roundCell(v float64) int { return int(math.Floor(v + 0.5)) }

// wrapCol/wrapRow wrap an integer cell onto the arena so multi-cell sprites
// (craters, explosions) draw seamlessly across the toroidal edges.
func wrapCol(c int) int { return ((c % cols) + cols) % cols }

func wrapRow(r int) int { return ((r-top)%playH+playH)%playH + top }
