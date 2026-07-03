package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// newGame spins up a room handler over a kittest double with the given seats
// already joined, and returns the concrete *room for white-box assertions.
func newGame(t *testing.T, ids ...string) (*kittest.Room, *room) {
	t.Helper()
	players := make([]kit.Player, len(ids))
	for i, id := range ids {
		players[i] = kittest.Player(id)
	}
	r := kittest.NewRoom(players...)
	// Mirror the declared per-seat payout ceiling so the double exercises the
	// same clamp the host applies.
	r.CreditsMaxPayoutMultiplier = maxPayoutMult
	h := Game{}.NewRoom(r.Config(), r.Services())
	rm, ok := h.(*room)
	if !ok {
		t.Fatal("NewRoom did not return *room")
	}
	rm.OnStart(r)
	for _, p := range players {
		rm.OnJoin(r, p)
	}
	return r, rm
}

// setCursorNumber drives the player's cursor onto the straight bet for number n
// (master bet n) directly (navigation itself is covered in board_test.go).
func setCursorNumber(rm *room, id string, n int) {
	rm.players[id].sel = selection{spot: n}
}

// seedBal is the kittest credits double's first-touch balance (its default).
const seedBal = 1000

func TestJoinSeedsBalance(t *testing.T) {
	r, rm := newGame(t, "p1")
	if rm.players["p1"].bal != seedBal {
		t.Errorf("fresh player balance = %d, want %d", rm.players["p1"].bal, seedBal)
	}
	if got := r.Credits["p1"]; got != seedBal {
		t.Errorf("credits ledger = %d, want %d", got, seedBal)
	}
	if rm.phase != phBetting {
		t.Errorf("phase = %q, want betting", rm.phase)
	}
}

// TestPlaceUndoClear verifies chip placement is LOCAL bookkeeping only: nothing
// is escrowed during betting (the ledger never moves), and undo/clear are pure
// list edits — availability tracks bal minus staked.
func TestPlaceUndoClear(t *testing.T) {
	r, rm := newGame(t, "p1")
	pl := rm.players["p1"]
	setCursorNumber(rm, "p1", 17) // straight on 17 is the armed bet (chip 10)

	rm.placeBet(pl)
	if pl.staked() != 10 || len(pl.bets) != 1 {
		t.Fatalf("after place: staked=%d bets=%d", pl.staked(), len(pl.bets))
	}
	rm.adjustStake(pl, +1) // chip 25
	rm.placeBet(pl)
	if pl.staked() != 35 || pl.avail() != seedBal-35 {
		t.Fatalf("after second place: staked=%d avail=%d", pl.staked(), pl.avail())
	}
	rm.undoBet(pl) // drop the 25
	if pl.staked() != 10 || len(pl.bets) != 1 {
		t.Fatalf("after undo: staked=%d bets=%d", pl.staked(), len(pl.bets))
	}
	rm.clearBets(pl) // drop the rest
	if pl.staked() != 0 || len(pl.bets) != 0 {
		t.Fatalf("after clear: staked=%d bets=%d", pl.staked(), len(pl.bets))
	}
	// The ledger never moved through any of the betting-phase edits — no Wager
	// was taken and no escrow was opened.
	if got := r.Credits["p1"]; got != seedBal {
		t.Errorf("betting-phase edits touched the ledger: %d, want %d", got, seedBal)
	}
	if len(r.CreditsStakes) != 0 {
		t.Errorf("betting-phase edits opened an escrow: %+v", r.CreditsStakes)
	}
}

func TestStakeClamp(t *testing.T) {
	_, rm := newGame(t, "p1")
	pl := rm.players["p1"]
	pl.bal = 30 // can cover tier 10 and 25, not 50/100
	pl.stakeIdx = len(stakeTiers) - 1
	rm.clampStake(pl)
	if int64(stakeTiers[pl.stakeIdx]) > pl.avail() {
		t.Errorf("clamped stake %d exceeds available %d", stakeTiers[pl.stakeIdx], pl.avail())
	}
	// Cannot place a bet you can't afford.
	pl.bal = 5
	pl.bets = nil
	pl.stakeIdx = defaultStakeIdx // chip 10 > 5 available
	setCursorNumber(rm, "p1", 1)
	rm.placeBet(pl)
	if len(pl.bets) != 0 {
		t.Errorf("placed a bet with insufficient balance")
	}
}

