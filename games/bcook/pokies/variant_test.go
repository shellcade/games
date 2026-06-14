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

func TestWildCompletesLine(t *testing.T) {
	v := defaultVariant() // 7=500 $=150 *=55 B=10, top=500
	cases := []struct {
		name  string
		reels [3]symbol
		want  int
	}{
		{"7 W 7 pays as 777", [3]symbol{sym7, symWild, sym7}, 500},
		{"W W B pays as BBB", [3]symbol{symWild, symWild, symBar}, 10},
		{"W W W pays top", [3]symbol{symWild, symWild, symWild}, 500},
		{"W $ 7 no line", [3]symbol{symWild, symDollar, sym7}, 0},
		{"scatter on line breaks combo", [3]symbol{sym7, symScatter, sym7}, 0},
		{"W C W is cherries (pays 0)", [3]symbol{symWild, symCherry, symWild}, 0},
		{"plain 777 still 500", [3]symbol{sym7, sym7, sym7}, 500},
		{"plain no-match", [3]symbol{sym7, symDollar, symStar}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := v.payout(c.reels); got != c.want {
				t.Fatalf("payout(%v) = %d, want %d", c.reels, got, c.want)
			}
		})
	}
}

func TestScatterAwardThresholds(t *testing.T) {
	v := &variant{scatter: []scatterEntry{{Count: 5, Spins: 25}, {Count: 4, Spins: 15}, {Count: 3, Spins: 8}}}
	// build a 3x3 window [reel][row] with `n` scatters placed.
	win := func(n int) (w [3][3]symbol) {
		for reel := 0; reel < 3; reel++ {
			for row := 0; row < 3; row++ {
				w[reel][row] = sym7
			}
		}
		placed := 0
		for reel := 0; reel < 3 && placed < n; reel++ {
			for row := 0; row < 3 && placed < n; row++ {
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

func TestStatsFoldsFreeSpins(t *testing.T) {
	v, err := compileVariant(oddsVariant{
		Name:     "fs",
		Weights:  map[string]int{"7": 10, "C": 20, "S": 2},
		Paytable: []payEntry{{Faces: "7", Multiplier: 30}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s := v.stats()
	if s.TriggerRate <= 0 {
		t.Fatalf("trigger rate = %v, want > 0 with scatters present", s.TriggerRate)
	}
	if s.TotalRTP <= s.LineRTP {
		t.Fatalf("total RTP %.4f should exceed line RTP %.4f", s.TotalRTP, s.LineRTP)
	}
	want := s.LineRTP * (1 + s.TriggerRate*s.AvgFreeSpins)
	if diff := want - s.TotalRTP; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("closed form mismatch: total=%.9f want=%.9f", s.TotalRTP, want)
	}
}

func TestCompileRejectsRunawayRetrigger(t *testing.T) {
	// Huge scatter weight + large award -> t*maxAward >= 1 (non-converging).
	_, err := compileVariant(oddsVariant{
		Name:     "runaway",
		Weights:  map[string]int{"7": 1, "S": 30},
		Paytable: []payEntry{{Faces: "7", Multiplier: 5}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 50}},
	})
	if err == nil {
		t.Fatal("expected rejection of a non-converging retrigger feature")
	}
}

func TestCompileSortsScatterDescAndDefaultsGamble(t *testing.T) {
	v, err := compileVariant(oddsVariant{
		Name:     "ok",
		Weights:  map[string]int{"7": 10, "C": 20, "S": 2},
		Paytable: []payEntry{{Faces: "7", Multiplier: 30}},
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
