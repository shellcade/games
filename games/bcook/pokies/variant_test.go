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

func TestScatterAwardThresholds(t *testing.T) {
	v := &variant{scatter: []scatterEntry{{Count: 5, Spins: 25}, {Count: 4, Spins: 15}, {Count: 3, Spins: 8}}}
	// build a numReels × visRows window with `n` scatters placed.
	win := func(n int) (w [numReels][visRows]symbol) {
		for reel := 0; reel < numReels; reel++ {
			for row := 0; row < visRows; row++ {
				w[reel][row] = sym7
			}
		}
		placed := 0
		for reel := 0; reel < numReels && placed < n; reel++ {
			for row := 0; row < visRows && placed < n; row++ {
				w[reel][row] = symScatter
				placed++
			}
		}
		return w
	}
	for _, c := range []struct{ scatters, spins int }{
		{2, 0}, {3, 8}, {4, 15}, {5, 25}, {7, 25},
	} {
		if got := v.scatterAward(win(c.scatters)); got != c.spins {
			t.Errorf("%d scatters -> %d spins, want %d", c.scatters, got, c.spins)
		}
	}
}

func TestCompileRejectsRunawayRetrigger(t *testing.T) {
	// Scatter-saturated strip + large award -> t*maxAward >= 1 (non-converging).
	_, err := compileVariant(oddsVariant{
		Name:     "runaway",
		Weights:  map[string]int{"7": 1, "S": 30},
		Paytable: []payEntry{{Faces: "7", Pay3: 5, Pay4: 5, Pay5: 5}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 50}},
	})
	if err == nil {
		t.Fatal("expected rejection of a non-converging retrigger feature")
	}
}

func TestCompileSortsScatterDescAndDefaultsGamble(t *testing.T) {
	v, err := compileVariant(oddsVariant{
		Name:     "ok",
		Weights:  map[string]int{"7": 4, "C": 30, "S": 1},
		Paytable: []payEntry{{Faces: "7", Pay3: 10, Pay4: 30, Pay5: 80}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}, {Count: 5, Spins: 25}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if v.scatter[0].Count != 5 {
		t.Fatalf("scatter table not sorted desc: %+v", v.scatter)
	}
	if v.gamble.MaxRungs != defaultGamble.MaxRungs || v.gamble.MaxWin != defaultGamble.MaxWin {
		t.Fatalf("gamble defaults not applied: %+v", v.gamble)
	}
}
