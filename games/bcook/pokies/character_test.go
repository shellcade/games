package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// colIndex returns the rune-column of sub in row, or -1 (strings.Index byte
// offsets drift once a multi-byte character glyph sits to the left).
func colIndex(row, sub string) int {
	rs, ss := []rune(row), []rune(sub)
	for i := 0; i+len(ss) <= len(rs); i++ {
		match := true
		for j := range ss {
			if rs[i+j] != ss[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// TestFloorRendersOwnCharacterTile asserts the arcade character tile (kit
// v2.9.0) renders as the roaming player's avatar on the lounge floor — a styled
// cell, not a generic glyph.
func TestFloorRendersOwnCharacterTile(t *testing.T) {
	p := kittest.Player("alice")
	p.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	rm.render(r) // roaming on the floor

	f := r.LastFrame(p)
	if f == nil {
		t.Fatal("no frame after join")
	}
	want := kit.CharacterCell(p.Character)
	for row := 0; row < kit.Rows; row++ {
		for col := 0; col < kit.Cols; col++ {
			if f.Cells[row][col] == want {
				return // found the styled character tile
			}
		}
	}
	t.Error("the roaming player's character tile should render on the floor")
}

// TestTickerRendersCharacterTile asserts the big-win banner carries the
// winner's character tile immediately before their name.
func TestTickerRendersCharacterTile(t *testing.T) {
	p := kittest.Player("alice")
	p.Character = kit.Character{Glyph: "@", InkR: 1, InkG: 2, InkB: 3, BgR: 4, BgG: 5, BgB: 6, Fallback: '@'}
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)

	rm.ticker = ticker{text: "alice hit a big win  +600", ch: p.Character, until: r.Now().Add(time.Minute)}
	rm.render(r)

	f := r.LastFrame(p)
	row := kittest.String(f, 1)
	idx := colIndex(row, "alice hit a big win")
	if idx < 2 {
		t.Fatalf("ticker not on row 1: %q", row)
	}
	if got, want := f.Cells[1][idx-2], kit.CharacterCell(p.Character); got != want {
		t.Errorf("cell before winner's name = %+v, want character tile %+v", got, want)
	}
}
