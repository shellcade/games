package main

import (
	"context"
	"math"
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

func TestHealthBarShowsOnlyWhenHurt(t *testing.T) {
	r, rm := newGame(t, "p1")
	tk := rm.tanks[1]
	row := int(math.Round(tk.y)) - 1

	rm.render(r)
	full := r.LastFrame(kittest.Player("p1"))
	if full.Cells[row][tk.col].BG == hpBandColor(tk.health) {
		t.Error("a full-health tank should not show a health bar")
	}

	tk.health = 50 // amber band
	rm.render(r)
	hurt := r.LastFrame(kittest.Player("p1"))
	if hurt.Cells[row][tk.col-2].BG != hpBandColor(50) {
		t.Errorf("hurt tank bar cell BG = %v, want amber band %v", hurt.Cells[row][tk.col-2].BG, hpBandColor(50))
	}
}

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
	rm.startMatch(r) // skip the lobby wait — most tests want a live battle
	return r, rm
}

func TestLobbyGathersBeforeStart(t *testing.T) {
	r := kittest.NewRoom(kittest.Player("p1"), kittest.Player("p2"))
	rm := (Game{}).NewRoom(r.Config(), r.Services()).(*room)
	rm.OnStart(r)
	rm.OnJoin(r, kittest.Player("p1"))
	rm.OnJoin(r, kittest.Player("p2"))
	if rm.phase != phLobby {
		t.Fatalf("phase = %v, want lobby - joining must not auto-start", rm.phase)
	}
	if len(rm.tanks) != 0 {
		t.Errorf("tanks were created before the battle started: %d", len(rm.tanks))
	}
	// The lobby auto-starts after its window — and BOTH members get a tank, not
	// just whoever arrived first.
	r.Advance(lobbyWait + time.Second)
	rm.OnWake(r)
	if rm.phase != phAim {
		t.Fatalf("lobby did not start: phase = %v", rm.phase)
	}
	humans := 0
	for _, tk := range rm.tanks {
		if !tk.cpu {
			humans++
		}
	}
	if humans != 2 {
		t.Errorf("started with %d human tanks, want both lobby members", humans)
	}
}

func TestCpuCountSelectable(t *testing.T) {
	r, rm := newGame(t, "p1")
	rm.cpuWanted = 4 // 1 human + 4 CPUs = 5 tanks (within the 6 cap)
	rm.clampCpu()
	rm.startMatch(r)
	cpus := 0
	for _, tk := range rm.tanks {
		if tk.cpu {
			cpus++
		}
	}
	if cpus != 4 || len(rm.tanks) != 5 {
		t.Errorf("CPU count not honoured: %d CPUs, %d tanks (want 4 / 5)", cpus, len(rm.tanks))
	}
}

func TestCpuCountClamps(t *testing.T) {
	_, rm := newGame(t, "p1")
	rm.cpuWanted = 99
	rm.clampCpu()
	if rm.cpuWanted != len(tankPalette)-1 { // one human leaves room for five CPUs
		t.Errorf("cpuWanted = %d, want %d", rm.cpuWanted, len(tankPalette)-1)
	}
	rm.cpuWanted = -5
	rm.clampCpu()
	if rm.cpuWanted != 1 { // still need at least one opponent
		t.Errorf("cpuWanted = %d, want 1", rm.cpuWanted)
	}
}

func TestSoloMatchSetup(t *testing.T) {
	_, rm := newGame(t, "p1")
	if rm.phase != phAim {
		t.Fatalf("phase = %v, want aim once the solo battle starts", rm.phase)
	}
	if len(rm.tanks) != soloTanks {
		t.Fatalf("solo match has %d tanks, want %d (1 human + CPUs)", len(rm.tanks), soloTanks)
	}
	cur := rm.currentTank()
	if cur == nil || cur.cpu || cur.id != "p1" {
		t.Errorf("first turn should be the human, got %+v", cur)
	}
	cpus := 0
	for _, t := range rm.tanks {
		if t.cpu {
			cpus++
		}
	}
	if cpus != soloTanks-1 {
		t.Errorf("got %d CPU tanks, want %d", cpus, soloTanks-1)
	}
}

