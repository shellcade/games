package main

import (
	"math"
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

func newGame(t *testing.T, ids ...string) (*kittest.Room, *room) {
	t.Helper()
	players := make([]kit.Player, len(ids))
	for i, id := range ids {
		players[i] = kittest.Player(id)
	}
	r := kittest.NewRoom(players...)
	rm, ok := (Game{}).NewRoom(r.Config(), r.Services()).(*room)
	if !ok {
		t.Fatal("NewRoom did not return *room")
	}
	rm.OnStart(r)
	for _, p := range players {
		rm.OnJoin(r, p)
	}
	return r, rm
}

func TestJoinCreatesBoard(t *testing.T) {
	_, rm := newGame(t, "p1")
	b := rm.boards["p1"]
	if b == nil {
		t.Fatal("no board created on join")
	}
	if b.phase != phReady {
		t.Errorf("fresh board phase = %v, want ready", b.phase)
	}
}

func TestWakeAdvancesBits(t *testing.T) {
	r, rm := newGame(t, "p1")
	b := rm.boards["p1"]
	b.launch(r.Rand())
	y0 := b.balls[0].y
	for i := 0; i < 5; i++ {
		r.Advance(50_000_000) // 50ms in ns
		rm.OnWake(r)
	}
	if len(b.balls) > 0 && b.balls[0].y == y0 {
		t.Error("the bit did not move across several wakes")
	}
}

func TestBestPersistsAndPosts(t *testing.T) {
	r, rm := newGame(t, "p1")
	rm.boards["p1"].score = 750
	rm.OnWake(r) // banks the high score, posts it, persists the wallet

	store := r.Services().Accounts.For(kittest.Player("p1")).Store()
	if got, _ := kvInt(store, "best"); got != 750 {
		t.Errorf("persisted best = %d, want 750", got)
	}
	if len(r.Posted) == 0 {
		t.Error("a new high score did not reach the leaderboard")
	}

	// The score survives a leave + rejoin (durable per account).
	rm.OnLeave(r, kittest.Player("p1"))
	rm.OnJoin(r, kittest.Player("p1"))
	if b := rm.boards["p1"]; b == nil || b.best != 750 {
		t.Errorf("rejoined board best = %v, want 750", b)
	}
}

func TestRivalCharacterRendersBesideName(t *testing.T) {
	p1 := kittest.Player("p1")
	p2 := kittest.Player("p2")
	p2.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	r := kittest.NewRoom(p1, p2)
	rm := (Game{}).NewRoom(r.Config(), r.Services()).(*room)
	rm.OnStart(r)
	rm.OnJoin(r, p1)
	rm.OnJoin(r, p2)

	f := r.LastFrame(p1)
	idx := -1
	for c := 2; c < kit.Cols-1; c++ {
		if f.Cells[statusRow][c].Rune == 'p' && f.Cells[statusRow][c+1].Rune == '2' {
			idx = c
			break
		}
	}
	if idx < 0 {
		t.Fatalf("rival name not on status row: %q", kittest.String(f, statusRow))
	}
	got := f.Cells[statusRow][idx-2]
	want := kit.CharacterCell(p2.Character)
	if got != want {
		t.Errorf("cell before rival name = %+v, want character tile %+v", got, want)
	}
	if f.Cells[statusRow][idx-1].Rune != ' ' {
		t.Errorf("no space between character tile and rival name")
	}
}

func TestPaddleCentreShowsViewerCharacter(t *testing.T) {
	p1 := kittest.Player("p1")
	p1.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	r := kittest.NewRoom(p1)
	rm := (Game{}).NewRoom(r.Config(), r.Services()).(*room)
	rm.OnStart(r)
	rm.OnJoin(r, p1)

	b := rm.boards["p1"]
	half, center := b.paddleHalf(), int(math.Round(b.paddleX))
	f := r.LastFrame(p1)
	if got, want := f.Cells[paddleRow][center], kit.CharacterCell(p1.Character); got != want {
		t.Errorf("paddle centre cell = %+v, want character tile %+v", got, want)
	}
	// The rest of the run wears the character's BACKGROUND colour (the bar
	// is the player's colour, not the stock cyan).
	barBG := kit.RGB(p1.Character.BgR, p1.Character.BgG, p1.Character.BgB)
	for c := center - half; c <= center+half; c++ {
		if c == center {
			continue
		}
		if cell := f.Cells[paddleRow][c]; cell.Rune != ' ' || cell.BG != barBG {
			t.Errorf("paddle cell %d = %+v, want bar in character bg %v", c, cell, barBG)
		}
	}
}

func TestZeroCharacterPaddleUnchanged(t *testing.T) {
	r, rm := newGame(t, "p1") // kittest players carry the zero Character
	b := rm.boards["p1"]
	half, center := b.paddleHalf(), int(math.Round(b.paddleX))

	// Every cell of the run, centre included, is exactly what SetRune writes.
	exp := kit.NewFrame()
	exp.SetRune(paddleRow, center, ' ', stPaddle)
	want := exp.Cells[paddleRow][center]
	f := r.LastFrame(kittest.Player("p1"))
	for c := center - half; c <= center+half; c++ {
		if f.Cells[paddleRow][c] != want {
			t.Errorf("paddle cell %d = %+v, want %+v", c, f.Cells[paddleRow][c], want)
		}
	}
}

func TestMembersGetIndependentBoards(t *testing.T) {
	_, rm := newGame(t, "p1", "p2")
	if len(rm.boards) != 2 {
		t.Fatalf("boards = %d, want 2", len(rm.boards))
	}
	rm.boards["p1"].score = 100
	if rm.boards["p2"].score != 0 {
		t.Error("boards are not independent - p2 inherited p1's score")
	}
}
