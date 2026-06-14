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
		t.Fatalf("drawPaytable allocates %.0f/call — the paytable rows/labels must be precomputed, not rebuilt per render", allocs)
	}
}

// drawGamble runs per render while a gamble is active. Its only allocations are
// the unavoidable number formatting (the at-risk amount), matching the existing
// HI/BAL readout cost — the selector itself must build no slices. Pin a small
// budget so a future change that allocates per option is caught.
func TestDrawGambleAllocBudget(t *testing.T) {
	p := kittest.Player("alice")
	rm, _ := newGame(t, p)
	m := &machine{balance: 1000, gamble: &gambleState{atRisk: 12345, sel: selRed, card: suitHearts}}
	rm.machines[p.AccountID] = m
	f := kit.NewFrame()
	if n := testing.AllocsPerRun(100, func() { rm.drawGamble(f, 0, 0, m, true) }); n > 2 {
		t.Fatalf("owner drawGamble allocates %.0f/call, want <= 2 (only the at-risk formatting)", n)
	}
}
