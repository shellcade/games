package main

import (
	"math/rand"
	"strings"
)

// mech_find.go — real find-the-symbol engine (SPEC §5.4, AB-8).
//
// A t.Cols × t.Rows grid of panels hides symbols. Find THREE of the target
// symbol (t.Symbol) to win the prize. Some tickets (t.HasBust) hide a BUST
// panel that ends a losing card with no win.

// findDecoys is the fixed set of 4-char-safe non-target tokens used as filler.
var findDecoys = []string{"fish", "star", "coin", "bell", "frog", "crab"}

// findCard is the Card implementation for MechFind.
type findCard struct {
	ticket      *Ticket
	out         Outcome
	grid        *Grid
	view        int
	targetCount int // how many target panels have been revealed so far
	bustHit     bool
	resolved    bool
	won         int
}

// newFindCard constructs a findCard for the given ticket and predetermined outcome.
func newFindCard(t *Ticket, out Outcome, rng *rand.Rand) *findCard {
	cols, rows := t.Cols, t.Rows
	if cols <= 0 {
		cols, rows = 3, 3
	}
	n := cols * rows

	g := NewGrid(cols, rows)

	// Build a shuffled placement slice.
	// Slot 0..n-1; we assign roles then shuffle.
	type slot struct {
		reveal string
		ink    Style
		bust   bool
	}
	slots := make([]slot, n)

	if out.Win > 0 {
		// WINNING card: exactly 3 target panels, no BUST.
		slots[0] = slot{reveal: t.Symbol, ink: stMatch}
		slots[1] = slot{reveal: t.Symbol, ink: stMatch}
		slots[2] = slot{reveal: t.Symbol, ink: stMatch}
		// Fill the rest with decoys.
		di := 0
		for i := 3; i < n; i++ {
			slots[i] = slot{reveal: findDecoys[di%len(findDecoys)], ink: stReveal}
			di++
		}
	} else {
		// LOSING card: at most 2 target symbols.
		// Place 2 targets (more tension than 0 or 1), unless the grid is very
		// small (≤3 panels), in which case use 1.
		targets := 2
		if n <= 3 {
			targets = 1
		}
		// If HasBust, reserve 1 slot for BUST.
		bustSlots := 0
		if t.HasBust {
			bustSlots = 1
		}
		// Fill: targets targets, bustSlots busts, rest decoys.
		for i := 0; i < targets; i++ {
			slots[i] = slot{reveal: t.Symbol, ink: stReveal}
		}
		if t.HasBust {
			slots[targets] = slot{reveal: "BUST", ink: stBust, bust: true}
		}
		di := 0
		for i := targets + bustSlots; i < n; i++ {
			slots[i] = slot{reveal: findDecoys[di%len(findDecoys)], ink: stReveal}
			di++
		}
	}

	// Shuffle slots.
	rng.Shuffle(n, func(i, j int) { slots[i], slots[j] = slots[j], slots[i] })

	// Apply to grid panels.
	for i, s := range slots {
		g.Panels[i].Reveal = s.reveal
		g.Panels[i].Ink = s.ink
		g.Panels[i].Bust = s.bust
	}
	g.seedDepths(rng)

	return &findCard{
		ticket: t,
		out:    out,
		grid:   g,
		view:   viewportFor(rows),
	}
}

func (c *findCard) Title() string {
	sym := strings.ToLower(c.ticket.Symbol)
	title := c.ticket.Name + " · $" + itoa(c.ticket.Price) + " · find three " + sym
	if c.ticket.HasBust {
		title += " — mind the BUST!"
	}
	return title
}

func (c *findCard) Prompt() string {
	if c.resolved {
		if c.bustHit {
			return "BUST — no win"
		}
		if c.won > 0 {
			out := "WON " + itoa(c.won)
			if c.ticket.HasBust {
				out += " — dodged the BUST!"
			}
			return out
		}
		return "no win"
	}
	sym := strings.ToUpper(c.ticket.Symbol)
	switch c.targetCount {
	case 0:
		return "find three " + sym + " to win"
	case 1:
		return "one " + sym + " found — two more pays " + itoa(c.out.Win)
	case 2:
		return "two " + sym + " found — one more pays " + itoa(c.out.Win)
	default:
		return "find three " + sym + " to win"
	}
}

func (c *findCard) Move(dx, dy int) { c.grid.Move(dx, dy) }

func (c *findCard) Scratch() bool {
	if c.resolved {
		return false
	}
	revealed := c.grid.Scratch()
	if revealed {
		c.onReveal(c.grid.Cur)
	}
	return revealed
}

func (c *findCard) ScratchAll() {
	if c.resolved {
		return
	}
	// Reveal panels one at a time so auto-resolution logic triggers correctly.
	for i := range c.grid.Panels {
		if c.grid.Panels[i].Hidden {
			c.grid.Panels[i].Hidden = false
			c.grid.Panels[i].Layers = 0
			c.onReveal(i)
			if c.resolved {
				// Reveal the rest silently (cosmetic).
				c.grid.ScratchAll()
				return
			}
		}
	}
	// All panels revealed; resolve by final tally.
	if !c.resolved {
		c.finalize()
	}
}

func (c *findCard) Resolved() bool { return c.resolved }
func (c *findCard) Win() int       { return c.won }

func (c *findCard) Render(f *Frame, top int) {
	sym := strings.ToUpper(c.ticket.Symbol)
	header := "FIND 3  " + sym + "  TO WIN"
	f.Text(top, 8, header, stDim)
	drawGrid(f, c.grid, top+2, 8, c.view)
	promptRow := top + 2 + c.view*cellH
	f.Text(promptRow, 3, c.Prompt(), stDim)
}

// onReveal handles the logic triggered when panel idx is revealed.
func (c *findCard) onReveal(idx int) {
	p := &c.grid.Panels[idx]
	if p.Reveal == c.ticket.Symbol {
		c.targetCount++
		p.Ink = stMatch
		if c.targetCount >= 3 {
			// Three targets found → win.
			c.resolved = true
			c.won = c.out.Win
			c.grid.ScratchAll()
		}
	} else if p.Bust {
		// BUST hit on a losing card → immediate loss.
		c.bustHit = true
		c.resolved = true
		c.won = 0
		c.grid.ScratchAll()
	}
}

// finalize resolves the card after all panels are revealed (no earlier trigger).
func (c *findCard) finalize() {
	c.resolved = true
	if c.targetCount >= 3 {
		c.won = c.out.Win
	} else {
		c.won = 0
	}
}

func init() {
	builders[MechFind] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return newFindCard(t, out, rng)
	}
}
