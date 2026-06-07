package main

// Per-actor combat PRNGs (spec: all combat randomness derives from the week
// seed — monsters from (week_seed, floor, spawn_index), delvers from
// (week_seed, account, run) — NEVER the wall-clock host RNG, which is
// reserved for per-run cosmetic entropy. This is load-bearing for the
// stage-5 replay determinism.)

// actorSeed mixes the week seed with two actor coordinates (splitmix64).
func actorSeed(week int64, a, b uint64) uint64 {
	z := uint64(week) ^ (a * 0x9E3779B97F4A7C15) ^ (b * 0xBF58476D1CE4E5B9)
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	if z == 0 {
		z = 1 // xorshift must never sit at zero
	}
	return z
}

// fnvHash is FNV-1a over a string (account ids into actor coordinates).
func fnvHash(s string) uint64 {
	h := uint64(1469598103934665603)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// rngNext steps an xorshift64 state in place.
func rngNext(s *uint64) uint64 {
	x := *s
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	*s = x
	return x
}

// roll returns 1..n from the actor PRNG.
func roll(s *uint64, n int) int { return 1 + int(rngNext(s)%uint64(n)) }
