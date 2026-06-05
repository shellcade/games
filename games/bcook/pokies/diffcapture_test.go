package main

// Real-frame capture: drives a pokies machine through several reel spins (the
// busy/animated scenario — the 3x3 reel window scrolls every ~80ms). See
// diffcommon_test.go.
//
//	go test -run TestCaptureFrameSeq ./...

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit"
	"github.com/shellcade/kit/kittest"
)

// TestCaptureFrameSeq drives a single player through three full spins. Each
// spin scrolls the reels (cycleRate = 80ms) and lands them staggered over
// ~1.5s; the wake heartbeat is stepped at ~30fps so every scroll frame the host
// would coalesce/forward is recorded. The reel cycling is the busiest update
// pattern in the catalog.
func TestCaptureFrameSeq(t *testing.T) {
	r := kittest.NewRoom()
	r.Cfg = kit.RoomConfig{Mode: kit.ModeSolo, Capacity: 1, MinPlayers: 1, Seed: 1, SeedSet: true}
	rm := (Game{}).NewRoom(r.Cfg, r.Services())
	rec := dcNewRecorder(r)

	p := dcPlayer("penny")
	rm.OnStart(r)
	rec.drain()
	r.Players = append(r.Players, p)
	rm.OnJoin(r, p)
	rec.drain()

	step := func(total time.Duration) {
		const dt = 33 * time.Millisecond
		for elapsed := time.Duration(0); elapsed < total; elapsed += dt {
			r.Advance(dt)
			rm.OnWake(r)
			rec.drain()
		}
	}

	for spin := 0; spin < 3; spin++ {
		rm.OnInput(r, p, dcKeyIn(kit.KeyEnter)) // pull the lever
		rec.drain()
		step(3 * time.Second) // reels scroll, land staggered, flash settles
	}

	if len(rec.frames) < 5 {
		t.Fatalf("captured only %d frames", len(rec.frames))
	}
	dcWriteSeq(t, "pokies", rec.frames)
}
