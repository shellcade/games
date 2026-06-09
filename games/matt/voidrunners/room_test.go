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

		if len(rm.craters) < craterTarget {
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
