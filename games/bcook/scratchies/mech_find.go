package main

import "math/rand"

// mech_find.go — STUB. Replaced by Agent D with the real find-the-symbol engine
// (find 3 targets; optional BUST; SPEC §5.4, AB-8).

func init() {
	builders[MechFind] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return newGenericGridCard(t, out, rng)
	}
}
