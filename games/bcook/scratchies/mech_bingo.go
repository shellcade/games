package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// mech_bingo.go — Quick Bingo engine (5×5 bingo card, line-match mechanic).
//
// Layout: a header row lists all called numbers (pre-revealed, always visible).
// A 5×5 grid hides the card numbers; each cell whose number is in the called
// set is a MATCH. Win when any full row, column, or main diagonal is entirely
// matched. Prize = out.Win.
//
// Winning card (out.Win > 0): exactly one full line is guaranteed all-match.
// Losing card (out.Win == 0): no full line is entirely called.

func init() {
	builders[MechBingo] = bingoBuild
}

// bingoLines enumerates all 12 line index-sets for a 5×5 bingo card:
// 5 rows, 5 columns, 2 diagonals.
func bingoLines(cols, rows int) [][]int {
	var lines [][]int
	// Rows
	for r := 0; r < rows; r++ {
		row := make([]int, cols)
		for c := 0; c < cols; c++ {
			row[c] = r*cols + c
		}
		lines = append(lines, row)
	}
	// Columns
	for c := 0; c < cols; c++ {
		col := make([]int, rows)
		for r := 0; r < rows; r++ {
			col[r] = r*cols + c
		}
		lines = append(lines, col)
	}
	// Main diagonal (top-left → bottom-right), only square grids
	if cols == rows {
		diag := make([]int, cols)
		for i := 0; i < cols; i++ {
			diag[i] = i*cols + i
		}
		lines = append(lines, diag)
		// Anti-diagonal (top-right → bottom-left)
		anti := make([]int, cols)
		for i := 0; i < cols; i++ {
			anti[i] = i*cols + (cols - 1 - i)
		}
		lines = append(lines, anti)
	}
	return lines
}

// bingoCard satisfies the Card interface for the Quick Bingo mechanic.
type bingoCard struct {
	t          *Ticket
	out        Outcome
	calledNums []int       // the pre-revealed "called" numbers (length == t.WinNumbers)
	calledSet  map[int]bool // O(1) membership test
	cardNums   []int       // card number at each grid cell index
	grid       *Grid
	view       int
	resolved   bool
}