// TestFiveDollarFloor locks in the 5-chip floor: a player whittled down to a few
// dollars clamps to the 5 chip and can still bet (then bust into the re-buy)
// rather than being stuck unable to afford the 10.
func TestFiveDollarFloor(t *testing.T) {
	_, rm := newGame(t, "p1")
	pl := rm.players["p1"]
	pl.bal = 5
	rm.clampStake(pl) // every betting window re-clamps the chip to the balance
	if got := stakeTiers[pl.stakeIdx]; got != 5 {
		t.Fatalf("chip clamped to %d on a 5 balance, want the 5 floor", got)
	}
	setCursorNumber(rm, "p1", 17)
	rm.placeBet(pl)
	if len(pl.bets) != 1 || pl.avail() != 0 {
		t.Fatalf("could not bet the last 5: bets=%d avail=%d", len(pl.bets), pl.avail())
	}
}

// TestRoundSettles drives a full betting -> spin -> settle cycle and checks the
// outcome math against the rolled pocket (whatever the seeded RNG produces).
func TestRoundSettles(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	p1, p2 := rm.players["p1"], rm.players["p2"]

	// p1 backs RED for 25; p2 backs a straight on 7 for 10.
	setCursorOutside(rm, "p1", kRed)
	rm.adjustStake(p1, +1) // chip 25
	rm.placeBet(p1)
	p1Bet := masterBets[p1.bets[0].master]

	setCursorNumber(rm, "p2", 7)
	rm.placeBet(p2) // chip 10
	p2Bet := masterBets[p2.bets[0].master]

	// Both ready up -> grace -> spin. The single per-seat Wager is taken now.
	rm.toggleReady(r, p1)
	rm.toggleReady(r, p2)
	if !rm.closing {
		t.Fatal("table did not arm the early close after all ready")
	}
	r.Advance(gracePeriod + 100*time.Millisecond)
	rm.OnWake(r)
	if rm.phase != phSpinning {
		t.Fatalf("phase = %q, want spinning", rm.phase)
	}
	result := rm.result
	// Exactly one open escrow per seat, equal to the whole board staked.
	if r.CreditsStakes["p1"] != 25 || r.CreditsStakes["p2"] != 10 {
		t.Fatalf("open stakes = %+v, want p1:25 p2:10", r.CreditsStakes)
	}
	// The escrow left the wagerable balance.
	if r.Credits["p1"] != seedBal-25 || r.Credits["p2"] != seedBal-10 {
		t.Fatalf("post-wager ledger = p1:%d p2:%d", r.Credits["p1"], r.Credits["p2"])
	}

	// Let the wheel finish; settle.
	r.Advance(spinDur + 100*time.Millisecond)
	rm.OnWake(r)
	if rm.phase != phResults {
		t.Fatalf("phase = %q after spin, want results", rm.phase)
	}

	// Each seat's stake is closed exactly once with its gross; the escrow clears.
	if len(r.CreditsStakes) != 0 {
		t.Errorf("escrow leaked after settle: %+v", r.CreditsStakes)
	}
	wantBal1 := int64(seedBal - 25 + settleReturn(p1Bet, 25, result))
	wantBal2 := int64(seedBal - 10 + settleReturn(p2Bet, 10, result))
	if r.Credits["p1"] != wantBal1 {
		t.Errorf("p1 ledger = %d, want %d (result %d, RED bet)", r.Credits["p1"], wantBal1, result)
	}
	if r.Credits["p2"] != wantBal2 {
		t.Errorf("p2 ledger = %d, want %d (result %d, straight 7)", r.Credits["p2"], wantBal2, result)
	}
	if len(rm.history) != 1 || rm.history[0] != result {
		t.Errorf("history = %v, want [%d]", rm.history, result)
	}

	// A peak gain reaches the leaderboard.
	if r.Credits["p1"] > seedBal { // p1 won on RED this round
		if len(r.Posted) == 0 {
			t.Error("a new peak did not post to the leaderboard")
		}
	}
}

