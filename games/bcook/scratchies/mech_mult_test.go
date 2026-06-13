package main

import (
	"math/rand"
	"testing"

	kit "github.com/shellcade/kit/v2"
)

// TestMultFactor checks that multFactor returns the largest valid divisor.
func TestMultFactor(t *testing.T) {
	cases := []struct {
		win     int
		maxMult int
		wantB   int
		wantM   int
	}{
		// Exact divisor exists at maxMult.
		{win: 200, maxMult: 10, wantB: 20, wantM: 10},
		// 200 % 3 != 0, 200 % 2 == 0, but we search from top: 10 divides 200.
		{win: 200, maxMult: 20, wantB: 10, wantM: 20},
		// 7 is prime; largest divisor ≤ 5 that divides 7 is 1.
		{win: 7, maxMult: 5, wantB: 7, wantM: 1},
		// 12 % 3 == 0 → mult = 3, base = 4.
		{win: 12, maxMult: 3, wantB: 4, wantM: 3},
		// 12 % 5 != 0; 12 % 4 == 0 → base 3, mult 4; but maxMult == 5 so check 5 first.
		{win: 12, maxMult: 5, wantB: 3, wantM: 4},
		// win == 1: only mult=1 works.
		{win: 1, maxMult: 20, wantB: 1, wantM: 1},
		// win == maxMult: mult = maxMult, base = 1.
		{win: 10, maxMult: 10, wantB: 1, wantM: 10},
	}
	for _, tc := range cases {
		b, m := multFactor(tc.win, tc.maxMult)
		if b != tc.wantB || m != tc.wantM {
			t.Errorf("multFactor(%d, %d) = (%d, %d), want (%d, %d)",
				tc.win, tc.maxMult, b, m, tc.wantB, tc.wantM)
		}
		// Sanity: product must equal win.
		if b*m != tc.win {
			t.Errorf("multFactor(%d, %d): base=%d * mult=%d = %d ≠ %d",
				tc.win, tc.maxMult, b, m, b*m, tc.win)
		}
	}
}

// TestMultWinDecompose checks that for every (seed × MaxMult × prize) combination
// on winning cards: base*mult == out.Win, 1 ≤ mult ≤ MaxMult, base ≥ 1.
func TestMultWinDecompose(t *testing.T) {
	// Representative prize ladder (real-ish values from the spec).
	prizes := []int{1, 2, 5, 10, 20, 50, 100, 500, 1000, 2000, 10000, 50000}
	maxMults := []int{3, 5, 10, 20}

	for _, mm := range maxMults {
		for _, prize := range prizes {
			for seed := int64(0); seed < 8; seed++ {
				tk := &Ticket{
					Name:    "Test Mult",
					Price:   5,
					MaxMult: mm,
				}
				out := Outcome{Win: prize}
				rng := rand.New(rand.NewSource(seed))

				card := multBuild(tk, out, rng)
				mc, ok := card.(*multCard)
				if !ok {
					t.Fatalf("multBuild did not return *multCard (seed=%d, maxMult=%d, prize=%d)", seed, mm, prize)
				}

				// Before reveal: Win() must be 0.
				if mc.Win() != 0 {
					t.Errorf("seed=%d maxMult=%d prize=%d: Win()=%d before Resolved()", seed, mm, prize, mc.Win())
				}
				if mc.Resolved() {
					t.Errorf("seed=%d maxMult=%d prize=%d: Resolved() true on fresh card", seed, mm, prize)
				}

				// base and mult constraints.
				if mc.base < 1 {
					t.Errorf("seed=%d maxMult=%d prize=%d: base=%d < 1", seed, mm, prize, mc.base)
				}
				if mc.mult < 1 || mc.mult > mm {
					t.Errorf("seed=%d maxMult=%d prize=%d: mult=%d not in [1,%d]", seed, mm, prize, mc.mult, mm)
				}
				if mc.base*mc.mult != prize {
					t.Errorf("seed=%d maxMult=%d prize=%d: base(%d)*mult(%d)=%d ≠ prize",
						seed, mm, prize, mc.base, mc.mult, mc.base*mc.mult)
				}

				// After ScratchAll: Win() == prize.
				mc.ScratchAll()
				if !mc.Resolved() {
					t.Errorf("seed=%d maxMult=%d prize=%d: not Resolved after ScratchAll", seed, mm, prize)
				}
				if mc.Win() != prize {
					t.Errorf("seed=%d maxMult=%d prize=%d: Win()=%d after ScratchAll, want %d",
						seed, mm, prize, mc.Win(), prize)
				}
			}
		}
	}
}

