package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// findCell reports whether the exact cell appears in the map area of f.
func findCell(f *kit.Frame, want kit.Cell) bool {
	for r := 0; r < mapRows; r++ {
		for c := 0; c < kit.Cols; c++ {
			if f.Cells[r][c] == want {
				return true
			}
		}
	}
	return false
}

// The flagship character contract (kit v2.9.0): a delver's arcade character
// tile is what THEY see on the map — and what every fellow delver sees of
// them. Both viewers get both tiles on the same floor.
func TestDelversRenderCharacterTiles(t *testing.T) {
	a, b := bp("ada"), bp("bob")
	a.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	b.Character = kit.Character{Glyph: "ζ", InkR: 200, InkG: 100, InkB: 50, BgR: 4, BgG: 5, BgB: 6, Fallback: 'Z'}
	tr := kittest.NewRoom(a, b)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	// Both spawn on B1's up-stairs (the Gate hub screen): step ada off so
	// her viewport shows the live map, with bob adjacent and in sight.
	f0 := rm.world.at(1)
	for _, try := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		if f0.open(f0.upX+try[0], f0.upY+try[1]) {
			rm.OnInput(tr, a, kit.Input{Kind: kit.InputRune, Rune: dirRune(try[0], try[1])})
			break
		}
	}
	tr.Advance(100 * time.Millisecond)
	rm.OnWake(tr)

	fa := tr.LastFrame(a)
	if fa == nil {
		t.Fatal("no frame rendered for ada")
	}
	if !findCell(fa, kit.CharacterCell(a.Character)) {
		t.Error("ada's own character tile not on her map")
	}
	if !findCell(fa, kit.CharacterCell(b.Character)) {
		t.Error("bob's character tile not visible on ada's map")
	}
}

// A characterless connection (zero Character) keeps the classic '@'.
func TestNoCharacterFallsBackToAt(t *testing.T) {
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	f0 := rm.world.at(1)
	for _, try := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		if f0.open(f0.upX+try[0], f0.upY+try[1]) {
			rm.OnInput(tr, a, kit.Input{Kind: kit.InputRune, Rune: dirRune(try[0], try[1])})
			break
		}
	}
	tr.Advance(100 * time.Millisecond)
	rm.OnWake(tr)
	if fa := tr.LastFrame(a); fa == nil || !findGlyph(fa, '@') {
		t.Fatal("no '@' rendered for a characterless delver")
	}
}

// Death freezes the character into the corpse, and the memorial wall renders
// the tile immediately before the dead's name (one cell + one space).
func TestMemorialShowsCharacterTile(t *testing.T) {
	a := bp("ada")
	a.Character = kit.Character{Glyph: "λ", InkR: 9, InkG: 8, InkB: 7, Fallback: 'L'}
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)

	d := rm.delvers[a.AccountID]
	rm.die(tr, d, "kobold")

	var c *corpse
	for _, bc := range rm.bones {
		if bc.handle == "ada" {
			c = bc
		}
	}
	if c == nil {
		t.Fatal("no corpse for ada")
	}
	if c.ch != a.Character {
		t.Fatalf("corpse character = %+v, want frozen %+v", c.ch, a.Character)
	}

	// Make ada's bones the MOST MOURNED entry, then read the wall: the tile
	// sits at col 16, one space before the name at col 18.
	c.respects = 99
	rm.memorial(d)
	const row = 4 // first memorial line
	if got, want := rm.frame.Cells[row][16], kit.CharacterCell(c.ch); got != want {
		t.Errorf("memorial cell before name = %+v, want character tile %+v", got, want)
	}
	if got := kittest.String(rm.frame, row); !containsAt(got, "ada", 18) {
		t.Errorf("memorial name row = %q, want %q at col 18", got, "ada")
	}
}

// containsAt reports whether sub starts at rune-column col of row.
func containsAt(row, sub string, col int) bool {
	rs, ss := []rune(row), []rune(sub)
	if col+len(ss) > len(rs) {
		return false
	}
	for j := range ss {
		if rs[col+j] != ss[j] {
			return false
		}
	}
	return true
}
