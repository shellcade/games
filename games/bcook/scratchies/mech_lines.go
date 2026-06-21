package main

import (
	"fmt"
	"math/rand"
)

// mech_lines.go — Lucky Lines engine (SPEC §5.5).
//
// A Cols×Rows grid of cash amounts. Win by uncovering three EQUAL amounts in
// three consecutive cells along any row, column, or diagonal.
//
// Winning card  (out.Win > 0): plant out.Win in exactly one consecutive-3 run
//   on a random line; fill remaining cells with decoys that never form a
//   consecutive-3 run of equal amounts anywhere.
//
// Losing card (out.Win == 0): fill the grid so no consecutive-3 equal-amount
//   run exists anywhere.
//
// Resolution: auto-resolve when a full winning line of three is revealed, or
//   when all panels are revealed.
//
// NO-SPOILER rule: winning cells are coloured stMatch only at resolution,
//   inside Render — never at build time.

func init() {
	builders[MechLines] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return linesBuild(t, out, rng)
	}
}

// linesAmounts is the palette of cash values used for panel content.
// 21 distinct values so we can fill any grid up to 5×5 (25 cells) without
// exhausting per-value limits; the palette intentionally mirrors match3Amounts.
var linesAmounts = []int{
	1, 2, 3, 5, 7, 10, 15, 20, 25, 30,
	50, 75, 100, 150, 200, 250, 500, 750, 1000, 2000, 5000,
}

// linesFmt formats a credit amount to fit a 4-char cell: "$5", "$50", "$1k", "10k".
func linesFmt(n int) string {
	if n < 1000 {
		return fmt.Sprintf("$%d", n)
	}
	k := n / 1000
	if k < 10 {
		return fmt.Sprintf("$%dk", k)
	}
	return fmt.Sprintf("%dk", k)
}

// linesParseAmt converts a linesFmt label back to credits (used in tests).
func linesParseAmt(s string) int {
	if len(s) == 0 {
		return 0
	}
	t := s
	if len(t) > 0 && t[0] == '$' {
		t = t[1:]
	}
	if len(t) > 0 && t[len(t)-1] == 'k' {
		t = t[:len(t)-1]
		return linesAtoiSimple(t) * 1000
	}
	return linesAtoiSimple(t)
}

