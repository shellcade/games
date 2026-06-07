package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

func bp(id string) kit.Player {
	return kit.Player{AccountID: id, Handle: id, Kind: kit.KindMember, Conn: "c-" + id}
}

// findGlyph reports whether g appears anywhere in the map area of f.
func findGlyph(f *kit.Frame, g rune) bool {
	for r := 0; r < mapRows; r++ {
		for c := 0; c < kit.Cols; c++ {
			if f.Cells[r][c].Rune == g {
				return true
			}
		}
	}
	return false
}

// The walking skeleton: join, render, move under the move cooldown, descend
// the week's staircases, and see a fellow delver — all on the kittest double.
func TestWalkAndDescend(t *testing.T) {
	a, b := bp("a"), bp("b")
	tr := kittest.NewRoom(a, b)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	tr.Advance(100 * time.Millisecond)
	rm.OnWake(tr)
	fa := tr.LastFrame(a)
	if fa == nil || !findGlyph(fa, '@') {
		t.Fatal("no rendered @ after join+wake")
	}

	// Both spawn on B1's up-stairs: each sees the OTHER delver too (cyan @ —
	// glyph identical, so assert the world line instead).
	d := rm.delvers[a.AccountID]
	if d.floor != 1 {
		t.Fatalf("spawn floor = B%d", d.floor)
	}

	// Movement honors the cooldown: two instant steps move only once.
	sx, sy := d.x, d.y
	var dir kit.Input
	f := rm.world.at(1)
	for _, try := range []struct{ r rune; dx, dy int }{{'l', 1, 0}, {'h', -1, 0}, {'j', 0, 1}, {'k', 0, -1}} {
		if f.open(sx+try.dx, sy+try.dy) {
			dir = kit.Input{Kind: kit.InputRune, Rune: try.r}
			break
		}
	}
	rm.OnInput(tr, a, dir)
	rm.OnInput(tr, a, dir) // within moveCD: ignored
	moved1 := (d.x != sx) || (d.y != sy)
	if !moved1 {
		t.Fatal("first step did not move")
	}
	mx, my := d.x, d.y
	if dist(mx-sx)+dist(my-sy) != 1 {
		t.Fatalf("two instant steps moved %d tiles, want 1 (moveCD)", dist(mx-sx)+dist(my-sy))
	}
	tr.Advance(d.moveCD() + time.Millisecond)
	rm.OnInput(tr, a, dir)
	if d.x == mx && d.y == my {
		t.Fatal("step after cooldown did not move")
	}

	// Teleport-to-stairs (test seam: place directly) and descend.
	d.x, d.y = f.downX, f.downY
	rm.OnInput(tr, a, kit.Input{Kind: kit.InputRune, Rune: '>'})
	if d.floor != 2 || d.deepest != 2 {
		t.Fatalf("descend: floor=B%d deepest=B%d", d.floor, d.deepest)
	}

	// The frame after descending renders B2 around the arrival stairs.
	tr.Advance(100 * time.Millisecond)
	rm.OnWake(tr)
	if fa := tr.LastFrame(a); fa == nil || !findGlyph(fa, '@') {
		t.Fatal("no render after descent")
	}
}

// Torch burns on action and on the 2s passive clock, and the dark collapses
// sight.
func TestTorchBurn(t *testing.T) {
	a := bp("a")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	d := rm.delvers[a.AccountID]
	d.applyKit(&kits[0]) // BLADE: 600t at the 1.0x baseline burn

	start := d.torch
	// Passive: 10 wakes over 2.0s = exactly one passive tick.
	for i := 0; i < 20; i++ {
		tr.Advance(100 * time.Millisecond)
		rm.OnWake(tr)
	}
	if d.torch != start-1 {
		t.Fatalf("passive burn over 2s: %d -> %d, want -1", start, d.torch)
	}

	d.torch, d.centiburn = 1, 0
	d.burn(1)
	if d.torch != 0 || d.sightRadius() != 2 {
		t.Fatalf("the dark: torch=%d sight=%d, want 0/2", d.torch, d.sightRadius())
	}
}

func dist(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
