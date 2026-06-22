package main

import (
	"math/rand"
	"testing"
)

func TestCountForWildSubstitutes(t *testing.T) {
	if numReels != 5 || visRows != 3 {
		t.Fatalf("want 5 reels x 3 rows, got %dx%d", numReels, visRows)
	}
	cases := []struct {
		w    [visRows]symbol
		s    symbol
		want int
	}{
		{[visRows]symbol{sym7, sym7, symBar}, sym7, 2},
		{[visRows]symbol{sym7, symWild, symBar}, sym7, 2}, // wild counts toward 7
		{[visRows]symbol{symWild, symWild, symWild}, symBar, 3},
		{[visRows]symbol{symScatter, symCherry, symBar}, sym7, 0},
	}
	for _, c := range cases {
		if got := countFor(c.s, c.w); got != c.want {
			t.Errorf("countFor(%c, %v) = %d, want %d", rune(c.s), c.w, got, c.want)
		}
	}
}

// waysVariant builds a variant whose pays map is set directly (strip irrelevant).
func waysVariant(pays map[symbol][3]int) *variant { return &variant{pays: pays} }

// win5 builds a 5x3 window from five 3-row reels.
func win5(reels ...[visRows]symbol) (w [numReels][visRows]symbol) {
	for i := 0; i < numReels && i < len(reels); i++ {
		w[i] = reels[i]
	}
	return w
}

func TestWaysPayout(t *testing.T) {
	col := func(a, b, c symbol) [visRows]symbol { return [visRows]symbol{a, b, c} }
	blank := col(symBar, symBar, symBar) // no 7
	v := waysVariant(map[symbol][3]int{sym7: {10, 50, 250}})

	w := win5(col(sym7, symBar, symBar), col(sym7, symBar, symBar), col(sym7, symBar, symBar), blank, blank)
	if got := v.waysPayout(w); got != 10 {
		t.Errorf("3-of-a-kind 1-way = %d, want 10", got)
	}
	w = win5(col(sym7, sym7, symBar), col(sym7, symBar, symBar), col(sym7, symBar, symBar), blank, blank)
	if got := v.waysPayout(w); got != 20 {
		t.Errorf("ways=2 three-of-a-kind = %d, want 20", got)
	}
	r := col(sym7, symBar, symBar)
	if got := v.waysPayout(win5(r, r, r, r, r)); got != 250 {
		t.Errorf("5-of-a-kind = %d, want 250", got)
	}
	if got := v.waysPayout(win5(r, r, blank, blank, blank)); got != 0 {
		t.Errorf("2-of-a-kind = %d, want 0", got)
	}
	w = win5(r, col(symWild, symBar, symBar), r, blank, blank)
	if got := v.waysPayout(w); got != 10 {
		t.Errorf("wild-completed run = %d, want 10", got)
	}
}

func TestStatsClosedFormWays(t *testing.T) {
	v, err := compileVariant(oddsVariant{
		Name:     "w",
		Weights:  map[string]int{"7": 4, "C": 30, "S": 2},
		Paytable: []payEntry{{Faces: "7", Pay3: 4, Pay4: 15, Pay5: 60}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 5}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s := v.stats()
	if s.LineRTP <= 0 || s.TotalRTP < s.LineRTP {
		t.Fatalf("bad stats: %+v", s)
	}
	if s.TriggerRate <= 0 {
		t.Fatalf("scatters present but trigger rate %v", s.TriggerRate)
	}
	want := s.LineRTP * (1 + s.TriggerRate*s.AvgFreeSpins)
	if d := want - s.TotalRTP; d > 1e-9 || d < -1e-9 {
		t.Fatalf("fold mismatch: total=%.9f want=%.9f", s.TotalRTP, want)
	}
}

func TestThemesCompileInBand(t *testing.T) {
	vs := themeVariants()
	if len(vs) != 6 {
		t.Fatalf("themes = %d, want 6", len(vs))
	}
	names := map[string]bool{}
	for _, v := range vs {
		if v == nil {
			t.Fatal("nil theme variant — a PAR sheet failed to compile")
		}
		s := v.stats()
		if s.TotalRTP < 0.80 || s.TotalRTP > 0.95 {
			t.Errorf("theme %q total RTP %.3f outside [0.80,0.95]", v.name, s.TotalRTP)
		}
		names[v.name] = true
	}
	if len(names) != 6 {
		t.Errorf("theme names not distinct: %v", names)
	}
}

// TestClosedFormMatchesSampling validates the closed-form line RTP against a
// Monte-Carlo of real 5-reel i.i.d. spins. The closed form is exact, so this
// passes comfortably; a failure means the math is wrong (do not loosen).
func TestClosedFormMatchesSampling(t *testing.T) {
	v := defaultVariant()
	st := v.stats()
	rng := rand.New(rand.NewSource(1))
	const N = 400000
	total := 0
	for n := 0; n < N; n++ {
		var w [numReels][visRows]symbol
		for r := 0; r < numReels; r++ {
			w[r] = windowAt(v.strip, rng.Intn(len(v.strip)))
		}
		total += v.waysPayout(w)
	}
	got := float64(total) / float64(N)
	rel := (got - st.LineRTP) / st.LineRTP
	if rel > 0.05 || rel < -0.05 {
		t.Fatalf("sampled line RTP %.4f vs closed-form %.4f (rel %.3f) — closed form wrong", got, st.LineRTP, rel)
	}
}
