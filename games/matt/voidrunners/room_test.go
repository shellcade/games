package main

import (
	"math"
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

	if len(rm.ships) != 2 {
		t.Fatalf("want 2 ships, got %d", len(rm.ships))
	}

	// Drive a few seconds of random input + heartbeats; expect no panic and a
	// stable invariant set.
	inputs := []kit.Input{
		keyNamed(kit.KeyLeft), keyNamed(kit.KeyRight), keyNamed(kit.KeyUp),
		keyNamed(kit.KeyDown), keyRune(' '),
	}
	players := []kit.Player{a, b}
	for i := 0; i < 400; i++ {
		p := players[i%2]
		rm.OnInput(tr, p, inputs[i%len(inputs)])
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)

		if len(rm.craters) < rm.craterTarget() {
			t.Fatalf("craters not topped up: %d", len(rm.craters))
		}
		for id, s := range rm.ships {
			row, col := roundCell(s.y), roundCell(s.x)
			if col < 0 || col >= cols || row < top || row > bottom {
				t.Fatalf("ship %s rounds off-field to (row %d,col %d) from (%.2f,%.2f)", id, row, col, s.x, s.y)
			}
		}
	}
}

func TestDirectionalSteerSetsHeadingAndThrust(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	s := rm.ships[a.AccountID]
	s.vx, s.vy = 0, 0

	// Pressing Up must immediately face north and start moving up (-y).
	rm.OnInput(tr, a, keyNamed(kit.KeyUp))
	if s.heading != dirHeading[dirN] {
		t.Fatalf("Up: heading %.3f, want north %.3f", s.heading, dirHeading[dirN])
	}
	if s.vy >= 0 {
		t.Fatalf("Up: vy %.3f, want negative (upward)", s.vy)
	}
	if math.Abs(s.vx) > 1e-9 {
		t.Fatalf("Up: vx %.3f, want ~0 (no sideways drift from a pure-up press)", s.vx)
	}
}

func TestDiagonalChord(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	s := rm.ships[a.AccountID]

	// Up then Left within the chord window -> heads up-left.
	rm.OnInput(tr, a, keyNamed(kit.KeyUp))
	tr.Advance(chordWindow / 2)
	rm.OnInput(tr, a, keyNamed(kit.KeyLeft))

	wantHeading := math.Atan2(-1, -1) // up-left
	if math.Abs(s.heading-wantHeading) > 1e-6 {
		t.Fatalf("chord up+left: heading %.3f, want %.3f", s.heading, wantHeading)
	}
	if s.vx >= 0 || s.vy >= 0 {
		t.Fatalf("chord up+left: velocity (%.3f,%.3f), want both negative", s.vx, s.vy)
	}
}

func TestNoChordOutsideWindow(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	s := rm.ships[a.AccountID]

	// Two perpendicular presses too far apart stay cardinal (the later wins).
	rm.OnInput(tr, a, keyNamed(kit.KeyUp))
	tr.Advance(chordWindow + 50*time.Millisecond)
	rm.OnInput(tr, a, keyNamed(kit.KeyLeft))

	if s.heading != dirHeading[dirW] {
		t.Fatalf("stale presses: heading %.3f, want due west %.3f", s.heading, dirHeading[dirW])
	}
}

func TestCraterCountDropsInMultiplayer(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	rm.OnJoin(tr, tr.Players[0]) // solo arena seeds soloCraters
	if got := rm.craterTarget(); got != soloCraters {
		t.Fatalf("1 player: target %d, want %d", got, soloCraters)
	}
	rm.OnJoin(tr, tr.Players[1]) // second pilot -> dogfight
	if got := rm.craterTarget(); got != pvpCraters {
		t.Fatalf("2 players: target %d, want %d", got, pvpCraters)
	}
	// A few wakes must trim the field down to the PvP target.
	for i := 0; i < 5; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if len(rm.craters) != pvpCraters {
		t.Fatalf("after trim: %d craters, want %d", len(rm.craters), pvpCraters)
	}
}

func TestBulletDestroysCrater(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)

	s := rm.ships[a.AccountID]
	s.alive = true
	s.invulnUntil = tr.Clock        // not invulnerable
	s.x, s.y, s.heading = 40, 11, 0 // facing east
	s.kills = 0

	// One small crater dead ahead; clear the rest so nothing else interferes.
	rm.craters = []crater{{x: 45, y: 11, size: 1}}

	rm.fire(tr, a, s)
	if len(rm.bullets) != 1 {
		t.Fatalf("expected a bullet after firing, got %d", len(rm.bullets))
	}

	for i := 0; i < 10 && s.kills == 0; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if s.kills < killCrater {
		t.Fatalf("expected crater kill credit, kills=%d", s.kills)
	}
}

