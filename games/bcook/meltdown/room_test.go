package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// newTestRoom builds a started room driven by a kittest.Room (deterministic
// clock + rng), with no crew joined yet.
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
func keyNamed(k kit.Key) kit.Input {
	return kit.Input{Kind: kit.InputKey, Key: k}
}

// standOn moves a crew member directly onto a cell (test helper that bypasses
// the walls — we are positioning, not pathfinding).
func standOn(m *crewMember, row, col int) { m.row, m.col = row, col }

// --- map invariants ----------------------------------------------------------

func TestShipHasStationsAndCore(t *testing.T) {
	rm, _ := newTestRoom(t, "alice")
	if len(rm.stations) < 8 {
		t.Fatalf("want a decent spread of stations, got %d", len(rm.stations))
	}
	cores := 0
	for r := 0; r < interiorRows; r++ {
		for c := 0; c < cols; c++ {
			if rm.grid[r][c] == cellCore {
				cores++
			}
		}
	}
	if cores == 0 {
		t.Fatal("no core cells in the reactor")
	}
	// Every station must be walkable (so a crew member can reach it to work it).
	for _, st := range rm.stations {
		if !rm.walkable(st[0], st[1]) {
			t.Fatalf("station %v is not walkable", st)
		}
	}
}

func TestWallsBlockMovement(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	m := rm.crew[a.AccountID]
	// Park against the top hull and try to walk into it.
	standOn(m, 1, 5)
	rm.OnInput(tr, a, keyNamed(kit.KeyUp)) // row 0 is the hull wall
	if m.row != 1 {
		t.Fatalf("crew walked into a wall: row %d", m.row)
	}
}

// --- LEAK: mash space --------------------------------------------------------

func TestLeakPatchedByMashing(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	m := rm.crew[a.AccountID]

	st := rm.stations[0]
	standOn(m, st[0], st[1])
	rm.faults = []*fault{{kind: faultLeak, row: st[0], col: st[1], born: tr.Clock}}

	for i := 0; i < leakMashes-1; i++ {
		rm.OnInput(tr, a, keyRune(' '))
	}
	if len(rm.faults) != 1 {
		t.Fatalf("leak should still be open after %d mashes", leakMashes-1)
	}
	rm.OnInput(tr, a, keyRune(' ')) // the final mash
	if len(rm.faults) != 0 {
		t.Fatalf("leak should be patched after %d mashes, %d remain", leakMashes, len(rm.faults))
	}
	if m.fixes != 1 {
		t.Fatalf("patching a leak should credit a fix, got %d", m.fixes)
	}
}

func TestLeakIgnoresMashWhenNotStandingOnIt(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	m := rm.crew[a.AccountID]

	st := rm.stations[0]
	standOn(m, st[0]+0, st[1]) // ensure on a known floor cell, not the leak
	// Put the crew member somewhere that is NOT the leak cell.
	other := rm.stations[1]
	standOn(m, other[0], other[1])
	rm.faults = []*fault{{kind: faultLeak, row: st[0], col: st[1], born: tr.Clock}}

	for i := 0; i < leakMashes*2; i++ {
		rm.OnInput(tr, a, keyRune(' '))
	}
	if len(rm.faults) != 1 {
		t.Fatal("a leak should not be patched from a distance")
	}
}

// --- FIRE: hold space, regrows on release ------------------------------------

