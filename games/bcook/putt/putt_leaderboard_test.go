package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
)

// parsum returns the sum of par over hole indices [from, to).
func parsum(from, to int) int {
	t := 0
	for h := from; h < to; h++ {
		t += holes[h].par
	}
	return t
}

// TestLeaveMidRoundPostsParExtrapolatedDNF: a golfer who has completed some
// holes and disconnects mid-round must post a FAIR full-round estimate —
// their actual strokes on completed holes plus par for every remaining hole —
// with Status DNF, never the raw partial total (which on a lower-better board
// would unfairly top the leaderboard).
func TestLeaveMidRoundPostsParExtrapolatedDNF(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]

	// Alice has finished 3 holes (scores committed) and is mid-way through the
	// 4th when she disconnects.
	g.scores = []int{2, 4, 3} // 9 actual strokes on holes 1..3
	rm.holeIdx = 3
	g.strokes = 5 // in-progress strokes on hole 4 — must NOT count as actual

	rm.OnLeave(tr, a)

	if len(tr.Posted) != 1 {
		t.Fatalf("OnLeave should Post exactly one result, got %d", len(tr.Posted))
	}
	res := tr.Posted[0]
	if len(res.Rankings) != 1 {
		t.Fatalf("want 1 ranking, got %d", len(res.Rankings))
	}
	pr := res.Rankings[0]
	// 9 actual + par for holes 4..9 (indices 3..8).
	want := 9 + parsum(3, len(holes))
	if pr.Metric != want {
		t.Fatalf("DNF metric = %d, want %d (9 actual + par fill of remaining holes)", pr.Metric, want)
	}
	if pr.Status != kit.StatusDNF {
		t.Fatalf("status = %v, want StatusDNF", pr.Status)
	}
	if pr.Player.AccountID != a.AccountID {
		t.Fatalf("posted ranking is for the wrong player")
	}
}

// TestLeaveBeforeAnyHolePostsFullPar: a golfer who disconnects having completed
// zero holes must post the full par total of the course (NOT 0), Status DNF.
func TestLeaveBeforeAnyHolePostsFullPar(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	g := rm.golfers[a.AccountID]

	// No holes completed; a few strokes flailed on hole 1 before quitting.
	g.scores = nil
	rm.holeIdx = 0
	g.strokes = 6

	rm.OnLeave(tr, a)

	if len(tr.Posted) != 1 {
		t.Fatalf("OnLeave should Post exactly one result, got %d", len(tr.Posted))
	}
	pr := tr.Posted[0].Rankings[0]
	want := parsum(0, len(holes)) // full course par
	if pr.Metric != want {
		t.Fatalf("DNF metric = %d, want full par total %d (must not be 0)", pr.Metric, want)
	}
	if pr.Metric == 0 {
		t.Fatal("DNF metric must never be 0 - that would top a lower-better board")
	}
	if pr.Status != kit.StatusDNF {
		t.Fatalf("status = %v, want StatusDNF", pr.Status)
	}
}