// TestPlacingChipCancelsEarlyClose guards the grace beat: a player who places
// another chip while the early close is armed is un-readied AND the close backs
// out (the wheel must never spin out from under someone still betting).
func TestPlacingChipCancelsEarlyClose(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	p1, p2 := rm.players["p1"], rm.players["p2"]
	setCursorNumber(rm, "p1", 5)
	rm.placeBet(p1)
	rm.toggleReady(r, p1)
	rm.toggleReady(r, p2)
	if !rm.closing {
		t.Fatal("early close not armed after all ready")
	}
	// p2 drops a chip during the grace beat.
	setCursorNumber(rm, "p2", 8)
	rm.placeBet(p2)
	if p2.ready {
		t.Error("placing a chip did not un-ready the player")
	}
	if rm.closing {
		t.Error("placing a chip did not cancel the early close")
	}
	if rm.pendAt != rm.deadline {
		t.Error("the betting-window deadline was not restored")
	}
	// The grace instant passing must NOT spin now.
	r.Advance(gracePeriod + 100*time.Millisecond)
	rm.OnWake(r)
	if rm.phase != phBetting {
		t.Fatalf("phase = %q after cancelled grace, want betting", rm.phase)
	}
	// Re-readying spins as usual.
	rm.toggleReady(r, p2)
	if !rm.closing {
		t.Fatal("early close not re-armed after re-ready")
	}
	r.Advance(gracePeriod + 100*time.Millisecond)
	rm.OnWake(r)
	if rm.phase != phSpinning {
		t.Fatalf("phase = %q, want spinning", rm.phase)
	}
}

// TestRebuyOnBust confirms a wiped-out player is topped up by the platform
// Buyback after settlement (default rebuy amount 1000).
func TestRebuyOnBust(t *testing.T) {
	r, rm := newGame(t, "p1")
	pl := rm.players["p1"]
	// Start this seat at exactly 100 in the ledger and cache.
	r.Credits["p1"] = 100
	pl.bal = 100
	// Stake the whole 100 on a single straight that will miss unless the wheel
	// lands on it; then resolve and check either a clean win or a re-buy.
	setCursorNumber(rm, "p1", 33)
	pl.stakeIdx = defaultStakeIdx // chip 10
	for i := 0; i < 10; i++ {     // 10 x 10 = the whole 100 on straight 33
		rm.placeBet(pl)
	}
	if pl.staked() != 100 {
		t.Fatalf("expected 100 staked, got %d", pl.staked())
	}
	rm.toggleReady(r, pl)
	r.Advance(gracePeriod + 100*time.Millisecond)
	rm.OnWake(r)
	r.Advance(spinDur + 100*time.Millisecond)
	rm.OnWake(r)
	if len(r.CreditsStakes) != 0 {
		t.Fatalf("escrow leaked: %+v", r.CreditsStakes)
	}
	if rm.result == 33 {
		if pl.bal != 100*36 { // 3600 gross, no rebuy
			t.Errorf("won straight 33 but balance = %d, want %d", pl.bal, 100*36)
		}
		if r.CreditsRebuys["p1"] != 0 {
			t.Errorf("a winner should not rebuy: %d", r.CreditsRebuys["p1"])
		}
	} else {
		// Busted to 0 -> Buyback tops up to the default rebuy amount (1000).
		if pl.bal != 1000 {
			t.Errorf("busted but balance = %d, want re-buy 1000", pl.bal)
		}
		if r.CreditsRebuys["p1"] != 1 {
			t.Errorf("expected exactly one Buyback, got %d", r.CreditsRebuys["p1"])
		}
	}
}

