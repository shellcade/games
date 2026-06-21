package main

import (
	"math/rand"
	"testing"
)

// smallTicket builds a minimal Ticket for 3-winning × 3×3-grid key-number tests.
func keynumTicket3x3() Ticket {
	return Ticket{
		Slug:       "test-keynum-3x3",
		Name:       "Test KeyNum 3×3",
		Price:      2,
		Mechanic:   MechKeyNum,
		Cols:       3,
		Rows:       3,
		WinNumbers: 3,
		Prizes: PrizeTable{
			{Credits: 1, OneIn: 6},
			{Credits: 2, OneIn: 18},
			{Credits: 5, OneIn: 40},
			{Credits: 10, OneIn: 120},
			{Credits: 25, OneIn: 400},
			{Credits: 50, OneIn: 2000},
			{Credits: 100, OneIn: 10000},
		},
	}
}

// bigTicket builds a Ticket for 6-winning × 4×6-grid key-number tests.
func keynumTicket4x6() Ticket {
	return Ticket{
		Slug:       "test-keynum-4x6",
		Name:       "Test KeyNum 4×6",
		Price:      10,
		Mechanic:   MechKeyNum,
		Cols:       4,
		Rows:       6,
		WinNumbers: 6,
		Prizes: PrizeTable{
			{Credits: 1, OneIn: 6},
			{Credits: 5, OneIn: 40},
			{Credits: 10, OneIn: 120},
			{Credits: 50, OneIn: 2000},
			{Credits: 100, OneIn: 10000},
			{Credits: 250, OneIn: 250000},
		},
	}
}

// buildKeynumCard is a helper that builds a keynumCard with a specific Outcome.
func buildKeynumCard(t *Ticket, win int, seed int64) *keynumCard {
	rng := rand.New(rand.NewSource(seed))
	out := Outcome{Win: win}
	c := keynumBuild(t, out, rng)
	return c.(*keynumCard)
}

// winNumSet returns the set of winning numbers for a card.
func winNumSet(c *keynumCard) map[int]bool {
	s := make(map[int]bool, len(c.winNums))
	for _, n := range c.winNums {
		s[n] = true
	}
	return s
}

// --- WINNING card tests -------------------------------------------------------

// TestKeynumWin_MatchedPrizesSumToOutWin checks that the matched cells' prizes
// sum to exactly out.Win on a winning card, across several seeds.
func TestKeynumWin_MatchedPrizesSumToOutWin(t *testing.T) {
	tk := keynumTicket3x3()
	for _, tc := range []struct {
		seed int64
		win  int
	}{
		{1, 5},
		{2, 10},
		{3, 25},
		{4, 50},
		{5, 1},
		{6, 100},
		{7, 2},
	} {
		c := buildKeynumCard(&tk, tc.win, tc.seed)
		ws := winNumSet(c)

		// Scratch all and verify Win().
		c.ScratchAll()
		got := c.Win()
		if got != tc.win {
			t.Errorf("seed=%d win=%d: Win()=%d, want %d", tc.seed, tc.win, got, tc.win)
		}

		// Verify the matched cells' prizes sum to out.Win.
		matchSum := 0
		for i, p := range c.grid.Panels {
			num := keynumParseNum(p.Reveal)
			if ws[num] {
				matchSum += c.prizes[i]
			}
		}
		if matchSum != tc.win {
			t.Errorf("seed=%d win=%d: matched-prize sum=%d, want %d", tc.seed, tc.win, matchSum, tc.win)
		}
	}
}

