package main

import "math/rand"

// mech_crossword.go — STUB. Replaced by an agent with the real Cashword engine:
// scratch a bank of letters; complete enough listed words to win.

func init() {
	builders[MechCrossword] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return newGenericGridCard(t, out, rng)
	}
}
