package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// dnfPost returns the first posted result whose single ranking is for player p
// with StatusDNF, or nil.
func dnfPost(tr *kittest.Room, p kit.Player) *kit.PlayerResult {
	for i := range tr.Posted {
		for j := range tr.Posted[i].Rankings {
			pr := &tr.Posted[i].Rankings[j]
			if pr.Player.AccountID == p.AccountID && pr.Status == kit.StatusDNF {
				return pr
			}
		}
	}
	return nil
}

// TestOnLeaveFlushesSoloScore: a solo player who disconnects mid-game (before
// any crash) has their current score recorded with StatusDNF.
func TestOnLeaveFlushesSoloScore(t *testing.T) {
	a := kittest.Player("a")
	rm, tr := newRoom(t, a)

	// Drive snake1's head onto the food so it scores 10 this tick. Park snake2
	// out of the way along row 2.
	rm.snake1 = []Point{{X: 10, Y: 9}, {X: 9, Y: 9}}
	rm.entityDir1 = Point{X: 1, Y: 0}
	rm.lastMovedDir1 = rm.entityDir1
	rm.snake2 = []Point{{X: 30, Y: 2}, {X: 31, Y: 2}}
	rm.entityDir2 = Point{X: -1, Y: 0}
	rm.lastMovedDir2 = Point{X: -1, Y: 0}
	rm.food = Point{X: 11, Y: 9}

	rm.tick(tr)

	if rm.score1 != 10 {
		t.Fatalf("setup: snake1 should have scored 10, got %d", rm.score1)
	}
	if rm.gameOver {
		t.Fatal("setup: game should still be running")
	}

	// Player disconnects mid-game.
	rm.OnLeave(tr, a)

	pr := dnfPost(tr, a)
	if pr == nil {
		t.Fatal("expected a StatusDNF leaderboard Post for the leaving player")
	}
	if pr.Metric != 10 {
		t.Fatalf("DNF post should carry the current score 10, got %d", pr.Metric)
	}
}

// TestOnLeaveFlushesHeadToHeadScore: in a two-player game, the leaving player's
// own current score is flushed with StatusDNF.
func TestOnLeaveFlushesHeadToHeadScore(t *testing.T) {
	a, b := kittest.Player("a"), kittest.Player("b")
	rm, tr := newRoom(t, a, b)

	// Drive snake2 (player b) onto the food; park snake1 out of the way.
	rm.snake1 = []Point{{X: 30, Y: 2}, {X: 31, Y: 2}}
	rm.entityDir1 = Point{X: -1, Y: 0}
	rm.lastMovedDir1 = Point{X: -1, Y: 0}
	rm.snake2 = []Point{{X: 10, Y: 9}, {X: 9, Y: 9}}
	rm.entityDir2 = Point{X: 1, Y: 0}
	rm.lastMovedDir2 = rm.entityDir2
	rm.food = Point{X: 11, Y: 9}

	rm.tick(tr)

	if rm.score2 != 10 {
		t.Fatalf("setup: snake2 should have scored 10, got %d", rm.score2)
	}

	rm.OnLeave(tr, b)

	pr := dnfPost(tr, b)
	if pr == nil {
		t.Fatal("expected a StatusDNF leaderboard Post for leaving player b")
	}
	if pr.Metric != 10 {
		t.Fatalf("DNF post for b should carry score 10, got %d", pr.Metric)
	}
}
