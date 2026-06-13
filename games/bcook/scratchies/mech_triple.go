package main

import "math/rand"

// mech_triple.go — STUB. Replaced by an agent with the real Triple Word engine:
// scratch letter tiles to spell listed bonus words; a 3× tile triples a word.

func init() {
	builders[MechTriple] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return newGenericGridCard(t, out, rng)
	}
}
