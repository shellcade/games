package main

import (
	"math/rand"
	"testing"

	"github.com/shellcade/kit/v2/kittest"
)

// spinSeq drives n real spins (the same code path the game uses) over a room
// seeded with `seed`, returning the rolled pockets.
func spinSeq(seed int64, n int) []int {
	r := kittest.NewRoom(kittest.Player("p"))
	r.RNG = rand.New(rand.NewSource(seed))
	rm := newRoom(r.Config(), r.Services())
	out := make([]int, n)
	for i := range out {
		rm.startSpin(r)
		out[i] = rm.result
	}
	return out
}

// TestSpinRangeAndCoverage checks every spin lands on a real pocket and that, over
// many spins, all 38 pockets (0, 00, 1..36) come up.
func TestSpinRangeAndCoverage(t *testing.T) {
	seen := map[int]int{}
	for _, n := range spinSeq(1, 38*500) {
		if n < 0 || n >= pockets {
			t.Fatalf("spin produced %d, outside [0,%d)", n, pockets)
		}
		seen[n]++
	}
	if len(seen) != pockets {
		t.Fatalf("only %d/%d pockets ever came up", len(seen), pockets)
	}
	// 00 (the American-only pocket) must appear.
	if seen[doubleZero] == 0 {
		t.Error("00 never came up")
	}
}

// TestSpinUniformity checks the draw is even across the 38 pockets — no pocket is
// favoured or starved. Deterministic (fixed seed); the band is generous (~±15%
// of the mean) so it only fails on a genuinely skewed distribution.
func TestSpinUniformity(t *testing.T) {
	const perPocket = 2000
	counts := make([]int, pockets)
	for _, n := range spinSeq(1, pockets*perPocket) {
		counts[n]++
	}
	lo, hi := perPocket*85/100, perPocket*115/100
	for n, c := range counts {
		if c < lo || c > hi {
			t.Errorf("pocket %s came up %d times, want within [%d,%d]", pocketLabel(n), c, lo, hi)
		}
	}
}

// TestSpinDeterminism checks a seeded room reproduces its sequence (so smoke runs
// and hibernation are stable) and that different seeds diverge.
func TestSpinDeterminism(t *testing.T) {
	a, b := spinSeq(7, 32), spinSeq(7, 32)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("seed 7 not reproducible at spin %d: %d vs %d", i, a[i], b[i])
		}
	}
	c := spinSeq(8, 32)
	same := true
	for i := range a {
		if a[i] != c[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("seeds 7 and 8 produced identical spin sequences")
	}
}
