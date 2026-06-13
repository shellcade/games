package main

import "math/rand"

// mech_keynum.go — STUB. Replaced by Agent B with the real key-number-match
// engine (SPEC §5.2, AB-6).

func init() {
	builders[MechKeyNum] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return newGenericGridCard(t, out, rng)
	}
}
