package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// newTestRoom builds a started room driven by a kittest.Room (deterministic
// clock + rng), with no players joined yet.
func newTestRoom(t *testing.T, handles ...string) (*room, *kittest.Room) {
	t.Helper()
	players := make([]kit.Player, len(handles))
	for i, h := range handles {
		players[i] = kittest.Player(h)
	}
	tr := kittest.NewRoom(players...)
	rm := newRoom(tr.Cfg, tr.Services())
	rm.OnStart(tr)
	return rm, tr
}

func keyRune(ru rune) kit.Input { return kit.Input{Kind: kit.InputRune, Rune: ru} }

func keyNamed(k kit.Key) kit.Input { return kit.Input{Kind: kit.InputKey, Key: k} }

func TestStartAndSmokeNoPanic(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	if len(rm.players) != 2 {
		t.Fatalf("want 2 players, got %d", len(rm.players))
	}

	inputs := []kit.Input{
		keyNamed(kit.KeyLeft), keyNamed(kit.KeyRight), keyNamed(kit.KeyUp),
		keyNamed(kit.KeyDown), keyRune('h'), keyRune('l'),
	}
	players := []kit.Player{a, b}
	for i := 0; i < 500; i++ {
		p := players[i%2]
		rm.OnInput(tr, p, inputs[i%len(inputs)])
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)

		for id, pl := range rm.players {
			if pl.alive {
				if pl.col < 0 || pl.col >= arenaW || pl.row < top || pl.row > bottom {
					t.Fatalf("player %s off-field at (row %d,col %d)", id, pl.row, pl.col)
				}
				if pl.layer < 0 || pl.layer >= layers {
					t.Fatalf("player %s on impossible layer %d", id, pl.layer)
				}
			}
		}
	}
}

// TestTileDecayProgression: a stepped-off tile waits the grace delay, then walks
// █ → ▓ → ░ → gone one stage per decayStep.
func TestTileDecayProgression(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	pl := rm.players[a.AccountID]

	// Place the player on a known tile with solid footing to its right.
	pl.layer, pl.row, pl.col = 0, 11, 40
	rm.floors[0][11-top][40] = tileSolid
	rm.floors[0][11-top][41] = tileSolid

	// Step right: the tile at col 40 should now be scheduled to decay.
	rm.OnInput(tr, a, keyNamed(kit.KeyRight))
	if pl.col != 41 {
		t.Fatalf("expected to move to col 41, at %d", pl.col)
	}
	if rm.tileAt(0, 11, 40) != tileSolid {
		t.Fatalf("tile should still be solid during the grace beat")
	}

	advanceWakes := func(d time.Duration) {
		// Advance in 50ms heartbeats so decay events fire on the wake cadence.
		for elapsed := time.Duration(0); elapsed < d; elapsed += 50 * time.Millisecond {
			tr.Advance(50 * time.Millisecond)
			rm.OnWake(tr)
		}
	}

	advanceWakes(decayDelay + decayStep/2)
	if got := rm.tileAt(0, 11, 40); got != tileCracked {
		t.Fatalf("after grace: stage %d, want cracked %d", got, tileCracked)
	}
	advanceWakes(decayStep)
	if got := rm.tileAt(0, 11, 40); got != tileWorn {
		t.Fatalf("next step: stage %d, want worn %d", got, tileWorn)
	}
	advanceWakes(decayStep)
	if got := rm.tileAt(0, 11, 40); got != tileGone {
		t.Fatalf("final step: stage %d, want gone %d", got, tileGone)
	}
}

// TestFallThroughDropsOneLayer: stepping onto a hole drops you exactly one layer
// when the layer below is solid there.
func TestFallThroughDropsOneLayer(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	pl := rm.players[a.AccountID]

	pl.layer, pl.row, pl.col = 0, 11, 40
	rm.floors[0][11-top][41] = tileGone  // hole to the right on this layer
	rm.floors[1][11-top][41] = tileSolid // solid catch one floor down

	rm.OnInput(tr, a, keyNamed(kit.KeyRight))
	if pl.layer != 1 {
		t.Fatalf("expected to drop to layer 1, on %d", pl.layer)
	}
	if pl.col != 41 || pl.row != 11 {
		t.Fatalf("expected to land at same (row,col), at (%d,%d)", pl.row, pl.col)
	}
	if !pl.alive {
		t.Fatal("a single-floor drop should not eliminate")
	}
}

