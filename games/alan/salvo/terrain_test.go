package main

import "testing"

func TestGenTerrainBounds(t *testing.T) {
	terr := genTerrain(testRng())
	if len(terr) != scrW {
		t.Fatalf("terrain width = %d, want %d", len(terr), scrW)
	}
	for c, h := range terr {
		if h < surfMin || h > surfMax {
			t.Errorf("column %d surface %d out of [%d,%d]", c, h, surfMin, surfMax)
		}
	}
}

func TestSolidAtSurface(t *testing.T) {
	terr := make([]int, scrW)
	for c := range terr {
		terr[c] = 15
	}
	if solidAt(terr, 10, 14) {
		t.Error("a point above the surface should be air")
	}
	if !solidAt(terr, 10, 15) || !solidAt(terr, 10, 18) {
		t.Error("a point at/below the surface should be solid")
	}
}

func TestCraterLowersSurface(t *testing.T) {
	terr := make([]int, scrW)
	for c := range terr {
		terr[c] = 12
	}
	before := terr[20]
	craterAt(terr, 20, 13, 5)
	if terr[20] <= before {
		t.Errorf("crater did not lower the surface at the centre: %d -> %d", before, terr[20])
	}
	// Far from the blast, the ground is untouched.
	if terr[60] != 12 {
		t.Errorf("crater reached too far: column 60 surface = %d", terr[60])
	}
}

func TestCraterCanPunchThroughToAPit(t *testing.T) {
	terr := make([]int, scrW)
	for c := range terr {
		terr[c] = groundBottom // a thin sliver of ground
	}
	craterAt(terr, 30, float64(groundBottom), 5)
	if grounded(terr, 30) {
		t.Error("a blast at the very bottom should punch a pit through the sliver")
	}
}
