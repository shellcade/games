package main

import (
	"math"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// The flight model. Units: columns (1 column = 1 m), seconds, radians.
// A glider carries an airspeed v along its flight-path angle; diving trades
// height for speed, climbing trades speed for height, and below the stall
// speed the nose mushes down no matter the trim. Updrafts push the glider up
// while it crosses them. Trim is adjusted in discrete nudges per arrow-key
// event (terminal auto-repeat makes holding the key a continuous rotation).
const (
	gravity   = 26.0  // along-path acceleration when diving (m/s²)
	dragCoeff = 0.011 // quadratic drag; terminal dive ≈ 48 m/s
	launchV   = 22.0  // airspeed off the launch fold: enough to glide level
	// to the first gate (~106m) without being forced into an early dive
	vStall    = 9.0  // below this the nose mushes down
	mushMax   = 1.0  // radians of uncommanded nose-drop at zero airspeed
	sinkRate  = 1.0  // baseline sink (m/s): paper always settles
	thermLift = 18.0 // updraft lift (m/s)
	vMin      = 2.0
	vMax      = 30.0 // terminal velocity: a dive pegs the HUD bar, so the
	// speed readout actually spans slow (stall, ~2 bars) to fast (8 bars)

	pitchStep = 0.0873 // 5° of trim per arrow event
	pitchMax  = 1.13   // trim clamp, ±65°

	stallWarn = 0.08 // mush beyond this renders the STALL warning

	// The storm front: a wall of weather that chases the gliders at just
	// under cruise speed. It is what keeps a run honest — a glider can
	// stall-hover almost stationary forever, so without the storm a round
	// would only ever end at the time cap.
	stormV     = 11.0 // m/s, escapable at cruise (~14) but not while hovering
	stormStart = 60.0 // head start, metres behind the launch fold
)

// physStep is the fixed integration quantum: physics catches up to r.Now()
// in 25ms substeps so flight is heartbeat-rate independent, and maxCatchup
// bounds the work after a hibernation gap (don't simulate the void).
const (
	physStep   = 25 * time.Millisecond
	maxCatchup = 250 * time.Millisecond
)

type trailDot struct{ x, y float64 }

// pilot is one player's glider. Keyed by account id in the room (never by
// connection), so it survives a hibernation freeze/thaw.
type pilot struct {
	x, y    float64 // x in columns; y in column units, 0 = sky top, down +
	v       float64 // airspeed along the flight path
	pitch   float64 // trim, radians, positive = nose up
	alive   bool
	flew    bool // launched in the current round
	left    bool // departed mid-round (ranks as DNF)
	dist    int  // final distance once down; live distance derives from x
	stalled bool

	joinOrder int
	best      int // personal best distance (durable KV)
	newPB     bool

	trail     [10]trailDot
	trailN    int
	lastTrail time.Time
}

// liveDist is the distance flown so far this round.
func (ps *pilot) liveDist() int { return int(ps.x - spawnX) }

// advance integrates every live glider from the last physics time to now in
// fixed quanta, checking terrain collisions after each substep. Iteration is
// by join order (never map order) so runs are deterministic.
func (rm *room) advance(r kit.Room, now time.Time) {
	dt := now.Sub(rm.lastPhys)
	if dt > maxCatchup {
		dt = maxCatchup
	}
	rm.lastPhys = now
	rm.simAccum += dt

	// Keep terrain generated comfortably ahead of the leading glider.
	maxX := 0.0
	for _, id := range rm.order {
		if ps := rm.pilots[id]; ps != nil && ps.x > maxX {
			maxX = ps.x
		}
	}
	rm.terr.ensure(r.Rand(), int(maxX)+140)

	h := physStep.Seconds()
	for rm.simAccum >= physStep {
		rm.simAccum -= physStep
		for _, id := range rm.order {
			ps := rm.pilots[id]
			if ps == nil || !ps.alive {
				continue
			}
			rm.stepPilot(ps, h)
		}
	}

	// The storm swallows anyone it catches. Its position derives from the
	// round clock (never accumulated), so it is heartbeat-rate independent.
	storm := rm.stormX(now)
	for _, id := range rm.order {
		ps := rm.pilots[id]
		if ps == nil || !ps.alive {
			continue
		}
		if ps.x <= storm {
			ps.alive = false
			ps.dist = ps.liveDist()
			ps.stalled = false
			continue
		}
		// Trail dots record at a slow fixed cadence.
		if now.Sub(ps.lastTrail) >= 70*time.Millisecond {
			ps.trail[ps.trailN%len(ps.trail)] = trailDot{ps.x, ps.y}
			ps.trailN++
			ps.lastTrail = now
		}
	}
}

// stormX is the storm front's world column at time now.
func (rm *room) stormX(now time.Time) float64 {
	if rm.roundStart.IsZero() {
		return spawnX - stormStart
	}
	return spawnX - stormStart + stormV*now.Sub(rm.roundStart).Seconds()
}

func (rm *room) stepPilot(ps *pilot, h float64) {
	// Stall mush: too slow and the nose drops no matter the trim.
	mush := 0.0
	if ps.v < vStall {
		mush = mushMax * (vStall - ps.v) / vStall
	}
	ps.stalled = mush > stallWarn
	gamma := ps.pitch - mush // the actual flight-path angle

	sg, cg := math.Sin(gamma), math.Cos(gamma)
	ps.v += (-gravity*sg - dragCoeff*ps.v*ps.v) * h
	if ps.v < vMin {
		ps.v = vMin
	}
	if ps.v > vMax {
		ps.v = vMax
	}

	dy := -ps.v*sg + sinkRate
	if xi := int(ps.x); xi >= 0 && xi < len(rm.terr.therm) && rm.terr.therm[xi] {
		dy -= thermLift
	}
	ps.x += ps.v * cg * h
	ps.y += dy * h
	if ps.y < 0 {
		ps.y = 0
	}

	if rm.terr.solid(int(ps.x), ps.y/rowUnits) {
		ps.alive = false
		ps.dist = ps.liveDist()
		ps.stalled = false
	}
}