// linesAtoiSimple converts a decimal string to int; returns 0 on error.
func linesAtoiSimple(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// linesLine represents a consecutive-3 run: the three cell indices.
type linesLine [3]int

// linesAllLines returns all possible consecutive-3 runs (rows, columns,
// diagonals) for a cols×rows grid. Each entry is the three grid indices.
func linesAllLines(cols, rows int) []linesLine {
	var lines []linesLine

	// Rows: three consecutive horizontal cells.
	for r := 0; r < rows; r++ {
		for c := 0; c+2 < cols; c++ {
			lines = append(lines, linesLine{
				r*cols + c,
				r*cols + c + 1,
				r*cols + c + 2,
			})
		}
	}

	// Columns: three consecutive vertical cells.
	for c := 0; c < cols; c++ {
		for r := 0; r+2 < rows; r++ {
			lines = append(lines, linesLine{
				r*cols + c,
				(r+1)*cols + c,
				(r+2)*cols + c,
			})
		}
	}

	// Diagonals top-left to bottom-right.
	for r := 0; r+2 < rows; r++ {
		for c := 0; c+2 < cols; c++ {
			lines = append(lines, linesLine{
				r*cols + c,
				(r+1)*cols + c + 1,
				(r+2)*cols + c + 2,
			})
		}
	}

	// Diagonals top-right to bottom-left.
	for r := 0; r+2 < rows; r++ {
		for c := 2; c < cols; c++ {
			lines = append(lines, linesLine{
				r*cols + c,
				(r+1)*cols + c - 1,
				(r+2)*cols + c - 2,
			})
		}
	}

	return lines
}

// linesHasConsec3 checks whether any consecutive-3 run in allLines has the
// same label in all three positions of amounts[].
func linesHasConsec3(amounts []string, allLines []linesLine) bool {
	for _, ln := range allLines {
		if amounts[ln[0]] != "" &&
			amounts[ln[0]] == amounts[ln[1]] &&
			amounts[ln[1]] == amounts[ln[2]] {
			return true
		}
	}
	return false
}

// linesWouldCompleteRun reports whether placing label at position idx would
// create a consecutive-3 equal run (in partial or any triple in allLines).
func linesWouldCompleteRun(amounts []string, idx int, label string, allLines []linesLine) bool {
	for _, ln := range allLines {
		for k := 0; k < 3; k++ {
			if ln[k] == idx {
				// Temporarily place and check.
				a, b, c := ln[0], ln[1], ln[2]
				va := amounts[a]
				if a == idx {
					va = label
				}
				vb := amounts[b]
				if b == idx {
					vb = label
				}
				vc := amounts[c]
				if c == idx {
					vc = label
				}
				if va != "" && va == vb && vb == vc {
					return true
				}
			}
		}
	}
	return false
}

// linesBuild constructs a linesCard for ticket t with predetermined outcome out.
func linesBuild(t *Ticket, out Outcome, rng *rand.Rand) *linesCard {
	cols, rows := t.Cols, t.Rows
	if cols <= 0 {
		cols, rows = 3, 3
	}
	total := cols * rows

	g := NewGrid(cols, rows)
	g.seedDepths(rng)

	allLines := linesAllLines(cols, rows)
	amounts := make([]string, total)

	// Decoy pool: excludes the winning amount to keep it unique on the grid.
	decoyPool := linesAmounts
	if out.Win > 0 {
		decoyPool = linesDecoyPool(out.Win)
	}

	var winLineIdx int     // index into allLines of the planted winning run
	var winLabel string    // formatted winning amount

	if out.Win > 0 {
		winLabel = linesFmt(out.Win)

		// Pick a random line to plant the winning triple on.
		// Shuffle all line indices and pick the first one.
		lineIdxs := make([]int, len(allLines))
		for i := range lineIdxs {
			lineIdxs[i] = i
		}
		for i := len(lineIdxs) - 1; i > 0; i-- {
			j := rng.Intn(i + 1)
			lineIdxs[i], lineIdxs[j] = lineIdxs[j], lineIdxs[i]
		}
		winLineIdx = lineIdxs[0]
		wl := allLines[winLineIdx]

		// Plant the winning amount.
		amounts[wl[0]] = winLabel
		amounts[wl[1]] = winLabel
		amounts[wl[2]] = winLabel

		// Fill remaining cells with decoys that don't form any consecutive-3 run.
		// We fill in a random order so the constraint-checking is fair.
		remaining := make([]int, 0, total-3)
		winSet := map[int]bool{wl[0]: true, wl[1]: true, wl[2]: true}
		for i := 0; i < total; i++ {
			if !winSet[i] {
				remaining = append(remaining, i)
			}
		}
		// Shuffle remaining indices.
		for i := len(remaining) - 1; i > 0; i-- {
			j := rng.Intn(i + 1)
			remaining[i], remaining[j] = remaining[j], remaining[i]
		}

		for _, idx := range remaining {
			picked := linesPickSafe(amounts, idx, decoyPool, allLines, rng)
			amounts[idx] = picked
		}
	} else {
		// Losing card: fill all cells so no consecutive-3 run exists.
		// Fill in random order.
		order := make([]int, total)
		for i := range order {
			order[i] = i
		}
		for i := len(order) - 1; i > 0; i-- {
			j := rng.Intn(i + 1)
			order[i], order[j] = order[j], order[i]
		}
		for _, idx := range order {
			picked := linesPickSafe(amounts, idx, decoyPool, allLines, rng)
			amounts[idx] = picked
		}
	}

	// Assign panel labels. Ink is always stReveal at build time (no-spoiler rule).
	for i := range g.Panels {
		g.Panels[i].Reveal = amounts[i]
		g.Panels[i].Ink = stReveal
	}

	return &linesCard{
		grid:       g,
		win:        out.Win,
		winLabel:   winLabel,
		winLineIdx: winLineIdx,
		allLines:   allLines,
		view:       viewportFor(rows),
		title:      t.Name + " · $" + itoa(t.Price) + " · three in a line",
		resolved:   false,
	}
}

// linesDecoyPool returns the amount palette excluding the winning value.
func linesDecoyPool(exclude int) []int {
	pool := make([]int, 0, len(linesAmounts))
	for _, a := range linesAmounts {
		if a != exclude {
			pool = append(pool, a)
		}
	}
	return pool
}

// linesPickSafe picks a random amount from pool that won't form a consecutive-3
// run when placed at idx. Falls back to any available amount if all are
// constrained (should not happen with normal grid sizes and a 21-value palette).
func linesPickSafe(amounts []string, idx int, pool []int, allLines []linesLine, rng *rand.Rand) string {
	// Shuffle pool order for randomness.
	perm := make([]int, len(pool))
	for i := range perm {
		perm[i] = i
	}
	for i := len(perm) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		perm[i], perm[j] = perm[j], perm[i]
	}
	for _, pi := range perm {
		label := linesFmt(pool[pi])
		if !linesWouldCompleteRun(amounts, idx, label, allLines) {
			return label
		}
	}
	// Fallback: use any label (shouldn't happen on well-sized grids).
	return linesFmt(pool[rng.Intn(len(pool))])
}

