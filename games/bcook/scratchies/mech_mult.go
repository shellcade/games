package main

import (
	"fmt"
	"math/rand"
)

// mech_mult.go — Real multiplier engine (SPEC §5.3, AB-7).
//
// Two scratch panels: PRIZE (base amount) and MULTIPLIER (integer in [1,MaxMult]).
// Final win = base × mult. For a winning card the engine factors out.Win into the
// largest valid mult ≤ MaxMult that divides out.Win evenly (base ≥ 1).
// For a losing card the prize panel shows "—" and a random mult is displayed for
// flavour; Win() returns 0.

// multCard implements Card for the multiplier mechanic.
type multCard struct {
	ticket *Ticket
	grid   *Grid // 1-col × 2-row: panel 0 = PRIZE, panel 1 = MULTIPLIER
	base   int   // credits (0 on a loss)
	mult   int   // multiplier shown (always ≥ 1)
}

// multFactor finds the largest integer m in [1, maxMult] such that win%m == 0
// and win/m ≥ 1. Returns (base, mult) where win == base * mult.
func multFactor(win, maxMult int) (base, mult int) {
	for m := maxMult; m >= 1; m-- {
		if win%m == 0 {
			b := win / m
			if b >= 1 {
				return b, m
			}
		}
	}
	// Fallback: mult = 1, base = win (always valid since win ≥ 1 here).
	return win, 1
}

func init() {
	builders[MechMult] = multBuild
}

func multBuild(t *Ticket, out Outcome, rng *rand.Rand) Card {
	maxMult := t.MaxMult
	if maxMult < 1 {
		maxMult = 1
	}

	g := NewGrid(1, 2)
	g.seedDepths(rng)

	var base, mult int
	if out.Win > 0 {
		base, mult = multFactor(out.Win, maxMult)
	} else {
		// Loss: show a flavour multiplier (random in [1, maxMult]).
		base = 0
		mult = 1 + rng.Intn(maxMult)
	}

	// Prize panel (index 0).
	if base > 0 {
		g.Panels[0].Reveal = commaInt(base)
		g.Panels[0].Ink = stReveal
	} else {
		g.Panels[0].Reveal = " — "
		g.Panels[0].Ink = stDim
	}

	// Multiplier panel (index 1).
	g.Panels[1].Reveal = fmt.Sprintf("%d×", mult)
	g.Panels[1].Ink = stWin

	return &multCard{
		ticket: t,
		grid:   g,
		base:   base,
		mult:   mult,
	}
}

func (c *multCard) Title() string {
	return c.ticket.Name + " · $" + itoa(c.ticket.Price) + " · find a prize, then multiply it"
}

func (c *multCard) Prompt() string {
	if !c.Resolved() {
		return "scratch to reveal your prize and multiplier"
	}
	if c.base == 0 {
		return "no prize this time"
	}
	return fmt.Sprintf("%s × %d = %s",
		commaInt(c.base), c.mult, commaInt(c.base*c.mult))
}

func (c *multCard) Move(dx, dy int) { c.grid.Move(dx, dy) }
func (c *multCard) Scratch() bool   { return c.grid.Scratch() }
func (c *multCard) ScratchAll()     { c.grid.ScratchAll() }
func (c *multCard) Resolved() bool  { return c.grid.AllRevealed() }
func (c *multCard) Win() int {
	if !c.Resolved() {
		return 0
	}
	return c.base * c.mult
}

// multPanelInner is the interior width of each labelled box: 12 usable columns
// between the border characters.
const multPanelInner = 12

// multPanelOuter = inner + 2 border chars.
const multPanelOuter = multPanelInner + 2

// Render draws the two-panel multiplier card per AB-7.
//
//	YOUR PRIZE                  MULTIPLIER
//	┌────────────┐              ┌────────────┐
//	│            │              │            │
//	│    200     │      ×       │    10×     │
//	│            │              │            │
//	└────────────┘              └────────────┘
//
//	         200 × 10 = 2,000
//	    ✦ ✦ ✦  B I G  W I N  ✦ ✦ ✦
//	          WON 2,000 CREDITS
func (c *multCard) Render(f *Frame, top int) {
	const (
		leftEdge  = 4  // column for the left box's left border
		midGap    = 14 // columns between the two boxes (holds " × ")
		rightEdge = leftEdge + multPanelOuter + midGap
	)

	labelRow := top + 1
	boxTop := top + 2
	boxBot := boxTop + 4
	midRow := boxTop + 2

	// Labels.
	f.Text(labelRow, leftEdge+1, "YOUR PRIZE", stDim)
	f.Text(labelRow, rightEdge+1, "MULTIPLIER", stDim)

	// Box borders (highlight whichever panel is focused).
	prizeStyle := stDim
	multStyle := stDim
	if c.grid.Cur == 0 {
		prizeStyle = stCoin
	} else {
		multStyle = stCoin
	}
	box(f, boxTop, leftEdge, boxBot, leftEdge+multPanelOuter-1, prizeStyle)
	box(f, boxTop, rightEdge, boxBot, rightEdge+multPanelOuter-1, multStyle)

	// "×" operator in the gap.
	xCol := leftEdge + multPanelOuter + midGap/2 - 1
	f.Text(midRow, xCol, "×", stDim)

	// Panel interiors.
	multDrawPanelInner(f, boxTop, leftEdge, c.grid.Panels[0])
	multDrawPanelInner(f, boxTop, rightEdge, c.grid.Panels[1])

	// Prompt.
	promptRow := boxBot + 2
	f.Text(promptRow, leftEdge, c.Prompt(), stDim)

	// Win banner (only when resolved and won).
	if c.Resolved() && c.base > 0 {
		win := c.Win()
		isBig := win >= 500 && win >= 50*c.ticket.Price
		bannerRow := promptRow + 2
		winStyle := stWin
		if isBig {
			winStyle = stBig
			f.Text(bannerRow, leftEdge, "✦ ✦ ✦   B I G   W I N   ✦ ✦ ✦", winStyle)
			bannerRow++
		}
		f.Text(bannerRow, leftEdge, fmt.Sprintf("WON %s CREDITS", commaInt(win)), winStyle)
	}
}

// multDrawPanelInner renders the interior of one panel box (3 content rows
// between the border). boxTopRow is the top-border row, leftEdge is the left
// border column.
func multDrawPanelInner(f *Frame, boxTopRow, leftEdge int, p Panel) {
	inner := leftEdge + 1 // first interior column
	if p.Hidden {
		// Blank top/bottom interior rows; latex on the middle row.
		f.Text(boxTopRow+1, inner, spaces(multPanelInner), stLatex)
		latex := "    " + latexCells(p.Layers) + "    " // centre 4-wide latex in 12 cols
		f.Text(boxTopRow+2, inner, latex, stLatex)
		f.Text(boxTopRow+3, inner, spaces(multPanelInner), stLatex)
	} else {
		// Blank all rows then write the value centred on the middle row.
		f.Text(boxTopRow+1, inner, spaces(multPanelInner), stDim)
		f.Text(boxTopRow+3, inner, spaces(multPanelInner), stDim)
		// Centre the reveal in 12 cols: pad left = (12 - len) / 2.
		reveal := []rune(p.Reveal)
		rLen := len(reveal)
		pad := (multPanelInner - rLen) / 2
		if pad < 0 {
			pad = 0
		}
		f.Text(boxTopRow+2, inner, spaces(multPanelInner), stDim)
		f.Text(boxTopRow+2, inner+pad, string(reveal), p.Ink)
	}
}
