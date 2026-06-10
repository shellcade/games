package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

func newGame(t *testing.T, ids ...string) (*kittest.Room, *room) {
	t.Helper()
	players := make([]kit.Player, len(ids))
	for i, id := range ids {
		players[i] = kittest.Player(id)
	}
	r := kittest.NewRoom(players...)
	rm, ok := (Game{}).NewRoom(r.Config(), r.Services()).(*room)
	if !ok {
		t.Fatal("NewRoom did not return *room")
	}
	rm.OnStart(r)
	for _, p := range players {
		rm.OnJoin(r, p)
	}
	return r, rm
}

func TestJoinCreatesBoard(t *testing.T) {
	_, rm := newGame(t, "p1")
	b := rm.boards["p1"]
	if b == nil {
		t.Fatal("no board created on join")
	}
	if b.phase != phReady {
		t.Errorf("fresh board phase = %v, want ready", b.phase)
	}
}

func TestWakeAdvancesBits(t *testing.T) {
	r, rm := newGame(t, "p1")
	b := rm.boards["p1"]
	b.launch(r.Rand())
	y0 := b.balls[0].y
	for i := 0; i < 5; i++ {
		r.Advance(50_000_000) // 50ms in ns
		rm.OnWake(r)
	}
	if len(b.balls) > 0 && b.balls[0].y == y0 {
		t.Error("the bit did not move across several wakes")
	}
}

func TestBestPersistsAndPosts(t *testing.T) {
	r, rm := newGame(t, "p1")
	rm.boards["p1"].score = 750
	rm.OnWake(r) // banks the high score, posts it, persists the wallet

	store := r.Services().Accounts.For(kittest.Player("p1")).Store()
	if got, _ := kvInt(store, "best"); got != 750 {
		t.Errorf("persisted best = %d, want 750", got)
	}
	if len(r.Posted) == 0 {
		t.Error("a new high score did not reach the leaderboard")
	}

	// The score survives a leave + rejoin (durable per account).
	rm.OnLeave(r, kittest.Player("p1"))
	rm.OnJoin(r, kittest.Player("p1"))
	if b := rm.boards["p1"]; b == nil || b.best != 750 {
		t.Errorf("rejoined board best = %v, want 750", b)
	}
}

func TestMembersGetIndependentBoards(t *testing.T) {
	_, rm := newGame(t, "p1", "p2")
	if len(rm.boards) != 2 {
		t.Fatalf("boards = %d, want 2", len(rm.boards))
	}
	rm.boards["p1"].score = 100
	if rm.boards["p2"].score != 0 {
		t.Error("boards are not independent — p2 inherited p1's score")
	}
}
