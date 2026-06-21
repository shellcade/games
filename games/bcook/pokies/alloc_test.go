package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// drawPaytable runs on every render, per viewer. It used to rebuild the sorted
// pay rows (a slice + sort.SliceStable) and " x%d" labels (a make + Sprintf per
// row) each call — permanent growth under -gc=leaking. Those are now precomputed
// once per variant; drawPaytable must allocate nothing.
func TestDrawPaytableAllocFree(t *testing.T) {
	rm, _ := newGame(t, kittest.Player("alice"))
	f := kit.NewFrame()
	allocs := testing.AllocsPerRun(100, func() {
		rm.drawPaytable(f, 5)
	})
	if allocs != 0 {
		t.Fatalf("drawPaytable allocates %.0f/call - the paytable rows/labels must be precomputed, not rebuilt per render", allocs)
	}
}
