package main

import (
	"math"

	kit "github.com/shellcade/kit/v2"
)

// Battlefield geometry. The arena is the full 80x24: a HUD strip on top, the
// sky/terrain in the middle, and an aiming panel along the bottom.
const (
	scrW = kit.Cols // 80

	hudRow       = 0
	skyTop       = 1
	groundBottom = 22 // ground fills down to here; a tank knocked below it is gone
	panelRow     = 23

	surfMin = 7  // hilltops can rise to this row
	surfMax = 21 // valleys can sink to this row

	// Shell flight (tuned for satisfying lobs over an 80-wide field).
	aspect      = 2.0  // a cell is ~twice as tall as wide
	gravity     = 24.0 // rows/sec^2
	minSpeed    = 12.0
	maxSpeed    = 76.0
	barrelLen   = 2.4
	maxTrail    = 14
	startHealth = 100
)

// weapon kinds, cycled at aim time.
type weapon struct {
	name   string
	radius float64 // blast radius in cells
	damage int     // peak damage at the centre of the blast
	ammo   int     // starting rounds; -1 = unlimited
	glyph  rune    // the in-flight shell
	color  kit.Color
}

var weapons = []weapon{
	{"SHELL", 4.5, 34, -1, '*', kit.RGB(0xff, 0xe0, 0x3a)},
	{"HEAVY", 7.5, 60, 3, '@', kit.RGB(0xff, 0x6b, 0x3a)},
	{"TRACER", 2.6, 16, -1, '.', kit.RGB(0x6f, 0xff, 0xe0)},
}

// tankPalette colours the tanks by seat order (CPU tanks included).
var tankPalette = []kit.Color{
	kit.RGB(0x4f, 0xd6, 0xff), // cyan
	kit.RGB(0xff, 0x8a, 0x4f), // orange
	kit.RGB(0x7d, 0xff, 0x6b), // green
	kit.RGB(0xff, 0x6b, 0xc7), // pink
	kit.RGB(0xb9, 0x8a, 0xff), // purple
	kit.RGB(0xff, 0xe1, 0x55), // yellow
}

type tank struct {
	id     string
	player kit.Player // zero-value for a CPU tank
	name   string
	cpu    bool

	col    int     // fixed battlefield column
	y      float64 // current row (animates when the ground is blown out from under it)
	health int
	alive  bool

	angle  float64 // degrees: 0 = east, 90 = straight up, 180 = west
	power  float64 // 0..100
	weapon int
	ammo   []int // per-weapon rounds left (-1 unlimited)

	color kit.Color
}

func newTank(id string, p kit.Player, cpu bool, name string, col, colorIdx int) *tank {
	ammo := make([]int, len(weapons))
	for i, w := range weapons {
		ammo[i] = w.ammo
	}
	face := 90.0
	if col < scrW/2 {
		face = 60 // left side leans right
	} else {
		face = 120 // right side leans left
	}
	return &tank{
		id: id, player: p, cpu: cpu, name: name,
		col: col, health: startHealth, alive: true,
		angle: face, power: 55, ammo: ammo,
		color: tankPalette[colorIdx%len(tankPalette)],
	}
}

// aimVec is the unit barrel direction in screen space (dy negative = up).
func (t *tank) aimVec() (dx, dy float64) {
	r := t.angle * math.Pi / 180
	return math.Cos(r), -math.Sin(r)
}

// barrelTip is where a fired shell first appears (clear of the tank body).
func (t *tank) barrelTip() (x, y float64) {
	dx, dy := t.aimVec()
	return float64(t.col) + dx*barrelLen, t.y - 0.6 + dy*barrelLen
}

// launchVel is the shell's initial velocity for the tank's angle + power. The
// vertical component is scaled by the cell aspect so the launch angle on screen
// matches the dialled angle.
func (t *tank) launchVel() (vx, vy float64) {
	spd := minSpeed + t.power/100*(maxSpeed-minSpeed)
	r := t.angle * math.Pi / 180
	return spd * math.Cos(r), -spd * math.Sin(r) / aspect
}

func (t *tank) adjustAngle(d float64) {
	t.angle = clampF(t.angle+d, 1, 179)
}

func (t *tank) adjustPower(d float64) {
	t.power = clampF(t.power+d, 5, 100)
}

// cycleWeapon advances to the next weapon the tank still has rounds for.
func (t *tank) cycleWeapon() {
	for i := 1; i <= len(weapons); i++ {
		w := (t.weapon + i) % len(weapons)
		if t.ammo[w] != 0 {
			t.weapon = w
			return
		}
	}
}

// --- the in-flight shell -----------------------------------------------------

type pt struct{ x, y float64 }

type shell struct {
	x, y   float64
	vx, vy float64
	w      weapon
	owner  *tank
	trail  []pt
}

// integrate advances a ballistic point by dt under gravity + wind (Euler).
func integrate(x, y, vx, vy, wind, dt float64) (float64, float64, float64, float64) {
	vy += gravity * dt
	vx += wind * dt
	return x + vx*dt, y + vy*dt, vx, vy
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampI(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
