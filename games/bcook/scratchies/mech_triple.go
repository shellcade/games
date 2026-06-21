package main

import (
	"math/rand"
	"strings"
)

// mech_triple.go — Triple Word engine.
//
// A t.Cols × t.Rows grid of letter tiles where EACH ROW is a word slot. A row,
// fully revealed, spells a word using its first len(word) tiles; the remaining
// tiles in that row are blank (Reveal==""). A row spelling a word in t.WordList
// is FOUND and pays that word's prize. One special TRIPLE-WORD (3×) tile sits on
// exactly one row; if that row is FOUND its prize is tripled.
//
// The WordList is just vocabulary — the engine assigns the prize from out.Win:
//   - WINNING card (out.Win>0): plant exactly ONE WordList word in a random row.
//     If out.Win is divisible by 3 the 3× tile MAY land on the winning row, with
//     base prize = out.Win/3; otherwise the 3× tile goes on a non-winning row (or
//     is omitted) and base prize = out.Win. Resolved payout is exactly out.Win.
//   - LOSING card (out.Win==0): no row spells any WordList word.
//
// NO-SPOILER: a found row is only coloured stMatch AT RESOLUTION (in Render),
// never at build — otherwise the win would be telegraphed. The vocabulary list
// is known information and may always be shown.

func init() {
	builders[MechTriple] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return twBuild(t, out, rng)
	}
}

// twCard implements Card for the Triple Word mechanic.
type twCard struct {
	grid      *Grid
	ticket    *Ticket
	words     []string // vocabulary (upper-cased), shown as the bonus list
	wordSet   map[string]bool
	win       int    // predetermined payout (0 = loss) == Win() at resolution
	base      int    // base prize of the planted word (win == base or base*3)
	winRow    int    // row that spells the planted word (-1 on a loss)
	winWord   string // the planted word (upper-cased), "" on a loss
	tripleRow int    // row carrying the 3× tile (-1 = none)
	view      int
}

// twBuild constructs a twCard for ticket t with predetermined outcome out.
func twBuild(t *Ticket, out Outcome, rng *rand.Rand) *twCard {
	cols, rows := t.Cols, t.Rows
	if cols <= 0 {
		cols, rows = 6, 4
	}

	// Normalise vocabulary to upper-case and drop any word longer than the grid.
	words := make([]string, 0, len(t.WordList))
	set := make(map[string]bool)
	for _, w := range t.WordList {
		u := strings.ToUpper(w)
		if u == "" || len([]rune(u)) > cols {
			continue
		}
		if set[u] {
			continue
		}
		words = append(words, u)
		set[u] = true
	}

	g := NewGrid(cols, rows)
	g.seedDepths(rng)

	c := &twCard{
		grid:      g,
		ticket:    t,
		words:     words,
		wordSet:   set,
		win:       out.Win,
		winRow:    -1,
		tripleRow: -1,
		view:      viewportFor(rows),
	}

	if out.Win > 0 && len(words) > 0 {
		c.winWord = words[rng.Intn(len(words))]
		c.winRow = rng.Intn(rows)

		// Decide the 3× tile placement and the base prize so the resolved payout
		// is exactly out.Win.
		if out.Win%3 == 0 && rng.Intn(2) == 0 {
			// Triple tile on the winning row: base × 3 == out.Win.
			c.base = out.Win / 3
			c.tripleRow = c.winRow
		} else {
			// Triple tile must NOT be on the winning row. Place it on some other
			// row (or omit it if the grid is a single row).
			c.base = out.Win
			if rows > 1 {
				c.tripleRow = twPickOtherRow(rows, c.winRow, rng)
			}
		}
	} else {
		// Loss: no planted word. The 3× tile may still sit somewhere for flavour.
		c.win = 0
		if rows > 0 {
			c.tripleRow = rng.Intn(rows)
		}
	}

	c.fillTiles(rng)
	return c
}

// twPickOtherRow returns a random row index in [0,rows) that is != avoid.
func twPickOtherRow(rows, avoid int, rng *rand.Rand) int {
	r := rng.Intn(rows - 1)
	if r >= avoid {
		r++
	}
	return r
}

// fillTiles writes Reveal/Ink for every tile. The winning row (if any) spells
// winWord in its leading tiles with blanks after; every other row gets letters
// that do NOT spell any vocabulary word.
func (c *twCard) fillTiles(rng *rand.Rand) {
	cols := c.grid.Cols
	for r := 0; r < c.grid.Rows; r++ {
		var letters string
		if r == c.winRow && c.winWord != "" {
			letters = c.winWord
		} else {
			letters = c.nonWord(cols, rng)
		}
		runes := []rune(letters)
		for col := 0; col < cols; col++ {
			idx := r*cols + col
			if col < len(runes) {
				c.grid.Panels[idx].Reveal = string(runes[col])
			} else {
				c.grid.Panels[idx].Reveal = "" // blank tail tile
			}
			// Neutral ink at build — found rows go green only at resolution.
			c.grid.Panels[idx].Ink = stReveal
		}
	}
}

const twAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// nonWord returns a letter string (1..cols letters) that, read as the leading
// tiles of a row, does NOT spell any vocabulary word. We bias toward a near-miss
// of a real word (one letter changed) for verisimilitude, then verify.
func (c *twCard) nonWord(cols int, rng *rand.Rand) string {
	for attempt := 0; attempt < 64; attempt++ {
		var s string
		if len(c.words) > 0 && rng.Intn(2) == 0 {
			// Near-miss: take a vocab word and change one letter.
			base := []rune(c.words[rng.Intn(len(c.words))])
			pos := rng.Intn(len(base))
			base[pos] = rune(twAlphabet[rng.Intn(len(twAlphabet))])
			s = string(base)
		} else {
			// Random letters, random length 1..cols.
			n := 1 + rng.Intn(cols)
			b := make([]byte, n)
			for i := range b {
				b[i] = twAlphabet[rng.Intn(len(twAlphabet))]
			}
			s = string(b)
		}
		if !c.wordSet[s] {
			return s
		}
	}
	// Fallback: a guaranteed non-word (single 'X' is only a word if "X" is in
	// the list; in that pathological case append until it differs).
	s := "X"
	for c.wordSet[s] && len(s) < cols {
		s += "X"
	}
	return s
}

// rowWord returns the word spelled by row r's leading non-blank tiles (reading
// until the first blank), in upper-case. Tiles are stored upper-case already.
func (c *twCard) rowWord(r int) string {
	cols := c.grid.Cols
	var b strings.Builder
	for col := 0; col < cols; col++ {
		rv := c.grid.Panels[r*cols+col].Reveal
		if rv == "" {
			break
		}
		b.WriteString(rv)
	}
	return b.String()
}

// foundRow returns the index of the row that spells a vocabulary word, or -1.
// Used only at resolution (for colouring and the win line).
func (c *twCard) foundRow() int {
	for r := 0; r < c.grid.Rows; r++ {
		if c.wordSet[c.rowWord(r)] {
			return r
		}
	}
	return -1
}

// -- Card interface --

func (c *twCard) Title() string {
	return c.ticket.Name + " · $" + itoa(c.ticket.Price) + " · spell a bonus word"
}

func (c *twCard) Move(dx, dy int) { c.grid.Move(dx, dy) }
func (c *twCard) Scratch() bool   { return c.grid.Scratch() }
func (c *twCard) ScratchAll()     { c.grid.ScratchAll() }
func (c *twCard) Resolved() bool  { return c.grid.AllRevealed() }

func (c *twCard) Win() int {
	if !c.Resolved() {
		return 0
	}
	// The payout is predetermined: a winning card pays its planted prize, tripled
	// iff the 3× tile sits on the winning row. This equals out.Win by construction.
	if c.winRow < 0 {
		return 0
	}
	if c.tripleRow == c.winRow {
		return c.base * 3
	}
	return c.base
}

func (c *twCard) Prompt() string {
	if c.Resolved() {
		if c.Win() > 0 {
			fr := c.foundRow()
			word := ""
			if fr >= 0 {
				word = c.rowWord(fr)
			}
			line := "✦ " + word + " - WON " + commaInt(c.Win()) + " CREDITS ✦"
			if c.tripleRow == fr && fr >= 0 {
				line = "✦ " + word + " ×3 - WON " + commaInt(c.Win()) + " CREDITS ✦"
			}
			return line
		}
		return "no bonus word - no win"
	}
	return "scratch each row - spell a bonus word to win"
}

func (c *twCard) Render(f *Frame, top int) {
	// NO-SPOILER: colour the found row green only once the card has resolved.
	if c.Resolved() {
		if fr := c.foundRow(); fr >= 0 {
			cols := c.grid.Cols
			for col := 0; col < cols; col++ {
				if c.grid.Panels[fr*cols+col].Reveal != "" {
					c.grid.Panels[fr*cols+col].Ink = stMatch
				}
			}
		}
	}

	drawGrid(f, c.grid, top, 10, c.view)

	// Mark the 3× tile's row with a "3×" rail tag to the left of the grid.
	if c.tripleRow >= 0 {
		c.markTriple(f, top)
	}

	// Bonus-word vocabulary list, to the right of the grid (known information).
	listX := 10 + c.grid.Cols*cellStepX + 4
	f.Text(top, listX, "BONUS WORDS", stTitle)
	for i, w := range c.words {
		ry := top + 1 + i
		if ry >= top+c.view*cellH {
			break
		}
		f.Text(ry, listX, "· "+w, stDim)
	}

	// Prompt / win line below the grid.
	f.Text(top+c.view*cellH+1, 3, c.Prompt(), stDim)
}

// markTriple draws a "3×" tag on the triple-word row, if it's within the current
// viewport. The viewport's first visible row mirrors drawGrid's scroll logic.
func (c *twCard) markTriple(f *Frame, top int) {
	curRow := 0
	if c.grid.Cols > 0 {
		curRow = c.grid.Cur / c.grid.Cols
	}
	first := 0
	if curRow >= c.view {
		first = curRow - c.view + 1
	}
	if maxFirst := c.grid.Rows - c.view; first > maxFirst && maxFirst > 0 {
		first = maxFirst
	}
	if c.tripleRow < first || c.tripleRow >= first+c.view {
		return
	}
	ry := top + (c.tripleRow-first)*cellH + 1
	f.Text(ry, 6, "3×", stWin)
}