func TestBulletDestroysRivalAndScores(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	rm.craters = nil // isolate the duel

	sa, sb := rm.ships[a.AccountID], rm.ships[b.AccountID]
	sa.alive, sb.alive = true, true
	sa.invulnUntil, sb.invulnUntil = tr.Clock, tr.Clock
	sa.x, sa.y, sa.heading = 30, 11, 0 // alice faces east at bob
	sb.x, sb.y = 35, 11
	sa.kills, sb.deaths = 0, 0

	rm.fire(tr, a, sa)
	for i := 0; i < 10 && sb.alive; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if sb.alive {
		t.Fatalf("bob should have been destroyed")
	}
	if sa.kills != killPlayer {
		t.Fatalf("alice should have %d kill credit, got %d", killPlayer, sa.kills)
	}
	if sb.deaths != 1 {
		t.Fatalf("bob should have 1 death, got %d", sb.deaths)
	}
}

func TestRespawnAfterDeath(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	s := rm.ships[a.AccountID]

	rm.killShip(a.AccountID)
	if s.alive {
		t.Fatal("ship should be dead immediately after killShip")
	}
	// Advance past the respawn delay.
	tr.Advance(respawnDelay + 200*time.Millisecond)
	rm.OnWake(tr)
	if !s.alive {
		t.Fatal("ship should have respawned")
	}
	if !tr.Clock.Before(s.invulnUntil) {
		t.Fatal("respawned ship should be briefly invulnerable")
	}
}

