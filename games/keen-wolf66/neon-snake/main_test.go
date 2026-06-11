package main

import (
	"context"
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// newRoom spins up a started room on the kittest double with the given members.
func newRoom(t *testing.T, players ...kit.Player) (*room, *kittest.Room) {
	t.Helper()
	tr := kittest.NewRoom(players...)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	for _, p := range players {
		rm.OnJoin(tr, p)
	}
	// Clear the procedurally-placed food/obstacles so tests control the board.
	rm.food = Point{X: 0, Y: 0}
	rm.obstacles = nil
	return rm, tr
}

// TestHeadOnCollisionEndsGame: two snakes a cell apart, driving into each other,
// both crash on the next deterministic step → draw / game over.
func TestHeadOnCollisionEndsGame(t *testing.T) {
	a, b := kittest.Player("a"), kittest.Player("b")
	rm, tr := newRoom(t, a, b)

	rm.snake1 = []Point{{X: 10, Y: 9}, {X: 9, Y: 9}}
	rm.entityDir1 = Point{X: 1, Y: 0}
	rm.lastMovedDir1 = rm.entityDir1
	rm.snake2 = []Point{{X: 12, Y: 9}, {X: 13, Y: 9}}
	rm.entityDir2 = Point{X: -1, Y: 0}
	rm.lastMovedDir2 = rm.entityDir2

	rm.tick(tr)

	if !rm.gameOver {
		t.Fatal("expected game over after a head-on collision")
	}
	if !rm.crashed1 || !rm.crashed2 {
		t.Fatalf("expected both snakes crashed (draw); crashed1=%v crashed2=%v", rm.crashed1, rm.crashed2)
	}
	if len(tr.Posted) == 0 {
		t.Fatal("expected a leaderboard Post on game over")
	}
}

// TestObstacleCollisionEndsGame: a snake driving into an obstacle crashes.
func TestObstacleCollisionEndsGame(t *testing.T) {
	a := kittest.Player("a")
	rm, tr := newRoom(t, a)

	rm.snake1 = []Point{{X: 10, Y: 9}, {X: 9, Y: 9}}
	rm.entityDir1 = Point{X: 1, Y: 0}
	rm.lastMovedDir1 = rm.entityDir1
	rm.snake2 = []Point{{X: 30, Y: 2}, {X: 31, Y: 2}} // parked out of the way
	rm.entityDir2 = Point{X: 0, Y: 0}
	rm.lastMovedDir2 = Point{X: -1, Y: 0}
	rm.obstacles = []Point{{X: 11, Y: 9}}

	rm.tick(tr)

	if !rm.crashed1 {
		t.Fatal("snake 1 should have crashed into the obstacle")
	}
}

// TestShieldPreventsCrash: an obstacle that would be fatal is survived while a
// shield is active.
func TestShieldPreventsCrash(t *testing.T) {
	a := kittest.Player("a")
	rm, tr := newRoom(t, a)

	rm.snake1 = []Point{{X: 10, Y: 9}, {X: 9, Y: 9}}
	rm.entityDir1 = Point{X: 1, Y: 0}
	rm.lastMovedDir1 = rm.entityDir1
	rm.snake2 = []Point{{X: 30, Y: 2}, {X: 31, Y: 2}} // crawls left along row 2, out of the way
	rm.entityDir2 = Point{X: -1, Y: 0}
	rm.lastMovedDir2 = Point{X: -1, Y: 0}
	rm.obstacles = []Point{{X: 11, Y: 9}}

	rm.p1PowerUpType = "SHIELD"
	rm.p1PowerUpExpiry = tr.Now().Add(5 * time.Second)

	rm.tick(tr)

	if rm.crashed1 || rm.gameOver {
		t.Fatal("shield should have prevented the obstacle crash")
	}
}

// TestPowerUpPickup: driving a head onto a power-up grants it and clears it.
func TestPowerUpPickup(t *testing.T) {
	a := kittest.Player("a")
	rm, tr := newRoom(t, a)

	rm.snake1 = []Point{{X: 10, Y: 9}, {X: 9, Y: 9}}
	rm.entityDir1 = Point{X: 1, Y: 0}
	rm.lastMovedDir1 = rm.entityDir1
	rm.snake2 = []Point{{X: 30, Y: 2}, {X: 31, Y: 2}} // crawls left along row 2, out of the way
	rm.entityDir2 = Point{X: -1, Y: 0}
	rm.lastMovedDir2 = Point{X: -1, Y: 0}

	rm.powerUpActive = true
	rm.powerUpType = "FREEZE"
	rm.powerUpPos = Point{X: 11, Y: 9}
	rm.powerUpSpawnedAt = tr.Now()

	rm.tick(tr)

	if rm.powerUpActive {
		t.Fatal("power-up should be consumed after pickup")
	}
	if rm.p1PowerUpType != "FREEZE" {
		t.Fatalf("player 1 should hold FREEZE, got %q", rm.p1PowerUpType)
	}
}

// TestFoodTie: with shields up, two heads landing on the food the same tick both
// score (the explicit tie rule).
func TestFoodTie(t *testing.T) {
	a, b := kittest.Player("a"), kittest.Player("b")
	rm, tr := newRoom(t, a, b)

	rm.snake1 = []Point{{X: 10, Y: 9}, {X: 9, Y: 9}}
	rm.entityDir1 = Point{X: 1, Y: 0}
	rm.lastMovedDir1 = rm.entityDir1
	rm.snake2 = []Point{{X: 12, Y: 9}, {X: 13, Y: 9}}
	rm.entityDir2 = Point{X: -1, Y: 0}
	rm.lastMovedDir2 = rm.entityDir2
	rm.food = Point{X: 11, Y: 9}

	// Shields so the simultaneous head-overlap doesn't end the game first.
	rm.p1PowerUpType = "SHIELD"
	rm.p1PowerUpExpiry = tr.Now().Add(5 * time.Second)
	rm.p2PowerUpType = "SHIELD"
	rm.p2PowerUpExpiry = tr.Now().Add(5 * time.Second)

	rm.tick(tr)

	if rm.gameOver {
		t.Fatal("shielded snakes should not crash")
	}
	if rm.score1 != 10 || rm.score2 != 10 {
		t.Fatalf("both snakes should score on a tie; s1=%d s2=%d", rm.score1, rm.score2)
	}
}

// TestModeSwitchGuarded: 'm' cycles the mode only in the lobby / paused / over
// states, never mid-round.
func TestModeSwitchGuarded(t *testing.T) {
	a := kittest.Player("a")
	rm, tr := newRoom(t, a)

	// Paused: cycling is allowed.
	rm.gameStarted = false
	start := rm.gameMode
	rm.OnInput(tr, a, kit.Input{Kind: kit.InputRune, Rune: 'm'})
	if want := (start + 1) % modeCount; rm.gameMode != want {
		t.Fatalf("mode should advance from %d to %d, got %d", start, want, rm.gameMode)
	}

	// Mid-round: cycling is ignored (reset would silently wipe the game).
	rm.gameStarted = true
	rm.gameOver = false
	cur := rm.gameMode
	rm.OnInput(tr, a, kit.Input{Kind: kit.InputRune, Rune: 'm'})
	if rm.gameMode != cur {
		t.Fatalf("mode must not change mid-round (was %d, now %d)", cur, rm.gameMode)
	}
}

// TestPersonalBestSaveLoad: a solo run saves the better of the two snake scores,
// and a fresh room loads it on OnStart.
func TestPersonalBestSaveLoad(t *testing.T) {
	a := kittest.Player("a")
	rm, tr := newRoom(t, a)

	rm.score1 = 30
	rm.score2 = 80 // solo controls both — the better score is the PB
	rm.savePersonalBests(tr)

	val, ok, _ := tr.Services().Accounts.For(a).Store().Get(context.Background(), "personal_best")
	if !ok || string(val) != "80" {
		t.Fatalf("expected stored PB 80, got %q ok=%v", val, ok)
	}

	// A fresh room on the same store loads the PB during OnStart.
	rm2 := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm2.OnStart(tr)
	if rm2.pb1 != 80 {
		t.Fatalf("OnStart should load PB 80, got %d", rm2.pb1)
	}
}

// TestPersonalBestNotClobberedOnStoreOutage: a transient KV outage must not zero
// the in-memory PB.
func TestPersonalBestNotClobberedOnStoreOutage(t *testing.T) {
	a := kittest.Player("a")
	rm, tr := newRoom(t, a)

	rm.pb1 = 77
	tr.KVUnavailable = true
	rm.loadPersonalBests(tr)

	if rm.pb1 != 77 {
		t.Fatalf("PB should survive a store outage, got %d", rm.pb1)
	}
}

// TestSmokeSequenceStates mirrors smoke.yaml's step sequence (seats 2, 50ms
// heartbeat) and asserts each shot genuinely reaches the state its name claims.
// This is what the original smoke suite got wrong: names asserting state the run
// never reached.
func TestSmokeSequenceStates(t *testing.T) {
	a, b := kittest.Player("seat-0"), kittest.Player("seat-1")
	tr := kittest.NewRoom(a, b)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	heartbeat := 50 * time.Millisecond
	pump := func(d time.Duration) {
		for e := time.Duration(0); e < d; e += heartbeat {
			tr.Advance(heartbeat)
			rm.OnWake(tr)
		}
	}

	// shot: start — a frame has been rendered for both seats.
	rm.OnWake(tr)
	if tr.LastFrame(a) == nil || tr.LastFrame(b) == nil {
		t.Fatal("start: expected a rendered frame for each seat")
	}

	// shot: theme_cycled — 't' from seat 0 advances the palette.
	startTheme := rm.themeIndex
	rm.OnInput(tr, a, kit.Input{Kind: kit.InputRune, Rune: 't'})
	if rm.themeIndex == startTheme {
		t.Fatal("theme_cycled: theme did not change")
	}

	// shot: game_over — hands-off head-on collision ends the round.
	pump(1500 * time.Millisecond)
	if !rm.gameOver {
		t.Fatal("game_over: round should have ended from the head-on collision")
	}

	// shot: mode_hazards — 'm' after game over cycles into HAZARDS and restarts.
	rm.OnInput(tr, a, kit.Input{Kind: kit.InputRune, Rune: 'm'})
	pump(150 * time.Millisecond)
	if rm.gameMode != ModeHazard {
		t.Fatalf("mode_hazards: expected ModeHazard, got %d", rm.gameMode)
	}
	if rm.gameOver {
		t.Fatal("mode_hazards: a fresh round should be running, not over")
	}

	// shot: settings_menu — pause, then 's' opens the settings overlay.
	rm.OnInput(tr, a, kit.Input{Kind: kit.InputRune, Rune: ' '}) // space: pause
	rm.OnInput(tr, a, kit.Input{Kind: kit.InputRune, Rune: 's'})
	if !rm.settingsOpen {
		t.Fatal("settings_menu: settings overlay should be open")
	}
}

// TestEdgesWrap: a head crossing the right edge wraps to the left, it does not
// crash.
func TestEdgesWrap(t *testing.T) {
	a := kittest.Player("a")
	rm, tr := newRoom(t, a)

	rm.snake1 = []Point{{X: 38, Y: 9}, {X: 37, Y: 9}}
	rm.entityDir1 = Point{X: 1, Y: 0}
	rm.lastMovedDir1 = rm.entityDir1
	rm.snake2 = []Point{{X: 30, Y: 2}, {X: 31, Y: 2}} // crawls left along row 2, out of the way
	rm.entityDir2 = Point{X: -1, Y: 0}
	rm.lastMovedDir2 = Point{X: -1, Y: 0}

	rm.tick(tr)

	if rm.gameOver {
		t.Fatal("wrapping the edge should not end the game")
	}
	if got := rm.snake1[0]; got.X != 0 || got.Y != 9 {
		t.Fatalf("head should wrap to {0,9}, got %+v", got)
	}
}
