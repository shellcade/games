package main

import (
	"math/rand"
	"testing"
)

// A panel needs exactly `Layers` rubs to reveal.
func TestScratchLayers(t *testing.T) {
	g := NewGrid(1, 1)
	g.Panels[0].Layers = 3
	g.Panels[0].Hidden = true
	if g.Scratch() {
		t.Fatal("revealed after 1 rub, want 3")
	}
	if g.Scratch() {
		t.Fatal("revealed after 2 rubs, want 3")
	}
	if !g.Scratch() {
		t.Fatal("not revealed after 3 rubs")
	}
	if g.Panels[0].Hidden {
		t.Fatal("panel still hidden after full scratch")
	}
}

// seedDepths assigns every panel a depth in [1,3].
func TestSeedDepthsRange(t *testing.T) {
	g := NewGrid(6, 6)
	g.seedDepths(rand.New(rand.NewSource(1)))
	for i, p := range g.Panels {
		if p.Layers < 1 || p.Layers > 3 {
			t.Fatalf("panel %d depth %d out of [1,3]", i, p.Layers)
		}
	}
}

// Move clamps to the grid bounds.
func TestMoveBounds(t *testing.T) {
	g := NewGrid(3, 3)
	g.Move(-1, -1) // already top-left
	if g.Cur != 0 {
		t.Fatalf("cur %d, want 0", g.Cur)
	}
	g.Move(10, 10) // clamp to bottom-right
	if g.Cur != 8 {
		t.Fatalf("cur %d, want 8 (bottom-right of 3x3)", g.Cur)
	}
}

// ScratchAll reveals every panel.
func TestScratchAll(t *testing.T) {
	g := NewGrid(4, 4)
	g.seedDepths(rand.New(rand.NewSource(2)))
	g.ScratchAll()
	if !g.AllRevealed() {
		t.Fatal("not all revealed after ScratchAll")
	}
}

// Every catalog ticket has a sane shape (used as a smoke check until Agent E's
// stats() test lands).
func TestCatalogShape(t *testing.T) {
	if len(tickets) != 16 {
		t.Fatalf("catalog has %d tickets, want 16", len(tickets))
	}
	for _, tk := range tickets {
		if tk.Cols > 6 {
			t.Errorf("%s: %d cols > 6 (won't fit 80 wide)", tk.Slug, tk.Cols)
		}
		if len(tk.Prizes) == 0 {
			t.Errorf("%s: empty prize table", tk.Slug)
		}
		prev := 0
		for _, row := range tk.Prizes {
			if row.Credits < prev {
				t.Errorf("%s: prize table not ascending (%d after %d)", tk.Slug, row.Credits, prev)
			}
			prev = row.Credits
		}
	}
}

// A drawn outcome is always either 0 or one of the table's prize values.
func TestDrawOutcomeValid(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	tk := &tickets[0]
	valid := map[int]bool{0: true}
	for _, row := range tk.Prizes {
		valid[row.Credits] = true
	}
	for i := 0; i < 5000; i++ {
		out := drawOutcome(tk, rng)
		if !valid[out.Win] {
			t.Fatalf("drew invalid win %d", out.Win)
		}
	}
}
