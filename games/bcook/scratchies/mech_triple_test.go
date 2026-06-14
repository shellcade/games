package main

import (
	"math/rand"
	"strings"
	"testing"

	kit "github.com/shellcade/kit/v2"
)

// Real-catalog-style vocabularies.
var (
	twWords6x4 = []string{"WIN", "CASH", "GOLD", "LUCK", "RICH", "MONEY", "BONUS"}
	twWords6x6 = []string{"WIN", "CASH", "GOLD", "LUCK", "RICH", "MONEY", "BONUS", "WINNER", "RICHES", "WEALTH"}
)

func twTestTicket(cols, rows int, words []string) *Ticket {
	return &Ticket{
		Slug:     "test-triple",
		Name:     "Triple Word",
		Price:    5,
		Mechanic: MechTriple,
		Cols:     cols,
		Rows:     rows,
		WordList: words,
		Prizes:   tier5Table(100000),
	}
}

// twWordSet upper-cases the vocabulary into a lookup set, dropping over-long words
// the engine would also drop.
func twWordSet(cols int, words []string) map[string]bool {
	m := make(map[string]bool)
	for _, w := range words {
		u := strings.ToUpper(w)
		if u == "" || len([]rune(u)) > cols {
			continue
		}
		m[u] = true
	}
	return m
}

