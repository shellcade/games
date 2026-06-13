package main

import (
	"math/rand"
	"testing"

	kit "github.com/shellcade/kit/v2"
)

// --- fixtures -----------------------------------------------------------------

// cwTicket4x4 is a 4×4 Cashword ticket with a small word list, registered in
// cwWordLists under its slug.
func cwTicket4x4() Ticket {
	tk := Ticket{
		Slug:     "test-cashword-4x4",
		Name:     "Test Cashword 4×4",
		Price:    5,
		Mechanic: MechCrossword,
		Cols:     4,
		Rows:     4,
		WordList: []string{"GOLD", "CASH", "LUCKY", "RICH", "COIN", "WIN"},
		Prizes: PrizeTable{
			{Credits: 1, OneIn: 6},
			{Credits: 5, OneIn: 40},
			{Credits: 25, OneIn: 400},
			{Credits: 100, OneIn: 10000},
		},
	}
	return tk
}

// cwTicket5x4 is a larger 5×4 ticket with a longer word list.
func cwTicket5x4() Ticket {
	tk := Ticket{
		Slug:     "test-cashword-5x4",
		Name:     "Test Cashword 5×4",
		Price:    10,
		Mechanic: MechCrossword,
		Cols:     5,
		Rows:     4,
		WordList: []string{
			"GOLD", "CASH", "LUCKY", "RICH", "COIN", "WIN",
			"MONEY", "PRIZE", "JACKPOT", "BONUS",
		},
		Prizes: PrizeTable{
			{Credits: 5, OneIn: 6},
			{Credits: 50, OneIn: 200},
			{Credits: 500, OneIn: 9000},
		},
	}
	return tk
}

// buildCwCard builds a cwCard with a specific Outcome and seed.
func buildCwCard(t *Ticket, win int, seed int64) *cwCard {
	rng := rand.New(rand.NewSource(seed))
	out := Outcome{Win: win}
	return cwBuild(t, out, rng).(*cwCard)
}

// cwTestRevealedSet is an independent (test-owned) computation of the revealed
// letter set, used to cross-check the engine's own evaluation.
func cwTestRevealedSet(c *cwCard) map[byte]bool {
	set := map[byte]bool{}
	for _, p := range c.grid.Panels {
		if !p.Hidden && len(p.Reveal) > 0 {
			set[p.Reveal[0]] = true
		}
	}
	return set
}

// cwTestWordComplete is an independent helper: word W is complete iff every
// distinct letter of W is in the revealed set. Must agree with the engine.
func cwTestWordComplete(word string, revealed map[byte]bool) bool {
	seen := map[byte]bool{}
	any := false
	for i := 0; i < len(word); i++ {
		ch := word[i]
		if ch >= 'a' && ch <= 'z' {
			ch -= 'a' - 'A'
		}
		if ch < 'A' || ch > 'Z' {
			continue
		}
		any = true
		seen[ch] = true
	}
	if !any {
		return false
	}
	for ch := range seen {
		if !revealed[ch] {
			return false
		}
	}
	return true
}

// cwTestCompleteCount counts complete words via the independent helper.
func cwTestCompleteCount(c *cwCard) int {
	revealed := cwTestRevealedSet(c)
	n := 0
	for _, w := range c.words {
		if cwTestWordComplete(w, revealed) {
			n++
		}
	}
	return n
}

// --- helper agreement ---------------------------------------------------------

