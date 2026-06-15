package main

import (
	"fmt"
	"math/rand"
)

// mech_keynum.go — key-number-match engine (SPEC §5.2, AB-6).
//
// Layout: a row of WinNumbers winning numbers shown at the top of the card;
// a Cols×Rows grid of "your numbers", each paired with a prize amount. Any
// your-number that equals a winning number wins its paired prize; all such
// prizes sum to out.Win. A losing card has zero overlap.
//
// Winning numbers are pre-revealed (always visible). Only the your-number
// panels need scratching.

func init() {
	builders[MechKeyNum] = keynumBuild
}

// keynumCard satisfies the Card interface.
type keynumCard struct {
	t       *Ticket
	out     Outcome
	winNums []int  // the winning numbers (length == t.WinNumbers), always visible
	prizes  []int  // per-cell prize, parallel to grid.Panels
	grid    *Grid
	view    int
}

// keynumBuild constructs a keynumCard from a drawn Outcome.
func keynumBuild(t *Ticket, out Outcome, rng *rand.Rand) Card {
	cols, rows := t.Cols, t.Rows
	if cols <= 0 {
		cols = 3
	}
	if rows <= 0 {
		rows = 3
	}
	total := cols * rows
	winCount := t.WinNumbers
	if winCount <= 0 {
		winCount = 2
	}
	if winCount > 6 {
		winCount = 6
	}

	// Build a pool 1..99. We'll draw winning numbers and your-numbers from it.
	// Use a Fisher-Yates shuffle over the pool.
	pool := make([]int, 99)
	for i := range pool {
		pool[i] = i + 1
	}
	keynumShuffle(pool, rng)

	// Draw `winCount` distinct winning numbers from the pool.
	winNums := make([]int, winCount)
	copy(winNums, pool[:winCount])
	remaining := pool[winCount:]

	// winSet for O(1) lookup.
	winSet := make(map[int]bool, winCount)
	for _, n := range winNums {
		winSet[n] = true
	}

	// Build the prize pool for your-cells. We pull non-winning numbers from
	// remaining (already shuffled), so we can pick safely.
	// We need `total` your-numbers. Some will be matching (winning), most non-matching.

	// Determine match pattern based on out.Win.
	// Strategy: find which cells will match and their prizes.
	// Simple: pick prizes from the ticket's prize table entries (ascending).
	// For a winning card, plant one or two matches whose prizes sum to out.Win.
	// For a losing card, plant no matching numbers.

	prizes := make([]int, total)
	yourNums := make([]int, total)

	// Collect valid prize values from the table for assigning to cells.
	// All cells get a prize from the table (losers' prizes just aren't collected).
	prizePool := keynumPrizePool(t)
	for i := range prizes {
		prizes[i] = prizePool[rng.Intn(len(prizePool))]
	}

	if out.Win > 0 {
		// Winning card: plant matching your-numbers on cells whose prizes sum to out.Win.
		// Find a single cell with prize == out.Win, or two cells that sum to out.Win.
		matchIdx := keynumFindMatchCells(prizes, out.Win, total, rng)
		if matchIdx == nil {
			// Fallback: override cell 0's prize to out.Win and match it.
			prizes[0] = out.Win
			matchIdx = []int{0}
		}

		// Assign matching cells a winning number.
		// Pick one winning number to use for matching cells (use the first).
		matchNum := winNums[0]

		// Assign your-numbers: matching cells get matchNum, others get non-winning pool values.
		nonWinIdx := 0
		for i := 0; i < total; i++ {
			isMatch := false
			for _, mi := range matchIdx {
				if mi == i {
					isMatch = true
					break
				}
			}
			if isMatch {
				yourNums[i] = matchNum
			} else {
				// Pick a non-winning number that's not already used and not in winSet.
				for nonWinIdx < len(remaining) && winSet[remaining[nonWinIdx]] {
					nonWinIdx++
				}
				if nonWinIdx < len(remaining) {
					yourNums[i] = remaining[nonWinIdx]
					nonWinIdx++
				} else {
					// Fallback: generate a number not in winSet.
					yourNums[i] = keynumSafeNum(winSet, rng)
				}
			}
		}
	} else {
		// Losing card: all your-numbers must differ from all winning numbers.
		nonWinIdx := 0
		for i := 0; i < total; i++ {
			for nonWinIdx < len(remaining) && winSet[remaining[nonWinIdx]] {
				nonWinIdx++
			}
			if nonWinIdx < len(remaining) {
				yourNums[i] = remaining[nonWinIdx]
				nonWinIdx++
			} else {
				yourNums[i] = keynumSafeNum(winSet, rng)
			}
		}
	}

	// Build the Grid. Each panel's Reveal stores the your-number as "07" / "23".
	g := NewGrid(cols, rows)
	g.seedDepths(rng)
	for i := range g.Panels {
		g.Panels[i].Reveal = fmt.Sprintf("%02d", yourNums[i])
		g.Panels[i].Ink = stReveal
	}

	return &keynumCard{
		t:       t,
		out:     out,
		winNums: winNums,
		prizes:  prizes,
		grid:    g,
		view:    viewportFor(rows),
	}
}

