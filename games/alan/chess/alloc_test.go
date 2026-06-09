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

// TestComposeAllocFree guards the whole per-viewer render path. compose runs on
// every wake, per viewer, and under -gc=leaking any heap allocation is permanent.
// The move list (move number + stored coordinate strings) and the clocks
// (minutes:seconds) used to allocate per render via strconv.Itoa + string
// concatenation; with putMoveListLine and putClockRight they must allocate
// nothing even with several moves on the board.
func TestComposeAllocFree(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	_ = b

	// Play a handful of moves so the move list and clocks have real content.
	mustPlay(t, rm, tr, "e2", "e4")
	mustPlay(t, rm, tr, "e7", "e5")
	mustPlay(t, rm, tr, "g1", "f3")
	mustPlay(t, rm, tr, "b8", "c6")
	mustPlay(t, rm, tr, "f1", "c4")
	mustPlay(t, rm, tr, "g8", "f6")

	f := kit.NewFrame()
	allocs := testing.AllocsPerRun(100, func() {
		f.Clear()
		rm.compose(a, f)
	})
	if allocs != 0 {
		t.Fatalf("compose allocates %.0f/call — the move list and clocks must format without strconv.Itoa or string concatenation", allocs)
	}
}