func TestFireExtinguishedByHolding(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	m := rm.crew[a.AccountID]

	st := rm.stations[0]
	standOn(m, st[0], st[1]) // stand on it (also counts as adjacent)
	rm.faults = []*fault{{kind: faultFire, row: st[0], col: st[1], born: tr.Clock}}

	// Hold space across several wakes: each wake observes a fresh space press
	// (auto-repeat) and integrates hold time.
	for i := 0; i < 60 && len(rm.faults) > 0; i++ {
		rm.OnInput(tr, a, keyRune(' ')) // repeat keeps the hold alive
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if len(rm.faults) != 0 {
		t.Fatalf("fire should be out after a sustained hold, %d remain", len(rm.faults))
	}
	if m.fixes != 1 {
		t.Fatalf("extinguishing a fire should credit a fix, got %d", m.fixes)
	}
}

func TestFireRegrowsWhenReleased(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	m := rm.crew[a.AccountID]

	st := rm.stations[0]
	standOn(m, st[0], st[1])
	f := &fault{kind: faultFire, row: st[0], col: st[1], born: tr.Clock}
	rm.faults = []*fault{f}

	// Hold briefly to build some progress.
	rm.OnInput(tr, a, keyRune(' '))
	tr.Advance(50 * time.Millisecond)
	rm.OnWake(tr)
	got := f.progress
	if got <= 0 {
		t.Fatal("holding should build fire progress")
	}

	// Now stop pressing space. The hold tracker lingers, so advance well past
	// the linger window before checking that progress decays.
	for i := 0; i < 30; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if len(rm.faults) == 0 {
		t.Fatal("an unattended fire should not extinguish itself")
	}
	if f.progress >= got {
		t.Fatalf("released fire should regrow (progress fell from %.3f to %.3f)", got, f.progress)
	}
}

// --- VALVE: key sequence -----------------------------------------------------

func TestValveOpenedByCorrectSequence(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	m := rm.crew[a.AccountID]

	st := rm.stations[0]
	standOn(m, st[0], st[1])
	seq := []rune{'a', 'b', 'c'}
	rm.faults = []*fault{{kind: faultValve, row: st[0], col: st[1], born: tr.Clock, seq: seq}}

	for _, ru := range seq {
		rm.OnInput(tr, a, keyRune(ru))
	}
	if len(rm.faults) != 0 {
		t.Fatalf("valve should open after the full sequence, %d remain", len(rm.faults))
	}
	if m.fixes != 1 {
		t.Fatalf("opening a valve should credit a fix, got %d", m.fixes)
	}
}

func TestValveWrongKeyResetsProgress(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	m := rm.crew[a.AccountID]

	st := rm.stations[0]
	standOn(m, st[0], st[1])
	f := &fault{kind: faultValve, row: st[0], col: st[1], born: tr.Clock, seq: []rune{'a', 'b', 'c'}}
	rm.faults = []*fault{f}

	rm.OnInput(tr, a, keyRune('a'))
	rm.OnInput(tr, a, keyRune('b'))
	if f.seqAt != 2 {
		t.Fatalf("two correct keys should advance to 2, got %d", f.seqAt)
	}
	rm.OnInput(tr, a, keyRune('z')) // wrong
	if f.seqAt != 0 || f.progress != 0 {
		t.Fatalf("a wrong key must reset the valve, got seqAt=%d progress=%.2f", f.seqAt, f.progress)
	}
	if len(rm.faults) != 1 {
		t.Fatal("valve should remain after a wrong key")
	}
}

// --- BREACH: two crew, never solo --------------------------------------------

func TestBreachNeedsTwoCrew(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	ma, mb := rm.crew[a.AccountID], rm.crew[b.AccountID]

	st := rm.stations[0]
	standOn(ma, st[0], st[1])
	// bob stands elsewhere first.
	other := rm.stations[1]
	standOn(mb, other[0], other[1])
	f := &fault{kind: faultBreach, row: st[0], col: st[1], born: tr.Clock}
	rm.faults = []*fault{f}

	// Only alice on it: progress must not build.
	for i := 0; i < 40; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if len(rm.faults) == 0 {
		t.Fatal("a breach with one crew should not seal")
	}

	// Both on it now: it seals.
	standOn(mb, st[0], st[1])
	for i := 0; i < 60 && len(rm.faults) > 0; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if len(rm.faults) != 0 {
		t.Fatalf("a breach with two crew should seal, %d remain", len(rm.faults))
	}
	if ma.fixes != 1 || mb.fixes != 1 {
		t.Fatalf("a sealed breach should credit both crew, got %d / %d", ma.fixes, mb.fixes)
	}
}

func TestBreachNeverSpawnsSolo(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	rng := tr.Rand()
	// pickFaultKind must never return a breach with a lone engineer, however
	// the rng falls.
	for i := 0; i < 5000; i++ {
		if rm.pickFaultKind(rng) == faultBreach {
			t.Fatal("a two-person breach was offered to a solo crew")
		}
	}
}

func TestBreachCanSpawnWithCrew(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	rm.OnJoin(tr, tr.Players[0])
	rm.OnJoin(tr, tr.Players[1])
	rng := tr.Rand()
	sawBreach := false
	for i := 0; i < 5000 && !sawBreach; i++ {
		if rm.pickFaultKind(rng) == faultBreach {
			sawBreach = true
		}
	}
	if !sawBreach {
		t.Fatal("a 2-crew shift should occasionally offer a breach")
	}
}

// --- core damage from neglect ------------------------------------------------

func TestNeglectedFaultsDrainCore(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	rm.faults = []*fault{
		{kind: faultLeak, row: rm.stations[0][0], col: rm.stations[0][1], born: tr.Clock},
		{kind: faultFire, row: rm.stations[1][0], col: rm.stations[1][1], born: tr.Clock},
	}
	before := rm.core
	for i := 0; i < 20; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if rm.core >= before {
		t.Fatalf("neglected faults should drain the core (was %.1f, now %.1f)", before, rm.core)
	}
}

func TestCoreDeathEndsRunAndFreezesTime(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	rm.core = 1.0
	rm.faults = []*fault{{kind: faultBreach, row: rm.stations[0][0], col: rm.stations[0][1], born: tr.Clock}}

	for i := 0; i < 200 && rm.phase == phaseRunning; i++ {
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
	}
	if rm.phase != phaseOver {
		t.Fatal("the run should end when the core hits zero")
	}
	frozen := rm.survivedSeconds()
	// Survival time must not keep climbing after the meltdown.
	tr.Advance(5 * time.Second)
	rm.OnWake(tr)
	if rm.survivedSeconds() != frozen {
		t.Fatalf("survival time kept ticking after meltdown: %.2f -> %.2f", frozen, rm.survivedSeconds())
	}
}

// --- spawn-rate scaling by crew size -----------------------------------------

func TestSpawnIntervalShrinksWithCrew(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob", "cleo")
	rm.now = tr.Clock // pin elapsed = 0 for an apples-to-apples comparison

	rm.OnJoin(tr, tr.Players[0])
	solo := rm.spawnInterval()

	rm.OnJoin(tr, tr.Players[1])
	rm.OnJoin(tr, tr.Players[2])
	rm.now = tr.Clock
	trio := rm.spawnInterval()

	if !(trio < solo) {
		t.Fatalf("more crew should shorten the spawn gap (solo %v, trio %v)", solo, trio)
	}
	// Sub-linear: a 3x crew must NOT triple the spawn rate. The gap should fall
	// by less than 3x, i.e. trio > solo/3.
	if trio <= solo/3 {
		t.Fatalf("crew scaling should be sub-linear: trio gap %v should exceed solo/3 %v", trio, solo/3)
	}
}

func TestSpawnIntervalShrinksOverTime(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	rm.now = rm.startedAt
	early := rm.spawnInterval()
	rm.now = rm.startedAt.Add(3 * time.Minute)
	late := rm.spawnInterval()
	if !(late < early) {
		t.Fatalf("the panic should ramp: late gap %v should be shorter than early %v", late, early)
	}
	floor := time.Duration(spawnFloor / 2 * float64(time.Second))
	if late < floor {
		t.Fatalf("spawn gap should respect the floor %v, got %v", floor, late)
	}
}

// --- survival scoring + persistence ------------------------------------------

func TestSurvivalScorePersisted(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)

	// Run a few seconds then end it.
	tr.Advance(7 * time.Second)
	rm.now = tr.Clock
	rm.core = 0
	rm.OnWake(tr)
	if rm.phase != phaseOver {
		t.Fatal("run should be over")
	}

	best := rm.loadBest(tr, a)
	if best < 7 {
		t.Fatalf("best survival should persist at least the run length, got %d", best)
	}
}

