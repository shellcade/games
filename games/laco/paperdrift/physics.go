package main

import (
	"math"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// The flight model. Units: columns (1 column = 1 m), seconds, radians.
//
// A glider is a point mass carrying airspeed v along its flight-path angle γ.
// Trim does NOT set γ directly — it sets the nose attitude, and the angle of
// attack α = pitch − γ is what the wing flies at. Lift (∝ α·v²) curves the
// velocity vector (dγ/dt = (L − g·cosγ)/v); gravity along the path and drag
// set the speed. So at speed there is lift to spare and a pull arcs the nose
// up smoothly, trading speed for height — a swoop. Slow down and lift fades
// below g·cosγ, so the nose falls no matter the trim: the stall is emergent,
// not a special case. Trim is nudged in discrete steps per arrow-key event
// (terminal auto-repeat makes holding the key a continuous rotation).
const (
	// Gravity is held below real (≈ 9.8 → here 22) to two ends at once: the
	// zoom height a glider trades speed for is (v² − v_stall²)/2g, so it stays
	// gentle enough for long arcing swoops (~6.5 rows from launch, ~14 after a
	// dive), while being firm enough that even a shallow nose-down builds speed
	// at a useful clip (g·sinγ) rather than just coasting.
	gravity = 22.0 // along-path gravity (m/s²)

	// Lift/drag as lumped per-unit-mass coefficients (½ρS folded in), solved
	// from a target equilibrium: at neutral trim the glider settles to a
	// ~15 m/s glide descending ~3° (glide ratio ≈ 19). Drag is deliberately
	// light so energy persists — a dive that builds speed keeps it, swoops run
	// long, and the glider floats rather than mushing out of the sky. That long
	// energy budget is what makes the full trim range usable: you can dive hard
	// and still have the speed to pull out.
	clAlpha    = 1.86   // lift-curve slope: CL per radian of angle of attack
	alphaStall = 0.30   // AoA (~17°) past which the wing stalls; lift caps here
	cd0        = 0.0050 // parasitic drag coefficient
	cdInduced  = 0.016  // induced drag, ∝ CL²
	maxTurn    = 0.16   // clamp on path rotation per substep (integrator guard)

	launchV   = 26.0 // airspeed off the launch fold (a touch above the old 22)
	vStall    = 9.0  // slow-air / stall-warning threshold
	thermLift = 11.0 // updraft vertical airspeed (m/s)
	vMin      = 2.0
	vMax      = 50.0 // terminal-dive clamp (a free dive settles ≈ 47 m/s)

	pitchStep = 0.0873 // 5° of trim per arrow event
	pitchMax  = 1.13   // trim clamp, ±65°

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
	gamma   float64 // flight-path angle, radians, positive = climbing
	pitch   float64 // trim (nose attitude), radians, positive = nose up
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
	// Angle of attack: the wing meets the air at the gap between the commanded
	// attitude and where the glider is actually going. Past the stall AoA the
	// wing can make no more lift — hauling back harder only flags the stall.
	alpha := ps.pitch - ps.gamma
	stalled := false
	if alpha > alphaStall {
		alpha, stalled = alphaStall, true
	} else if alpha < -alphaStall {
		alpha = -alphaStall
	}
	cl := clAlpha * alpha
	v2 := ps.v * ps.v
	lift := cl * v2
	drag := (cd0 + cdInduced*cl*cl) * v2

	sg, cg := math.Sin(ps.gamma), math.Cos(ps.gamma)
	// Lift curves the velocity vector; gravity's normal component straightens
	// it. With speed there is lift to spare so the path arcs up (a swoop);
	// slow, lift can't match g·cosγ and the nose falls — the stall.
	dg := (lift - gravity*cg) / ps.v * h
	if dg > maxTurn {
		dg = maxTurn
	} else if dg < -maxTurn {
		dg = -maxTurn
	}
	ps.gamma += dg

	// Gravity along the path and drag set the airspeed.
	ps.v += (-gravity*sg - drag) * h
	if ps.v < vMin {
		ps.v = vMin
	} else if ps.v > vMax {
		ps.v = vMax
	}
	if ps.v < vStall {
		stalled = true
	}
	ps.stalled = stalled

	dy := -ps.v * math.Sin(ps.gamma)
	if xi := int(ps.x); xi >= 0 && xi < len(rm.terr.therm) && rm.terr.therm[xi] {
		dy -= thermLift
	}
	ps.x += ps.v * math.Cos(ps.gamma) * h
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