// TestKeynumWin_NoUnintendedMatch verifies that on a winning card, the ONLY
// your-numbers that collide with a winning number are the intentionally planted
// match cells. (I.e. we don't accidentally duplicate a winning number into a
// non-match cell.)
func TestKeynumWin_NoUnintendedMatch(t *testing.T) {
	tk := keynumTicket3x3()
	// For each seed, confirm that all cells with your-number ∈ winSet are
	// exactly the intended match cells.
	for seed := int64(0); seed < 20; seed++ {
		c := buildKeynumCard(&tk, 25, seed)
		ws := winNumSet(c)
		c.ScratchAll()

		// Count how many cells have a winning number.
		matchCount := 0
		matchPrizeSum := 0
		for i, p := range c.grid.Panels {
			num := keynumParseNum(p.Reveal)
			if ws[num] {
				matchCount++
				matchPrizeSum += c.prizes[i]
			}
		}

		// Must be at least 1 match.
		if matchCount == 0 {
			t.Errorf("seed=%d: no match cells on a winning card", seed)
		}
		// Prize sum must equal out.Win.
		if matchPrizeSum != 25 {
			t.Errorf("seed=%d: matchPrizeSum=%d, want 25", seed, matchPrizeSum)
		}
		// Win() must equal out.Win.
		if got := c.Win(); got != 25 {
			t.Errorf("seed=%d: Win()=%d, want 25", seed, got)
		}
	}
}

// TestKeynumWin_WinEqualsOutWin_AfterFullReveal verifies Win() == out.Win after
// ScratchAll on a large 4×6 grid with 6 winning numbers.
func TestKeynumWin_WinEqualsOutWin_AfterFullReveal(t *testing.T) {
	tk := keynumTicket4x6()
	for _, tc := range []struct {
		seed int64
		win  int
	}{
		{10, 10},
		{11, 50},
		{12, 100},
		{13, 1},
		{14, 5},
		{15, 250},
	} {
		c := buildKeynumCard(&tk, tc.win, tc.seed)
		c.ScratchAll()
		if got := c.Win(); got != tc.win {
			t.Errorf("4x6 seed=%d win=%d: Win()=%d, want %d", tc.seed, tc.win, got, tc.win)
		}
	}
}

// TestKeynumWin_ResolvedAfterScratchAll checks Resolved() is true only after
// every panel has been revealed.
func TestKeynumWin_ResolvedAfterScratchAll(t *testing.T) {
	tk := keynumTicket3x3()
	c := buildKeynumCard(&tk, 5, 42)
	if c.Resolved() {
		t.Fatal("Resolved() == true before any scratching")
	}
	c.ScratchAll()
	if !c.Resolved() {
		t.Fatal("Resolved() == false after ScratchAll")
	}
}

// --- LOSING card tests --------------------------------------------------------

// TestKeynumLoss_NoYourNumberInWinSet checks that on a losing card, no
// your-number equals any winning number.
func TestKeynumLoss_NoYourNumberInWinSet(t *testing.T) {
	tk := keynumTicket3x3()
	for seed := int64(0); seed < 30; seed++ {
		c := buildKeynumCard(&tk, 0, seed)
		ws := winNumSet(c)
		c.ScratchAll()

		for i, p := range c.grid.Panels {
			num := keynumParseNum(p.Reveal)
			if ws[num] {
				t.Errorf("loss seed=%d: cell %d has your-number %d which is in winSet", seed, i, num)
			}
		}
	}
}

// TestKeynumLoss_WinIsZero checks Win()==0 after full reveal on a losing card.
func TestKeynumLoss_WinIsZero(t *testing.T) {
	tk := keynumTicket3x3()
	for seed := int64(0); seed < 30; seed++ {
		c := buildKeynumCard(&tk, 0, seed)
		c.ScratchAll()
		if got := c.Win(); got != 0 {
			t.Errorf("loss seed=%d: Win()=%d, want 0", seed, got)
		}
	}
}

// TestKeynumLoss_WinIsZero_BigGrid tests the 4×6 grid for losing cards.
func TestKeynumLoss_WinIsZero_BigGrid(t *testing.T) {
	tk := keynumTicket4x6()
	for seed := int64(100); seed < 130; seed++ {
		c := buildKeynumCard(&tk, 0, seed)
		c.ScratchAll()
		if got := c.Win(); got != 0 {
			t.Errorf("4x6 loss seed=%d: Win()=%d, want 0", seed, got)
		}
	}
}

