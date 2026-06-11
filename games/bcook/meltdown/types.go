package main

import (
	"time"

	kit "github.com/shellcade/kit/v2"
)

// Canvas geometry. The canvas is 80x24; row 0 is the crew roster / alarm
// header and row 23 is the controls/status bar, leaving rows 1..22 for the
// reactor ship interior.
const (
	cols   = kit.Cols // 80
	top    = 1        // first interior row
	bottom = 22       // last interior row (inclusive)
)

// The reactor floor plan is built at runtime by buildShip (see room.go) into a
// fixed interiorRows x cols grid of cellKind: an outer hull, interior bulkheads
// carving four corner rooms around a central glowing core chamber, doorways cut
// so every room connects, and stations sprinkled on the floor where faults
// erupt. Building it in code keeps the geometry provably rectangular (no
// ragged hand-drawn strings) and the connectivity easy to reason about.

// rows in the interior.
const interiorRows = bottom - top + 1 // 22

// Cell kinds derived from the blueprint.
type cellKind uint8

const (
	cellFloor cellKind = iota
	cellWall
	cellCore
	cellStation
)

// palette assigns each crew member a distinct bright color by join order.
var palette = []kit.Color{
	kit.RGB(0x4f, 0xd6, 0xff), // cyan
	kit.RGB(0xff, 0x8a, 0x4f), // orange
	kit.RGB(0x7d, 0xff, 0x6b), // green
	kit.RGB(0xff, 0x6b, 0xc7), // pink
	kit.RGB(0xb9, 0x8a, 0xff), // purple
	kit.RGB(0xff, 0xe1, 0x55), // yellow
}

var (
	wallColor = kit.RGB(0x44, 0x55, 0x66)
	coreHot   = kit.RGB(0xff, 0xe1, 0x55)
	coreWarm  = kit.RGB(0xff, 0x8a, 0x4f)
	coreCold  = kit.RGB(0x88, 0x44, 0x44)
)

// faultKind enumerates the four hazards.
type faultKind uint8

const (
	faultLeak   faultKind = iota // ≈ stand on it, mash space
	faultFire                    // ▲ stand adjacent, HOLD space
	faultValve                   // Φ stand on it, type the shown key sequence
	faultBreach                  // ◊ two crew stand on it together (2+ crew only)
)

// faultGlyph is the icon drawn for an active fault.
var faultGlyph = map[faultKind]rune{
	faultLeak:   '≈',
	faultFire:   '▲',
	faultValve:  'Φ',
	faultBreach: '◊',
}

// faultColor tints each fault type.
var faultColor = map[faultKind]kit.Color{
	faultLeak:   kit.RGB(0x55, 0xbb, 0xff),
	faultFire:   kit.RGB(0xff, 0x66, 0x33),
	faultValve:  kit.RGB(0xff, 0xcc, 0x44),
	faultBreach: kit.RGB(0xff, 0x44, 0x88),
}

// faultName is the human label shown on a fix progress bar.
var faultName = map[faultKind]string{
	faultLeak:   "LEAK",
	faultFire:   "FIRE",
	faultValve:  "VALVE",
	faultBreach: "BREACH",
}

// --- tuning ------------------------------------------------------------------

const (
	coreMax = 100.0 // full core integrity

	// Each active fault drains the core every second. Worse faults bite harder,
	// and a fault that has festered (age) bites a little harder still.
	leakDrain   = 1.4
	fireDrain   = 2.2
	valveDrain  = 1.8
	breachDrain = 3.0
	ageDrainMul = 0.06 // extra drain per second a fault has been alive

	// Leak: how many space mashes patch it.
	leakMashes = 6
	// Fire: seconds of continuous HOLD to extinguish; it regrows toward full
	// at regrowRate when nobody is holding on it.
	fireHoldSecs = 1.6
	fireRegrow   = 0.7 // progress lost per second when released
	// Valve: 3-4 keys in the shown sequence; a wrong key resets your progress.
	valveMinKeys = 3
	valveMaxKeys = 4
	// Breach: seconds two crew must stand on it together.
	breachHoldSecs = 1.2

	// Spawn cadence. The base interval shrinks as the run goes on (panic ramps
	// up), and is divided down toward a floor. Crew scaling is sub-linear so a
	// bigger crew faces proportionally fewer faults each.
	spawnBase     = 5.0 // seconds between spawns at the very start (solo)
	spawnFloor    = 1.1 // never spawn faster than this
	spawnRampSecs = 14.0
	maxFaults     = 10 // hard cap on simultaneous faults
)

// crewMember is one player's engineer. State is keyed by Player.AccountID so
// it survives room hibernation (connections change across a freeze; accounts
// don't).
type crewMember struct {
	row, col int       // cell position in the interior
	fixes    int       // faults this member has completed this run
	best     int       // all-time best survival seconds (seeded from durable KV)
	glyph    rune      // body glyph: the player's character, or '☺' when none
	color    kit.Color // body color: the character's BG colour, or a palette pick
	joined   bool      // currently in the room
}

// fault is one active hazard at a station.
type fault struct {
	kind     faultKind
	row, col int
	born     time.Time // when it erupted (for age-based extra drain)
	// progress is fix completion in [0,1]; the meaning depends on kind:
	//   leak   — mashes/leakMashes
	//   fire   — held seconds / fireHoldSecs (decays when nobody holds)
	//   valve  — keys matched / len(seq)
	//   breach — held seconds (two crew) / breachHoldSecs
	progress float64
	mashes   int    // leak only: space mashes landed so far
	seq      []rune // valve only: the key sequence to type
	seqAt    int    // valve only: how many keys matched so far
	holders  int    // crew currently working it this tick (fire/breach)
}

// phase is the room's lifecycle state.
type phase uint8

const (
	phaseRunning phase = iota
	phaseOver
)

func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