// keynumFindMatchCells returns indices of cells whose prizes sum to win.
// Tries a single cell first, then a pair.
func keynumFindMatchCells(prizes []int, win, total int, rng *rand.Rand) []int {
	// Single match: look for a cell with prize == win.
	// Shuffle candidate indices to pick randomly among equally valid cells.
	indices := make([]int, total)
	for i := range indices {
		indices[i] = i
	}
	keynumShuffle(indices, rng)

	for _, i := range indices {
		if prizes[i] == win {
			return []int{i}
		}
	}

	// Two-cell split: find two cells i, j where prizes[i] + prizes[j] == win.
	for a := 0; a < len(indices); a++ {
		for b := a + 1; b < len(indices); b++ {
			i, j := indices[a], indices[b]
			if prizes[i]+prizes[j] == win {
				return []int{i, j}
			}
		}
	}
	return nil
}

// keynumPrizePool returns a de-duplicated list of prize values from the ticket's
// table, used to assign per-cell prizes. Always includes at least one entry.
func keynumPrizePool(t *Ticket) []int {
	seen := map[int]bool{}
	var out []int
	for _, row := range t.Prizes {
		if row.Credits > 0 && !seen[row.Credits] {
			seen[row.Credits] = true
			out = append(out, row.Credits)
		}
	}
	if len(out) == 0 {
		out = []int{1}
	}
	return out
}

// keynumSafeNum returns a number in 1..99 not in winSet.
func keynumSafeNum(winSet map[int]bool, rng *rand.Rand) int {
	for {
		n := 1 + rng.Intn(99)
		if !winSet[n] {
			return n
		}
	}
}

// keynumShuffle is a Fisher-Yates shuffle over a []int.
func keynumShuffle[T any](s []T, rng *rand.Rand) {
	for i := len(s) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		s[i], s[j] = s[j], s[i]
	}
}

// --- Card interface -----------------------------------------------------------

func (c *keynumCard) Title() string {
	return c.t.Name + " · $" + itoa(c.t.Price) + " · match the winning numbers"
}

func (c *keynumCard) Prompt() string {
	if !c.grid.AllRevealed() {
		return "scratch your numbers — matches pay"
	}
	return c.resolvedPrompt()
}

func (c *keynumCard) resolvedPrompt() string {
	// Tally revealed matches.
	var matchPrizes []int
	sum := 0
	for i, p := range c.grid.Panels {
		if !p.Hidden && c.isMatch(i) {
			matchPrizes = append(matchPrizes, c.prizes[i])
			sum += c.prizes[i]
		}
	}
	if len(matchPrizes) == 0 {
		return "no matching numbers"
	}
	if len(matchPrizes) == 1 {
		return fmt.Sprintf("one match — %s CREDITS", commaInt(sum))
	}
	// Two or more matches: show the sum breakdown.
	expr := ""
	for i, pr := range matchPrizes {
		if i > 0 {
			expr += " + "
		}
		expr += commaInt(pr)
	}
	return fmt.Sprintf("%d matches — %s = %s CREDITS", len(matchPrizes), expr, commaInt(sum))
}

func (c *keynumCard) Move(dx, dy int)          { c.grid.Move(dx, dy) }
func (c *keynumCard) Scratch() (revealed bool) { return c.grid.Scratch() }
func (c *keynumCard) ScratchAll()              { c.grid.ScratchAll() }
func (c *keynumCard) Resolved() bool           { return c.grid.AllRevealed() }

func (c *keynumCard) Win() int {
	if !c.grid.AllRevealed() {
		return 0
	}
	total := 0
	for i, p := range c.grid.Panels {
		if !p.Hidden && c.isMatch(i) {
			total += c.prizes[i]
		}
	}
	return total
}

// isMatch reports whether panel i's your-number is in the winning set.
func (c *keynumCard) isMatch(i int) bool {
	reveal := c.grid.Panels[i].Reveal
	n := keynumParseNum(reveal)
	for _, w := range c.winNums {
		if w == n {
			return true
		}
	}
	return false
}

// keynumParseNum parses a 2-digit string like "07" to 7.
func keynumParseNum(s string) int {
	n := 0
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int(ch-'0')
		}
	}
	return n
}

func (c *keynumCard) Render(f *Frame, top int) {
	// Header: WINNING NUMBERS with boxes for each number.
	f.Text(top, 3, "WINNING NUMBERS", stTitle)
	for i, w := range c.winNums {
		col := 20 + i*8
		box(f, top, col, top+2, col+cellW-1, stReveal)
		f.Text(top+1, col+1, centre4(fmt.Sprintf("%02d", w)), stReveal)
	}

	// Apply stMatch ink to revealed matching cells before drawGrid renders them.
	for i, p := range c.grid.Panels {
		if !p.Hidden && c.isMatch(i) {
			c.grid.Panels[i].Ink = stMatch
		}
	}

	// Your-numbers grid, four rows below top (leaving room for the 3-row header).
	gridTop := top + 4
	drawGrid(f, c.grid, gridTop, 6, c.view)

	// Prompt line: below the viewport.
	promptRow := gridTop + c.view*cellH + 1
	f.Text(promptRow, 3, c.Prompt(), stDim)
}