// bingoBuild constructs a bingoCard from a drawn Outcome.
func bingoBuild(t *Ticket, out Outcome, rng *rand.Rand) Card {
	cols, rows := t.Cols, t.Rows
	if cols <= 0 {
		cols = 5
	}
	if rows <= 0 {
		rows = 5
	}
	total := cols * rows

	winCount := t.WinNumbers
	if winCount <= 0 {
		winCount = 10
	}

	// Build pool 1..75 (standard bingo range), shuffle it.
	const bingoMax = 75
	pool := make([]int, bingoMax)
	for i := range pool {
		pool[i] = i + 1
	}
	bingoShuffle(pool, rng)

	// Draw `winCount` distinct called numbers from the front of the pool.
	calledNums := make([]int, winCount)
	copy(calledNums, pool[:winCount])

	calledSet := make(map[int]bool, winCount)
	for _, n := range calledNums {
		calledSet[n] = true
	}

	// The rest of the pool is available for "not called" numbers.
	notCalled := make([]int, 0, bingoMax-winCount)
	for _, n := range pool[winCount:] {
		notCalled = append(notCalled, n)
	}

	// Enumerate all lines for this grid.
	lines := bingoLines(cols, rows)

	// cardNums holds the number for each grid cell (index by flat position).
	cardNums := make([]int, total)

	if out.Win > 0 {
		// WINNING card: plant exactly one guaranteed full line of called numbers,
		// fill other cells with distinct non-called numbers (best effort).
		//
		// 1. Pick a random winning line.
		lineIdx := rng.Intn(len(lines))
		winLine := lines[lineIdx]

		// 2. Need `cols` (for a row/diag) or `rows` (for a col) called numbers for
		//    the win line. We need exactly len(winLine) called numbers.
		lineLen := len(winLine)
		if lineLen > winCount {
			// Degenerate: not enough called numbers to fill the line. Fall back to
			// first available called numbers (already guaranteed >= lineLen by ticket
			// configuration; this guards against pathological tickets).
			lineLen = winCount
		}

		// Pick `lineLen` distinct called numbers for the win line cells.
		calledPerm := make([]int, len(calledNums))
		copy(calledPerm, calledNums)
		bingoShuffle(calledPerm, rng)
		winLineNums := calledPerm[:lineLen]

		// Place win line numbers.
		winLineCells := make(map[int]bool, lineLen)
		for i, cellIdx := range winLine {
			cardNums[cellIdx] = winLineNums[i]
			winLineCells[cellIdx] = true
		}

		// Fill remaining cells with distinct numbers. Prefer non-called numbers to
		// avoid creating accidental extra lines, but use any available distinct number.
		used := make(map[int]bool, total)
		for _, n := range winLineNums {
			used[n] = true
		}

		// Build a prioritised fill list: non-called first, then remaining called.
		// Shuffle each sub-list.
		notCalledCopy := make([]int, len(notCalled))
		copy(notCalledCopy, notCalled)
		bingoShuffle(notCalledCopy, rng)

		remainingCalled := make([]int, 0, winCount-lineLen)
		for _, n := range calledPerm[lineLen:] {
			remainingCalled = append(remainingCalled, n)
		}
		bingoShuffle(remainingCalled, rng)

		fillQueue := append(notCalledCopy, remainingCalled...)

		fillIdx := 0
		for i := 0; i < total; i++ {
			if winLineCells[i] {
				continue
			}
			// Pick next unused number from fill queue.
			for fillIdx < len(fillQueue) && used[fillQueue[fillIdx]] {
				fillIdx++
			}
			if fillIdx < len(fillQueue) {
				cardNums[i] = fillQueue[fillIdx]
				used[fillQueue[fillIdx]] = true
				fillIdx++
			} else {
				// Absolute fallback: find any number 1..75 not used.
				cardNums[i] = bingoFindUnused(used, bingoMax, rng)
				used[cardNums[i]] = true
			}
		}

	} else {
		// LOSING card: no full line may be entirely called.
		// Strategy: fill all cells with distinct numbers; then for each line,
		// if it would be entirely called, replace one of its cells with a non-called number.

		// Start by filling all cells with distinct numbers from the pool (shuffled).
		// Use numbers from the full shuffled pool (called + notCalled mixed).
		allNums := make([]int, bingoMax)
		copy(allNums, pool) // pool is already shuffled
		bingoShuffle(allNums, rng)

		used := make(map[int]bool, total)
		idx := 0
		for i := 0; i < total; i++ {
			for idx < len(allNums) && used[allNums[idx]] {
				idx++
			}
			if idx < len(allNums) {
				cardNums[i] = allNums[idx]
				used[allNums[idx]] = true
				idx++
			} else {
				cardNums[i] = bingoFindUnused(used, bingoMax, rng)
				used[cardNums[i]] = true
			}
		}

		// Now fix any line that is entirely called: replace one of its cells
		// with a non-called number (picking from notCalled or any uncalled available).
		notCalledAvail := make([]int, 0, bingoMax-winCount)
		for n := 1; n <= bingoMax; n++ {
			if !calledSet[n] {
				notCalledAvail = append(notCalledAvail, n)
			}
		}
		bingoShuffle(notCalledAvail, rng)

		// Re-check all lines.
		for _, line := range lines {
			allCalled := true
			for _, cellIdx := range line {
				if !calledSet[cardNums[cellIdx]] {
					allCalled = false
					break
				}
			}
			if !allCalled {
				continue
			}
			// This line is entirely called — replace one cell.
			// Pick the first cell whose replacement won't already be in use.
			replaced := false
			for _, cellIdx := range line {
				for _, nc := range notCalledAvail {
					if !used[nc] {
						used[cardNums[cellIdx]] = false // release old number
						cardNums[cellIdx] = nc
						used[nc] = true
						replaced = true
						break
					}
				}
				if replaced {
					break
				}
			}
			// If replacement failed (extremely unlikely with 75 numbers), leave as-is;
			// the caller's prize table makes this essentially impossible in practice.
		}
	}

	// Build the Grid.
	g := NewGrid(cols, rows)
	g.seedDepths(rng)
	for i := range g.Panels {
		g.Panels[i].Reveal = fmt.Sprintf("%02d", cardNums[i])
		g.Panels[i].Ink = stReveal
	}

	return &bingoCard{
		t:          t,
		out:        out,
		calledNums: calledNums,
		calledSet:  calledSet,
		cardNums:   cardNums,
		grid:       g,
		view:       viewportFor(rows),
	}
}