// TestCwHelperAgreesWithEngine ensures the test-owned completeness computation
// agrees with the engine's own cwWordComplete/cwCompleteCount on revealed banks.
func TestCwHelperAgreesWithEngine(t *testing.T) {
	for _, mk := range []func() Ticket{cwTicket4x4, cwTicket5x4} {
		tk := mk()
		for seed := int64(0); seed < 30; seed++ {
			for _, win := range []int{0, tk.Prizes[len(tk.Prizes)-1].Credits} {
				c := buildCwCard(&tk, win, seed)
				c.ScratchAll()
				engRevealed := c.cwRevealedSet()
				testRevealed := cwTestRevealedSet(c)
				for _, w := range c.words {
					eng := cwWordComplete(w, engRevealed)
					tst := cwTestWordComplete(w, testRevealed)
					if eng != tst {
						t.Errorf("%s seed=%d win=%d word=%s: engine=%v test=%v",
							tk.Slug, seed, win, w, eng, tst)
					}
				}
				if c.cwCompleteCount(engRevealed) != cwTestCompleteCount(c) {
					t.Errorf("%s seed=%d win=%d: complete-count mismatch eng=%d test=%d",
						tk.Slug, seed, win, c.cwCompleteCount(engRevealed), cwTestCompleteCount(c))
				}
			}
		}
	}
}

// --- WINNING cards ------------------------------------------------------------

// TestCwWin_AtLeastThreeComplete checks that a winning card has ≥3 complete words
// (their distinct letters ⊆ revealed bank) and Win()==out.Win after full reveal.
func TestCwWin_AtLeastThreeComplete(t *testing.T) {
	for _, mk := range []func() Ticket{cwTicket4x4, cwTicket5x4} {
		tk := mk()
		win := tk.Prizes[len(tk.Prizes)-1].Credits
		for seed := int64(0); seed < 40; seed++ {
			c := buildCwCard(&tk, win, seed)

			if c.Resolved() {
				t.Fatalf("%s seed=%d: Resolved() before scratch", tk.Slug, seed)
			}
			c.ScratchAll()
			if !c.Resolved() {
				t.Fatalf("%s seed=%d: not Resolved() after ScratchAll", tk.Slug, seed)
			}

			if got := cwTestCompleteCount(c); got < cwMinWords {
				t.Errorf("%s seed=%d: win card has only %d complete words, want ≥%d",
					tk.Slug, seed, got, cwMinWords)
			}
			if got := c.Win(); got != win {
				t.Errorf("%s seed=%d: Win()=%d, want %d", tk.Slug, seed, got, win)
			}
		}
	}
}

// TestCwWin_CompleteWordsAreSubsetOfBank verifies that each complete word's
// distinct letters are genuinely a subset of the revealed bank letters.
func TestCwWin_CompleteWordsAreSubsetOfBank(t *testing.T) {
	tk := cwTicket4x4()
	win := tk.Prizes[len(tk.Prizes)-1].Credits
	for seed := int64(0); seed < 25; seed++ {
		c := buildCwCard(&tk, win, seed)
		c.ScratchAll()
		revealed := cwTestRevealedSet(c)
		for _, w := range c.words {
			if !cwTestWordComplete(w, revealed) {
				continue
			}
			// Every distinct letter must be in the revealed bank.
			for i := 0; i < len(w); i++ {
				ch := w[i]
				if !revealed[ch] {
					t.Errorf("%s seed=%d: word %s marked complete but letter %c absent",
						tk.Slug, seed, w, ch)
				}
			}
		}
	}
}

// --- LOSING cards -------------------------------------------------------------

// TestCwLoss_AtMostTwoComplete checks a losing card has ≤2 complete words and
// Win()==0 after full reveal.
func TestCwLoss_AtMostTwoComplete(t *testing.T) {
	for _, mk := range []func() Ticket{cwTicket4x4, cwTicket5x4} {
		tk := mk()
		for seed := int64(0); seed < 40; seed++ {
			c := buildCwCard(&tk, 0, seed)
			c.ScratchAll()
			if !c.Resolved() {
				t.Fatalf("%s seed=%d: not Resolved() after ScratchAll", tk.Slug, seed)
			}
			if got := cwTestCompleteCount(c); got > 2 {
				t.Errorf("%s seed=%d: loss card has %d complete words, want ≤2",
					tk.Slug, seed, got)
			}
			if got := c.Win(); got != 0 {
				t.Errorf("%s seed=%d: Win()=%d, want 0", tk.Slug, seed, got)
			}
		}
	}
}

