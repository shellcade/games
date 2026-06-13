package main

import (
	"math/rand"
	"testing"

	kit "github.com/shellcade/kit/v2"
)

// sdTicket returns a minimal Ticket for Showdown tests.
func sdTicket(cols int) *Ticket {
	return &Ticket{
		Name:     "Test Showdown",
		Price:    5,
		Mechanic: MechShowdown,
		Cols:     cols,
		Rows:     2,
		Prizes:   tier5Table(100000),
	}
}

// TestSdWinExactColumn verifies that on a winning card:
//   - exactly the winning column(s)'s prizes sum to out.Win
//   - your value > house value ONLY in those columns
//   - Win() == out.Win after full reveal
func TestSdWinExactColumn(t *testing.T) {
	prizes := []int{5, 10, 25, 50, 100, 250}
	colCounts := []int{3, 4}

	for _, cols := range colCounts {
		for _, prize := range prizes {
			for seed := int64(0); seed < 8; seed++ {
				tk := sdTicket(cols)
				out := Outcome{Win: prize}
				rng := rand.New(rand.NewSource(seed))

				card := sdBuild(tk, out, rng)
				sd, ok := card.(*sdCard)
				if !ok {
					t.Fatalf("sdBuild did not return *sdCard (cols=%d, seed=%d)", cols, seed)
				}

				// Before reveal: Win() must be 0.
				if sd.Win() != 0 {
					t.Errorf("cols=%d seed=%d prize=%d: Win()=%d before Resolved", cols, seed, prize, sd.Win())
				}
				if sd.Resolved() {
					t.Errorf("cols=%d seed=%d prize=%d: Resolved() true on fresh card", cols, seed, prize)
				}

				// Check structural invariants before reveal.
				wonPrizeSum := 0
				wonColCount := 0
				for c := 0; c < cols; c++ {
					if sd.yourValues[c] > sd.houseValues[c] {
						wonPrizeSum += sd.prizes[c]
						wonColCount++
					}
				}
				if wonColCount != 1 {
					t.Errorf("cols=%d seed=%d prize=%d: expected exactly 1 winning column, got %d",
						cols, seed, prize, wonColCount)
				}
				if wonPrizeSum != prize {
					t.Errorf("cols=%d seed=%d prize=%d: winning-column prize sum=%d, want %d",
						cols, seed, prize, wonPrizeSum, prize)
				}

				// Reveal all and check Win().
				sd.ScratchAll()
				if !sd.Resolved() {
					t.Errorf("cols=%d seed=%d prize=%d: Resolved() false after ScratchAll", cols, seed, prize)
				}
				if sd.Win() != prize {
					t.Errorf("cols=%d seed=%d prize=%d: Win()=%d after ScratchAll, want %d",
						cols, seed, prize, sd.Win(), prize)
				}
			}
		}
	}
}

// TestSdLossAllColumnsLose verifies that on a losing card:
//   - your value ≤ house value in every column
//   - Win() == 0 after full reveal
func TestSdLossAllColumnsLose(t *testing.T) {
	colCounts := []int{3, 4}

	for _, cols := range colCounts {
		for seed := int64(0); seed < 12; seed++ {
			tk := sdTicket(cols)
			out := Outcome{Win: 0}
			rng := rand.New(rand.NewSource(seed))

			card := sdBuild(tk, out, rng)
			sd, ok := card.(*sdCard)
			if !ok {
				t.Fatalf("sdBuild did not return *sdCard (cols=%d, seed=%d)", cols, seed)
			}

			// Check that no column is a winner.
			for c := 0; c < cols; c++ {
				if sd.yourValues[c] > sd.houseValues[c] {
					t.Errorf("cols=%d seed=%d: loss card column %d: your(%d) > house(%d)",
						cols, seed, c, sd.yourValues[c], sd.houseValues[c])
				}
			}

			// Win() must be 0 before reveal.
			if sd.Win() != 0 {
				t.Errorf("cols=%d seed=%d: Win()=%d before Resolved, want 0", cols, seed, sd.Win())
			}

			// After reveal, still 0.
			sd.ScratchAll()
			if !sd.Resolved() {
				t.Errorf("cols=%d seed=%d: Resolved() false after ScratchAll", cols, seed)
			}
			if sd.Win() != 0 {
				t.Errorf("cols=%d seed=%d: Win()=%d after ScratchAll on loss, want 0", cols, seed, sd.Win())
			}
		}
	}
}