// linesCard is a scratch card for the Lucky Lines mechanic.
type linesCard struct {
	grid       *Grid
	win        int    // predetermined prize (0 = loss)
	winLabel   string // formatted winning amount label (empty on loss)
	winLineIdx int    // index into allLines of the planted triple (valid when win>0)
	allLines   []linesLine
	view       int
	title      string
	resolved   bool
}

// -- Card interface --

func (c *linesCard) Title() string { return c.title }

func (c *linesCard) Move(dx, dy int) { c.grid.Move(dx, dy) }

func (c *linesCard) Scratch() (revealed bool) {
	revealed = c.grid.Scratch()
	if revealed {
		// Check whether we've just completed a winning consecutive-3 line.
		if c.win > 0 && !c.resolved {
			wl := c.allLines[c.winLineIdx]
			if c.grid.Revealed(wl[0]) && c.grid.Revealed(wl[1]) && c.grid.Revealed(wl[2]) {
				c.resolved = true
			}
		}
	}
	return
}

func (c *linesCard) ScratchAll() {
	c.grid.ScratchAll()
	c.resolved = true
}

func (c *linesCard) Resolved() bool {
	return c.resolved || c.grid.AllRevealed()
}

func (c *linesCard) Win() int {
	if !c.Resolved() {
		return 0
	}
	return c.win
}

func (c *linesCard) Prompt() string {
	if c.Resolved() {
		if c.win > 0 {
			return "✦ three " + c.winLabel + " in a line - WON " + itoa(c.win) + " CREDITS ✦"
		}
		return "no line of three - no win"
	}
	// Count how many of the winning triple have been revealed (for winning card).
	if c.win > 0 {
		wl := c.allLines[c.winLineIdx]
		seen := 0
		for _, idx := range wl {
			if c.grid.Revealed(idx) {
				seen++
			}
		}
		if seen == 2 {
			return "two " + c.winLabel + " in a line - one more wins!"
		}
		if seen == 1 {
			return "one " + c.winLabel + " found - keep scratching"
		}
	}
	return "scratch the grid - three in a line wins"
}

func (c *linesCard) Render(f *Frame, top int) {
	// Apply stMatch to all three winning cells only after the card resolves.
	// This is the NO-SPOILER rule: ink must stay stReveal until resolution.
	if c.Resolved() && c.win > 0 {
		wl := c.allLines[c.winLineIdx]
		for _, idx := range wl {
			c.grid.Panels[idx].Ink = stMatch
		}
	}
	drawGrid(f, c.grid, top, 10, c.view)
	f.Text(top+c.view*cellH+1, 3, c.Prompt(), stDim)
}
