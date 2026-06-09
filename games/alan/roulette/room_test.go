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

func TestJoinSeedsWallet(t *testing.T) {
	_, rm := newGame(t, "p1")
	if rm.players["p1"].balance != startBalance {
		t.Errorf("fresh player balance = %d, want %d", rm.players["p1"].balance, startBalance)
	}
	if rm.phase != phBetting {
		t.Errorf("phase = %q, want betting", rm.phase)
	}
}

func TestPlaceUndoClear(t *testing.T) {
	_, rm := newGame(t, "p1")
	pl := rm.players["p1"]
	setCursorNumber(rm, "p1", 17) // straight on 17 is the armed bet (chip 10)

	rm.placeBet(pl)
	if pl.balance != startBalance-10 || len(pl.bets) != 1 {
		t.Fatalf("after place: balance=%d bets=%d", pl.balance, len(pl.bets))
	}
	rm.adjustStake(pl, +1) // chip 25
	rm.placeBet(pl)
	if pl.balance != startBalance-35 || pl.staked() != 35 {
		t.Fatalf("after second place: balance=%d staked=%d", pl.balance, pl.staked())
	}
	rm.undoBet(pl) // refund the 25
	if pl.balance != startBalance-10 || len(pl.bets) != 1 {
		t.Fatalf("after undo: balance=%d bets=%d", pl.balance, len(pl.bets))
	}
	rm.clearBets(pl) // refund the rest
	if pl.balance != startBalance || len(pl.bets) != 0 {
		t.Fatalf("after clear: balance=%d bets=%d", pl.balance, len(pl.bets))
	}
}

func TestStakeClamp(t *testing.T) {
	_, rm := newGame(t, "p1")
	pl := rm.players["p1"]
	pl.balance = 30 // can cover tier 10 and 25, not 50/100
	pl.stakeIdx = len(stakeTiers) - 1
	rm.clampStake(pl)
	if stakeTiers[pl.stakeIdx] > pl.balance {
		t.Errorf("clamped stake %d exceeds balance %d", stakeTiers[pl.stakeIdx], pl.balance)
	}
	// Cannot place a bet you can't afford.
	pl.balance = 5
	setCursorNumber(rm, "p1", 1)
	rm.placeBet(pl)
	if len(pl.bets) != 0 {
		t.Errorf("placed a bet with insufficient balance")
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

	bal1, bal2 := p1.balance, p2.balance // already net of stakes

	// Both ready up -> grace -> spin.
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

	// Let the wheel finish; settle.
	r.Advance(spinDur + 100*time.Millisecond)
	rm.OnWake(r)
	if rm.phase != phResults {
		t.Fatalf("phase = %q after spin, want results", rm.phase)
	}

	wantBal1 := bal1 + settleReturn(p1Bet, 25, result)
	wantBal2 := bal2 + settleReturn(p2Bet, 10, result)
	if p1.balance != wantBal1 {
		t.Errorf("p1 balance = %d, want %d (result %d, RED bet)", p1.balance, wantBal1, result)
	}
	if p2.balance != wantBal2 {
		t.Errorf("p2 balance = %d, want %d (result %d, straight 7)", p2.balance, wantBal2, result)
	}
	if len(rm.history) != 1 || rm.history[0] != result {
		t.Errorf("history = %v, want [%d]", rm.history, result)
	}

	// Durable wallet persisted, and a peak gain reaches the leaderboard.
	if got, _ := kvInt(walletStore(r, "p1"), keyBalance); got != p1.balance {
		t.Errorf("persisted balance = %d, want %d", got, p1.balance)
	}
	if p1.balance > startBalance { // p1 won on RED this round
		if len(r.Posted) == 0 {
			t.Error("a new peak did not post to the leaderboard")
		}
	}
}

// TestRebuyOnBust confirms a wiped-out player is staked again.
func TestRebuyOnBust(t *testing.T) {
	r, rm := newGame(t, "p1")
	pl := rm.players["p1"]
	pl.balance = 100
	// Stake the whole 100 on a single straight that will miss unless the wheel
	// lands on it; then resolve and check either a clean win or a re-buy.
	setCursorNumber(rm, "p1", 33)
	pl.stakeIdx = 0
	for i := 0; i < 10; i++ { // 10 x 10 = the whole 100 on straight 33
		rm.placeBet(pl)
	}
	if pl.balance != 0 {
		t.Fatalf("expected balance 0 after staking all, got %d", pl.balance)
	}
	rm.toggleReady(r, pl)
	r.Advance(gracePeriod + 100*time.Millisecond)
	rm.OnWake(r)
	r.Advance(spinDur + 100*time.Millisecond)
	rm.OnWake(r)
	if rm.result == 33 {
		if pl.balance != 100*36 {
			t.Errorf("won straight 33 but balance = %d, want %d", pl.balance, 100*36)
		}
	} else if pl.balance != rebuyAmount {
		t.Errorf("busted but balance = %d, want re-buy %d", pl.balance, rebuyAmount)
	}
}

// TestLeaveRefundsDuringBetting checks an open-window departure gets its chips
// back (the round hasn't resolved).
func TestLeaveRefundsDuringBetting(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	pl := rm.players["p1"]
	setCursorNumber(rm, "p1", 5)
	rm.placeBet(pl)
	rm.placeBet(pl)
	if pl.balance != startBalance-20 {
		t.Fatalf("balance before leave = %d", pl.balance)
	}
	rm.OnLeave(r, pl.p)
	// Persisted balance should reflect the refund (back to the full start).
	if got, _ := kvInt(walletStore(r, "p1"), keyBalance); got != startBalance {
		t.Errorf("persisted balance after refunded leave = %d, want %d", got, startBalance)
	}
	if _, ok := rm.players["p1"]; ok {
		t.Error("player not removed on leave")
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

func walletStore(r *kittest.Room, id string) kit.KVStore {
	return r.Services().Accounts.For(kittest.Player(id)).Store()
}
