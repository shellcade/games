package main

import "math/rand"

// mech_bingo.go — STUB. Replaced by an agent with the real Quick Bingo engine:
// reveal your card; complete a line of called numbers to win.

func init() {
	builders[MechBingo] = func(t *Ticket, out Outcome, rng *rand.Rand) Card {
		return newGenericGridCard(t, out, rng)
	}
}
