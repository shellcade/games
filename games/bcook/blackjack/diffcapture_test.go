package main

// Real-frame capture: drives a two-player blackjack round and records every
// sent frame. See diffcommon_test.go for the serialization helpers.
//
//	go test -run TestCaptureFrameSeq ./...

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit"
	"github.com/shellcade/kit/kittest"
)

// TestCaptureFrameSeq drives a realistic two-player blackjack round: two seats
// join, both bet (up/confirm), the deal animates, players hit/stand, the dealer
// reveals and settles, results flash, and a new betting window opens. Wakes are
// stepped at ~30fps through every animation window (the busy path).
func TestCaptureFrameSeq(t *testing.T) {
	r := kittest.NewRoom()
	r.Cfg = kit.RoomConfig{Mode: kit.ModeQuick, Capacity: 5, MinPlayers: 1, Seed: 1, SeedSet: true}
	rm := newRoom(r.Cfg, r.Services())
	rec := dcNewRecorder(r)

	p1, p2 := dcPlayer("alice"), dcPlayer("bob")

	rm.OnStart(r)
	rec.drain()

	r.Players = append(r.Players, p1)
	rm.OnJoin(r, p1)
	rec.drain()
	r.Players = append(r.Players, p2)
	rm.OnJoin(r, p2)
	rec.drain()

	step := func(total time.Duration) {
		const dt = 33 * time.Millisecond
		for elapsed := time.Duration(0); elapsed < total; elapsed += dt {
			r.Advance(dt)
			rm.OnWake(r)
			rec.drain()
		}
	}

	rm.OnInput(r, p1, dcRuneIn('k')) // bet up
	rec.drain()
	rm.OnInput(r, p2, dcRuneIn('k'))
	rec.drain()
	rm.OnInput(r, p1, dcKeyIn(kit.KeyEnter)) // place
	rec.drain()
	rm.OnInput(r, p2, dcKeyIn(kit.KeyEnter)) // place -> grace -> deal
	rec.drain()

	step(3 * time.Second) // grace + deal animation

	rm.OnInput(r, p1, dcRuneIn('h')) // alice hits
	rec.drain()
	step(1 * time.Second)
	rm.OnInput(r, p1, dcRuneIn('s')) // alice stands
	rec.drain()
	step(500 * time.Millisecond)
	rm.OnInput(r, p2, dcRuneIn('s')) // bob stands
	rec.drain()

	step(10 * time.Second) // dealer reveal/draw, settle, results flash, next betting

	if len(rec.frames) < 5 {
		t.Fatalf("captured only %d frames; drive script produced too little", len(rec.frames))
	}
	dcWriteSeq(t, "blackjack", rec.frames)
}
