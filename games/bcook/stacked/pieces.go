package main

import (
	"math/rand"

	kit "github.com/shellcade/kit/v2"
)

// pieces is Stacked's original block set: a custom mix of four-cell, three-cell,
// and five-cell shapes with their own names and colors. Each shape lists its
// rotation states as explicit (drow, dcol) cell offsets from the anchor, so
// rotation is a deterministic table lookup. States are authored compact (near
// 0,0) so the spawn anchor and wall-kicks behave predictably.
//
// The set intentionally avoids guideline trade dress: it includes a tromino
// (Wedge), a plus-pentomino (Star), and a U-pentomino (Cup) alongside the
// familiar four-cell silhouettes, all renamed.
var pieces = buildPieces()

func buildPieces() []piece {
	// Color palette: each shape gets a distinct bright color.
	var (
		cBar   = kit.RGB(0x4f, 0xd6, 0xff) // cyan
		cBox   = kit.RGB(0xff, 0xe1, 0x55) // yellow
		cTee   = kit.RGB(0xb9, 0x8a, 0xff) // purple
		cEll   = kit.RGB(0xff, 0x8a, 0x4f) // orange
		cJay   = kit.RGB(0x6b, 0x9a, 0xff) // blue
		cZig   = kit.RGB(0x7d, 0xff, 0x6b) // green
		cZag   = kit.RGB(0xff, 0x6b, 0x7d) // red
		cWedge = kit.RGB(0xff, 0xb3, 0xe6) // pink
		cStar  = kit.RGB(0xff, 0xa5, 0x33) // amber
		cCup   = kit.RGB(0x66, 0xe0, 0xd0) // teal
	)

	// Helper to make a 2-state (only two distinct orientations) piece spin by
	// repeating its states so rot%len cycles correctly.
	two := func(a, b [][2]int) [][][2]int { return [][][2]int{a, b, a, b} }

	return []piece{
		// BAR — four in a row (horizontal <-> vertical).
		{name: "Bar", color: cBar, glyph: '#', states: two(
			[][2]int{{1, 0}, {1, 1}, {1, 2}, {1, 3}},
			[][2]int{{0, 1}, {1, 1}, {2, 1}, {3, 1}},
		)},
		// BOX — 2x2, rotation-invariant.
		{name: "Box", color: cBox, glyph: '#', states: [][][2]int{
			{{0, 0}, {0, 1}, {1, 0}, {1, 1}},
			{{0, 0}, {0, 1}, {1, 0}, {1, 1}},
			{{0, 0}, {0, 1}, {1, 0}, {1, 1}},
			{{0, 0}, {0, 1}, {1, 0}, {1, 1}},
		}},
		// TEE — three across with a center stem.
		{name: "Tee", color: cTee, glyph: '#', states: [][][2]int{
			{{0, 1}, {1, 0}, {1, 1}, {1, 2}},
			{{0, 1}, {1, 1}, {1, 2}, {2, 1}},
			{{1, 0}, {1, 1}, {1, 2}, {2, 1}},
			{{0, 1}, {1, 0}, {1, 1}, {2, 1}},
		}},
		// ELL — an L hook.
		{name: "Ell", color: cEll, glyph: '#', states: [][][2]int{
			{{0, 2}, {1, 0}, {1, 1}, {1, 2}},
			{{0, 1}, {1, 1}, {2, 1}, {2, 2}},
			{{1, 0}, {1, 1}, {1, 2}, {2, 0}},
			{{0, 0}, {0, 1}, {1, 1}, {2, 1}},
		}},
		// JAY — a mirrored L.
		{name: "Jay", color: cJay, glyph: '#', states: [][][2]int{
			{{0, 0}, {1, 0}, {1, 1}, {1, 2}},
			{{0, 1}, {0, 2}, {1, 1}, {2, 1}},
			{{1, 0}, {1, 1}, {1, 2}, {2, 2}},
			{{0, 1}, {1, 1}, {2, 0}, {2, 1}},
		}},
		// ZIG — an S step.
		{name: "Zig", color: cZig, glyph: '#', states: two(
			[][2]int{{0, 1}, {0, 2}, {1, 0}, {1, 1}},
			[][2]int{{0, 0}, {1, 0}, {1, 1}, {2, 1}},
		)},
		// ZAG — a Z step.
		{name: "Zag", color: cZag, glyph: '#', states: two(
			[][2]int{{0, 0}, {0, 1}, {1, 1}, {1, 2}},
			[][2]int{{0, 1}, {1, 0}, {1, 1}, {2, 0}},
		)},
		// WEDGE — a three-cell corner tromino.
		{name: "Wedge", color: cWedge, glyph: '%', states: [][][2]int{
			{{0, 0}, {1, 0}, {1, 1}},
			{{0, 0}, {0, 1}, {1, 1}},
			{{0, 0}, {0, 1}, {1, 1}},
			{{0, 1}, {1, 0}, {1, 1}},
		}},
		// STAR — a five-cell plus pentomino.
		{name: "Star", color: cStar, glyph: '*', states: [][][2]int{
			{{0, 1}, {1, 0}, {1, 1}, {1, 2}, {2, 1}},
			{{0, 1}, {1, 0}, {1, 1}, {1, 2}, {2, 1}},
			{{0, 1}, {1, 0}, {1, 1}, {1, 2}, {2, 1}},
			{{0, 1}, {1, 0}, {1, 1}, {1, 2}, {2, 1}},
		}},
		// CUP — a five-cell U pentomino.
		{name: "Cup", color: cCup, glyph: '%', states: [][][2]int{
			{{0, 0}, {0, 2}, {1, 0}, {1, 1}, {1, 2}},
			{{0, 0}, {0, 1}, {1, 0}, {2, 0}, {2, 1}},
			{{0, 0}, {0, 1}, {0, 2}, {1, 0}, {1, 2}},
			{{0, 0}, {0, 1}, {1, 1}, {2, 0}, {2, 1}},
		}},
	}
}

// refillBag refills a well's shuffle bag with one of every piece in a
// Fisher-Yates order drawn from the room rng — fair distribution with a
// deterministic sequence per room seed.
func refillBag(rng *rand.Rand) []int {
	bag := make([]int, len(pieces))
	for i := range pieces {
		bag[i] = i
	}
	for i := len(bag) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		bag[i], bag[j] = bag[j], bag[i]
	}
	return bag
}

// drawPiece pops the next piece index from the well's bag, refilling it from the
// room rng when empty.
func (w *well) drawPiece(rng *rand.Rand) int {
	if len(w.bag) == 0 {
		w.bag = refillBag(rng)
	}
	idx := w.bag[len(w.bag)-1]
	w.bag = w.bag[:len(w.bag)-1]
	return idx
}
