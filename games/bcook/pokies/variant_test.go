package main

import "testing"

func TestSpecialSymbolsRegistered(t *testing.T) {
	if symbolByName["W"] != symWild {
		t.Errorf("W should map to symWild")
	}
	if symbolByName["S"] != symScatter {
		t.Errorf("S should map to symScatter")
	}
	if faceArt[symWild] == "" || faceArt[symScatter] == "" {
		t.Errorf("wild/scatter need reel art")
	}
}

func TestDistributeSpacesEvenly(t *testing.T) {
	base := []symbol{sym7, sym7, sym7, sym7, sym7, sym7} // 6 regulars
	got := distribute(base, symScatter, 2)
	if len(got) != 8 {
		t.Fatalf("len = %d, want 8", len(got))
	}
	// 2 scatters across 8 slots land at positions 0 and 4 (k*n/count).
	var pos []int
	for i, s := range got {
		if s == symScatter {
			pos = append(pos, i)
		}
	}
	if len(pos) != 2 || pos[0] != 0 || pos[1] != 4 {
		t.Fatalf("scatter positions = %v, want [0 4]", pos)
	}
	sevens := 0
	for _, s := range got {
		if s == sym7 {
			sevens++
		}
	}
	if sevens != 6 {
		t.Fatalf("sevens = %d, want 6 preserved", sevens)
	}
}

func TestDistributeZeroCountIsNoop(t *testing.T) {
	base := []symbol{sym7, symBar}
	got := distribute(base, symWild, 0)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (no-op)", len(got))
	}
}
