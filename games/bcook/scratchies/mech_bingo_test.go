package main

import (
	"math/rand"
	"testing"
)

// bingoTicket5x5 returns a 5×5 bingo ticket for testing.
func bingoTicket5x5(winNumbers int) Ticket {
	return Ticket{
		Slug:       "quick-bingo-test",
		Name:       "Quick Bingo",
		Price:      2,
		Mechanic:   MechBingo,
		Cols:       5,
		Rows:       5,
		WinNumbers: winNumbers,
		Prizes: PrizeTable{
			{Credits: 2, OneIn: 7},
			{Credits: 4, OneIn: 10},
			{Credits: 10, OneIn: 28},
			{Credits: 20, OneIn: 80},
			{Credits: 40, OneIn: 500},
			{Credits: 100, OneIn: 2500},
			{Credits: 200, OneIn: 12000},
		},
	}
}

// buildBingoCard builds a bingoCard directly using a seeded rng and a fixed Outcome.
func buildBingoCard(t *Ticket, win int, seed int64) *bingoCard {
	rng := rand.New(rand.NewSource(seed))
	out := Outcome{Win: win}
	c := bingoBuild(t, out, rng)
	return c.(*bingoCard)
}

// bingoAllLines returns all 12 lines of a 5×5 bingo card (5 rows + 5 cols + 2 diags).
func bingoAllLines(cols, rows int) [][]int {
	return bingoLines(cols, rows)
}

// bingoCountFullMatchLines returns the number of lines where every cell is in the called set.
func bingoCountFullMatchLines(c *bingoCard) int {
	count := 0
	lines := bingoAllLines(c.t.Cols, c.t.Rows)
	for _, line := range lines {
		allMatch := true
		for _, cellIdx := range line {
			if !c.calledSet[c.cardNums[cellIdx]] {
				allMatch = false
				break
			}
		}
		if allMatch {
			count++
		}
	}
	return count
}

// --- WINNING card tests -------------------------------------------------------

// TestBingoWin_AtLeastOneFullLine checks that a winning card has at least one full
// line (row, col, or diagonal) whose 5 numbers are all in the called set.
func TestBingoWin_AtLeastOneFullLine(t *testing.T) {
	for _, winNums := range []int{10, 12, 15, 20} {
		tk := bingoTicket5x5(winNums)
		for _, win := range []int{2, 10, 40, 100} {
			for seed := int64(0); seed < 15; seed++ {
				c := buildBingoCard(&tk, win, seed)
				fullLines := bingoCountFullMatchLines(c)
				if fullLines == 0 {
					t.Errorf("WinNumbers=%d win=%d seed=%d: no full matched line on winning card",
						winNums, win, seed)
				}
			}
		}
	}
}

// TestBingoWin_RevealAllGivesCorrectWin checks that after ScratchAll, Win() == out.Win
// on a winning card.
func TestBingoWin_RevealAllGivesCorrectWin(t *testing.T) {
	for _, winNums := range []int{10, 12} {
		tk := bingoTicket5x5(winNums)
		for _, win := range []int{2, 4, 10, 20, 40, 100, 200} {
			for seed := int64(0); seed < 20; seed++ {
				c := buildBingoCard(&tk, win, seed)
				c.ScratchAll()
				if !c.Resolved() {
					t.Errorf("WinNumbers=%d win=%d seed=%d: not Resolved() after ScratchAll",
						winNums, win, seed)
					continue
				}
				got := c.Win()
				if got != win {
					t.Errorf("WinNumbers=%d win=%d seed=%d: Win()=%d, want %d",
						winNums, win, seed, got, win)
				}
			}
		}
	}
}

// TestBingoWin_CalledNumbersAreDistinct checks that all called numbers are distinct
// and in range 1..75 on a winning card.
func TestBingoWin_CalledNumbersAreDistinct(t *testing.T) {
	for _, winNums := range []int{10, 12, 20} {
		tk := bingoTicket5x5(winNums)
		for seed := int64(0); seed < 15; seed++ {
			c := buildBingoCard(&tk, 10, seed)
			if len(c.calledNums) != winNums {
				t.Errorf("WinNumbers=%d seed=%d: got %d called numbers, want %d",
					winNums, seed, len(c.calledNums), winNums)
			}
			seen := make(map[int]bool)
			for _, n := range c.calledNums {
				if n < 1 || n > 75 {
					t.Errorf("WinNumbers=%d seed=%d: called number %d out of range 1..75",
						winNums, seed, n)
				}
				if seen[n] {
					t.Errorf("WinNumbers=%d seed=%d: duplicate called number %d",
						winNums, seed, n)
				}
				seen[n] = true
			}
		}
	}
}

// TestBingoWin_CardNumbersAreDistinct checks that all card cell numbers are
// distinct on a winning card.
func TestBingoWin_CardNumbersAreDistinct(t *testing.T) {
	tk := bingoTicket5x5(12)
	for seed := int64(0); seed < 25; seed++ {
		c := buildBingoCard(&tk, 10, seed)
		seen := make(map[int]bool)
		for i, n := range c.cardNums {
			if seen[n] {
				t.Errorf("seed=%d: duplicate card number %d at cell %d", seed, n, i)
			}
			seen[n] = true
		}
	}
}

