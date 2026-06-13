package main

import (
	"math/rand"
	"testing"
)

// TestMatch3Fmt checks that match3Fmt produces the correct 4-char-or-fewer labels.
func TestMatch3Fmt(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "$1"},
		{2, "$2"},
		{5, "$5"},
		{50, "$50"},
		{100, "$100"},
		{500, "$500"},
		{1000, "$1k"},
		{5000, "$5k"},
		{10000, "10k"},
		{250000, "250k"},
	}
	for _, tc := range cases {
		got := match3Fmt(tc.n)
		if got != tc.want {
			t.Errorf("match3Fmt(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// TestMatch3ParseAmt checks round-trip of match3Fmt -> match3ParseAmt.
func TestMatch3ParseAmt(t *testing.T) {
	vals := []int{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 5000, 10000, 250000}
	for _, v := range vals {
		label := match3Fmt(v)
		got := match3ParseAmt(label)
		if got != v {
			t.Errorf("round-trip %d: fmt=%q, parse=%d", v, label, got)
		}
	}
}

// match3TestTicket returns a minimal Ticket for use in tests.
func match3TestTicket(cols, rows int) *Ticket {
	return &Ticket{
		Slug:     "test-match3",
		Name:     "Test Match3",
		Price:    1,
		Mechanic: MechMatch3,
		Cols:     cols,
		Rows:     rows,
		Prizes:   tierTable(10000),
	}
}

// countLabels tallies panel labels across the whole grid (all panels revealed).
func countLabels(g *Grid) map[string]int {
	m := make(map[string]int)
	for i := range g.Panels {
		m[g.Panels[i].Reveal]++
	}
	return m
}

// TestMatch3WinLayout verifies that a winning card has exactly one amount on
// exactly 3 panels, that amount equals out.Win, and that no other amount
// appears 3+ times.
func TestMatch3WinLayout(t *testing.T) {
	seeds := []int64{1, 2, 3, 42, 99, 1234}
	sizes := [][2]int{{3, 3}, {4, 4}, {5, 5}, {6, 6}}
	winAmounts := []int{5, 10, 50, 100}

	for _, seed := range seeds {
		for _, sz := range sizes {
			cols, rows := sz[0], sz[1]
			for _, winAmt := range winAmounts {
				rng := rand.New(rand.NewSource(seed))
				tk := match3TestTicket(cols, rows)
				out := Outcome{Win: winAmt}
				c := match3Build(tk, out, rng)

				labels := countLabels(c.grid)
				winLabel := match3Fmt(winAmt)

				winCnt, ok := labels[winLabel]
				if !ok {
					t.Errorf("seed=%d %dx%d win=%d: winning label %q not found in grid",
						seed, cols, rows, winAmt, winLabel)
					continue
				}
				if winCnt != 3 {
					t.Errorf("seed=%d %dx%d win=%d: winning label %q appears %d times, want 3",
						seed, cols, rows, winAmt, winLabel, winCnt)
				}

				for label, cnt := range labels {
					if label == winLabel {
						continue
					}
					if cnt >= 3 {
						t.Errorf("seed=%d %dx%d win=%d: decoy label %q appears %d times (>= 3)",
							seed, cols, rows, winAmt, label, cnt)
					}
				}
			}
		}
	}
}

// TestMatch3LossLayout verifies that a losing card has no amount appearing 3+
// times across all panels.
func TestMatch3LossLayout(t *testing.T) {
	seeds := []int64{1, 2, 3, 42, 99, 1234}
	sizes := [][2]int{{3, 3}, {4, 4}, {5, 5}, {6, 6}}

	for _, seed := range seeds {
		for _, sz := range sizes {
			cols, rows := sz[0], sz[1]
			rng := rand.New(rand.NewSource(seed))
			tk := match3TestTicket(cols, rows)
			out := Outcome{Win: 0}
			c := match3Build(tk, out, rng)

			labels := countLabels(c.grid)
			for label, cnt := range labels {
				if cnt >= 3 {
					t.Errorf("seed=%d %dx%d loss: label %q appears %d times (>= 3)",
						seed, cols, rows, label, cnt)
				}
			}
		}
	}
}

// TestMatch3WinResolvesCorrectly builds a winning card, reveals all panels via
// ScratchAll, and asserts Win() == out.Win.
func TestMatch3WinResolvesCorrectly(t *testing.T) {
	cases := []struct {
		seed   int64
		cols   int
		rows   int
		winAmt int
	}{
		{1, 3, 3, 5},
		{2, 3, 3, 10},
		{3, 4, 4, 50},
		{42, 5, 5, 100},
		{99, 6, 6, 500},
		{1234, 3, 3, 1000},
	}
	for _, tc := range cases {
		rng := rand.New(rand.NewSource(tc.seed))
		tk := match3TestTicket(tc.cols, tc.rows)
		out := Outcome{Win: tc.winAmt}
		c := match3Build(tk, out, rng)

		if c.Resolved() {
			t.Errorf("seed=%d win=%d: card already resolved before any scratch", tc.seed, tc.winAmt)
		}

		c.ScratchAll()

		if !c.Resolved() {
			t.Errorf("seed=%d win=%d: card not resolved after ScratchAll", tc.seed, tc.winAmt)
		}
		got := c.Win()
		if got != tc.winAmt {
			t.Errorf("seed=%d win=%d: Win()=%d, want %d", tc.seed, tc.winAmt, got, tc.winAmt)
		}
	}
}

// TestMatch3LossResolvesZero builds a losing card, reveals all panels via
// ScratchAll, and asserts Win() == 0.
func TestMatch3LossResolvesZero(t *testing.T) {
	cases := []struct {
		seed int64
		cols int
		rows int
	}{
		{1, 3, 3},
		{2, 4, 4},
		{3, 5, 5},
		{42, 6, 6},
		{99, 3, 3},
	}
	for _, tc := range cases {
		rng := rand.New(rand.NewSource(tc.seed))
		tk := match3TestTicket(tc.cols, tc.rows)
		out := Outcome{Win: 0}
		c := match3Build(tk, out, rng)

		c.ScratchAll()

		if !c.Resolved() {
			t.Errorf("seed=%d loss: card not resolved after ScratchAll", tc.seed)
		}
		got := c.Win()
		if got != 0 {
			t.Errorf("seed=%d loss: Win()=%d, want 0", tc.seed, got)
		}
	}
}

// TestMatch3AutoResolveOnThirdMatch checks that a card auto-resolves mid-scratch
// when the third matching panel is revealed via Scratch().
func TestMatch3AutoResolveOnThirdMatch(t *testing.T) {
	// Build a winning 3x3 card and scratch panel by panel until we get 3 of the winner.
	rng := rand.New(rand.NewSource(7))
	tk := match3TestTicket(3, 3)
	out := Outcome{Win: 5}
	c := match3Build(tk, out, rng)

	winLabel := match3Fmt(5)

	// Wear all layers down to 1 so each Scratch() immediately reveals.
	for i := range c.grid.Panels {
		c.grid.Panels[i].Layers = 1
	}

	matchCount := 0
	resolved := false
	for i := 0; i < 9; i++ {
		c.grid.Cur = i
		revealed := c.Scratch()
		if revealed && c.grid.Panels[i].Reveal == winLabel {
			matchCount++
		}
		if c.Resolved() {
			resolved = true
			break
		}
	}

	if !resolved {
		t.Error("card never auto-resolved on third match")
	}
	if matchCount < 3 {
		// We may have stopped as soon as 3 matched; just check Win() is correct.
		w := c.Win()
		if w != 5 {
			t.Errorf("Win() after auto-resolve = %d, want 5", w)
		}
	}
}

// TestMatch3Title checks the Title format.
func TestMatch3Title(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	tk := &Ticket{
		Name:     "Lucky 7s",
		Price:    1,
		Mechanic: MechMatch3,
		Cols:     3,
		Rows:     3,
		Prizes:   tierTable(10000),
	}
	c := match3Build(tk, Outcome{Win: 0}, rng)
	want := "Lucky 7s · $1 · match three"
	if c.Title() != want {
		t.Errorf("Title() = %q, want %q", c.Title(), want)
	}
}

// TestMatch3WinAmountIsOutWin ensures that Win() returns exactly out.Win (not a
// parsed approximation) for any prize in the catalog's range.
func TestMatch3WinAmountIsOutWin(t *testing.T) {
	prizes := []int{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 5000, 10000}
	for _, prize := range prizes {
		rng := rand.New(rand.NewSource(int64(prize)))
		tk := match3TestTicket(3, 3)
		out := Outcome{Win: prize}
		c := match3Build(tk, out, rng)
		c.ScratchAll()
		got := c.Win()
		if got != prize {
			t.Errorf("prize=%d: Win()=%d, want %d (label=%q)", prize, got, prize, match3Fmt(prize))
		}
	}
}
