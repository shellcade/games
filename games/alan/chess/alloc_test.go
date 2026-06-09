package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"

	"alan/chess/engine"
)

// drawBoard runs on every render, per viewer. It used to allocate a fresh
// map[Square]tgt every call (and the move list a growing []line) — permanent
// growth under -gc=leaking. The legal-target marks now live in a stack array,
// so drawBoard must allocate nothing.
func TestDrawBoardAllocFree(t *testing.T) {
	rm, tr := newGame(t)
	f := kit.NewFrame()
	allocs := testing.AllocsPerRun(100, func() {
		rm.drawBoard(f, tr.Players[0], engine.White)
	})
	if allocs != 0 {
		t.Fatalf("drawBoard allocates %.0f/call — legal-target marks must use the stack array, not a per-render map", allocs)
	}
}