func TestInvulnerableShipSurvivesFire(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	rm.craters = nil

	sa, sb := rm.ships[a.AccountID], rm.ships[b.AccountID]
	sa.alive, sb.alive = true, true
	sa.invulnUntil = tr.Clock
	sb.invulnUntil = tr.Clock.Add(5 * time.Second) // bob is freshly respawned
	sa.x, sa.y, sa.heading = 30, 11, 0
	sb.x, sb.y = 35, 11

	rm.fire(tr, a, sa)
	for i := 0; i < 10; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if !sb.alive {
		t.Fatal("invulnerable bob should have survived")
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

// TestScoreboardCharacterRendersBesideName asserts each pilot's character tile
// lands one cell + one space before their name on the scoreboard, for every
// pilot in join order.
func TestScoreboardCharacterRendersBesideName(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	a.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	b.Character = kit.Character{Glyph: "@", InkR: 1, InkG: 2, InkB: 3, BgR: 4, BgG: 5, BgB: 6, Fallback: '@'}
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	f := kit.NewFrame()
	rm.composeFor(f, a)

	// Each scoreboard segment is "● " then the tile, a space, and the name.
	want := []struct {
		ch       kit.Character
		nameRune rune
	}{{a.Character, 'a'}, {b.Character, 'b'}}
	i := 0
	for c := 0; c < kit.Cols && i < len(want); c++ {
		if f.Cells[0][c].Rune != '●' {
			continue
		}
		if got := f.Cells[0][c+2]; got != kit.CharacterCell(want[i].ch) {
			t.Errorf("pilot %d: cell after marker = %+v, want character tile", i, got)
		}
		if f.Cells[0][c+3].Rune != ' ' {
			t.Errorf("pilot %d: no space between character tile and name", i)
		}
		if f.Cells[0][c+4].Rune != want[i].nameRune {
			t.Errorf("pilot %d: name does not follow the tile (got %q)", i, f.Cells[0][c+4].Rune)
		}
		i++
	}
	if i != len(want) {
		t.Fatalf("found %d scoreboard segments, want %d", i, len(want))
	}
}

// TestShipRendersCharacterGlyphInBgColour asserts the pilot's character IS the
// spacecraft: the hull cell carries the character glyph drawn with FG = the
// character's BACKGROUND colour (the owner's "ship colour"), and the
// directional nose keeps the same colour. Composed for the rival so the cell
// is free of the viewer's reverse-video highlight.
func TestShipRendersCharacterGlyphInBgColour(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	a.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	want := kit.RGB(a.Character.BgR, a.Character.BgG, a.Character.BgB)
	sa, sb := rm.ships[a.AccountID], rm.ships[b.AccountID]
	if sa.color != want {
		t.Fatalf("ship colour = %v, want character bg %v", sa.color, want)
	}
	sa.x, sa.y, sa.heading = 20, 8, 0 // facing east; nose at (8, 21)
	sa.invulnUntil = tr.Clock         // no invuln blink
	sb.x, sb.y = 60, 18               // park the rival well clear
	sb.invulnUntil = tr.Clock

	f := kit.NewFrame()
	rm.composeFor(f, b)
	hull := f.Cells[8][20]
	if hull.Rune != 'λ' {
		t.Fatalf("hull rune = %q, want the character glyph 'λ'", hull.Rune)
	}
	if hull.FG != want {
		t.Fatalf("hull FG = %v, want character bg colour %v", hull.FG, want)
	}
	if nose := f.Cells[8][21]; nose.Rune != '→' || nose.FG != want {
		t.Fatalf("nose = %q FG %v, want '→' in ship colour %v", nose.Rune, nose.FG, want)
	}

	// Shots carry the same colour, so fire stays attributable to the pilot.
	rm.fire(tr, a, sa)
	if len(rm.bullets) != 1 || rm.bullets[0].color != want {
		t.Fatalf("bullet colour = %v, want ship colour %v", rm.bullets[0].color, want)
	}
}

// TestZeroCharacterShipRendersPaletteCraft is the regression guard: a pilot
// with no character (test doubles, hosts that don't declare the feature) keeps
// today's '◆' hull and join-order palette colour.
func TestZeroCharacterShipRendersPaletteCraft(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0] // kittest player: zero Character
	rm.OnJoin(tr, a)

	s := rm.ships[a.AccountID]
	if s.color != palette[0] {
		t.Fatalf("ship colour = %v, want palette[0] %v", s.color, palette[0])
	}
	s.x, s.y, s.heading = 20, 8, 0
	s.invulnUntil = tr.Clock

	f := kit.NewFrame()
	rm.composeFor(f, a)
	hull := f.Cells[8][20]
	if hull.Rune != '◆' {
		t.Fatalf("hull rune = %q, want '◆' for a zero character", hull.Rune)
	}
	if hull.FG != palette[0] {
		t.Fatalf("hull FG = %v, want palette[0] %v", hull.FG, palette[0])
	}
}

// TestScoreboardMarkerMatchesShipColour asserts the scoreboard ● marker uses
// the same bg-derived colour as the craft, so the strip identifies each ship.
func TestScoreboardMarkerMatchesShipColour(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	a.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	rm.OnJoin(tr, a)

	f := kit.NewFrame()
	rm.composeFor(f, a)
	want := kit.RGB(a.Character.BgR, a.Character.BgG, a.Character.BgB)
	found := false
	for c := 0; c < kit.Cols; c++ {
		if f.Cells[0][c].Rune != '●' {
			continue
		}
		found = true
		if got := f.Cells[0][c].FG; got != want {
			t.Fatalf("scoreboard marker FG = %v, want ship colour %v", got, want)
		}
	}
	if !found {
		t.Fatal("no ● marker found on the scoreboard row")
	}
}

// TestSteadyStateWakeAllocs guards the OOM that quarantined v1. Production runs
// -gc=leaking: every byte allocated is permanent for the room's life, so a
// per-tick allocation grows without bound until the guest OOMs. The original
// bug allocated a fresh 24x80 frame PER VIEWER PER WAKE (~2KB each), so it grew
// with player count — which is exactly why it only crashed in multiplayer. The
// fix reuses one long-lived rm.frame. The bound here is well below one frame
// allocation, so any regression to per-tick framing trips it, while tolerating
// the HUD's handful of small strings (the same budget shape shipped games use).
func TestSteadyStateWakeAllocs(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob", "cleo")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	// Settle, with the busy paths live (bullets in flight, an explosion).
	for _, p := range tr.Players {
		rm.fire(tr, p, rm.ships[p.AccountID])
	}
	rm.addExplosion(40, 11, kit.Red)
	for i := 0; i < 10; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}

	allocs := testing.AllocsPerRun(50, func() {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	})
	t.Logf("3-player wake allocs/op: %.1f", allocs)
	// kittest's Send copies each frame into a growing per-player history (~3
	// allocs/tick, a harness artifact the real host doesn't have); the budget
	// rides above that but far below the ~3 frame allocations the old code did.
	if allocs > 40 {
		t.Fatalf("3-player wake allocates %.1f/op — permanent growth under -gc=leaking (budget 40); did render() stop reusing rm.frame?", allocs)
	}
}

// TestRenderReusesFrame asserts render keeps using the one long-lived buffer
// rather than allocating a fresh frame per tick.
func TestRenderReusesFrame(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	before := rm.frame
	rm.render(tr)
	rm.render(tr)
	if rm.frame != before {
		t.Fatal("render replaced rm.frame — it must reuse the single long-lived buffer")
	}
}
