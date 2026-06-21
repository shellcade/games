package main

import (
	"fmt"
	"math/rand"
	"sort"
)

// mech_crossword.go — Cashword (letter-bank) engine.
//
// The Cols×Rows grid is a LETTER BANK: each panel hides one uppercase letter.
// t's word list (registered via cwWordLists, keyed by slug) holds target words.
// A word is COMPLETE when every distinct letter of it appears among the REVEALED
// bank panels — i.e. word.distinctLetters ⊆ revealed-bank-letters (set
// membership; the bank need only CONTAIN each letter once, not in order). You win
// when ≥3 words are complete; the prize is out.Win. Fewer than 3 complete pays 0.
//
// The bank is constructed up-front to be self-consistent with out.Win:
//   - WIN  (out.Win>0): ≥3 words are made completable (their union of distinct
//     letters is planted in the bank; remaining slots get fillers).
//   - LOSS (out.Win==0): the bank is restricted so that AT MOST 2 words can
//     complete — at least one letter of every other word is kept out of the bank.
//
// Render: the letter bank via drawGrid, then a checklist of the word list (each
// word goes green only once it is genuinely complete from the revealed bank), an
// "N words found" prompt, and the win line at resolution.

// cwMinWords is how many words must be complete to win.
const cwMinWords = 3

func init() {
	builders[MechCrossword] = cwBuild
}

// cwCard satisfies the Card interface for the Cashword mechanic.
type cwCard struct {
	t     *Ticket
	out   Outcome
	words []string // target words (uppercased, original order)
	grid  *Grid    // the letter bank; each panel's Reveal is one uppercase letter
	view  int
}

// cwBuild constructs a cwCard from a drawn Outcome. The word list is taken from
// cwWordLists[t.Slug]; words are uppercased and de-duplicated by value.
func cwBuild(t *Ticket, out Outcome, rng *rand.Rand) Card {
	cols, rows := t.Cols, t.Rows
	if cols <= 0 {
		cols = 4
	}
	if rows <= 0 {
		rows = 4
	}
	total := cols * rows

	words := cwNormalizeWords(t.WordList)

	var bank []byte
	if out.Win > 0 {
		bank = cwBuildWinBank(words, total, rng)
	} else {
		bank = cwBuildLossBank(words, total, rng)
	}

	g := NewGrid(cols, rows)
	g.seedDepths(rng)
	for i := range g.Panels {
		letter := byte('?')
		if i < len(bank) {
			letter = bank[i]
		}
		g.Panels[i].Reveal = string(letter)
		g.Panels[i].Ink = stReveal
	}

	return &cwCard{
		t:     t,
		out:   out,
		words: words,
		grid:  g,
		view:  viewportFor(rows),
	}
}

// cwNormalizeWords uppercases each word, drops empties/non-letters, and removes
// duplicate words (by value) while preserving first-seen order.
func cwNormalizeWords(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range in {
		u := cwUpper(w)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// cwUpper uppercases ASCII letters and strips anything that isn't a letter.
func cwUpper(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'a' && ch <= 'z':
			b = append(b, ch-('a'-'A'))
		case ch >= 'A' && ch <= 'Z':
			b = append(b, ch)
		}
	}
	return string(b)
}

