package main

import "math/rand"

// mech_match3.go — STUB. Replaced by Agent A with the real match-3 cash engine
// (SPEC §5.1, AB-3/4/5/9/12). For now it registers a generic grid card so the
// package compiles and the game is playable end-to-end.

func init() {
	builders[MechMatch3] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return newGenericGridCard(t, out, rng)
	}
}