// TestSdResolvedGate checks that Win() returns 0 until all YOU-row panels are
// revealed, even on a winning card.
func TestSdResolvedGate(t *testing.T) {
	for _, cols := range []int{3, 4} {
		tk := sdTicket(cols)
		out := Outcome{Win: 50}
		rng := rand.New(rand.NewSource(42))

		card := sdBuild(tk, out, rng)
		sd := card.(*sdCard)

		// Force each YOU-row panel to exactly 1 layer for determinism.
		for c := 0; c < cols; c++ {
			sd.grid.Panels[cols+c].Layers = 1
		}

		if sd.Resolved() {
			t.Fatalf("cols=%d: Resolved() true on fresh card", cols)
		}
		if sd.Win() != 0 {
			t.Fatalf("cols=%d: Win()=%d before Resolved, want 0", cols, sd.Win())
		}

		// Reveal columns one by one; Win() must stay 0 until all are revealed.
		for c := 0; c < cols-1; c++ {
			sd.grid.Cur = cols + c // row 1, column c
			sd.Scratch()
			if sd.Resolved() {
				t.Fatalf("cols=%d: Resolved() true after only %d of %d YOU panels revealed", cols, c+1, cols)
			}
			if sd.Win() != 0 {
				t.Fatalf("cols=%d: Win()=%d after partial reveal, want 0", cols, sd.Win())
			}
		}

		// Reveal the last panel.
		sd.grid.Cur = cols + cols - 1
		sd.Scratch()
		if !sd.Resolved() {
			t.Fatalf("cols=%d: Resolved() false after all YOU panels revealed", cols)
		}
		if sd.Win() != out.Win {
			t.Fatalf("cols=%d: Win()=%d after full reveal, want %d", cols, sd.Win(), out.Win)
		}
	}
}

// TestSdHouseRowPreRevealed checks that all HOUSE panels (row 0) are
// pre-revealed at build time.
func TestSdHouseRowPreRevealed(t *testing.T) {
	for _, cols := range []int{3, 4} {
		tk := sdTicket(cols)
		rng := rand.New(rand.NewSource(7))
		card := sdBuild(tk, Outcome{Win: 25}, rng)
		sd := card.(*sdCard)

		for c := 0; c < cols; c++ {
			idx := 0*cols + c
			if sd.grid.Panels[idx].Hidden {
				t.Errorf("cols=%d: HOUSE panel %d is still hidden at build time", cols, c)
			}
			if sd.grid.Panels[idx].Layers != 0 {
				t.Errorf("cols=%d: HOUSE panel %d has Layers=%d, want 0", cols, c, sd.grid.Panels[idx].Layers)
			}
		}
	}
}

// TestSdYouRowStartsHidden checks that all YOU panels (row 1) are hidden at build time.
func TestSdYouRowStartsHidden(t *testing.T) {
	for _, cols := range []int{3, 4} {
		tk := sdTicket(cols)
		rng := rand.New(rand.NewSource(3))
		card := sdBuild(tk, Outcome{Win: 0}, rng)
		sd := card.(*sdCard)

		for c := 0; c < cols; c++ {
			idx := 1*cols + c
			if !sd.grid.Panels[idx].Hidden {
				t.Errorf("cols=%d: YOU panel %d is not hidden at build time", cols, c)
			}
		}
	}
}

// TestSdValuesInRange checks that all house and your values are within [2, 99].
func TestSdValuesInRange(t *testing.T) {
	colCounts := []int{3, 4}
	for _, cols := range colCounts {
		for seed := int64(0); seed < 10; seed++ {
			tk := sdTicket(cols)
			rng := rand.New(rand.NewSource(seed))
			card := sdBuild(tk, Outcome{Win: 50}, rng)
			sd := card.(*sdCard)

			for c := 0; c < cols; c++ {
				hv := sd.houseValues[c]
				yv := sd.yourValues[c]
				if hv < 2 || hv > 99 {
					t.Errorf("cols=%d seed=%d col=%d: house value %d out of [2,99]", cols, seed, c, hv)
				}
				if yv < 2 || yv > 99 {
					t.Errorf("cols=%d seed=%d col=%d: your value %d out of [2,99]", cols, seed, c, yv)
				}
			}

			// Loss card too.
			rng2 := rand.New(rand.NewSource(seed + 100))
			card2 := sdBuild(tk, Outcome{Win: 0}, rng2)
			sd2 := card2.(*sdCard)
			for c := 0; c < cols; c++ {
				hv := sd2.houseValues[c]
				yv := sd2.yourValues[c]
				if hv < 2 || hv > 99 {
					t.Errorf("loss cols=%d seed=%d col=%d: house value %d out of [2,99]", cols, seed, c, hv)
				}
				if yv < 2 || yv > 99 {
					t.Errorf("loss cols=%d seed=%d col=%d: your value %d out of [2,99]", cols, seed, c, yv)
				}
			}
		}
	}
}

