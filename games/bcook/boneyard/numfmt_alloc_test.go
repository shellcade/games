package main

import (
	"testing"

	"github.com/shellcade/kit/v2/kittest"
)

// midRun returns a room and a delver wired into a busy week with non-trivial
// numeric state, so the render paths exercise multi-digit putInt writes (the
// shape that used to allocate via itoa(...)+concat under -gc=leaking).
func midRunRoom(t *testing.T) (*room, *delver) {
	t.Helper()
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	d := rm.delvers[a.AccountID]

	// Mid-run vitals: multi-digit values across every HUD/sub-screen field.
	d.floor = 12
	rm.floorAt(d.floor)
	f := rm.world.at(d.floor)
	d.x, d.y = f.upX, f.upY
	d.reveal(f)
	d.hp, d.maxHP = 23, 40
	d.gold = 1234
	d.banked = 9
	d.torch = 357
	d.kills = 17
	d.respects = 5
	d.avenges = 2

	// Non-empty countdown so the "collapses in <cdCache>" line is rendered on
	// the HUD (row 22), memorial (row 2), and gate screen (row 3). The literal
	// + cdCache concat that used to build this string allocated per render.
	rm.cdCache = "2d 13h 47m"

	// A death card so deathCardScreen has frozen stats.
	d.deathCard = &deathSummary{
		killer: "a tomb mimic", floor: 12, banked: 9,
		kills: 17, gold: 1234, respects: 5, avenges: 2,
		deepestThisWeek: 21, deepestHandle: "an-overlong-delver-handle-indeed",
	}
	return rm, d
}

// GATE — the constantly-shown HUD render path must allocate ZERO bytes: under
// -gc=leaking every alloc here is a permanent leak that accumulates for the
// whole resident week.
func TestHUDRenderZeroAllocs(t *testing.T) {
	rm, d := midRunRoom(t)
	if n := testing.AllocsPerRun(100, func() {
		rm.hud(d)
	}); n != 0 {
		t.Fatalf("hud() allocates %.0f/op — permanent leak under -gc=leaking", n)
	}
}

// GATE — the full playing compose path (map + HUD) must also be alloc-free.
func TestComposeRenderZeroAllocs(t *testing.T) {
	rm, d := midRunRoom(t)
	if n := testing.AllocsPerRun(100, func() {
		rm.compose(d)
	}); n != 0 {
		t.Fatalf("compose() allocates %.0f/op — permanent leak under -gc=leaking", n)
	}
}

// GATE — the occasionally-shown sub-screens (memorial, YOU DIED, gate) leak too
// over a week-long resident room, so they must be alloc-free as well.
func TestSubScreenRenderZeroAllocs(t *testing.T) {
	rm, d := midRunRoom(t)
	cases := []struct {
		name string
		fn   func()
	}{
		{"memorial", func() { rm.memorial(d) }},
		{"deathCardScreen", func() { rm.deathCardScreen(d) }},
		{"gateScreen", func() { rm.gateScreen(d) }},
	}
	for _, tc := range cases {
		if n := testing.AllocsPerRun(100, tc.fn); n != 0 {
			t.Fatalf("%s allocates %.0f/op — permanent leak under -gc=leaking", tc.name, n)
		}
	}
}
