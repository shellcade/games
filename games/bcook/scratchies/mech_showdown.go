package main

import "math/rand"

// mech_showdown.go — STUB. Replaced by an agent with the real Showdown engine:
// your value vs the house, column by column; beat it to win that column.

func init() {
	builders[MechShowdown] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return newGenericGridCard(t, out, rng)
	}
}
