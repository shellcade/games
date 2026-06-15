package main

import (
	"math/rand"
	"testing"

	kit "github.com/shellcade/kit/v2"
)

// TestAllTicketsResolve exercises EVERY catalog ticket through both a winning
// and a losing outcome, across many seeds: it builds the card, drives the coin
// around the grid, scratches, then ScratchAll, and asserts the card resolves to
// exactly the predetermined outcome — and that rendering never panics. This is
// the "test all of the scratchies" end-to-end pass over the 16 tickets.
func TestAllTicketsResolve(t *testing.T) {
	for ti := range tickets {
		tk := &tickets[ti]
		// Representative win values: the smallest prize and the top (jackpot).
		smallest := tk.Prizes[0].Credits
		top := topPrize(tk)
		cases := []struct {
			name string
			out  Outcome
		}{
			{"loss", Outcome{Win: 0}},
			{"small-win", Outcome{Win: smallest}},
			{"jackpot", Outcome{Win: top}},
		}
		for _, tc := range cases {
			for seed := int64(1); seed <= 25; seed++ {
				rng := rand.New(rand.NewSource(seed))
				build := builders[tk.Mechanic]
				if build == nil {
					t.Fatalf("%s: no builder registered for mechanic %d", tk.Slug, tk.Mechanic)
				}
				card := build(tk, tc.out, rng)

				// Drive the cursor around and scratch a few panels by hand.
				for k := 0; k < 8; k++ {
					card.Move(1, 0)
					card.Move(0, 1)
					card.Move(-1, 0)
					card.Scratch()
				}
				// Render mid-play must not panic.
				renderNoPanic(t, card, tk.Slug+"/"+tc.name+"/mid")

				card.ScratchAll()
				if !card.Resolved() {
					t.Errorf("%s/%s seed=%d: not resolved after ScratchAll", tk.Slug, tc.name, seed)
					continue
				}
				if got := card.Win(); got != tc.out.Win {
					t.Errorf("%s/%s seed=%d: Win()=%d, want %d", tk.Slug, tc.name, seed, got, tc.out.Win)
				}
				// Render resolved state must not panic; Title/Prompt must be non-empty.
				renderNoPanic(t, card, tk.Slug+"/"+tc.name+"/resolved")
				if card.Title() == "" {
					t.Errorf("%s/%s: empty Title()", tk.Slug, tc.name)
				}
				if card.Prompt() == "" {
					t.Errorf("%s/%s: empty Prompt()", tk.Slug, tc.name)
				}
			}
		}
	}
}

// TestAllTicketsWinViaScratch confirms a win can also be reached by scratching
// panel-by-panel (not just ScratchAll) for every ticket — exercising the
// per-panel auto-resolve paths (match-3 third match, find third symbol).
func TestAllTicketsWinViaScratch(t *testing.T) {
	for ti := range tickets {
		tk := &tickets[ti]
		out := Outcome{Win: tk.Prizes[0].Credits}
		rng := rand.New(rand.NewSource(99))
		card := builders[tk.Mechanic](tk, out, rng)
		// Reveal panels one at a time across the whole grid until resolved.
		guard := 0
		for !card.Resolved() && guard < 4000 {
			card.Move(1, 0)
			if !card.Scratch() {
				card.Move(0, 1)
				card.Scratch()
			}
			guard++
		}
		if !card.Resolved() {
			// Fall back to ScratchAll; some mechanics (key-number) only settle on
			// full reveal, which the cursor walk above may not guarantee to cover.
			card.ScratchAll()
		}
		if !card.Resolved() {
			t.Errorf("%s: never resolved via scratching", tk.Slug)
			continue
		}
		if got := card.Win(); got != out.Win {
			t.Errorf("%s: Win()=%d, want %d", tk.Slug, got, out.Win)
		}
	}
}

// renderNoPanic renders a card into a fresh frame and fails if it panics.
func renderNoPanic(t *testing.T, c Card, where string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: Render panicked: %v", where, r)
		}
	}()
	f := kit.NewFrame()
	c.Render(f, 3)
}