// TestBingoWin_WinLineNumbersInCalledSet checks that the guaranteed winning line's
// numbers are all in the called set on a winning card.
func TestBingoWin_WinLineNumbersInCalledSet(t *testing.T) {
	tk := bingoTicket5x5(10)
	for seed := int64(0); seed < 20; seed++ {
		c := buildBingoCard(&tk, 20, seed)
		// Verify at least one complete line exists (all 5 numbers in called set).
		lines := bingoAllLines(c.t.Cols, c.t.Rows)
		found := false
		for _, line := range lines {
			allInCalled := true
			for _, cellIdx := range line {
				if !c.calledSet[c.cardNums[cellIdx]] {
					allInCalled = false
					break
				}
			}
			if allInCalled {
				found = true
				// Verify all 5 numbers in this line are indeed in calledNums.
				for _, cellIdx := range line {
					n := c.cardNums[cellIdx]
					if !c.calledSet[n] {
						t.Errorf("seed=%d: line cell number %d not in called set", seed, n)
					}
				}
				break
			}
		}
		if !found {
			t.Errorf("seed=%d: winning card has no complete matched line", seed)
		}
	}
}

// TestBingoWin_ResolvedAfterScratchAll checks Resolved() is true after ScratchAll.
func TestBingoWin_ResolvedAfterScratchAll(t *testing.T) {
	tk := bingoTicket5x5(10)
	c := buildBingoCard(&tk, 10, 42)
	if c.Resolved() {
		t.Fatal("Resolved() == true before any scratching")
	}
	c.ScratchAll()
	if !c.Resolved() {
		t.Fatal("Resolved() == false after ScratchAll")
	}
}

// TestBingoWin_TitleAndPromptNonEmpty checks that Title() and Prompt() are
// non-empty before and after resolution.
func TestBingoWin_TitleAndPromptNonEmpty(t *testing.T) {
	tk := bingoTicket5x5(10)
	c := buildBingoCard(&tk, 10, 7)
	if c.Title() == "" {
		t.Fatal("Title() is empty before scratch")
	}
	if c.Prompt() == "" {
		t.Fatal("Prompt() is empty before scratch")
	}
	c.ScratchAll()
	if c.Prompt() == "" {
		t.Fatal("Prompt() is empty after resolution")
	}
}

// --- LOSING card tests --------------------------------------------------------

// TestBingoLoss_NoFullLineAllCalled checks that a losing card has NO full line
// entirely in the called set.
func TestBingoLoss_NoFullLineAllCalled(t *testing.T) {
	for _, winNums := range []int{10, 12, 15, 20} {
		tk := bingoTicket5x5(winNums)
		for seed := int64(0); seed < 30; seed++ {
			c := buildBingoCard(&tk, 0, seed)
			fullLines := bingoCountFullMatchLines(c)
			if fullLines > 0 {
				t.Errorf("WinNumbers=%d seed=%d: losing card has %d full matched line(s)",
					winNums, seed, fullLines)
			}
		}
	}
}

// TestBingoLoss_WinIsZero checks Win()==0 after full reveal on a losing card.
func TestBingoLoss_WinIsZero(t *testing.T) {
	for _, winNums := range []int{10, 12} {
		tk := bingoTicket5x5(winNums)
		for seed := int64(0); seed < 30; seed++ {
			c := buildBingoCard(&tk, 0, seed)
			c.ScratchAll()
			if got := c.Win(); got != 0 {
				t.Errorf("WinNumbers=%d seed=%d: Win()=%d, want 0", winNums, seed, got)
			}
		}
	}
}

// TestBingoLoss_CardNumbersAreDistinct checks card cells are distinct on losing cards.
func TestBingoLoss_CardNumbersAreDistinct(t *testing.T) {
	tk := bingoTicket5x5(12)
	for seed := int64(0); seed < 25; seed++ {
		c := buildBingoCard(&tk, 0, seed)
		seen := make(map[int]bool)
		for i, n := range c.cardNums {
			if seen[n] {
				t.Errorf("seed=%d: duplicate card number %d at cell %d", seed, n, i)
			}
			seen[n] = true
		}
	}
}

// TestBingoLoss_WinIsZero_HighCalledCount stress-tests losing cards when many
// numbers are called (harder to avoid accidental lines).
func TestBingoLoss_WinIsZero_HighCalledCount(t *testing.T) {
	tk := bingoTicket5x5(20)
	for seed := int64(50); seed < 80; seed++ {
		c := buildBingoCard(&tk, 0, seed)
		c.ScratchAll()
		if got := c.Win(); got != 0 {
			t.Errorf("WinNumbers=20 seed=%d: Win()=%d, want 0", seed, got)
		}
		fullLines := bingoCountFullMatchLines(c)
		if fullLines > 0 {
			t.Errorf("WinNumbers=20 seed=%d: losing card has %d full line(s)", seed, fullLines)
		}
	}
}