// bingoShuffle is a Fisher-Yates shuffle over a []int.
func bingoShuffle[T any](s []T, rng *rand.Rand) {
	for i := len(s) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		s[i], s[j] = s[j], s[i]
	}
}

// bingoFindUnused returns a number in 1..max not in used (linear scan).
func bingoFindUnused(used map[int]bool, max int, rng *rand.Rand) int {
	// Scan from a random offset to spread across the range.
	start := 1 + rng.Intn(max)
	for i := 0; i < max; i++ {
		n := 1 + (start-1+i)%max
		if !used[n] {
			return n
		}
	}
	return 1 // should never happen
}

// bingoIsMatch reports whether card cell i's number is in the called set.
func (c *bingoCard) bingoIsMatch(i int) bool {
	return c.calledSet[c.cardNums[i]]
}

// bingoHasCompletedLine reports whether any full line (row/col/diag) is all matched
// among the revealed cells.
func (c *bingoCard) bingoHasCompletedLine() bool {
	lines := bingoLines(c.t.Cols, c.t.Rows)
	for _, line := range lines {
		complete := true
		for _, cellIdx := range line {
			// Only count revealed cells for resolution during play.
			if c.grid.Panels[cellIdx].Hidden || !c.bingoIsMatch(cellIdx) {
				complete = false
				break
			}
		}
		if complete {
			return true
		}
	}
	return false
}

// --- Card interface -----------------------------------------------------------

func (c *bingoCard) Title() string {
	return c.t.Name + " · $" + itoa(c.t.Price) + " · mark a line"
}

func (c *bingoCard) Prompt() string {
	if c.Resolved() {
		w := c.Win()
		if w > 0 {
			return "✦ BINGO - WON " + itoa(w) + " CREDITS ✦"
		}
		return "no complete line - no win"
	}
	// Count how many called cells have been revealed.
	matchRevealed := 0
	for i := range c.grid.Panels {
		if !c.grid.Panels[i].Hidden && c.bingoIsMatch(i) {
			matchRevealed++
		}
	}
	if matchRevealed == 0 {
		return "scratch the card - complete a line to win"
	}
	return fmt.Sprintf("%d called numbers revealed - complete a line to win", matchRevealed)
}

func (c *bingoCard) Move(dx, dy int)          { c.grid.Move(dx, dy) }
func (c *bingoCard) Scratch() (revealed bool) {
	revealed = c.grid.Scratch()
	if revealed && c.bingoHasCompletedLine() {
		c.resolved = true
	}
	return
}
func (c *bingoCard) ScratchAll() {
	c.grid.ScratchAll()
	c.resolved = true
}
func (c *bingoCard) Resolved() bool { return c.resolved || c.grid.AllRevealed() }

func (c *bingoCard) Win() int {
	if !c.Resolved() {
		return 0
	}
	// Check all lines over the final (fully-revealed) state.
	lines := bingoLines(c.t.Cols, c.t.Rows)
	for _, line := range lines {
		allMatch := true
		for _, cellIdx := range line {
			if !c.bingoIsMatch(cellIdx) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return c.out.Win
		}
	}
	return 0
}

func (c *bingoCard) Render(f *Frame, top int) {
	cols := c.t.Cols

	// Header: "CALLED:" label then called numbers.
	f.Text(top, 3, "CALLED:", stTitle)
	calledStr := bingoFormatCalled(c.calledNums)
	f.Text(top, 12, calledStr, stDim)

	// Apply stMatch ink to revealed matching cells.
	for i := range c.grid.Panels {
		if !c.grid.Panels[i].Hidden && c.bingoIsMatch(i) {
			c.grid.Panels[i].Ink = stMatch
		} else if !c.grid.Panels[i].Hidden {
			c.grid.Panels[i].Ink = stReveal
		}
	}

	// Draw the bingo grid, offset by 2 rows for the header.
	gridTop := top + 2
	left := 3
	_ = cols
	drawGrid(f, c.grid, gridTop, left, c.view)

	// Prompt line below viewport.
	promptRow := gridTop + c.view*cellH + 1
	f.Text(promptRow, 3, c.Prompt(), stDim)
}

// bingoFormatCalled formats the called numbers as a compact space-separated list.
func bingoFormatCalled(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = fmt.Sprintf("%02d", n)
	}
	return strings.Join(parts, " ")
}