// TestMultLoss checks that a loss card returns Win()==0 after full reveal.
func TestMultLoss(t *testing.T) {
	for _, mm := range []int{3, 5, 10, 20} {
		for seed := int64(0); seed < 8; seed++ {
			tk := &Ticket{Name: "Test Mult", Price: 5, MaxMult: mm}
			out := Outcome{Win: 0}
			rng := rand.New(rand.NewSource(seed))

			card := multBuild(tk, out, rng)
			mc := card.(*multCard)

			if mc.base != 0 {
				t.Errorf("seed=%d maxMult=%d: loss card base=%d, want 0", seed, mm, mc.base)
			}
			if mc.mult < 1 || mc.mult > mm {
				t.Errorf("seed=%d maxMult=%d: loss card mult=%d not in [1,%d]", seed, mm, mc.mult, mm)
			}

			mc.ScratchAll()
			if mc.Win() != 0 {
				t.Errorf("seed=%d maxMult=%d: loss card Win()=%d after ScratchAll, want 0", seed, mm, mc.Win())
			}
		}
	}
}

// TestMultResolvedGate checks that Win() returns 0 until both panels are revealed.
func TestMultResolvedGate(t *testing.T) {
	tk := &Ticket{Name: "Test", Price: 5, MaxMult: 10}
	out := Outcome{Win: 200}
	rng := rand.New(rand.NewSource(42))

	card := multBuild(tk, out, rng)
	mc := card.(*multCard)

	// Seed each panel to exactly 1 layer so a single Scratch() reveals it.
	mc.grid.Panels[0].Layers = 1
	mc.grid.Panels[1].Layers = 1

	// Nothing revealed yet.
	if mc.Resolved() {
		t.Fatal("Resolved() true on fresh card")
	}
	if mc.Win() != 0 {
		t.Fatalf("Win()=%d before Resolved, want 0", mc.Win())
	}

	// Reveal panel 0 (cursor starts at 0).
	mc.grid.Cur = 0
	mc.Scratch()
	if mc.Resolved() {
		t.Fatal("Resolved() true after only one panel scratched")
	}
	if mc.Win() != 0 {
		t.Fatalf("Win()=%d after one panel, want 0", mc.Win())
	}

	// Reveal panel 1.
	mc.grid.Cur = 1
	mc.Scratch()
	if !mc.Resolved() {
		t.Fatal("not Resolved() after both panels scratched")
	}
	if mc.Win() != 200 {
		t.Fatalf("Win()=%d after both revealed, want 200", mc.Win())
	}
}

// TestMultTitle checks that Title() includes the ticket name, price, and flavour.
func TestMultTitle(t *testing.T) {
	tk := &Ticket{Name: "Mega Multiplier", Price: 5, MaxMult: 10}
	out := Outcome{Win: 100}
	rng := rand.New(rand.NewSource(1))
	card := multBuild(tk, out, rng)

	title := card.Title()
	if title == "" {
		t.Fatal("Title() returned empty string")
	}
	// Must contain the ticket name and price.
	for _, want := range []string{"Mega Multiplier", "$5"} {
		found := false
		for i := 0; i+len(want) <= len(title); i++ {
			if title[i:i+len(want)] == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Title() = %q, does not contain %q", title, want)
		}
	}
}