// --- Card interface tests -----------------------------------------------------

// TestBingoCard_MoveAndScratch checks Move and incremental scratch via the Card interface.
func TestBingoCard_MoveAndScratch(t *testing.T) {
	tk := bingoTicket5x5(10)
	c := buildBingoCard(&tk, 10, 5)

	c.Move(1, 0)
	if c.grid.Cur != 1 {
		t.Fatalf("after Move(1,0) Cur=%d, want 1", c.grid.Cur)
	}

	// Scratch until the panel is revealed.
	for c.grid.Panels[c.grid.Cur].Hidden {
		c.Scratch()
	}
	if c.grid.Panels[1].Hidden {
		t.Fatal("panel 1 still hidden after scratching")
	}
}

// TestBingoCard_WinBeforeResolved checks Win() == 0 before resolution.
func TestBingoCard_WinBeforeResolved(t *testing.T) {
	tk := bingoTicket5x5(10)
	c := buildBingoCard(&tk, 10, 3)
	// Fresh card — not resolved yet.
	if c.Resolved() {
		t.Fatal("card resolved before any scratching")
	}
	if c.Win() != 0 {
		t.Fatalf("Win()=%d before resolved, want 0", c.Win())
	}
}

// TestBingoCard_AutoResolveOnLine checks that the card auto-resolves when a
// complete matched line is revealed incrementally (without ScratchAll).
func TestBingoCard_AutoResolveOnLine(t *testing.T) {
	// Build a winning card and reveal all cells of the guaranteed line one by one.
	tk := bingoTicket5x5(10)
	for seed := int64(0); seed < 10; seed++ {
		c := buildBingoCard(&tk, 10, seed)

		// Find a complete matched line.
		lines := bingoAllLines(c.t.Cols, c.t.Rows)
		var winLine []int
		for _, line := range lines {
			allMatch := true
			for _, cellIdx := range line {
				if !c.calledSet[c.cardNums[cellIdx]] {
					allMatch = false
					break
				}
			}
			if allMatch {
				winLine = line
				break
			}
		}
		if winLine == nil {
			t.Errorf("seed=%d: no complete matched line on winning card", seed)
			continue
		}

		// Reveal all cells of the winning line one by one.
		for _, cellIdx := range winLine {
			c.grid.Cur = cellIdx
			for c.grid.Panels[cellIdx].Hidden {
				c.Scratch()
			}
			if c.Resolved() {
				break
			}
		}
		// Card must now be resolved (line complete).
		if !c.Resolved() {
			t.Errorf("seed=%d: card not resolved after revealing a complete matched line", seed)
		}
		if got := c.Win(); got != 10 {
			t.Errorf("seed=%d: Win()=%d, want 10 after line completion", seed, got)
		}
	}
}

// TestBingoCard_LinesCount checks that bingoLines returns 12 lines for a 5×5 grid.
func TestBingoCard_LinesCount(t *testing.T) {
	lines := bingoLines(5, 5)
	if len(lines) != 12 {
		t.Fatalf("bingoLines(5,5) = %d lines, want 12 (5 rows + 5 cols + 2 diags)", len(lines))
	}
}

// TestBingoCard_LinesContent checks rows, columns, and diagonals are correct.
func TestBingoCard_LinesContent(t *testing.T) {
	lines := bingoLines(5, 5)

	// Row 0: cells 0..4
	row0 := lines[0]
	for i, got := range row0 {
		if got != i {
			t.Errorf("row0[%d]=%d, want %d", i, got, i)
		}
	}
	// Row 4: cells 20..24
	row4 := lines[4]
	for i, got := range row4 {
		if got != 20+i {
			t.Errorf("row4[%d]=%d, want %d", i, got, 20+i)
		}
	}
	// Col 0: cells 0,5,10,15,20
	col0 := lines[5]
	expected := []int{0, 5, 10, 15, 20}
	for i, got := range col0 {
		if got != expected[i] {
			t.Errorf("col0[%d]=%d, want %d", i, got, expected[i])
		}
	}
	// Main diagonal: cells 0,6,12,18,24
	diag := lines[10]
	expectedDiag := []int{0, 6, 12, 18, 24}
	for i, got := range diag {
		if got != expectedDiag[i] {
			t.Errorf("diag[%d]=%d, want %d", i, got, expectedDiag[i])
		}
	}
	// Anti-diagonal: cells 4,8,12,16,20
	anti := lines[11]
	expectedAnti := []int{4, 8, 12, 16, 20}
	for i, got := range anti {
		if got != expectedAnti[i] {
			t.Errorf("anti[%d]=%d, want %d", i, got, expectedAnti[i])
		}
	}
}

// TestBingoCard_InitRegistered verifies that the init() function registered
// a builder for MechBingo.
func TestBingoCard_InitRegistered(t *testing.T) {
	if builders[MechBingo] == nil {
		t.Fatal("no builder registered for MechBingo after init()")
	}
}