// TestFallThroughMultipleLayers: aligned holes drop you through several floors at
// once, and through the bottom eliminates you.
func TestFallThroughBottomEliminates(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob") // 2 players so the round doesn't end on the drop
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, tr.Players[1])
	pl := rm.players[a.AccountID]

	pl.layer, pl.row, pl.col = 0, 11, 40
	// Holes straight down through every layer to the right.
	for l := 0; l < layers; l++ {
		rm.floors[l][11-top][41] = tileGone
	}
	rm.OnInput(tr, a, keyNamed(kit.KeyRight))
	if pl.alive {
		t.Fatal("falling through all floors must eliminate")
	}
	if pl.lastSecs < 0 {
		t.Fatalf("survival time should be recorded, got %d", pl.lastSecs)
	}
}

// TestSoloEndAndLeaderboardPost: a solo player who falls out ends the round and
// posts their survival time to the leaderboard.
func TestSoloEndAndLeaderboardPost(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	pl := rm.players[a.AccountID]

	tr.Advance(4 * time.Second) // bank ~4s of survival
	pl.layer, pl.row, pl.col = 0, 11, 40
	for l := 0; l < layers; l++ {
		rm.floors[l][11-top][41] = tileGone
	}
	rm.OnInput(tr, a, keyNamed(kit.KeyRight))
	rm.OnWake(tr) // round-end check happens on the wake

	if rm.playing {
		t.Fatal("solo round should end once the lone player falls out")
	}
	if len(tr.Posted) == 0 {
		t.Fatal("a fallen solo player should post a leaderboard result")
	}
	last := tr.Posted[len(tr.Posted)-1]
	if len(last.Rankings) != 1 || last.Rankings[0].Metric < 4 {
		t.Fatalf("posted metric %+v, want ~4s survival", last.Rankings)
	}
}

// TestMultiplayerWinDetection: when all but one have fallen, the survivor wins.
func TestMultiplayerWinDetection(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	pa, pb := rm.players[a.AccountID], rm.players[b.AccountID]

	// Drop bob through the bottom directly; alice stays alive.
	pb.layer = layers
	pb.alive = false
	_ = pa

	rm.OnWake(tr)
	if rm.playing {
		t.Fatal("round should end with one survivor")
	}
	if rm.winnerID != a.AccountID {
		t.Fatalf("winner = %q, want alice %q", rm.winnerID, a.AccountID)
	}
}

// TestSoloCrumbleAccelerates: the ambient crumble rate is strictly higher later
// in a solo round than at the start, and higher than the pvp rate.
func TestSoloCrumbleAccelerates(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])

	// Count tiles scheduled to crumble over a fixed window early vs. late.
	countOverWindow := func(start time.Duration) int {
		rm.startRound(tr)
		tr.Advance(start)
		rm.lastNow = rm.now // reset dt baseline so the window measures cleanly
		rm.now = tr.Clock
		before := len(rm.decay)
		for i := 0; i < 20; i++ { // 1 second of 50ms wakes
			tr.Advance(50 * time.Millisecond)
			rm.OnWake(tr)
		}
		return len(rm.decay) - before
	}

	early := countOverWindow(0)
	late := countOverWindow(20 * time.Second)
	if late <= early {
		t.Fatalf("solo crumble should accelerate: early=%d late=%d", early, late)
	}
}