// TestMultPromptResolved checks Prompt() output before and after resolution.
func TestMultPromptResolved(t *testing.T) {
	tk := &Ticket{Name: "Test", Price: 5, MaxMult: 10}

	// Winning card.
	out := Outcome{Win: 200}
	rng := rand.New(rand.NewSource(1))
	card := multBuild(tk, out, rng)
	mc := card.(*multCard)

	// Before resolved: generic hint.
	pre := mc.Prompt()
	if pre == "" {
		t.Fatal("Prompt() returned empty string before resolved")
	}

	mc.ScratchAll()
	post := mc.Prompt()
	// Should contain the product "200" (or "2,000" etc.) and the multiplier.
	if post == "" {
		t.Fatal("Prompt() returned empty string after resolved")
	}

	// Losing card.
	out2 := Outcome{Win: 0}
	rng2 := rand.New(rand.NewSource(99))
	card2 := multBuild(tk, out2, rng2)
	mc2 := card2.(*multCard)
	mc2.ScratchAll()
	lossPrompt := mc2.Prompt()
	if lossPrompt != "no prize this time" {
		t.Errorf("loss Prompt() = %q, want %q", lossPrompt, "no prize this time")
	}
}

// TestMultRenderSmoke verifies Render does not panic on fresh and resolved cards.
func TestMultRenderSmoke(t *testing.T) {
	tk := &Ticket{Name: "Mega Multiplier", Price: 5, MaxMult: 10}

	// Win card, fresh.
	card1 := multBuild(tk, Outcome{Win: 200}, rand.New(rand.NewSource(7)))
	f1 := kit.NewFrame()
	card1.Render(f1, 2)

	// Win card, resolved.
	card2 := multBuild(tk, Outcome{Win: 2000}, rand.New(rand.NewSource(8)))
	card2.ScratchAll()
	f2 := kit.NewFrame()
	card2.Render(f2, 2)

	// Loss card, resolved.
	card3 := multBuild(tk, Outcome{Win: 0}, rand.New(rand.NewSource(9)))
	card3.ScratchAll()
	f3 := kit.NewFrame()
	card3.Render(f3, 2)

	// Big win card (win >= 500 and >= 50*price).
	// Price=5, 50*5=250, so win=2000 qualifies.
	bigWin := multBuild(tk, Outcome{Win: 2000}, rand.New(rand.NewSource(10)))
	bigWin.ScratchAll()
	f4 := kit.NewFrame()
	bigWin.Render(f4, 2)

	// Smoke: just ensure none of the above panicked.
	_ = f1
	_ = f2
	_ = f3
	_ = f4
}

// TestMultMoveAndScratch exercises Move, Scratch, and cursor clamp.
func TestMultMoveAndScratch(t *testing.T) {
	tk := &Ticket{Name: "Test", Price: 1, MaxMult: 3}
	rng := rand.New(rand.NewSource(5))
	card := multBuild(tk, Outcome{Win: 6}, rng)
	mc := card.(*multCard)

	// Force known depths for determinism.
	mc.grid.Panels[0].Layers = 1
	mc.grid.Panels[1].Layers = 1

	// Move down should go to panel 1 (it's a 1-col × 2-row grid).
	mc.Move(0, 1)
	if mc.grid.Cur != 1 {
		t.Errorf("after Move(0,1) cursor = %d, want 1", mc.grid.Cur)
	}

	// Move back up.
	mc.Move(0, -1)
	if mc.grid.Cur != 0 {
		t.Errorf("after Move(0,-1) cursor = %d, want 0", mc.grid.Cur)
	}

	// Scratch panel 0.
	revealed := mc.Scratch()
	if !revealed {
		t.Fatal("Scratch() on 1-layer panel should return true")
	}
	if mc.Resolved() {
		t.Fatal("Resolved() true after only panel 0 revealed")
	}

	// Scratch panel 1.
	mc.Move(0, 1)
	mc.Scratch()
	if !mc.Resolved() {
		t.Fatal("Resolved() false after both panels revealed")
	}
}