func TestFireLaunchesShell(t *testing.T) {
	r, rm := newGame(t, "p1")
	rm.fire(r)
	if rm.phase != phFlight || rm.shell == nil {
		t.Fatalf("after firing: phase=%v shell=%v, want flight + a shell", rm.phase, rm.shell)
	}
}

func TestExplosionDamagesNearbyTanks(t *testing.T) {
	_, rm := newGame(t, "p1")
	victim := rm.tanks[1]
	hp := victim.health
	rm.now = time.Unix(100, 0)
	rm.explode(float64(victim.col), victim.y, weapons[0])
	if victim.health >= hp {
		t.Errorf("a blast at the tank did no damage: %d -> %d", hp, victim.health)
	}
}

func TestDirectHeavyHitCanKill(t *testing.T) {
	_, rm := newGame(t, "p1")
	victim := rm.tanks[1]
	victim.health = 20
	rm.now = time.Unix(100, 0)
	rm.explode(float64(victim.col), victim.y, weapons[1]) // HEAVY, point blank
	if victim.alive {
		t.Error("a point-blank HEAVY on a 20-hp tank should destroy it")
	}
}

func TestMatchEndsWithAWinnerAndAward(t *testing.T) {
	r, rm := newGame(t, "p1")
	// Wipe out every tank but the human.
	for _, tk := range rm.tanks {
		if tk.id != "p1" {
			tk.alive = false
		}
	}
	rm.now = time.Unix(100, 0)
	if !rm.checkMatchEnd(r) {
		t.Fatal("match should end with one tank left")
	}
	if rm.phase != phOver || rm.winner == nil || rm.winner.id != "p1" {
		t.Fatalf("winner not set: phase=%v winner=%v", rm.phase, rm.winner)
	}
	if rm.wins["p1"] != 1 {
		t.Errorf("win not tallied: %d", rm.wins["p1"])
	}
	if got, _ := kvInt(r.Services().Accounts.For(kittest.Player("p1")).Store(), "wins"); got != 1 {
		t.Errorf("persisted wins = %d, want 1", got)
	}
	if len(r.Posted) == 0 {
		t.Error("the win did not reach the leaderboard")
	}
}

func TestCPUTakesItsTurnAndFires(t *testing.T) {
	r, rm := newGame(t, "p1")
	rm.advanceTurn(r) // hand the turn to the first CPU
	cur := rm.currentTank()
	if cur == nil || !cur.cpu {
		t.Fatalf("expected a CPU turn, got %+v", cur)
	}
	r.Advance(thinkDelay + 200*time.Millisecond)
	rm.OnWake(r)
	if rm.phase != phFlight || rm.shell == nil {
		t.Errorf("CPU did not fire: phase=%v shell=%v", rm.phase, rm.shell)
	}
}

func TestTurnAdvancesPastDeadTanks(t *testing.T) {
	r, rm := newGame(t, "p1")
	rm.tanks[1].alive = false // kill the next-in-line
	start := rm.turn
	rm.advanceTurn(r)
	if rm.currentTank() == nil || !rm.currentTank().alive {
		t.Fatal("advanced onto a dead tank")
	}
	if rm.turn == start {
		t.Error("turn did not advance")
	}
}

// kvInt reads an integer from a durable store (the win counter).
func kvInt(store kit.KVStore, key string) (int, bool) {
	v, ok, err := store.Get(context.Background(), key)
	if err != nil || !ok {
		return 0, false
	}
	n := 0
	for _, b := range v {
		if b < '0' || b > '9' {
			continue
		}
		n = n*10 + int(b-'0')
	}
	return n, true
}
