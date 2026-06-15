package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// lastPostFor returns the most recent posted result whose first ranking is for
// the given account id, or false if none.
func lastPostFor(posted []kit.Result, accountID string) (kit.PlayerResult, bool) {
	for i := len(posted) - 1; i >= 0; i-- {
		for _, pr := range posted[i].Rankings {
			if pr.Player.AccountID == accountID {
				return pr, true
			}
		}
	}
	return kit.PlayerResult{}, false
}

// countPostsFor returns how many posted results reference the account id.
func countPostsFor(posted []kit.Result, accountID string) int {
	n := 0
	for _, res := range posted {
		for _, pr := range res.Rankings {
			if pr.Player.AccountID == accountID {
				n++
			}
		}
	}
	return n
}

// TestSurvivalPostedLive asserts that as the reactor survives, a live result is
// posted to the leaderboard (StatusFinished) carrying the survival seconds.
func TestSurvivalPostedLive(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)

	// Advance the reactor a few seconds; each wake should record the growing
	// survival metric live.
	for i := 0; i < 100 && rm.phase == phaseRunning; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}

	pr, ok := lastPostFor(tr.Posted, a.AccountID)
	if !ok {
		t.Fatal("survival should be posted to the leaderboard during play")
	}
	if pr.Status != kit.StatusFinished {
		t.Fatalf("live survival post should be StatusFinished, got %v", pr.Status)
	}
	if pr.Metric < 1 {
		t.Fatalf("posted survival metric should reflect elapsed seconds, got %d", pr.Metric)
	}
}

// TestSurvivalPostedOnLeave asserts that a crew member who disconnects mid-run
// has their current survival posted with StatusDNF.
func TestSurvivalPostedOnLeave(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)

	// Survive a while, then disconnect.
	for i := 0; i < 60 && rm.phase == phaseRunning; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	want := int(rm.survivedSeconds())
	if want < 1 {
		t.Fatalf("need a non-trivial run before leaving, got %d", want)
	}

	rm.OnLeave(tr, a)

	pr, ok := lastPostFor(tr.Posted, a.AccountID)
	if !ok {
		t.Fatal("leaving mid-run should post the crew member's survival")
	}
	if pr.Status != kit.StatusDNF {
		t.Fatalf("disconnect post should be StatusDNF, got %v", pr.Status)
	}
	if pr.Metric < want {
		t.Fatalf("disconnect should post current survival %d, got %d", want, pr.Metric)
	}
}

// TestPeriodicFlushOnInterval asserts an abandoned-but-ticking reactor keeps
// recording: with crew present, FlushAll posts on a throttled interval, and not
// before that interval has elapsed.
func TestPeriodicFlushOnInterval(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)

	// Pull the flush deadline half a second off the whole-second grid so the
	// boundary (lastFlush + flushInterval) lands mid-second relative to
	// startedAt. The boundary-crossing wake below is then within a single
	// survived-second, so it cannot also produce an OnImprove live post — any new
	// post must be the periodic flush.
	rm.lastFlush = rm.lastFlush.Add(500 * time.Millisecond)

	// One initial wake to seed live tracking / the first post.
	tr.Advance(50 * time.Millisecond)
	rm.OnWake(tr)

	// Tick in small steps until we sit just shy of the flush interval — within
	// the same survived-second as where we'll cross the boundary. Stop as soon as
	// the very next sub-second step would cross flushInterval.
	for rm.phase == phaseRunning {
		if rm.now.Add(50*time.Millisecond).Sub(rm.lastFlush) >= flushInterval {
			break
		}
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}

	// Snapshot right before crossing: the post count and the current whole
	// survived-second. The boundary-crossing wake below is a sub-second step, so
	// the survived second must not change across it — meaning OnImprove can NOT
	// produce a new live post. Any new post is therefore the periodic flush.
	beforeSecs := int(rm.survivedSeconds())
	beforeCount := countPostsFor(tr.Posted, a.AccountID)

	tr.Advance(50 * time.Millisecond)
	rm.OnWake(tr)

	if got := int(rm.survivedSeconds()); got != beforeSecs {
		t.Fatalf("test setup: survived second changed across the boundary wake (%d -> %d)", beforeSecs, got)
	}
	afterCount := countPostsFor(tr.Posted, a.AccountID)
	if afterCount <= beforeCount {
		t.Fatalf("a periodic flush should post once the interval elapses, with no live improvement (before %d, after %d)", beforeCount, afterCount)
	}

	// The flush post carries StatusFinished.
	pr, _ := lastPostFor(tr.Posted, a.AccountID)
	if pr.Status != kit.StatusFinished {
		t.Fatalf("periodic flush post should be StatusFinished, got %v", pr.Status)
	}
}
