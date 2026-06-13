package main

import "math/rand"

// Panel is one scratch-off cell. Hidden until its latex is fully worn through
// (Layers reaches 0). Reveal is what shows once open (an amount, number, or
// symbol label); Ink styles it; Bust marks a find-symbol kill panel.
type Panel struct {
	Reveal string
	Ink    Style
	Layers int // rubs remaining (1–3); 0 once revealed
	Hidden bool
	Bust   bool
}

// Grid is the shared scratch surface used by the grid mechanics (match-3,
// key-number, find-symbol). It owns the coin cursor (Cur) and the panels; the
// scrolling viewport is computed at render time so the cursor stays visible.
type Grid struct {
	Cols   int
	Rows   int
	Panels []Panel
	Cur    int // focused panel index
}

// NewGrid allocates a cols×rows grid of hidden panels (depths unset; call
// seedDepths to assign 1–3 rubs each).
func NewGrid(cols, rows int) *Grid {
	g := &Grid{Cols: cols, Rows: rows, Panels: make([]Panel, cols*rows)}
	for i := range g.Panels {
		g.Panels[i].Hidden = true
		g.Panels[i].Layers = 1
	}
	return g
}

// seedDepths assigns each panel a random 1–3 rub depth, so a fresh card shows
// mixed latex wear and stubborn panels take more digging (cosmetic only).
func (g *Grid) seedDepths(rng *rand.Rand) {
	for i := range g.Panels {
		g.Panels[i].Layers = 1 + rng.Intn(3)
	}
}

// Move shifts the coin cursor by (dx,dy) within bounds.
func (g *Grid) Move(dx, dy int) {
	if g.Cols == 0 {
		return
	}
	r := g.Cur/g.Cols + dy
	c := g.Cur%g.Cols + dx
	if c < 0 {
		c = 0
	}
	if c >= g.Cols {
		c = g.Cols - 1
	}
	if r < 0 {
		r = 0
	}
	if r >= g.Rows {
		r = g.Rows - 1
	}
	g.Cur = r*g.Cols + c
}

// Scratch rubs the focused panel once. Returns true if that rub revealed it.
func (g *Grid) Scratch() bool {
	p := &g.Panels[g.Cur]
	if !p.Hidden {
		return false
	}
	p.Layers--
	if p.Layers <= 0 {
		p.Layers = 0
		p.Hidden = false
		return true
	}
	return false
}

// ScratchAll wears every remaining panel fully through.
func (g *Grid) ScratchAll() {
	for i := range g.Panels {
		g.Panels[i].Hidden = false
		g.Panels[i].Layers = 0
	}
}

// Revealed reports whether panel i is open.
func (g *Grid) Revealed(i int) bool { return !g.Panels[i].Hidden }

// AllRevealed reports whether every panel is open.
func (g *Grid) AllRevealed() bool {
	for i := range g.Panels {
		if g.Panels[i].Hidden {
			return false
		}
	}
	return true
}

// genericGridCard is the Phase-0 stub Card: a grid of the ticket's size that
// resolves to the drawn outcome once every panel is revealed. The four mechanic
// files replace this with their real engines; until then it keeps the package
// compiling and the game playable end-to-end.
type genericGridCard struct {
	title  string
	prompt string
	grid   *Grid
	win    int
	view   int
}

func newGenericGridCard(t *Ticket, out Outcome, rng *rand.Rand) *genericGridCard {
	cols, rows := t.Cols, t.Rows
	if cols <= 0 {
		cols, rows = 3, 3
	}
	g := NewGrid(cols, rows)
	g.seedDepths(rng)
	for i := range g.Panels {
		g.Panels[i].Reveal = "$?"
		g.Panels[i].Ink = stReveal
	}
	return &genericGridCard{
		title:  t.Name + " · $" + itoa(t.Price),
		prompt: "scratch the card",
		grid:   g,
		win:    out.Win,
		view:   viewportFor(rows),
	}
}

func (c *genericGridCard) Title() string  { return c.title }
func (c *genericGridCard) Prompt() string { return c.prompt }
func (c *genericGridCard) Move(dx, dy int) { c.grid.Move(dx, dy) }
func (c *genericGridCard) Scratch() bool   { return c.grid.Scratch() }
func (c *genericGridCard) ScratchAll()     { c.grid.ScratchAll() }
func (c *genericGridCard) Resolved() bool  { return c.grid.AllRevealed() }
func (c *genericGridCard) Win() int        { return c.win }
func (c *genericGridCard) Render(f *Frame, top int) {
	drawGrid(f, c.grid, top, 10, c.view)
	f.Text(top+c.view*cellH+1, 3, c.prompt, stDim)
}

// viewportFor returns how many cell-rows fit in the card body (rows ~2–20).
func viewportFor(rows int) int {
	const bodyRows = 18
	v := bodyRows / cellH
	if rows < v {
		return rows
	}
	return v
}

// itoa is a tiny strconv.Itoa shim kept local so card.go needs no extra import.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