// TestSdTitle checks that Title() includes the ticket name, price, and "beat the house".
func TestSdTitle(t *testing.T) {
	tk := &Ticket{Name: "Showdown", Price: 5, Mechanic: MechShowdown, Cols: 3, Rows: 2}
	rng := rand.New(rand.NewSource(1))
	card := sdBuild(tk, Outcome{Win: 0}, rng)

	title := card.Title()
	for _, want := range []string{"Showdown", "$5", "beat the house"} {
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

// TestSdRenderSmoke verifies Render does not panic on fresh and resolved cards.
func TestSdRenderSmoke(t *testing.T) {
	tk := sdTicket(3)

	// Win card, fresh.
	card1 := sdBuild(tk, Outcome{Win: 100}, rand.New(rand.NewSource(7)))
	f1 := kit.NewFrame()
	card1.Render(f1, 2)

	// Win card, resolved.
	card2 := sdBuild(tk, Outcome{Win: 250}, rand.New(rand.NewSource(8)))
	card2.ScratchAll()
	f2 := kit.NewFrame()
	card2.Render(f2, 2)

	// Loss card, resolved.
	card3 := sdBuild(tk, Outcome{Win: 0}, rand.New(rand.NewSource(9)))
	card3.ScratchAll()
	f3 := kit.NewFrame()
	card3.Render(f3, 2)

	// 4-column win card.
	tk4 := sdTicket(4)
	card4 := sdBuild(tk4, Outcome{Win: 50}, rand.New(rand.NewSource(10)))
	card4.ScratchAll()
	f4 := kit.NewFrame()
	card4.Render(f4, 2)

	_ = f1
	_ = f2
	_ = f3
	_ = f4
}

// TestSdNoSpoilerBeforeReveal checks that the NO-SPOILER RULE is respected:
// panels in the YOU row must not use stMatch style until resolved.
func TestSdNoSpoilerBeforeReveal(t *testing.T) {
	for _, cols := range []int{3, 4} {
		tk := sdTicket(cols)
		rng := rand.New(rand.NewSource(5))
		card := sdBuild(tk, Outcome{Win: 100}, rng)
		sd := card.(*sdCard)

		// Before resolution, no YOU panel should have stMatch ink set.
		for c := 0; c < cols; c++ {
			idx := 1*cols + c
			if sd.grid.Panels[idx].Ink == stMatch {
				t.Errorf("cols=%d col=%d: YOU panel ink is stMatch before resolution (spoiler!)", cols, c)
			}
		}
	}
}

// TestSdMoveStaysInYouRow checks that Move does not allow the cursor to land
// on the HOUSE row.
func TestSdMoveStaysInYouRow(t *testing.T) {
	for _, cols := range []int{3, 4} {
		tk := sdTicket(cols)
		rng := rand.New(rand.NewSource(2))
		card := sdBuild(tk, Outcome{Win: 0}, rng)
		sd := card.(*sdCard)

		// Try moving up (which would be toward row 0).
		sd.Move(0, -1)
		if sd.grid.Cur < cols {
			t.Errorf("cols=%d: Move(0,-1) moved cursor to row 0 (idx=%d)", cols, sd.grid.Cur)
		}

		// Move right repeatedly to hit the boundary.
		for i := 0; i < cols+5; i++ {
			sd.Move(1, 0)
			if sd.grid.Cur < cols {
				t.Errorf("cols=%d: Move(1,0) moved cursor to row 0 (idx=%d)", cols, sd.grid.Cur)
			}
		}

		// Move left repeatedly.
		for i := 0; i < cols+5; i++ {
			sd.Move(-1, 0)
			if sd.grid.Cur < cols {
				t.Errorf("cols=%d: Move(-1,0) moved cursor to row 0 (idx=%d)", cols, sd.grid.Cur)
			}
		}
	}
}
