package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// compose runs every wake, per viewer. Production builds use -gc=leaking, so any
// per-frame allocation is a permanent leak over a long session — compose must
// allocate nothing (numbers are drawn digit-by-digit, never formatted).
func TestComposeAllocFree(t *testing.T) {
	_, rm := newGame(t, "p1")
	b := rm.boards["p1"]
	b.launch(testRng())
	b.balls = append(b.balls, ball{x: 30, y: 10, vx: 5, vy: -5})
	b.parts = append(b.parts, particle{x: 20, y: 8, glyph: '*', color: kit.White})
	b.powerups = append(b.powerups, powerup{x: 25, y: 9, kind: puWide})
	b.score, b.best, b.level = 12345, 99999, 4

	f := kit.NewFrame()
	v := kittest.Player("p1")
	if a := testing.AllocsPerRun(100, func() { rm.compose(f, v) }); a != 0 {
		t.Errorf("compose allocates %.0f/frame - the render must stay alloc-free under -gc=leaking", a)
	}
}
