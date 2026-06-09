package main

import "testing"

func masterOf(t *testing.T, k betKind, label string) int {
	t.Helper()
	for i, b := range masterBets {
		if b.kind == k && b.label == label {
			return i
		}
	}
	t.Fatalf("no %s bet labelled %q", k.name(), label)
	return -1
}

// TestChipPositions pins where chips render for representative bets so the
// markers stay aligned with the felt (and the street/split chip stays centred
// on its line rather than drifting to the left grid line).
func TestChipPositions(t *testing.T) {
	// Straight on 17: the cell's left (chip) slot.
	if row, col := chipPos(17); row != rowOfRR(1) || col != colInterior(5) {
		t.Errorf("straight 17 chip at (%d,%d), want (%d,%d)", row, col, rowOfRR(1), colInterior(5))
	}
	// Street 16-18 sits on the outer (bottom) edge of column 5, centred in the
	// cell's line segment — not on the left grid line.
	st := masterOf(t, kStreet, "Str 16-18")
	if row, col := chipPos(st); row != gridTop+6 || col != colInterior(5)+iw/2 {
		t.Errorf("street 16-18 chip at (%d,%d), want (%d,%d)", row, col, gridTop+6, colInterior(5)+iw/2)
	}
	// Corner 17-21 sits on the intersection between columns 5 and 6.
	cn := masterOf(t, kCorner, "Cnr 17-21")
	if row, col := chipPos(cn); row != gridTop+2 || col != lineCol(6) {
		t.Errorf("corner 17-21 chip at (%d,%d), want (%d,%d)", row, col, gridTop+2, lineCol(6))
	}
	// A horizontal split (17-20) sits on the vertical line right of 17.
	sp := masterOf(t, kSplit, "17-20")
	if row, col := chipPos(sp); row != rowOfRR(1) || col != lineCol(6) {
		t.Errorf("split 17-20 chip at (%d,%d), want (%d,%d)", row, col, rowOfRR(1), lineCol(6))
	}
}
