package main

import (
	"math"
	"math/rand"
	"testing"
)

// autopilotPitch is a near-ideal, swoop-capable controller used as a
// reachability oracle: it aims the glider at the centre of the next gate gap
// (corridor mid when none is near) and drives the flight-path angle toward the
// intercept with a strong proportional term, committing to a zoom when the gap
// sits above and there is speed to spend. It does no energy hoarding — if it
// can't reach a gap, the gap is genuinely unreachable. If this oracle clears a
// gate, a skilled human can too.
func autopilotPitch(tr *terrain, ps *pilot) float64 {
	xi := int(ps.x)
	targetX, targetY := xi+50, tr.midY(xi+50)
	for x := xi; x < xi+160 && x < len(tr.gTop); x++ {
		if tr.gTop[x] >= 0 {
			targetX = x
			targetY = float64(int(tr.gTop[x]) + int(tr.gBot[x]) + 1) // gap centre, col units
			break
		}
	}
	dx := math.Max(float64(targetX)-ps.x, 5)
	desired := math.Atan2(ps.y-targetY, dx) // +ve = climb (target above)
	if desired > 0.7 {
		desired = 0.7
	} else if desired < -1.0 {
		desired = -1.0
	}
	alphaHold := gravity * math.Cos(ps.gamma) / (clAlpha * ps.v * ps.v)
	pitch := ps.gamma + alphaHold + 2.5*(desired-ps.gamma)
	if pitch > pitchMax {
		pitch = pitchMax
	} else if pitch < -pitchMax {
		pitch = -pitchMax
	}
	return pitch
}

// TestSwoopTradesSpeedForHeight locks in the feel: a sustained pull is a long
// arcing zoom that converts speed to real height, and the faster you enter the
// bigger the arc — not an instant stall.
func TestSwoopTradesSpeedForHeight(t *testing.T) {
	rm := &room{} // empty terrain: open-air flight, no collisions or thermals
	swoop := func(v0 float64) float64 {
		ps := &pilot{x: 1000, y: 80, v: v0, alive: true}
		minY := ps.y
		for i := 0; i < 200; i++ {
			ps.pitch = 0.8
			rm.stepPilot(ps, 0.025)
			if ps.y < minY {
				minY = ps.y
			}
		}
		return (80 - minY) / rowUnits // rows climbed
	}
	if c := swoop(launchV); c < 4 {
		t.Errorf("launch-speed swoop climbed %.1f rows, want >=4 (long arcs, not a stall)", c)
	}
	if c := swoop(38); c < 10 {
		t.Errorf("dive-built swoop climbed %.1f rows, want >=10 (speed buys height)", c)
	}
}

// gatesClearedByAutopilot flies the oracle from launch and returns how many of
// the first wantGates it threads before crashing.
func gatesClearedByAutopilot(seed int64, wantGates int) int {
	rng := rand.New(rand.NewSource(seed))
	rm := &room{}
	rm.terr.reset()
	rm.terr.ensure(rng, 600)
	ps := &pilot{x: spawnX, y: rm.terr.midY(int(spawnX)), v: launchV, alive: true}

	cleared, nextGate := 0, nextGateAt(&rm.terr, int(spawnX))
	for step := 0; step < 8000 && ps.alive && cleared < wantGates; step++ {
		rm.terr.ensure(rng, int(ps.x)+200)
		ps.pitch = autopilotPitch(&rm.terr, ps)
		rm.stepPilot(ps, 0.025)
		if nextGate >= 0 && ps.x > float64(nextGate)+1.5 && ps.alive {
			cleared++
			nextGate = nextGateAt(&rm.terr, int(ps.x)+1)
		}
	}
	return cleared
}

func nextGateAt(tr *terrain, from int) int {
	for x := from; x < len(tr.gTop); x++ {
		if tr.gTop[x] >= 0 {
			return x
		}
	}
	return -1
}

// TestEarlyGatesAlwaysReachable is the balance guarantee: from launch energy
// alone, an ideal pilot must thread the opening earlyGates on essentially every
// seed. Later gates are meant to be hard, so they are only reported.
func TestEarlyGatesAlwaysReachable(t *testing.T) {
	const seeds = 4000
	deaths := make([]int, earlyGates+1)
	for s := int64(0); s < seeds; s++ {
		if c := gatesClearedByAutopilot(s, earlyGates); c < earlyGates {
			deaths[c]++
		}
	}
	total := 0
	for g := 0; g < earlyGates; g++ {
		total += deaths[g]
		if deaths[g] > 0 {
			t.Logf("died on gate %d: %d/%d (%.2f%%)", g+1, deaths[g], seeds, 100*float64(deaths[g])/seeds)
		}
	}
	// Allow a tiny tail for pathological corridor walks; the opening must be
	// reliably survivable from launch energy.
	if rate := float64(total) / seeds; rate > 0.005 {
		t.Errorf("opening %d gates unreachable on %.2f%% of seeds (want <0.5%%)", earlyGates, 100*rate)
	}
}

// TestMeasureGateReachability reports the full distribution for tuning.
func TestMeasureGateReachability(t *testing.T) {
	const seeds, wantGates = 3000, 6
	fails := make([]int, wantGates+1)
	allCleared := 0
	for s := int64(0); s < seeds; s++ {
		c := gatesClearedByAutopilot(s, wantGates)
		if c >= wantGates {
			allCleared++
		} else {
			fails[c]++
		}
	}
	t.Logf("cleared all %d gates: %d/%d (%.1f%%)", wantGates, allCleared, seeds, 100*float64(allCleared)/seeds)
	for k := 0; k < wantGates; k++ {
		if fails[k] > 0 {
			t.Logf("  died on gate %d: %d (%.1f%%)", k+1, fails[k], 100*float64(fails[k])/seeds)
		}
	}
}