// twRowWord reads the leading non-blank tiles of row r (mirrors twCard.rowWord).
func twRowWord(c *twCard, r int) string {
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

// twSpelledRows returns the indices of every row that spells a vocabulary word.
func twSpelledRows(c *twCard, set map[string]bool) []int {
	var out []int
	for r := 0; r < c.grid.Rows; r++ {
		if set[twRowWord(c, r)] {
			out = append(out, r)
		}
	}
	return out
}

// TestTwWinLayout: a winning card has exactly one row spelling a listed word, and
// that word's prize ×3-iff-triple-tile-on-its-row equals out.Win. Covers both a
// divisible-by-3 prize (triple tile may land on the winner) and a non-divisible
// prize (triple tile must NOT be on the winner). Reveal-all gives Win()==out.Win.
func TestTwWinLayout(t *testing.T) {
	type sz struct {
		cols, rows int
		words      []string
	}
	sizes := []sz{
		{6, 4, twWords6x4},
		{6, 6, twWords6x6},
	}
	// Divisible-by-3 and non-divisible prizes.
	div3 := []int{30, 60, 150, 300, 3000}
	nonDiv3 := []int{20, 50, 100, 500, 1000}
	seeds := []int64{1, 2, 3, 7, 42, 99, 1234, 5678}

	for _, s := range sizes {
		set := twWordSet(s.cols, s.words)
		for _, seed := range seeds {
			for _, win := range append(append([]int{}, div3...), nonDiv3...) {
				rng := rand.New(rand.NewSource(seed))
				tk := twTestTicket(s.cols, s.rows, s.words)
				c := twBuild(tk, Outcome{Win: win}, rng)

				// Exactly one row spells a listed word.
				rows := twSpelledRows(c, set)
				if len(rows) != 1 {
					t.Fatalf("%dx%d seed=%d win=%d: %d rows spell a word, want 1 (rows=%v)",
						s.cols, s.rows, seed, win, len(rows), rows)
				}
				wr := rows[0]

				// The triple tile must be on the winning row iff win is divisible
				// by 3; if not divisible it must not be on the winning row.
				if win%3 != 0 && c.tripleRow == wr {
					t.Fatalf("%dx%d seed=%d win=%d: triple tile on winning row but win not divisible by 3",
						s.cols, s.rows, seed, win)
				}

				// Base prize, ×3 iff triple on winning row, equals out.Win.
				want := c.base
				if c.tripleRow == wr {
					want = c.base * 3
				}
				if want != win {
					t.Fatalf("%dx%d seed=%d win=%d: base=%d tri/win=%v resolves to %d",
						s.cols, s.rows, seed, win, c.base, c.tripleRow == wr, want)
				}

				// Not resolved before scratching.
				if c.Resolved() {
					t.Fatalf("%dx%d seed=%d win=%d: resolved before scratch", s.cols, s.rows, seed, win)
				}

				// Reveal all -> Win() == out.Win.
				c.ScratchAll()
				if !c.Resolved() {
					t.Fatalf("%dx%d seed=%d win=%d: not resolved after ScratchAll", s.cols, s.rows, seed, win)
				}
				if got := c.Win(); got != win {
					t.Fatalf("%dx%d seed=%d win=%d: Win()=%d", s.cols, s.rows, seed, win, got)
				}
			}
		}
	}
}

// TestTwDiv3CanTriple: across enough seeds, a divisible-by-3 prize sometimes
// lands the triple tile on the winning row (with base = win/3). Guards that the
// div3 branch is actually reachable.
func TestTwDiv3CanTriple(t *testing.T) {
	const win = 30
	tk := twTestTicket(6, 4, twWords6x4)
	tripled := false
	for seed := int64(0); seed < 200 && !tripled; seed++ {
		rng := rand.New(rand.NewSource(seed))
		c := twBuild(tk, Outcome{Win: win}, rng)
		if c.tripleRow == c.winRow {
			if c.base != win/3 {
				t.Fatalf("seed=%d: triple on winner but base=%d, want %d", seed, c.base, win/3)
			}
			tripled = true
		}
	}
	if !tripled {
		t.Fatal("triple tile never landed on the winning row for a divisible-by-3 prize")
	}
}

// TestTwNonDiv3NeverTriples: a non-divisible prize must never place the triple
// tile on the winning row, and base must equal out.Win.
func TestTwNonDiv3NeverTriples(t *testing.T) {
	const win = 100 // not divisible by 3
	tk := twTestTicket(6, 6, twWords6x6)
	for seed := int64(0); seed < 200; seed++ {
		rng := rand.New(rand.NewSource(seed))
		c := twBuild(tk, Outcome{Win: win}, rng)
		if c.tripleRow == c.winRow {
			t.Fatalf("seed=%d: triple tile on winning row for non-divisible prize", seed)
		}
		if c.base != win {
			t.Fatalf("seed=%d: base=%d, want %d", seed, c.base, win)
		}
		c.ScratchAll()
		if got := c.Win(); got != win {
			t.Fatalf("seed=%d: Win()=%d, want %d", seed, got, win)
		}
	}
}

// TestTwLossLayout: a losing card has no row spelling any listed word, and
// Win()==0 after reveal-all.
func TestTwLossLayout(t *testing.T) {
	sizes := [][3]interface{}{
		{6, 4, twWords6x4},
		{6, 6, twWords6x6},
	}
	seeds := []int64{1, 2, 3, 7, 42, 99, 1234, 5678, 31337}
	for _, s := range sizes {
		cols := s[0].(int)
		rows := s[1].(int)
		words := s[2].([]string)
		set := twWordSet(cols, words)
		for _, seed := range seeds {
			rng := rand.New(rand.NewSource(seed))
			tk := twTestTicket(cols, rows, words)
			c := twBuild(tk, Outcome{Win: 0}, rng)

			if spelled := twSpelledRows(c, set); len(spelled) != 0 {
				t.Fatalf("%dx%d seed=%d loss: rows %v spell a word", cols, rows, seed, spelled)
			}
			c.ScratchAll()
			if got := c.Win(); got != 0 {
				t.Fatalf("%dx%d seed=%d loss: Win()=%d, want 0", cols, rows, seed, got)
			}
		}
	}
}

// TestTwNoSpoiler: the winning row must not be coloured stMatch at build, nor
// after a single render before the card resolves; only at resolution does the
// found row go green.
func TestTwNoSpoiler(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tk := twTestTicket(6, 4, twWords6x4)
	c := twBuild(tk, Outcome{Win: 30}, rng)

	for i, p := range c.grid.Panels {
		if p.Ink == stMatch {
			t.Fatalf("tile %d pre-coloured green at build", i)
		}
	}

	// Render while unresolved — still nothing green.
	c.Render(kit.NewFrame(), 3)
	for i, p := range c.grid.Panels {
		if p.Ink == stMatch {
			t.Fatalf("tile %d coloured green before resolution", i)
		}
	}

	// After resolution + render, exactly the winning row's letters are green.
	c.ScratchAll()
	c.Render(kit.NewFrame(), 3)
	cols := c.grid.Cols
	greens := 0
	for col := 0; col < cols; col++ {
		idx := c.winRow*cols + col
		if c.grid.Panels[idx].Reveal != "" && c.grid.Panels[idx].Ink == stMatch {
			greens++
		}
	}
	if greens != len([]rune(c.winWord)) {
		t.Fatalf("want %d green winning-row tiles, got %d", len([]rune(c.winWord)), greens)
	}
}

// TestTwTitle checks the title format.
func TestTwTitle(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	tk := twTestTicket(6, 4, twWords6x4)
	c := twBuild(tk, Outcome{Win: 0}, rng)
	want := "Triple Word · $5 · spell a bonus word"
	if c.Title() != want {
		t.Fatalf("Title() = %q, want %q", c.Title(), want)
	}
}

// TestTwBlankTail verifies a winning row's trailing tiles (after the word) are
// blank, and the leading tiles spell the planted word.
func TestTwBlankTail(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	tk := twTestTicket(6, 4, twWords6x4)
	c := twBuild(tk, Outcome{Win: 50}, rng) // "WIN"/"CASH"/... shorter than 6

	cols := c.grid.Cols
	word := []rune(c.winWord)
	for col := 0; col < cols; col++ {
		got := c.grid.Panels[c.winRow*cols+col].Reveal
		if col < len(word) {
			if got != string(word[col]) {
				t.Fatalf("winRow col %d = %q, want %q", col, got, string(word[col]))
			}
		} else if got != "" {
			t.Fatalf("winRow tail col %d = %q, want blank", col, got)
		}
	}
}