// TestLeaveRefundsDuringBetting checks an open-window departure drops its chips
// with no ledger movement (pre-Wager, so there is no escrow to close).
func TestLeaveRefundsDuringBetting(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	pl := rm.players["p1"]
	setCursorNumber(rm, "p1", 5)
	rm.placeBet(pl)
	rm.placeBet(pl)
	rm.OnLeave(r, pl.p)
	// No Wager was taken, so the ledger is untouched at the seed and no stake
	// is left open.
	if got := r.Credits["p1"]; got != seedBal {
		t.Errorf("ledger after refunded leave = %d, want %d", got, seedBal)
	}
	if len(r.CreditsStakes) != 0 {
		t.Errorf("betting-phase leave left an escrow: %+v", r.CreditsStakes)
	}
	if _, ok := rm.players["p1"]; ok {
		t.Error("player not removed on leave")
	}
}

// TestLeaveMidSpinSettlesEscrow guards the money path: a seat that Wagered and
// then leaves while the wheel is spinning (open escrow) must be Settled so the
// stake never leaks.
func TestLeaveMidSpinSettlesEscrow(t *testing.T) {
	r, rm := newGame(t, "p1", "p2") // p2 keeps the table from closing on p1 leave
	pl := rm.players["p1"]
	setCursorNumber(rm, "p1", 7)
	rm.placeBet(pl) // 10 on straight 7
	rm.toggleReady(r, pl)
	rm.toggleReady(r, rm.players["p2"]) // both ready -> grace -> spin
	r.Advance(gracePeriod + 100*time.Millisecond)
	rm.OnWake(r)
	if rm.phase != phSpinning || !pl.wagered {
		t.Fatalf("setup: phase=%q wagered=%v", rm.phase, pl.wagered)
	}
	if r.CreditsStakes["p1"] != 10 {
		t.Fatalf("open escrow = %d, want 10", r.CreditsStakes["p1"])
	}
	// Leave mid-spin: the open escrow is Settled at the fair gross for the rolled
	// result (the win pays 10*36; a miss books the loss). Either way it clears.
	want := int64(seedBal - 10 + settleReturn(masterBets[7], 10, rm.result))
	rm.OnLeave(r, pl.p)
	if _, open := r.CreditsStakes["p1"]; open {
		t.Errorf("escrow leaked on mid-spin leave: %+v", r.CreditsStakes)
	}
	if got := r.Credits["p1"]; got != want {
		t.Errorf("mid-spin leave ledger = %d, want %d (result %d)", got, want, rm.result)
	}
}

// TestTopPrizeDoesNotClamp proves the declared MaxPayoutMultiplier (36) covers
// the richest wager: a whole-stake straight-up that hits pays the full stake*36
// with no truncation by either the game clamp or the host double.
func TestTopPrizeDoesNotClamp(t *testing.T) {
	r, rm := newGame(t, "p1")
	pl := rm.players["p1"]
	setCursorNumber(rm, "p1", 20)
	pl.stakeIdx = defaultStakeIdx
	for i := 0; i < 10; i++ { // 100 on straight 20
		rm.placeBet(pl)
	}
	staked := int64(pl.staked())
	// Take the one escrow, force the wheel onto 20 (the 35:1 jackpot), settle.
	if err := r.Services().Credits.Wager(pl.p, staked); err != nil {
		t.Fatalf("wager: %v", err)
	}
	pl.wagered = true
	rm.result = 20
	rm.settle(r)
	want := seedBal - staked + staked*36 // full 36x, un-clamped
	if got := r.Credits["p1"]; got != want {
		t.Fatalf("top prize clamped: ledger = %d, want %d", got, want)
	}
}

// --- helpers ---------------------------------------------------------------

func setCursorOutside(rm *room, id string, k betKind) {
	for i, b := range masterBets {
		if b.outside && b.kind == k {
			rm.players[id].sel = selection{spot: i}
			return
		}
	}
}
