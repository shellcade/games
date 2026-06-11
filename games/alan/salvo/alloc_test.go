package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// compose runs every wake, per viewer. Production builds use -gc=leaking, so any
// per-frame allocation is a permanent leak over a long battle — compose must
// allocate nothing (numbers are drawn digit-by-digit, never formatted).
func TestComposeAllocFree(t *testing.T) {
	_, rm := newGame(t, "p1")
	rm.now = time.Unix(100, 0)
	// A busy frame: a shell mid-flight, an explosion, and debris.
	rm.shell = &shell{x: 30, y: 6, vx: 10, vy: -8, w: weapons[0], owner: rm.tanks[0]}
	rm.shell.trail = append(rm.shell.trail, pt{29, 7}, pt{28, 8})
	rm.booms = append(rm.booms, boom{x: 40, y: 16, radius: 6, bornAt: rm.now, color: weapons[1].color})
	rm.parts = append(rm.parts, particle{x: 35, y: 14, glyph: '*', color: weapons[0].color, until: rm.now.Add(time.Second)})

	f := kit.NewFrame()
	v := kittest.Player("p1")
	if a := testing.AllocsPerRun(50, func() { rm.compose(f, v) }); a != 0 {
		t.Errorf("compose allocates %.0f/frame — the render must stay alloc-free under -gc=leaking", a)
	}
}
