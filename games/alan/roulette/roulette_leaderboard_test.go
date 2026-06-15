package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// peakOf returns the metric of the most recent peak post for the given account
// in r.Posted, or (0, false) if the player has no post.
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

// TestPeriodicPeakFlush locks in the "constantly saved" guarantee for an
// abandoned table: a seated player whose peak does NOT change is still re-posted
// to the leaderboard on the throttled OnWake interval, so the board reflects a
// still-seated player even when the table is idle mid-round. A wake BEFORE the
// interval elapses must not re-post.
func TestPeriodicPeakFlush(t *testing.T) {
	r, rm := newGame(t, "p1")
	pl := rm.players["p1"]

	// Establish a peak via the normal increase path so the keeper is tracking
	// the player, then clear the recorded posts.
	pl.peak = 4242
	rm.postPeak(r, pl)
	if _, ok := lastPostedMetric(r.Posted, "p1"); !ok {
		t.Fatal("postPeak did not record an initial leaderboard post")
	}
	r.Posted = nil

	// A wake BEFORE the interval elapses must NOT re-post (still inside the
	// open betting window, so no round one-shot fires either).
	r.Advance(peakFlushInterval - time.Second)
	rm.OnWake(r)
	if _, ok := lastPostedMetric(r.Posted, "p1"); ok {
		t.Fatalf("re-posted before the flush interval elapsed: %+v", r.Posted)
	}

	// Crossing the interval (with NO new peak/bet) must re-post the current peak.
	r.Advance(2 * time.Second) // now past peakFlushInterval since last flush
	rm.OnWake(r)
	got, ok := lastPostedMetric(r.Posted, "p1")
	if !ok {
		t.Fatal("periodic flush did not re-post the seated player's peak")
	}
	if got != pl.peak {
		t.Errorf("periodic flush posted metric %d, want current peak %d", got, pl.peak)
	}
}
