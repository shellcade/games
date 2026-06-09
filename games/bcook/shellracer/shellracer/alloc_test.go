package shellracer

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// composePassage runs on every render, per viewer. It used to word-wrap the
// (fixed) passage on every call, allocating a fresh [][2]int — permanent growth
// under -gc=leaking. The wrap is now computed once in OnStart, so composePassage
// must allocate nothing.
func TestComposePassageAllocFree(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	p := kittest.Player("alice")
	d.join(p)
	f := kit.NewFrame()
	allocs := testing.AllocsPerRun(100, func() {
		d.rm.composePassage(f, p)
	})
	if allocs != 0 {
		t.Fatalf("composePassage allocates %.0f/call — the passage wrap must be cached (computed once in OnStart), not re-wrapped per render", allocs)
	}
}