// cwDistinct returns the sorted set of distinct letters in word.
func cwDistinct(word string) []byte {
	seen := [26]bool{}
	var out []byte
	for i := 0; i < len(word); i++ {
		ch := word[i]
		if ch >= 'A' && ch <= 'Z' && !seen[ch-'A'] {
			seen[ch-'A'] = true
			out = append(out, ch)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// cwBuildWinBank fills a bank of `total` letters so that ≥cwMinWords words are
// completable. It greedily picks words (fewest distinct letters first) whose
// cumulative distinct-letter union fits in `total`, until at least cwMinWords are
// chosen, then plants that union and fills the rest with fillers.
func cwBuildWinBank(words []string, total int, rng *rand.Rand) []byte {
	// Order candidate words by distinct-letter count (ascending) so we can pack
	// the most words into the bank. Ties are shuffled for variety.
	idx := cwShuffledIndices(len(words), rng)
	sort.SliceStable(idx, func(a, b int) bool {
		return len(cwDistinct(words[idx[a]])) < len(cwDistinct(words[idx[b]]))
	})

	union := map[byte]bool{}
	chosen := 0
	for _, wi := range idx {
		need := cwDistinct(words[wi])
		// Tentatively add this word's letters to the union.
		trial := map[byte]bool{}
		for k, v := range union {
			trial[k] = v
		}
		for _, ch := range need {
			trial[ch] = true
		}
		if len(trial) > total {
			continue // wouldn't fit; skip
		}
		union = trial
		chosen++
		if chosen >= cwMinWords {
			// Keep packing only while it still fits, but we have enough; stop to
			// leave slots for filler diversity (and to keep the win minimal).
			break
		}
	}

	bank := make([]byte, 0, total)
	// Plant the union (sorted for determinism given the rng-seeded ordering).
	var planted []byte
	for ch := byte('A'); ch <= 'Z'; ch++ {
		if union[ch] {
			planted = append(planted, ch)
		}
	}
	bank = append(bank, planted...)

	// Fill remaining slots with random letters (any letter is fine on a winning
	// card — extra completions don't change the ≥3 outcome).
	for len(bank) < total {
		bank = append(bank, byte('A'+rng.Intn(26)))
	}
	cwShuffleBytes(bank, rng)
	return bank
}

// cwBuildLossBank fills a bank of `total` letters so AT MOST 2 words can complete.
// Strategy: choose a single "banned" letter that appears in as many words as
// possible and is excluded from the bank, guaranteeing those words can never
// complete. If a banned letter alone can't push completable words to ≤2, ban
// additional letters until at most 2 words remain completable. The bank is then
// filled from the surviving (non-banned) alphabet.
func cwBuildLossBank(words []string, total int, rng *rand.Rand) []byte {
	banned := map[byte]bool{}

	// Greedily ban letters until at most 2 words could still complete. A word can
	// still complete iff none of its distinct letters is banned.
	for cwCompletableCount(words, banned) > 2 {
		ch := cwBestLetterToBan(words, banned)
		if ch == 0 {
			break // nothing left to ban (degenerate); fall through
		}
		banned[ch] = true
	}

	// Allowed alphabet = letters not banned. Guard against an empty alphabet.
	var allowed []byte
	for ch := byte('A'); ch <= 'Z'; ch++ {
		if !banned[ch] {
			allowed = append(allowed, ch)
		}
	}
	if len(allowed) == 0 {
		allowed = []byte{'A'} // degenerate fallback; only if ≥26 banned letters
	}

	bank := make([]byte, 0, total)
	for len(bank) < total {
		bank = append(bank, allowed[rng.Intn(len(allowed))])
	}
	cwShuffleBytes(bank, rng)
	return bank
}

// cwBestLetterToBan returns the not-yet-banned letter whose banning eliminates
// the most still-completable words (it appears in the most completable words).
// Returns 0 if no useful letter exists.
func cwBestLetterToBan(words []string, banned map[byte]bool) byte {
	count := [26]int{}
	for _, w := range words {
		if !cwWordCompletable(w, banned) {
			continue // already blocked; its letters don't help us
		}
		for _, ch := range cwDistinct(w) {
			if !banned[ch] {
				count[ch-'A']++
			}
		}
	}
	best := byte(0)
	bestN := 0
	for i := 0; i < 26; i++ {
		if count[i] > bestN {
			bestN = count[i]
			best = byte('A' + i)
		}
	}
	return best
}

// cwWordCompletable reports whether word could complete given a set of banned
// letters (i.e. none of its distinct letters is banned).
func cwWordCompletable(word string, banned map[byte]bool) bool {
	for _, ch := range cwDistinct(word) {
		if banned[ch] {
			return false
		}
	}
	return true
}

// cwCompletableCount counts words that could complete given the banned set.
func cwCompletableCount(words []string, banned map[byte]bool) int {
	n := 0
	for _, w := range words {
		if cwWordCompletable(w, banned) {
			n++
		}
	}
	return n
}

// cwShuffledIndices returns 0..n-1 in random order.
func cwShuffledIndices(n int, rng *rand.Rand) []int {
	s := make([]int, n)
	for i := range s {
		s[i] = i
	}
	for i := len(s) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// cwShuffleBytes Fisher-Yates shuffles a byte slice.
func cwShuffleBytes(s []byte, rng *rand.Rand) {
	for i := len(s) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		s[i], s[j] = s[j], s[i]
	}
}

// --- runtime word/bank evaluation --------------------------------------------

// cwRevealedSet returns the set of letters present among REVEALED bank panels.
func (c *cwCard) cwRevealedSet() map[byte]bool {
	set := map[byte]bool{}
	for i, p := range c.grid.Panels {
		if c.grid.Revealed(i) && len(p.Reveal) > 0 {
			set[p.Reveal[0]] = true
		}
	}
	return set
}

// cwWordComplete reports whether `word` is complete given a revealed-letter set:
// every distinct letter of the word is present in the set.
func cwWordComplete(word string, revealed map[byte]bool) bool {
	d := cwDistinct(word)
	if len(d) == 0 {
		return false
	}
	for _, ch := range d {
		if !revealed[ch] {
			return false
		}
	}
	return true
}

// cwCompleteCount counts how many target words are complete given revealed bank.
func (c *cwCard) cwCompleteCount(revealed map[byte]bool) int {
	n := 0
	for _, w := range c.words {
		if cwWordComplete(w, revealed) {
			n++
		}
	}
	return n
}

// --- Card interface -----------------------------------------------------------

func (c *cwCard) Title() string {
	return c.t.Name + " · $" + itoa(c.t.Price) + " · complete the words"
}

func (c *cwCard) Prompt() string {
	revealed := c.cwRevealedSet()
	found := c.cwCompleteCount(revealed)
	noun := "words"
	if found == 1 {
		noun = "word"
	}
	if !c.grid.AllRevealed() {
		return fmt.Sprintf("%d %s found — scratch the bank", found, noun)
	}
	if found >= cwMinWords {
		return fmt.Sprintf("%d words complete — WIN %s CREDITS", found, commaInt(c.Win()))
	}
	return fmt.Sprintf("%d words — need %d to win", found, cwMinWords)
}

func (c *cwCard) Move(dx, dy int)          { c.grid.Move(dx, dy) }
func (c *cwCard) Scratch() (revealed bool) { return c.grid.Scratch() }
func (c *cwCard) ScratchAll()              { c.grid.ScratchAll() }
func (c *cwCard) Resolved() bool           { return c.grid.AllRevealed() }

// Win returns out.Win when ≥cwMinWords words are complete (computed from the
// revealed bank), else 0. Valid once Resolved().
func (c *cwCard) Win() int {
	if !c.grid.AllRevealed() {
		return 0
	}
	if c.cwCompleteCount(c.cwRevealedSet()) >= cwMinWords {
		return c.out.Win
	}
	return 0
}

func (c *cwCard) Render(f *Frame, top int) {
	// Header label for the bank.
	f.Text(top, 3, "LETTER BANK", stTitle)

	gridTop := top + 1
	drawGrid(f, c.grid, gridTop, 3, c.view)

	// Checklist of words, to the right of the bank, then below it. The bank
	// occupies left columns; lay the checklist starting after the bank columns.
	revealed := c.cwRevealedSet()
	checkLeft := 3 + c.grid.Cols*cellStepX + 3
	if checkLeft > 40 {
		checkLeft = 40
	}
	for i, w := range c.words {
		row := gridTop + i
		st := stDim
		mark := "☐"
		if cwWordComplete(w, revealed) {
			st = stWin
			mark = "☑"
		}
		f.Text(row, checkLeft, mark+" "+w, st)
	}

	// Prompt + win line below the bank viewport.
	promptRow := gridTop + c.view*cellH + 1
	f.Text(promptRow, 3, c.Prompt(), stDim)
	if c.grid.AllRevealed() && c.Win() > 0 {
		f.Text(promptRow+1, 3, fmt.Sprintf("WIN $%s", commaInt(c.Win())), stWin)
	}
}
