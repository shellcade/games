package main

import (
	"math"
	"testing"

	kit "github.com/shellcade/kit/v2"
)

// flatField is level ground for predictable aim tests.
func flatField(row int) []int {
	t := make([]int, scrW)
	for c := range t {
		t[c] = row
	}
	return t
}

func zeroJitter() float64 { return 0.5 } // jitter()*2-1 == 0, i.e. no miss

func TestCpuAimsTowardEnemy(t *testing.T) {
	terr := flatField(18)
	me := newTank("cpu", kit.Player{}, true, "CPU", 20, 0)
	me.y = surfaceAt(terr, me.col)
	foe := newTank("p1", kit.Player{}, false, "P1", 60, 1)
	foe.y = surfaceAt(terr, foe.col)
	tanks := []*tank{me, foe}

	cpuAim(me, tanks, terr, 0, zeroJitter)
	// The foe is to the right, so the barrel should lean right (angle < 90).
	if me.angle >= 90 {
		t.Errorf("CPU aiming right-side enemy at angle %.0f, want < 90", me.angle)
	}
}

func TestCpuSolutionLandsNearTarget(t *testing.T) {
	terr := flatField(18)
	me := newTank("cpu", kit.Player{}, true, "CPU", 15, 0)
	me.y = surfaceAt(terr, me.col)
	foe := newTank("p1", kit.Player{}, false, "P1", 55, 1)
	foe.y = surfaceAt(terr, foe.col)
	tanks := []*tank{me, foe}

	cpuAim(me, tanks, terr, 0, zeroJitter)
	ix, _, ok := simImpact(me, me.angle, me.power, 0, terr, tanks)
	if !ok {
		t.Fatal("CPU picked a shot that sails off the field")
	}
	if d := math.Abs(ix - float64(foe.col)); d > 10 {
		t.Errorf("CPU shot lands %.0f cols from the target — too wild", d)
	}
}
