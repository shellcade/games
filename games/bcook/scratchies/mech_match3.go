package main

import (
	"fmt"
	"math/rand"
)

// mech_match3.go — match-3 cash engine (SPEC §5.1, AB-3/4/5/9/12).
// A grid of cash-amount panels; uncover three equal amounts to win that amount.
// Winning card: exactly three panels show out.Win, decoys never reach 3-of-a-kind.
// Losing card: no amount appears three or more times.

func init() {
	builders[MechMatch3] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return match3Build(t, out, rng)
	}
}

// match3Amounts is the palette of plausible cash values used for panel content.
// The palette is large enough that 2-per-value covers any grid up to 6×6 (36
// panels): a 6×6 winning card needs 33 decoy slots; with 18+ distinct values
// we always have 2×18 = 36 ≥ 33 slots before the cap is hit. For a losing
// 6×6 card all 36 slots must be filled, requiring ceil(36/2) = 18 values.
// This 21-value palette exceeds that bound with room to spare.
var match3Amounts = []int{
	1, 2, 3, 5, 7, 10, 15, 20, 25, 30,
	50, 75, 100, 150, 200, 250, 500, 750, 1000, 2000, 5000,
}

// match3Fmt formats a credit amount to fit a 4-char cell: "$5", "$50", "$1k", "10k".
func match3Fmt(n int) string {
	if n < 1000 {
		return fmt.Sprintf("$%d", n)
	}
	k := n / 1000
	if k < 10 {
		return fmt.Sprintf("$%dk", k) // "$1k", "$5k"
	}
	return fmt.Sprintf("%dk", k) // "10k", "250k"
}

// match3Card is a scratch card for the match-3 cash mechanic.
type match3Card struct {
	grid     *Grid
	win      int    // predetermined prize (0 = loss)
	winAmt   string // formatted winning amount label
	view     int
	title    string
	resolved bool
	counts   map[string]int // revealed label -> count, updated on each Scratch
}

// match3Build constructs a match3Card for ticket t with predetermined outcome out.
func match3Build(t *Ticket, out Outcome, rng *rand.Rand) *match3Card {
	cols, rows := t.Cols, t.Rows
	if cols <= 0 {
		cols, rows = 3, 3
	}
	total := cols * rows

	g := NewGrid(cols, rows)
	g.seedDepths(rng)

	amounts := make([]int, total)

	if out.Win > 0 {
		// Winning card: plant out.Win in exactly three panels.
		// Choose 3 positions via Fisher-Yates on indices.
		perm := make([]int, total)
		for i := range perm {
			perm[i] = i
		}
		for i := total - 1; i > 0; i-- {
			j := rng.Intn(i + 1)
			perm[i], perm[j] = perm[j], perm[i]
		}
		winPos := map[int]bool{perm[0]: true, perm[1]: true, perm[2]: true}

		// Decoy palette excludes the winning amount.
		decoyPool := match3DecoyPool(out.Win)
		decoyCounts := make(map[int]int)

		for i := 0; i < total; i++ {
			if winPos[i] {
				amounts[i] = out.Win
			} else {
				amounts[i] = match3PickDecoy(decoyPool, decoyCounts, rng)
			}
		}
	} else {
		// Losing card: no amount may appear 3+ times.
		decoyCounts := make(map[int]int)
		for i := 0; i < total; i++ {
			amounts[i] = match3PickDecoy(match3Amounts, decoyCounts, rng)
		}
	}

	// Assign Reveal and Ink to each panel.
	for i := range g.Panels {
		g.Panels[i].Reveal = match3Fmt(amounts[i])
		if out.Win > 0 && amounts[i] == out.Win {
			g.Panels[i].Ink = stMatch
		} else {
			g.Panels[i].Ink = stReveal
		}
	}

	winAmt := ""
	if out.Win > 0 {
		winAmt = match3Fmt(out.Win)
	}

	return &match3Card{
		grid:   g,
		win:    out.Win,
		winAmt: winAmt,
		view:   viewportFor(rows),
		title:  t.Name + " · $" + itoa(t.Price) + " · match three",
		counts: make(map[string]int),
	}
}

// match3DecoyPool returns the amounts palette excluding the given value.
func match3DecoyPool(exclude int) []int {
	pool := make([]int, 0, len(match3Amounts))
	for _, a := range match3Amounts {
		if a != exclude {
			pool = append(pool, a)
		}
	}
	return pool
}

// match3PickDecoy picks a random amount from pool where its count < 2,
// ensuring no decoy appears 3+ times. Falls back to the full pool if all
// are already at the limit (only possible on extremely small grids).
func match3PickDecoy(pool []int, counts map[int]int, rng *rand.Rand) int {
	avail := make([]int, 0, len(pool))
	for _, a := range pool {
		if counts[a] < 2 {
			avail = append(avail, a)
		}
	}
	if len(avail) == 0 {
		avail = pool // fallback: shouldn't happen in normal play
	}
	pick := avail[rng.Intn(len(avail))]
	counts[pick]++
	return pick
}

// -- Card interface --

func (c *match3Card) Title() string { return c.title }

func (c *match3Card) Move(dx, dy int) { c.grid.Move(dx, dy) }

func (c *match3Card) Scratch() (revealed bool) {
	revealed = c.grid.Scratch()
	if revealed {
		label := c.grid.Panels[c.grid.Cur].Reveal
		c.counts[label]++
		if c.counts[label] >= 3 {
			c.resolved = true
		}
	}
	return
}

func (c *match3Card) ScratchAll() {
	c.grid.ScratchAll()
	c.resolved = true
}

func (c *match3Card) Resolved() bool {
	return c.resolved || c.grid.AllRevealed()
}

func (c *match3Card) Win() int {
	if !c.Resolved() {
		return 0
	}
	// The winning amount is predetermined; return it directly if the card is a
	// winner (c.win > 0). This avoids any ambiguity from label scanning.
	if c.win > 0 {
		return c.win
	}
	return 0
}

func (c *match3Card) Prompt() string {
	// Build live counts from currently-revealed panels.
	live := make(map[string]int)
	for i := range c.grid.Panels {
		if !c.grid.Panels[i].Hidden {
			live[c.grid.Panels[i].Reveal]++
		}
	}

	if c.Resolved() {
		w := c.Win()
		if w > 0 {
			return "✦ three " + c.winAmt + " — WON " + itoa(w) + " CREDITS ✦"
		}
		return "no three-of-a-kind — no win"
	}

	// Find the label with the highest in-progress count.
	best := ""
	bestCnt := 0
	for label, cnt := range live {
		if cnt > bestCnt || (cnt == bestCnt && label > best) {
			bestCnt = cnt
			best = label
		}
	}
	if bestCnt == 2 {
		return "two " + best + " so far — one more pays " + itoa(match3ParseAmt(best))
	}
	return "scratch the grid — match three amounts to win"
}

func (c *match3Card) Render(f *Frame, top int) {
	drawGrid(f, c.grid, top, 10, c.view)
	f.Text(top+c.view*cellH+1, 3, c.Prompt(), stDim)
}

// match3ParseAmt converts a formatted label ("$5", "$1k", "10k") back to credits.
func match3ParseAmt(s string) int {
	if len(s) == 0 {
		return 0
	}
	t := s
	if len(t) > 0 && t[0] == '$' {
		t = t[1:]
	}
	if len(t) > 0 && t[len(t)-1] == 'k' {
		t = t[:len(t)-1]
		return match3AtoiSimple(t) * 1000
	}
	return match3AtoiSimple(t)
}

// match3AtoiSimple converts a decimal string to int; returns 0 on error.
func match3AtoiSimple(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
