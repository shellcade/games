package shellracer

// Real-frame capture: drives a solo shellracer race, typing the passage
// keystroke-by-keystroke (the animated/typing scenario — each correct key
// advances the cursor and redraws the passage highlight, progress, and WPM).
// See diffcommon_test.go for serialization helpers.
//
//	go test -run TestCaptureFrameSeq ./...

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit"
	"github.com/shellcade/kit/kittest"
)

// TestCaptureFrameSeq boots a solo race (enters racing immediately), then types
// the whole passage one rune at a time with a small clock advance between keys
// (so the WPM clock moves and the live HUD updates). A handful of post-finish
// wakes capture the results screen. This is the per-keystroke redraw path.
func TestCaptureFrameSeq(t *testing.T) {
	r := kittest.NewRoom()
	r.Cfg = kit.RoomConfig{Mode: kit.ModeSolo, Capacity: 1, MinPlayers: 1, Seed: 1, SeedSet: true}
	rm := newRoom(r.Cfg, r.Services())
	rec := dcNewRecorder(r)

	a := dcPlayer("ace")
	rm.OnStart(r)
	rec.drain()
	r.Players = append(r.Players, a)
	rm.OnJoin(r, a) // solo -> racing immediately
	rec.drain()

	// Type the passage; advance the clock ~120ms per key so the WPM HUD ticks.
	for _, ru := range rm.passage {
		r.Advance(120 * time.Millisecond)
		rm.OnInput(r, a, dcRuneIn(ru))
		rec.drain()
		// a wake between keys (host heartbeat) to update the live timer/WPM
		rm.OnWake(r)
		rec.drain()
	}

	// Results hold + settle.
	for i := 0; i < 10; i++ {
		r.Advance(time.Second)
		rm.OnWake(r)
		rec.drain()
	}

	if len(rec.frames) < 5 {
		t.Fatalf("captured only %d frames", len(rec.frames))
	}
	dcWriteSeq(t, "shellracer", rec.frames)
}
