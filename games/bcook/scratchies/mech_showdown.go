package main

import (
	"fmt"
	"math/rand"
)

// mech_showdown.go — beat-the-house engine (Showdown).
//
// Layout: t.Cols columns, 2 rows.
//   Row 0 = HOUSE (pre-revealed at build time)
//   Row 1 = YOU   (hidden; player scratches)
//
// Each column has a value in [2, 99] and a per-column prize.
// For each column where YOUR value > HOUSE value you win that column's prize.
// out.Win == sum of won-column prizes.
//
// Winning card: exactly one column is the winner — its prize == out.Win and
// your value > house value there; all other columns have your value ≤ house.
//
// Losing card: your value ≤ house value in EVERY column; Win() == 0.
//
// NO-SPOILER RULE: winner/loser column colouring (stMatch / stDim) is applied
// only when the card is fully resolved, not before.

// sdCard implements Card for the Showdown mechanic.
type sdCard struct {
	ticket      *Ticket
	out         Outcome
	grid        *Grid   // Cols × 2; row 0 = house (pre-revealed), row 1 = you (hidden)
	houseValues []int   // per-column house value [2..99]
	yourValues  []int   // per-column your value  [2..99]
	prizes      []int   // per-column prize (credits); 0 for non-winning columns
	winCol      int     // index of the single winning column; -1 on a loss
}

func init() {
	builders[MechShowdown] = sdBuild
}

// sdBuild constructs an sdCard for the given ticket and predetermined outcome.
func sdBuild(t *Ticket, out Outcome, rng *rand.Rand) Card {
	cols := t.Cols
	if cols < 1 {
		cols = 3
	}

	house := make([]int, cols)
	yours := make([]int, cols)
	prizes := make([]int, cols)
	winCol := -1

	if out.Win > 0 {
		// Pick one column to be the winner at random.
		winCol = rng.Intn(cols)
		prizes[winCol] = out.Win

		// Build values for every column.
		for c := 0; c < cols; c++ {
			hv := 2 + rng.Intn(98) // house: [2, 99]
			if c == winCol {
				// YOUR value must strictly beat the house.
				// house value in [2, 98] to guarantee your value ≤ 99.
				if hv > 98 {
					hv = 98
				}
				yv := hv + 1 + rng.Intn(99-hv) // your: [hv+1, 99]
				house[c] = hv
				yours[c] = yv
			} else {
				// YOUR value must NOT beat the house (your ≤ house).
				if hv < 2 {
					hv = 2
				}
				yv := 2 + rng.Intn(hv) // your: [2, hv] (hv inclusive via Intn(hv) gives [0,hv-1]+2, cap at hv)
				// Ensure yv <= hv.
				if yv > hv {
					yv = hv
				}
				house[c] = hv
				yours[c] = yv
			}
		}
	} else {
		// Losing card: your value ≤ house in every column.
		for c := 0; c < cols; c++ {
			hv := 2 + rng.Intn(98) // house: [2, 99]
			if hv < 2 {
				hv = 2
			}
			yv := 2 + rng.Intn(hv) // your: [2, hv]
			if yv > hv {
				yv = hv
			}
			house[c] = hv
			yours[c] = yv
		}
	}

	// Build the grid: Cols × 2.
	g := NewGrid(cols, 2)

	// Row 0 = HOUSE: pre-revealed.
	for c := 0; c < cols; c++ {
		idx := 0*cols + c
		g.Panels[idx].Hidden = false
		g.Panels[idx].Layers = 0
		g.Panels[idx].Reveal = itoa(house[c])
		g.Panels[idx].Ink = stReveal
	}

	// Row 1 = YOU: hidden; seed depths.
	for c := 0; c < cols; c++ {
		idx := 1*cols + c
		g.Panels[idx].Hidden = true
		g.Panels[idx].Reveal = itoa(yours[c])
		g.Panels[idx].Ink = stReveal
	}
	// Seed depths only for row 1 (the hidden row).
	for c := 0; c < cols; c++ {
		idx := 1*cols + c
		g.Panels[idx].Layers = 1 + rng.Intn(3)
	}

	// Cursor starts on the first cell of row 1 (the YOU row).
	g.Cur = 1 * cols

	return &sdCard{
		ticket:      t,
		out:         out,
		grid:        g,
		houseValues: house,
		yourValues:  yours,
		prizes:      prizes,
		winCol:      winCol,
	}
}

func (c *sdCard) Title() string {
	return c.ticket.Name + " · $" + itoa(c.ticket.Price) + " · beat the house"
}

func (c *sdCard) Prompt() string {
	if !c.Resolved() {
		return "scratch YOUR row - beat the house value to win"
	}
	if c.out.Win > 0 {
		return fmt.Sprintf("you won - column %d pays %s credits",
			c.winCol+1, commaInt(c.out.Win))
	}
	return "no win - the house held"
}

