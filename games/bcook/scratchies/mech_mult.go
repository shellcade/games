package main

import "math/rand"

// mech_mult.go — STUB. Replaced by Agent C with the real multiplier engine
// (prize × multiplier; SPEC §5.3, AB-7).

func init() {
	builders[MechMult] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return newGenericGridCard(t, out, rng)
	}
}
