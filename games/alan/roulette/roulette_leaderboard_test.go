package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// lastPostedMetric returns the metric of the most recent leaderboard post for
// the given account in r.Posted, or (0, false) if the player has no post.
func lastPostedMetric(posted []kit.Result, id string) (int, bool) {
	metric, ok := 0, false
	for _, res := range posted {
		for _, rk := range res.Rankings {
			if rk.Player.AccountID == id {
				metric, ok = rk.Metric, true
			}
		}
	}
	return metric, ok
}

// settleStraight drives one seat through a single Wager -> forced result ->
// Settle for a whole-stake straight bet on number n, returning the seat.
func settleStraight(t *testing.T, r *kittest.Room, rm *room, id string, n int, chips int) *player {
	t.Helper()
	pl := rm.players[id]
	setCursorNumber(rm, id, n)
	pl.stakeIdx = defaultStakeIdx // chip 10
	pl.bets = nil
	for i := 0; i < chips; i++ {
		rm.placeBet(pl)
	}
	if err := r.Services().Credits.Wager(pl.p, int64(pl.staked())); err != nil {
		t.Fatalf("wager: %v", err)
	}
	pl.wagered = true
	rm.result = n // force the outcome so we control win/loss deterministically
	rm.settle(r)
	return pl
}

// TestLeaderboardPostsCreditsPeak locks in the converted board: the metric is
// the account-wide Credits balance, posted only when it sets a new personal high
// after a Settle; a losing round (a lower balance) never regresses the board.
func TestLeaderboardPostsCreditsPeak(t *testing.T) {
	r, rm := newGame(t, "p1")

	// A winning round: whole 100 on straight 20 that hits -> balance jumps to
	// seed - 100 + 100*36, which posts as the new peak.
	settleStraight(t, r, rm, "p1", 20, 10)
	wantPeak := int64(seedBal - 100 + 100*36)
	got, ok := lastPostedMetric(r.Posted, "p1")
	if !ok {
		t.Fatal("winning round did not post a leaderboard peak")
	}
	if int64(got) != wantPeak {
		t.Errorf("posted peak = %d, want %d (account-wide credits)", got, wantPeak)
	}

	// A losing round afterwards drops the balance well below the peak; it must
	// NOT post a regressed metric.
	r.Posted = nil
	// Advance into a fresh betting window then bet-and-miss.
	rm.enterBetting(r)
	pl := rm.players["p1"]
	setCursorNumber(rm, "p1", 5)
	pl.stakeIdx = defaultStakeIdx
	rm.placeBet(pl) // 10 on straight 5
	if err := r.Services().Credits.Wager(pl.p, int64(pl.staked())); err != nil {
		t.Fatalf("wager: %v", err)
	}
	pl.wagered = true
	rm.result = 6 // 5 loses
	rm.settle(r)
	if _, ok := lastPostedMetric(r.Posted, "p1"); ok {
		t.Errorf("a losing round regressed the leaderboard: %+v", r.Posted)
	}
}
