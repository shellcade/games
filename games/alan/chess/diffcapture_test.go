package main

// Real-frame capture: drives a few opening moves of a live chess game plus idle
// clock wakes, recording every per-viewer frame the room sends (the moderate
// turn-based + live-clock scenario). Moves are made through the REAL input path
// (cursor navigation with arrow keys + Enter), exactly as a player drives the
// game, so the captured frames are the genuine production renders.
//
//	go test -run TestCaptureFrameSeq ./...

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"

	"alan/chess/engine"
)

// dcKeyForDelta returns the arrow key that steps the cursor by on-screen
// (dfile, drank) for a white-orienting viewer. Chess maps Up->rank+1,
// Down->rank-1, Left->file-1, Right->file+1 in white perspective (room.go
// arrowDelta + moveCursor).
func dcKeyForDelta(dfile, drank int) (kit.Key, bool) {
	switch {
	case drank > 0:
		return kit.KeyUp, true
	case drank < 0:
		return kit.KeyDown, true
	case dfile < 0:
		return kit.KeyLeft, true
	case dfile > 0:
		return kit.KeyRight, true
	}
	return 0, false
}

// dcNavSelect drives the cursor of the side-to-move's controller from its
// current square to `from` using free-roam arrow steps (white perspective),
// then presses Enter to select the piece. It is white-box (reads rm.sel) so the
// navigation is exact, but every cursor move and the selection go through the
// real OnInput path — these are real frames.
func dcNavSelect(t *testing.T, rm *room, tr *kittest.Room, p kit.Player, from engine.Square, rec *dcRecorder) {
	t.Helper()
	sel := rm.sel[p.AccountID]
	flip := rm.color[p.AccountID] == engine.Black // Black views the board flipped
	for sel.cursor != from {
		df := from.File() - sel.cursor.File()
		dr := from.Rank() - sel.cursor.Rank()
		step := func(d int) int {
			if d > 0 {
				return 1
			}
			if d < 0 {
				return -1
			}
			return 0
		}
		sdf, sdr := step(df), step(dr)
		if flip {
			// moveCursor inverts on-screen df,dr for Black, so invert the key
			// we choose to make the board cursor step toward `from`.
			sdf, sdr = -sdf, -sdr
		}
		k, ok := dcKeyForDelta(sdf, sdr)
		if !ok {
			t.Fatalf("nav stuck reaching %v (cursor %v)", from, sel.cursor)
		}
		rm.OnInput(tr, p, dcKeyIn(k))
		rec.drain()
	}
	rm.OnInput(tr, p, dcKeyIn(kit.KeyEnter)) // select
	rec.drain()
	if sel.from != from {
		t.Fatalf("Enter on %v did not select (from=%v)", from, sel.from)
	}
}

// dcNavMove, with a piece already selected, snaps the cursor toward `to` (the
// selected piece's legal target) and confirms. The snap-to-target cursor logic
// moves the cursor to the nearest legal destination in the pressed direction, so
// we press toward `to` (in white screen space) until the cursor lands on it.
func dcNavMove(t *testing.T, rm *room, tr *kittest.Room, p kit.Player, to engine.Square, rec *dcRecorder) {
	t.Helper()
	sel := rm.sel[p.AccountID]
	orient := rm.color[p.AccountID]
	for guard := 0; sel.cursor != to && guard < 16; guard++ {
		// Work in SCREEN space: snapCursorToTarget keys on on-screen direction.
		// Up=row up (-row), Down=row down (+row), Left=-col, Right=+col.
		cr, cc := screenSquare(sel.cursor, orient)
		tr2, tc2 := screenSquare(to, orient)
		var k kit.Key
		switch {
		case tr2 < cr:
			k = kit.KeyUp
		case tr2 > cr:
			k = kit.KeyDown
		case tc2 < cc:
			k = kit.KeyLeft
		case tc2 > cc:
			k = kit.KeyRight
		default:
			t.Fatalf("nav-move: no screen direction to %v", to)
		}
		rm.OnInput(tr, p, dcKeyIn(k))
		rec.drain()
	}
	if sel.cursor != to {
		t.Fatalf("could not snap cursor to target %v (at %v)", to, sel.cursor)
	}
	rm.OnInput(tr, p, dcKeyIn(kit.KeyEnter)) // confirm move
	rec.drain()
}

// dcSquare maps algebraic ("e2") to an engine.Square.
func dcSquare(name string) engine.Square {
	return engine.SquareAt(int(name[0]-'a'), int(name[1]-'1'))
}

// TestCaptureFrameSeq plays the opening of a real game over the live input path
// with idle clock wakes between moves. Both seats see a per-viewer frame on
// every render; the live blitz clock ticks on idle wakes (a few changed cells),
// and each move + each cursor step is a real frame. This is the moderate
// turn-based scenario with a continuously-updating clock.
func TestCaptureFrameSeq(t *testing.T) {
	a, b := kittest.Player("alice"), kittest.Player("bob")
	tr := kittest.NewRoom(a, b)
	tr.Cfg.Mode = kit.ModeQuick
	tr.Cfg.MinPlayers = 2
	tr.Cfg.Capacity = 2
	tr.Cfg.Seed = 1
	tr.Cfg.SeedSet = true

	rm := newRoom(tr.Cfg, tr.Services())
	rm.OnStart(tr)
	rec := dcNewRecorder(tr)

	rm.OnJoin(tr, a)
	rec.drain()
	rm.OnJoin(tr, b)
	rec.drain()

	if rm.phase != phPlaying {
		t.Fatalf("game did not start (phase=%q)", rm.phase)
	}

	// Identify who holds White (the colour assignment is seeded).
	whitePlayer := a
	if rm.color[a.AccountID] != engine.White {
		whitePlayer = b
	}
	blackPlayer := a
	if whitePlayer == a {
		blackPlayer = b
	}

	idle := func(d time.Duration) {
		tr.Advance(d)
		rm.OnWake(tr)
		rec.drain()
	}

	// A short opening: 1. e4 e5 2. Nf3 Nc6 3. Bb5 (the Ruy Lopez), with idle
	// clock wakes between moves so the live clock display ticks.
	type ply struct {
		p        kit.Player
		from, to string
	}
	opening := []ply{
		{whitePlayer, "e2", "e4"},
		{blackPlayer, "e7", "e5"},
		{whitePlayer, "g1", "f3"},
		{blackPlayer, "b8", "c6"},
		{whitePlayer, "f1", "b5"},
	}
	for _, m := range opening {
		dcNavSelect(t, rm, tr, m.p, dcSquare(m.from), rec)
		dcNavMove(t, rm, tr, m.p, dcSquare(m.to), rec)
		idle(time.Second)     // an idle wake: clock ticks, nothing else changes
		idle(2 * time.Second) // another idle wake before the reply
	}

	if len(rec.frames) < 10 {
		t.Fatalf("captured only %d frames", len(rec.frames))
	}
	dcWriteSeq(t, "chess", rec.frames)
}
