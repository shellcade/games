package main

// Real-frame capture: drives a full tic-tac-toe match (the static/sparse
// scenario — a move changes only a handful of cells). See diffcommon_test.go.
//
//	go test -run TestCaptureFrameSeq ./...

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit"
	"github.com/shellcade/kit/kittest"
)

// TestCaptureFrameSeq drives a complete X-wins match: both seats join, then an
// alternating sequence of moves to a three-in-a-row, with a few idle wakes
// between moves (the host heartbeat fires even when nothing changes — those
// produce no send, which is correct; render-on-change means only real moves
// emit frames). This is the sparse-update path.
func TestCaptureFrameSeq(t *testing.T) {
	r := kittest.NewRoom()
	r.Cfg = kit.RoomConfig{Mode: kit.ModeQuick, Capacity: 2, MinPlayers: 2, Seed: 1, SeedSet: true}
	rm := (Game{}).NewRoom(r.Cfg, r.Services())
	rec := dcNewRecorder(r)

	x, o := dcPlayer("xavier"), dcPlayer("olive")

	rm.OnStart(r)
	rec.drain()
	r.Players = append(r.Players, x)
	rm.OnJoin(r, x)
	rec.drain()
	r.Players = append(r.Players, o)
	rm.OnJoin(r, o)
	rec.drain()

	idle := func() {
		r.Advance(time.Second)
		rm.OnWake(r)
		rec.drain()
	}

	// X wins down the left column: X1, O2, X4, O3, X7.
	moves := []struct {
		p kit.Player
		k rune
	}{
		{x, '1'}, {o, '2'}, {x, '4'}, {o, '3'}, {x, '7'},
	}
	for _, m := range moves {
		rm.OnInput(r, m.p, dcRuneIn(m.k))
		rec.drain()
		idle()
	}

	if len(rec.frames) < 5 {
		t.Fatalf("captured only %d frames", len(rec.frames))
	}
	dcWriteSeq(t, "tic-tac-toe", rec.frames)
}
