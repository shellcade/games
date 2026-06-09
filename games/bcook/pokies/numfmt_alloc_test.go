package main

import (
	"testing"
	"time"

	"github.com/shellcade/kit/v2/kittest"
)

// compose runs on every wake, per viewer, under -gc=leaking — any heap
// allocation it makes is permanent for the room's lifetime. The footer
// ("BAL n   HI n") and the per-cabinet numeric readouts (HI/BAL/BET) used to
// fmt.Sprintf a fresh string each render; numfmt.go writes them digit-by-digit
// instead. compose must allocate nothing on a steady-state spin.
func TestComposeAllocFree(t *testing.T) {
	alice := kittest.Player("alice")
	rm, r := newGame(t, alice)
	rm.OnJoin(r, alice)

	// Mid-spin: start a pull and advance the clock partway so reels are still
	// scrolling (the animated branch) and the readouts hold real numbers.
	rm.startSpin(r, alice)
	r.Advance(200 * time.Millisecond)
	rm.lastNow = r.Now()

	// Prime the package-global frame once so its backing storage is allocated
	// before the measured runs (compose reuses it via Clear).
	_ = rm.compose(alice)

	allocs := testing.AllocsPerRun(100, func() {
		_ = rm.compose(alice)
	})
	if allocs != 0 {
		t.Fatalf("compose allocates %.0f/call — the per-render path must be alloc-free under -gc=leaking", allocs)
	}
}