// --- general no-panic soak ---------------------------------------------------

func TestSoakNoPanic(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	inputs := []kit.Input{
		keyNamed(kit.KeyLeft), keyNamed(kit.KeyRight), keyNamed(kit.KeyUp),
		keyNamed(kit.KeyDown), keyRune(' '), keyRune('a'), keyRune('x'),
	}
	players := []kit.Player{a, b}
	for i := 0; i < 1200; i++ {
		p := players[i%2]
		rm.OnInput(tr, p, inputs[i%len(inputs)])
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
		// Faults must never exceed the cap, and crew must stay walkable.
		if len(rm.faults) > maxFaults {
			t.Fatalf("fault cap exceeded: %d", len(rm.faults))
		}
		for id, m := range rm.crew {
			if !rm.walkable(m.row, m.col) {
				t.Fatalf("crew %s stands on a non-walkable cell (%d,%d)", id, m.row, m.col)
			}
		}
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

// TestRenderReusesFrame asserts render keeps using the one long-lived buffer.
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

// TestRosterCharacterRendersBesideName asserts each crew member's character
// tile sits before their name on the roster, in join order.
func TestRosterCharacterRendersBesideName(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	a.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	b.Character = kit.Character{Glyph: "@", InkR: 1, InkG: 2, InkB: 3, BgR: 4, BgG: 5, BgB: 6, Fallback: '@'}
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	f := kit.NewFrame()
	rm.composeFor(f, a)

	// The first roster tile is alice's character at col 1, name 'a' two cols on.
	if got := f.Cells[0][1]; got != kit.CharacterCell(a.Character) {
		t.Errorf("first roster cell = %+v, want alice's character tile", got)
	}
	if f.Cells[0][3].Rune != 'a' {
		t.Errorf("name should follow the character tile, got %q", f.Cells[0][3].Rune)
	}
}

// TestCrewBodyUsesCharacterColour asserts a crew member's body glyph is their
// character glyph in the character's bg colour (composed for another viewer so
// the cell is free of reverse-video).
func TestCrewBodyUsesCharacterColour(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	a.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	ma, mb := rm.crew[a.AccountID], rm.crew[b.AccountID]
	want := kit.RGB(a.Character.BgR, a.Character.BgG, a.Character.BgB)
	if ma.color != want {
		t.Fatalf("crew colour = %v, want character bg %v", ma.color, want)
	}
	standOn(ma, 2, 2)
	standOn(mb, 20, 70) // park the rival clear

	f := kit.NewFrame()
	rm.composeFor(f, b)
	cell := f.Cells[top+2][2]
	if cell.Rune != 'λ' {
		t.Fatalf("body rune = %q, want the character glyph 'λ'", cell.Rune)
	}
	if cell.FG != want {
		t.Fatalf("body FG = %v, want character bg colour %v", cell.FG, want)
	}
}

// TestZeroCharacterCrewUsesPalette is the regression guard: a member with no
// character keeps the '☺' body and a join-order palette colour.
func TestZeroCharacterCrewUsesPalette(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0] // kittest player: zero Character
	rm.OnJoin(tr, a)
	m := rm.crew[a.AccountID]
	if m.glyph != '☺' {
		t.Fatalf("body glyph = %q, want '☺' for a zero character", m.glyph)
	}
	if m.color != palette[0] {
		t.Fatalf("crew colour = %v, want palette[0] %v", m.color, palette[0])
	}
}
