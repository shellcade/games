package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// lastPostFor returns the most recent posted result row for the given account
// id across all posts recorded by the test room, if any.
func lastPostFor(posts []kit.Result, accountID string) (kit.PlayerResult, bool) {
	var got kit.PlayerResult
	found := false
	for _, res := range posts {
		for _, row := range res.Rankings {
			if row.Player.AccountID == accountID {
				got = row
				found = true
			}
		}
	}
	return got, found
}

// TestKillsPostToLeaderboard drives a ship to register a kill and asserts the
// kill count reaches the leaderboard live (Metric>0, StatusFinished), then
// fires OnLeave and asserts a disconnect post carries the kill count with
// kit.StatusDNF — so a mid-game leave still records progress.
func TestKillsPostToLeaderboard(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)

	s := rm.ships[a.AccountID]
	s.alive = true
	s.invulnUntil = tr.Clock
	s.x, s.y, s.heading = 40, 11, 0 // facing east
	s.kills = 0

	// One small crater dead ahead; clear the rest so nothing else interferes.
	rm.craters = []crater{{x: 45, y: 11, size: 1}}

	rm.fire(tr, a, s)
	for i := 0; i < 10 && s.kills == 0; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if s.kills < killCrater {
		t.Fatalf("expected crater kill credit, kills=%d", s.kills)
	}

	// A live post must have reached the leaderboard with the kill count.
	live, ok := lastPostFor(tr.Posted, a.AccountID)
	if !ok {
		t.Fatalf("no leaderboard post for %s after a kill", a.AccountID)
	}
	if live.Metric <= 0 {
		t.Fatalf("live post metric = %d, want > 0", live.Metric)
	}
	if live.Status != kit.StatusFinished {
		t.Fatalf("live post status = %v, want StatusFinished", live.Status)
	}

	before := len(tr.Posted)
	rm.OnLeave(tr, a)

	if len(tr.Posted) <= before {
		t.Fatalf("OnLeave did not post; posts before=%d after=%d", before, len(tr.Posted))
	}
	dnf, ok := lastPostFor(tr.Posted, a.AccountID)
	if !ok {
		t.Fatalf("no leaderboard post for %s after OnLeave", a.AccountID)
	}
	if dnf.Status != kit.StatusDNF {
		t.Fatalf("disconnect post status = %v, want StatusDNF", dnf.Status)
	}
	if dnf.Metric <= 0 {
		t.Fatalf("disconnect post metric = %d, want the kill count > 0", dnf.Metric)
	}
}