// TestCwLoss_EveryBlockedWordMissesALetter verifies that on a losing card, every
// word that is NOT complete is missing at least one of its letters from the bank.
func TestCwLoss_EveryBlockedWordMissesALetter(t *testing.T) {
	tk := cwTicket5x4()
	for seed := int64(0); seed < 25; seed++ {
		c := buildCwCard(&tk, 0, seed)
		c.ScratchAll()
		revealed := cwTestRevealedSet(c)
		for _, w := range c.words {
			if cwTestWordComplete(w, revealed) {
				continue
			}
			missing := false
			for i := 0; i < len(w); i++ {
				if !revealed[w[i]] {
					missing = true
					break
				}
			}
			if !missing {
				t.Errorf("%s seed=%d: word %s incomplete yet all letters present", tk.Slug, seed, w)
			}
		}
	}
}

// --- Card interface / mechanics ----------------------------------------------

// TestCwResolvedOnlyWhenAllRevealed checks Resolved() flips only on full reveal.
func TestCwResolvedOnlyWhenAllRevealed(t *testing.T) {
	tk := cwTicket4x4()
	c := buildCwCard(&tk, 25, 7)
	if c.Resolved() {
		t.Fatal("Resolved() true before any scratch")
	}
	// Reveal all but one panel.
	for i := 0; i < len(c.grid.Panels)-1; i++ {
		c.grid.Panels[i].Hidden = false
		c.grid.Panels[i].Layers = 0
	}
	if c.Resolved() {
		t.Fatal("Resolved() true with one panel still hidden")
	}
	c.ScratchAll()
	if !c.Resolved() {
		t.Fatal("Resolved() false after ScratchAll")
	}
}

// TestCwNoSpoilerBeforeFullReveal asserts Win() stays 0 until the card resolves,
// even on a winning card (no early outcome reveal).
func TestCwNoSpoilerBeforeFullReveal(t *testing.T) {
	tk := cwTicket4x4()
	c := buildCwCard(&tk, 100, 3)
	if c.Win() != 0 {
		t.Fatal("Win() nonzero before resolution (spoiler)")
	}
	c.ScratchAll()
	if c.Win() != 100 {
		t.Fatalf("Win()=%d after full reveal, want 100", c.Win())
	}
}

// TestCwTitleAndPrompt checks Title()/Prompt() are non-empty in both states.
func TestCwTitleAndPrompt(t *testing.T) {
	tk := cwTicket4x4()
	for _, win := range []int{0, 25} {
		c := buildCwCard(&tk, win, 11)
		if c.Title() == "" {
			t.Error("empty Title()")
		}
		if c.Prompt() == "" {
			t.Error("empty Prompt() pre-scratch")
		}
		c.ScratchAll()
		if c.Prompt() == "" {
			t.Error("empty Prompt() post-resolve")
		}
	}
}

// TestCwRenderNoPanic renders mid-play and resolved without panicking.
func TestCwRenderNoPanic(t *testing.T) {
	for _, mk := range []func() Ticket{cwTicket4x4, cwTicket5x4} {
		tk := mk()
		for _, win := range []int{0, 50} {
			c := buildCwCard(&tk, win, 5)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s win=%d: mid render panicked: %v", tk.Slug, win, r)
					}
				}()
				f := kit.NewFrame()
				c.Render(f, 3)
			}()
			c.ScratchAll()
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s win=%d: resolved render panicked: %v", tk.Slug, win, r)
					}
				}()
				f := kit.NewFrame()
				c.Render(f, 3)
			}()
		}
	}
}

// TestCwMoveScratch drives the coin via the Card interface.
func TestCwMoveScratch(t *testing.T) {
	tk := cwTicket4x4()
	c := buildCwCard(&tk, 25, 9)
	c.Move(1, 0)
	if c.grid.Cur != 1 {
		t.Fatalf("after Move(1,0) Cur=%d, want 1", c.grid.Cur)
	}
	for c.grid.Panels[c.grid.Cur].Hidden {
		c.Scratch()
	}
	if c.grid.Panels[1].Hidden {
		t.Fatal("panel 1 still hidden after scratching")
	}
	if c.Resolved() {
		t.Fatal("Resolved() too early")
	}
}
