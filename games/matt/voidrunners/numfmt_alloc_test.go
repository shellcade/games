package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// TestDrawHUDZeroAllocs guards the HUD render hot path. Production runs
// -gc=leaking: any per-render allocation is permanent and leaks until OOM.
// drawHUD must therefore allocate ZERO bytes per call. The frame buffer is
// cleared (not reallocated) and reused exactly as render() does.
func TestDrawHUDZeroAllocs(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob", "cleo")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	a, b, c := tr.Players[0], tr.Players[1], tr.Players[2]

	// A lively mid-game scoreboard: multiple ships, multi-digit kills/deaths,
	// a long handle that must be truncated, and the viewer freshly destroyed so
	// the respawn-countdown branch is exercised too.
	rm.ships[a.AccountID].kills = 12
	rm.ships[a.AccountID].deaths = 4
	rm.ships[a.AccountID].best = 137
	rm.ships[b.AccountID].kills = 3
	rm.ships[c.AccountID].kills = 25
	rm.ships[a.AccountID].alive = false
	rm.ships[a.AccountID].respawnAt = tr.Clock.Add(2 * time.Second)

	f := kit.NewFrame()
	// Warm up so the frame's backing storage is already allocated.
	f.Clear()
	rm.drawHUD(f, a)

	allocs := testing.AllocsPerRun(100, func() {
		f.Clear()
		rm.drawHUD(f, a)
	})
	t.Logf("drawHUD allocs/op: %.1f", allocs)
	if allocs != 0 {
		t.Fatalf("drawHUD allocates %.1f/op — permanent leak under -gc=leaking; want 0", allocs)
	}
}
