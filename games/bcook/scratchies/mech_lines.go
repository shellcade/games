package main

import "math/rand"

// mech_lines.go — STUB. Replaced by an agent with the real Lucky Lines engine:
// three equal cash amounts in a line (row/column/diagonal) wins that amount.

func init() {
	builders[MechLines] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return newGenericGridCard(t, out, rng)
	}
}