// Move navigates within the YOU row (row 1). Only horizontal movement is
// meaningful since there is always just one scratchable row; vertical movement
// is accepted but clamped so the cursor stays in row 1.
func (c *sdCard) Move(dx, dy int) {
	cols := c.grid.Cols
	// Determine current column from cursor.
	cur := c.grid.Cur
	col := cur % cols
	// Always stay in row 1.
	col += dx
	if col < 0 {
		col = 0
	}
	if col >= cols {
		col = cols - 1
	}
	c.grid.Cur = 1*cols + col
}

func (c *sdCard) Scratch() bool {
	// Only panels in row 1 are scratchable; row 0 is pre-revealed.
	cur := c.grid.Cur
	cols := c.grid.Cols
	if cur < cols {
		// Cursor somehow landed on row 0 — redirect to row 1 same column.
		c.grid.Cur = cols + cur%cols
	}
	return c.grid.Scratch()
}

func (c *sdCard) ScratchAll() {
	cols := c.grid.Cols
	// Only reveal row 1 panels.
	for col := 0; col < cols; col++ {
		idx := 1*cols + col
		c.grid.Panels[idx].Hidden = false
		c.grid.Panels[idx].Layers = 0
	}
}

// Resolved reports whether every panel in the YOU row (row 1) has been revealed.
func (c *sdCard) Resolved() bool {
	cols := c.grid.Cols
	for col := 0; col < cols; col++ {
		if c.grid.Panels[1*cols+col].Hidden {
			return false
		}
	}
	return true
}

// Win sums the prizes of columns where your value > house value.
// Returns 0 until the card is resolved.
func (c *sdCard) Win() int {
	if !c.Resolved() {
		return 0
	}
	total := 0
	for col := 0; col < c.grid.Cols; col++ {
		if c.yourValues[col] > c.houseValues[col] {
			total += c.prizes[col]
		}
	}
	return total
}

// Render draws the Showdown card body.
//
// Layout (per column, left-to-right):
//
//	PRIZE: $N       (above house row)
//	┌────┐          HOUSE row (pre-revealed)
//	│ HH │
//	└────┘
//	┌────┐          YOU row (hidden until scratched)
//	│▓▓▓▓│  (or value once revealed)
//	└────┘
//
// At resolution, winning columns get stMatch borders/ink; losing get stDim.
func (c *sdCard) Render(f *Frame, top int) {
	cols := c.grid.Cols
	resolved := c.Resolved()

	const left = 4 // leftmost column for the first cell

	prizeRow := top
	houseBoxTop := top + 1
	youBoxTop := top + 1 + cellH

	// Row labels.
	houseLabel := "HOUSE"
	youLabel := "YOU"
	houseLabelCol := left + cols*cellStepX + 2
	f.Text(houseBoxTop+1, houseLabelCol, houseLabel, stDim)
	f.Text(youBoxTop+1, houseLabelCol, youLabel, stDim)

	for col := 0; col < cols; col++ {
		cx := left + col*cellStepX

		// Per-column prize label above the HOUSE box.
		prizeStr := "$" + commaInt(c.prizes[col])
		if c.prizes[col] == 0 {
			prizeStr = "  -  "
		}
		f.Text(prizeRow, cx, centre4(prizeStr), stDim)

		// Determine styles at resolution.
		colStyle := stDim
		valStyle := stReveal
		if resolved {
			if c.yourValues[col] > c.houseValues[col] {
				colStyle = stMatch
				valStyle = stMatch
			} else {
				colStyle = stDim
				valStyle = stDim
			}
		}

		// HOUSE box (always revealed; row 0).
		hIdx := 0*cols + col
		hPanel := c.grid.Panels[hIdx]
		if resolved {
			hPanel.Ink = valStyle
		}
		drawCell(f, houseBoxTop, cx, hPanel, false)
		// Redraw the border with the resolved colour.
		if resolved {
			box(f, houseBoxTop, cx, houseBoxTop+2, cx+cellW-1, colStyle)
			f.Text(houseBoxTop+1, cx+1, centre4(hPanel.Reveal), valStyle)
		}

		// YOU box (row 1): hidden until scratched.
		yIdx := 1*cols + col
		yPanel := c.grid.Panels[yIdx]
		focused := c.grid.Cur == yIdx
		bst := stDim
		if focused && !resolved {
			bst = stCoin
		} else if resolved {
			bst = colStyle
		}
		box(f, youBoxTop, cx, youBoxTop+2, cx+cellW-1, bst)
		if yPanel.Hidden {
			f.Text(youBoxTop+1, cx+1, latexCells(yPanel.Layers), stLatex)
		} else {
			ink := valStyle
			if !resolved {
				ink = stReveal
			}
			f.Text(youBoxTop+1, cx+1, centre4(yPanel.Reveal), ink)
		}
	}

	// Prompt line.
	promptRow := youBoxTop + cellH + 1
	f.Text(promptRow, left, c.Prompt(), stDim)

	// Win line at resolution.
	if resolved && c.out.Win > 0 {
		winRow := promptRow + 1
		f.Text(winRow, left,
			fmt.Sprintf("WON %s CREDITS", commaInt(c.out.Win)), stWin)
	}
}
