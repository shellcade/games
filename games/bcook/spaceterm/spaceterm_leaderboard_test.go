package main

import (
	"context"
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// A boarded crew member who disconnects mid-run must bank the run's progress to
// the leaderboard (StatusDNF) and persist their personal best to KV — otherwise
// a whole shift's sectors vanish if everyone beams out before the core dies.
func TestLeaveFlushesRunScore(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")

	// Clear a couple of sectors so the live run metric (sector-1) is positive.
	rm.sector = 4 // three sectors cleared

	rm.OnLeave(r, kittest.Player("p1"))

	if len(r.Posted) == 0 {
		t.Fatal("mid-run leave did not post the run score to the leaderboard")
	}
	res := r.Posted[len(r.Posted)-1]
	if len(res.Rankings) != 1 {
		t.Fatalf("rankings = %d, want only the leaver", len(res.Rankings))
	}
	pr := res.Rankings[0]
	if pr.Player.AccountID != "p1" {
		t.Errorf("posted player = %q, want the leaver p1", pr.Player.AccountID)
	}
	if pr.Metric != 3 {
		t.Errorf("metric = %d, want the 3 sectors cleared so far", pr.Metric)
	}
	if pr.Status != kit.StatusDNF {
		t.Errorf("status = %v, want DNF for a disconnect", pr.Status)
	}

	v, ok, _ := r.Services().Accounts.For(kittest.Player("p1")).Store().Get(context.Background(), "best")
	if !ok || string(v) != "3" {
		t.Errorf("persisted best = %q (ok=%v), want 3", v, ok)
	}
}

// The core-death debrief already posts and persists for everyone. A crew member
// who then beams out from the debrief must not post a second, stale result.
func TestLeaveAfterGameOverDoesNotDoublePost(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	rm.sector = 5
	rm.hull = 1
	r.Advance(15e9) // past any sector's order timer
	rm.OnWake(r)    // core dies -> gameOver posts once
	if rm.phase != phOver {
		t.Fatalf("phase = %v, want debrief", rm.phase)
	}
	posted := len(r.Posted)

	rm.OnLeave(r, kittest.Player("p1"))

	if len(r.Posted) != posted {
		t.Errorf("leave during debrief posted again: %d -> %d", posted, len(r.Posted))
	}
}