// TestKeynumLoss_NoYourNumberInWinSet_BigGrid tests the 4×6 grid.
func TestKeynumLoss_NoYourNumberInWinSet_BigGrid(t *testing.T) {
	tk := keynumTicket4x6()
	for seed := int64(200); seed < 220; seed++ {
		c := buildKeynumCard(&tk, 0, seed)
		ws := winNumSet(c)
		c.ScratchAll()

		for i, p := range c.grid.Panels {
			num := keynumParseNum(p.Reveal)
			if ws[num] {
				t.Errorf("4x6 loss seed=%d: cell %d has your-number %d in winSet", seed, i, num)
			}
		}
	}
}

// --- Card interface / mechanics tests ----------------------------------------

// TestKeynumCard_Title checks the Title() string format.
func TestKeynumCard_Title(t *testing.T) {
	tk := keynumTicket3x3()
	c := buildKeynumCard(&tk, 0, 1)
	title := c.Title()
	if title == "" {
		t.Fatal("Title() is empty")
	}
}

// TestKeynumCard_MoveAndScratch checks Move and incremental scratch via the Card
// interface delegates to the grid properly.
func TestKeynumCard_MoveAndScratch(t *testing.T) {
	tk := keynumTicket3x3()
	c := buildKeynumCard(&tk, 5, 7)

	// Move to the second cell and scratch it.
	c.Move(1, 0)
	if c.grid.Cur != 1 {
		t.Fatalf("after Move(1,0) Cur=%d, want 1", c.grid.Cur)
	}

	// Scratch until the panel is revealed (may need 1–3 rubs).
	for c.grid.Panels[c.grid.Cur].Hidden {
		c.Scratch()
	}
	if c.grid.Panels[1].Hidden {
		t.Fatal("panel 1 still hidden after scratching")
	}

	// Not yet resolved (other panels still hidden).
	if c.Resolved() {
		t.Fatal("Resolved() too early - only one panel scratched")
	}
}

// TestKeynumCard_WinNumbersCount verifies the right number of winning numbers
// are generated.
func TestKeynumCard_WinNumbersCount(t *testing.T) {
	for _, winCount := range []int{2, 3, 4, 6} {
		tk := Ticket{
			Slug:       "wc-test",
			Price:      1,
			Mechanic:   MechKeyNum,
			Cols:       3,
			Rows:       3,
			WinNumbers: winCount,
			Prizes: PrizeTable{
				{Credits: 5, OneIn: 4},
			},
		}
		c := buildKeynumCard(&tk, 0, 99)
		if len(c.winNums) != winCount {
			t.Errorf("WinNumbers=%d: got %d winNums", winCount, len(c.winNums))
		}
		// All winning numbers must be distinct.
		seen := map[int]bool{}
		for _, n := range c.winNums {
			if seen[n] {
				t.Errorf("duplicate winning number %d (WinNumbers=%d)", n, winCount)
			}
			seen[n] = true
		}
	}
}

// TestKeynumCard_WinAfterScratchAll verifies that Win() equals the drawn outcome
// after a full reveal, exercising the Prompt and Resolved paths.
func TestKeynumCard_WinAfterScratchAll(t *testing.T) {
	for _, tc := range []struct {
		seed int64
		win  int
	}{
		{300, 0},
		{301, 5},
		{302, 100},
		{303, 10},
	} {
		tk := keynumTicket3x3()
		c := buildKeynumCard(&tk, tc.win, tc.seed)

		if c.Resolved() {
			t.Errorf("seed=%d: Resolved() before scratch", tc.seed)
		}

		c.ScratchAll()

		if !c.Resolved() {
			t.Errorf("seed=%d: not Resolved() after ScratchAll", tc.seed)
		}
		if got := c.Win(); got != tc.win {
			t.Errorf("seed=%d win=%d: Win()=%d", tc.seed, tc.win, got)
		}
		// Prompt must be non-empty.
		if p := c.Prompt(); p == "" {
			t.Errorf("seed=%d: Prompt() empty", tc.seed)
		}
	}
}