func TestPvpCrumbleGentlerThanSolo(t *testing.T) {
	// The same elapsed time produces a far higher solo rate than the pvp rate.
	elapsed := 15.0
	solo := minf(soloCrumbleBase+soloCrumbleGrow*elapsed, soloCrumbleMax)
	pvp := minf(pvpCrumbleBase+pvpCrumbleGrow*elapsed, pvpCrumbleMax)
	if solo <= pvp {
		t.Fatalf("solo crumble rate %.1f should exceed pvp %.1f", solo, pvp)
	}
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// TestMoveCooldown: a second move within moveEvery is ignored.
func TestMoveCooldown(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	pl := rm.players[a.AccountID]
	pl.layer, pl.row, pl.col = 0, 11, 40

	rm.OnInput(tr, a, keyNamed(kit.KeyRight))
	gotCol := pl.col
	rm.OnInput(tr, a, keyNamed(kit.KeyRight)) // immediately again — should be ignored
	if pl.col != gotCol {
		t.Fatalf("second move within cooldown should be ignored: col %d -> %d", gotCol, pl.col)
	}
	tr.Advance(moveEvery + 10*time.Millisecond)
	rm.now = tr.Clock
	rm.OnInput(tr, a, keyNamed(kit.KeyRight))
	if pl.col == gotCol {
		t.Fatal("move after cooldown should be accepted")
	}
}

// TestArenaEdgeBlocks: you cannot step off the arena edge.
func TestArenaEdgeBlocks(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	pl := rm.players[a.AccountID]
	pl.layer, pl.row, pl.col = 0, top, 0

	rm.OnInput(tr, a, keyNamed(kit.KeyLeft))
	if pl.col != 0 {
		t.Fatalf("left edge: col %d, want 0", pl.col)
	}
	rm.OnInput(tr, a, keyNamed(kit.KeyUp))
	if pl.row != top {
		t.Fatalf("top edge: row %d, want %d", pl.row, top)
	}
}

func TestComposeRendersFrame(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	f := kit.NewFrame()
	rm.composeFor(f, a)
	if len(f.Cells) != kit.Rows || len(f.Cells[0]) != kit.Cols {
		t.Fatal("frame is not 24x80")
	}
}

// TestScoreboardCharacterRendersBesideName asserts each contestant's character
// tile lands one cell + one space before their name on the scoreboard.
func TestScoreboardCharacterRendersBesideName(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	a.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	b.Character = kit.Character{Glyph: "Ω", InkR: 1, InkG: 2, InkB: 3, BgR: 4, BgG: 5, BgB: 6, Fallback: 'O'}
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	f := kit.NewFrame()
	rm.composeFor(f, a)

	want := []struct {
		ch       kit.Character
		nameRune rune
	}{{a.Character, 'a'}, {b.Character, 'b'}}
	i := 0
	for c := 0; c < kit.Cols && i < len(want); c++ {
		r := f.Cells[0][c].Rune
		if r != '●' && r != '○' {
			continue
		}
		if got := f.Cells[0][c+2]; got != kit.CharacterCell(want[i].ch) {
			t.Errorf("player %d: cell after marker = %+v, want character tile", i, got)
		}
		if f.Cells[0][c+3].Rune != ' ' {
			t.Errorf("player %d: no space between character tile and name", i)
		}
		if f.Cells[0][c+4].Rune != want[i].nameRune {
			t.Errorf("player %d: name does not follow the tile (got %q)", i, f.Cells[0][c+4].Rune)
		}
		i++
	}
	if i != len(want) {
		t.Fatalf("found %d scoreboard segments, want %d", i, len(want))
	}
}

// TestAvatarUsesCharacterColour asserts a player's character drives the avatar
// glyph (bg colour = avatar colour) and a zero character falls back to '@'.
func TestAvatarUsesCharacterColour(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	a.Character = kit.Character{Glyph: "λ", BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	pa := rm.players[a.AccountID]
	want := kit.RGB(0x2D, 0x1B, 0x4E)
	if pa.color != want {
		t.Fatalf("avatar colour = %v, want character bg %v", pa.color, want)
	}
	if pa.glyph != 'λ' {
		t.Fatalf("avatar glyph = %q, want 'λ'", pa.glyph)
	}
	pb := rm.players[b.AccountID]
	if pb.glyph != '@' || pb.color != palette[1] {
		t.Fatalf("zero-character player: glyph %q colour %v, want '@' palette[1]", pb.glyph, pb.color)
	}
}

// TestRenderReusesFrame asserts render keeps the one long-lived buffer.
func TestRenderReusesFrame(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	before := rm.frame
	rm.render(tr)
	rm.render(tr)
	if rm.frame != before {
		t.Fatal("render replaced rm.frame - it must reuse the single long-lived buffer")
	}
}

// TestSteadyStateWakeAllocs guards against per-tick framing regressions under
// -gc=leaking (every byte allocated is permanent for the room's life).
func TestSteadyStateWakeAllocs(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob", "cleo")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	for i := 0; i < 10; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	allocs := testing.AllocsPerRun(50, func() {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	})
	t.Logf("3-player wake allocs/op: %.1f", allocs)
	if allocs > 60 {
		t.Fatalf("3-player wake allocates %.1f/op - did render() stop reusing rm.frame?", allocs)
	}
}
