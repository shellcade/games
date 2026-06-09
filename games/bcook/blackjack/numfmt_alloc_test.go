package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
)

// TestComposeZeroAlloc guards the render hot path: composing a realistic
// mid-game frame must do ZERO heap allocations. Under -gc=leaking every alloc
// in a render callback leaks for the room's lifetime, so this is load-bearing.
func TestComposeZeroAlloc(t *testing.T) {
	a, b := mkPlayer("alice"), mkPlayer("bob")
	rm, tr := newGame(t, a, b)
	rm.what = pendNone
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	sa := rm.seats[a.AccountID]
	sb := rm.seats[b.AccountID]
	sa.placed, sb.placed = true, true
	sa.chips, sa.highScore = 1234, 5678
	sb.chips, sb.highScore = 90, 4321
	sa.bet, sb.bet = 50, 100
	// alice: a split into two live hands (one soft) so the value-list join,
	// putValueLabel soft/numeric paths, and the multi-hand layout all exercise.
	sa.hands = []*phand{
		{cards: hand{{rankAce, suitSpade}, {6, suitHeart}}, bet: 50}, // soft 17
		{cards: hand{{10, suitClub}, {9, suitDiamond}}, bet: 50},     // 19
	}
	// bob: a single resolved hand.
	sb.hands = []*phand{{cards: hand{{10, suitHeart}, {7, suitClub}}, bet: 100}}
	rm.dealer = hand{{10, suitSpade}, {7, suitHeart}}
	rm.dealerHole = true // "shows N" dealer label path
	rm.phase = phTurns
	rm.beginTurn(tr) // arm the turn so remaining() > 0 and alice is active

	f := kit.NewFrame()
	// Warm up (first call may touch lazily-initialised frame state).
	rm.frame.Clear()
	rm.compose(f, a)

	allocs := testing.AllocsPerRun(100, func() {
		rm.frame.Clear()
		rm.compose(f, a)
	})
	if allocs != 0 {
		t.Fatalf("compose allocates %.0f/call (want 0) for the active viewer", allocs)
	}

	// Also gate the spectator-of-another-seat view (the "waiting on %s..." and
	// chips/HI right-anchored paths for bob as viewer).
	allocsB := testing.AllocsPerRun(100, func() {
		rm.frame.Clear()
		rm.compose(f, b)
	})
	if allocsB != 0 {
		t.Fatalf("compose allocates %.0f/call (want 0) for the spectator viewer", allocsB)
	}
}

// TestComposeBettingZeroAlloc gates the betting-phase compose path (bet labels,
// "$%d" chips, "waiting on N players..." action bar).
func TestComposeBettingZeroAlloc(t *testing.T) {
	a, b := mkPlayer("alice"), mkPlayer("bob")
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	rm.seats[a.AccountID].placed = true // alice placed, bob hasn't -> "waiting on 1 player"
	rm.seats[a.AccountID].bet = 50

	f := kit.NewFrame()
	rm.frame.Clear()
	rm.compose(f, a)

	allocs := testing.AllocsPerRun(100, func() {
		rm.frame.Clear()
		rm.compose(f, a)
	})
	if allocs != 0 {
		t.Fatalf("betting compose allocates %.0f/call (want 0)", allocs)
	}
}
