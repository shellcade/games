package main

import (
	"math/rand"
	"testing"

	kit "github.com/shellcade/kit/v2"
)

// linesTestTicket returns a minimal Ticket for use in Lucky Lines tests.
func linesTestTicket(cols, rows int) *Ticket {
	return &Ticket{
		Slug:     "test-lines",
		Name:     "Lucky Lines",
		Price:    1,
		Mechanic: MechLines,
		Cols:     cols,
		Rows:     rows,
		Prizes:   tier1Table(10000),
	}
}

// linesCountConsec3 counts the number of consecutive-3 equal-amount runs
// visible across all panels of the grid (regardless of Hidden state — this is
// used after ScratchAll when all panels are revealed).
func linesCountConsec3(g *Grid, allLines []linesLine) int {
	n := 0
	for _, ln := range allLines {
		a := g.Panels[ln[0]].Reveal
		b := g.Panels[ln[1]].Reveal
		c := g.Panels[ln[2]].Reveal
		if a != "" && a == b && b == c {
			n++
		}
	}
	return n
}

// TestLinesFmt checks linesFmt produces the expected short labels.
func TestLinesFmt(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "$1"},
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
		got := linesFmt(tc.n)
		if got != tc.want {
			t.Errorf("linesFmt(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// TestLinesParseAmtRoundTrip checks linesFmt→linesParseAmt round-trip.
func TestLinesParseAmtRoundTrip(t *testing.T) {
	vals := []int{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 5000, 10000, 250000}
	for _, v := range vals {
		label := linesFmt(v)
		got := linesParseAmt(label)
		if got != v {
			t.Errorf("round-trip %d: fmt=%q, parse=%d", v, label, got)
		}
	}
}

// TestLinesAllLines checks that linesAllLines returns the correct count of
// consecutive-3 runs for known grid sizes.
func TestLinesAllLines(t *testing.T) {
	// 3×3 grid: 3 rows×1 run + 3 cols×1 run + 1 diag↘ + 1 diag↙ = 8.
	ll33 := linesAllLines(3, 3)
	if len(ll33) != 8 {
		t.Errorf("3×3: want 8 lines, got %d", len(ll33))
	}

	// 5×5 grid:
	//  rows: 5 rows × 3 runs = 15
	//  cols: 5 cols × 3 runs = 15
	//  diag↘: 3×3 = 9
	//  diag↙: 3×3 = 9
	//  total = 48
	ll55 := linesAllLines(5, 5)
	if len(ll55) != 48 {
		t.Errorf("5×5: want 48 lines, got %d", len(ll55))
	}
}

// TestLinesWinLayout verifies that a winning card has exactly one consecutive-3
// equal-amount run, it equals out.Win, and no decoy forms another consecutive-3
// run. Table-driven over seeds and sizes.
func TestLinesWinLayout(t *testing.T) {
	seeds := []int64{1, 2, 3, 42, 99, 1234, 777, 8888}
	sizes := [][2]int{{3, 3}, {5, 5}}
	winAmounts := []int{5, 10, 50, 100, 500}

	for _, seed := range seeds {
		for _, sz := range sizes {
			cols, rows := sz[0], sz[1]
			for _, winAmt := range winAmounts {
				rng := rand.New(rand.NewSource(seed))
				tk := linesTestTicket(cols, rows)
				out := Outcome{Win: winAmt}
				c := linesBuild(tk, out, rng)

				allLines := linesAllLines(cols, rows)
				winLabel := linesFmt(winAmt)

				total := linesCountConsec3(c.grid, allLines)
				if total != 1 {
					t.Errorf("seed=%d %dx%d win=%d: want exactly 1 consec-3 run, got %d",
						seed, cols, rows, winAmt, total)
				}

				// Verify the one run has the winning label.
				foundWinLine := false
				for _, ln := range allLines {
					a := c.grid.Panels[ln[0]].Reveal
					b := c.grid.Panels[ln[1]].Reveal
					cc := c.grid.Panels[ln[2]].Reveal
					if a == b && b == cc && a == winLabel {
						foundWinLine = true
					}
				}
				if !foundWinLine {
					t.Errorf("seed=%d %dx%d win=%d: no consecutive-3 run of %q found",
						seed, cols, rows, winAmt, winLabel)
				}
			}
		}
	}
}

// TestLinesLossLayout verifies that a losing card has no consecutive-3
// equal-amount run anywhere.
func TestLinesLossLayout(t *testing.T) {
	seeds := []int64{1, 2, 3, 42, 99, 1234, 777, 8888}
	sizes := [][2]int{{3, 3}, {5, 5}}

	for _, seed := range seeds {
		for _, sz := range sizes {
			cols, rows := sz[0], sz[1]
			rng := rand.New(rand.NewSource(seed))
			tk := linesTestTicket(cols, rows)
			out := Outcome{Win: 0}
			c := linesBuild(tk, out, rng)

			allLines := linesAllLines(cols, rows)
			n := linesCountConsec3(c.grid, allLines)
			if n != 0 {
				t.Errorf("seed=%d %dx%d loss: want 0 consec-3 runs, got %d", seed, cols, rows, n)
			}
		}
	}
}

// TestLinesWinResolvesCorrectly builds a winning card, reveals all panels via
// ScratchAll, and asserts Resolved() and Win() == out.Win.
func TestLinesWinResolvesCorrectly(t *testing.T) {
	cases := []struct {
		seed   int64
		cols   int
		rows   int
		winAmt int
	}{
		{1, 3, 3, 5},
		{2, 3, 3, 10},
		{3, 5, 5, 50},
		{42, 5, 5, 100},
		{99, 3, 3, 500},
		{1234, 5, 5, 1000},
	}
	for _, tc := range cases {
		rng := rand.New(rand.NewSource(tc.seed))
		tk := linesTestTicket(tc.cols, tc.rows)
		out := Outcome{Win: tc.winAmt}
		c := linesBuild(tk, out, rng)

		if c.Resolved() {
			t.Errorf("seed=%d win=%d: card already resolved before any scratch", tc.seed, tc.winAmt)
		}

		c.ScratchAll()

		if !c.Resolved() {
			t.Errorf("seed=%d win=%d: not resolved after ScratchAll", tc.seed, tc.winAmt)
		}
		got := c.Win()
		if got != tc.winAmt {
			t.Errorf("seed=%d win=%d: Win()=%d, want %d", tc.seed, tc.winAmt, got, tc.winAmt)
		}
	}
}

// TestLinesLossResolvesZero builds a losing card, reveals all, and asserts Win()==0.
func TestLinesLossResolvesZero(t *testing.T) {
	cases := []struct {
		seed int64
		cols int
		rows int
	}{
		{1, 3, 3},
		{2, 5, 5},
		{3, 3, 3},
		{42, 5, 5},
		{99, 3, 3},
	}
	for _, tc := range cases {
		rng := rand.New(rand.NewSource(tc.seed))
		tk := linesTestTicket(tc.cols, tc.rows)
		out := Outcome{Win: 0}
		c := linesBuild(tk, out, rng)

		c.ScratchAll()

		if !c.Resolved() {
			t.Errorf("seed=%d loss: not resolved after ScratchAll", tc.seed)
		}
		got := c.Win()
		if got != 0 {
			t.Errorf("seed=%d loss: Win()=%d, want 0", tc.seed, got)
		}
	}
}

// TestLinesNoSpoiler guards the NO-SPOILER rule: winning cells must not be
// coloured stMatch before resolution. They turn green only in Render after
// the card resolves.
func TestLinesNoSpoiler(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tk := linesTestTicket(3, 3)
	c := linesBuild(tk, Outcome{Win: 10}, rng)

	// Nothing green at build time.
	for i, p := range c.grid.Panels {
		if p.Ink == stMatch {
			t.Fatalf("panel %d pre-highlighted green at build", i)
		}
	}

	// Reveal a single winning panel, render — still must not be green.
	wl := c.allLines[c.winLineIdx]
	firstWin := wl[0]
	c.grid.Cur = firstWin
	c.grid.Panels[firstWin].Layers = 1
	c.Scratch() // reveals it
	c.Render(kit.NewFrame(), 3)
	if c.grid.Panels[firstWin].Ink == stMatch {
		t.Fatal("winning panel highlighted green before line resolved")
	}

	// After full resolution + render, exactly the three winning cells are green.
	c.ScratchAll()
	c.Render(kit.NewFrame(), 3)
	winLabel := linesFmt(10)
	greens := 0
	for _, idx := range wl {
		if c.grid.Panels[idx].Reveal == winLabel && c.grid.Panels[idx].Ink == stMatch {
			greens++
		}
	}
	if greens != 3 {
		t.Fatalf("want 3 green winning panels after resolution, got %d", greens)
	}
}

// TestLinesAutoResolveOnWinLine checks that a card auto-resolves mid-scratch
// once all three cells of the winning line are revealed via Scratch().
func TestLinesAutoResolveOnWinLine(t *testing.T) {
	seeds := []int64{1, 7, 42, 99}
	for _, seed := range seeds {
		rng := rand.New(rand.NewSource(seed))
		tk := linesTestTicket(3, 3)
		out := Outcome{Win: 5}
		c := linesBuild(tk, out, rng)

		// Flatten all layers to 1 so every Scratch() immediately reveals.
		for i := range c.grid.Panels {
			c.grid.Panels[i].Layers = 1
		}

		wl := c.allLines[c.winLineIdx]

		// Reveal the first two winning cells — should not resolve yet.
		c.grid.Cur = wl[0]
		c.Scratch()
		if c.Resolved() {
			t.Errorf("seed=%d: resolved after 1 winning cell", seed)
		}
		c.grid.Cur = wl[1]
		c.Scratch()
		if c.Resolved() {
			t.Errorf("seed=%d: resolved after 2 winning cells", seed)
		}

		// Reveal the third — should auto-resolve.
		c.grid.Cur = wl[2]
		c.Scratch()
		if !c.Resolved() {
			t.Errorf("seed=%d: not resolved after all 3 winning cells revealed", seed)
		}
		if got := c.Win(); got != 5 {
			t.Errorf("seed=%d: Win()=%d after auto-resolve, want 5", seed, got)
		}
	}
}

// TestLinesTitle checks the Title format.
func TestLinesTitle(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	tk := &Ticket{
		Name:     "Lucky Lines",
		Price:    1,
		Mechanic: MechLines,
		Cols:     3,
		Rows:     3,
		Prizes:   tier1Table(10000),
	}
	c := linesBuild(tk, Outcome{Win: 0}, rng)
	want := "Lucky Lines · $1 · three in a line"
	if c.Title() != want {
		t.Errorf("Title() = %q, want %q", c.Title(), want)
	}
}

// TestLinesWinAmountIsOutWin ensures Win() returns exactly out.Win (not a
// parsed approximation) for various prize amounts.
func TestLinesWinAmountIsOutWin(t *testing.T) {
	prizes := []int{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 5000, 10000}
	for _, prize := range prizes {
		rng := rand.New(rand.NewSource(int64(prize)))
		tk := linesTestTicket(3, 3)
		out := Outcome{Win: prize}
		c := linesBuild(tk, out, rng)
		c.ScratchAll()
		got := c.Win()
		if got != prize {
			t.Errorf("prize=%d: Win()=%d, want %d", prize, got, prize)
		}
	}
}

// TestLinesRenderNoPanic exercises Render at various stages to ensure it does
// not panic.
func TestLinesRenderNoPanic(t *testing.T) {
	cases := []struct {
		seed  int64
		win   int
		stage string
	}{
		{1, 10, "pre-scratch"},
		{2, 0, "pre-scratch-loss"},
		{3, 50, "mid-scratch"},
		{4, 0, "mid-scratch-loss"},
		{5, 100, "resolved"},
		{6, 0, "resolved-loss"},
	}
	for _, tc := range cases {
		rng := rand.New(rand.NewSource(tc.seed))
		tk := linesTestTicket(3, 3)
		c := linesBuild(tk, Outcome{Win: tc.win}, rng)

		f := kit.NewFrame()
		c.Render(f, 3) // pre-scratch

		if tc.stage == "mid-scratch" || tc.stage == "mid-scratch-loss" {
			c.Move(1, 0)
			c.Scratch()
			f2 := kit.NewFrame()
			c.Render(f2, 3)
		}

		c.ScratchAll()
		f3 := kit.NewFrame()
		c.Render(f3, 3)
	}
}
