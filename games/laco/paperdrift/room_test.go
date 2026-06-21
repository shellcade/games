package main

import (
	"math"
	"math/rand"
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

func key(k kit.Key) kit.Input { return kit.Input{Kind: kit.InputKey, Key: k} }

// wakeUntil drives the heartbeat (50ms) until cond holds or limit elapses,
// returning the elapsed virtual time (limit if cond never held).
func wakeUntil(r *kittest.Room, rm *room, limit time.Duration, cond func() bool) time.Duration {
	const step = 50 * time.Millisecond
	for el := time.Duration(0); el < limit; el += step {
		r.Advance(step)
		rm.OnWake(r)
		if cond() {
			return el
		}
	}
	return limit
}

func newFlight(t *testing.T, ids ...string) (*kittest.Room, *room) {
	t.Helper()
	players := make([]kit.Player, len(ids))
	for i, id := range ids {
		players[i] = kittest.Player(id)
	}
	r := kittest.NewRoom(players...)
	rm := newRoom(r.Cfg, r.Services())
	rm.OnStart(r)
	for _, p := range players {
		rm.OnJoin(r, p)
	}
	return r, rm
}

func TestJoinFullRoomStartsCountdownThenLaunches(t *testing.T) {
	r, rm := newFlight(t, "p1", "p2")
	if rm.phase != phCountdown {
		t.Fatalf("phase after a full join = %q, want countdown", rm.phase)
	}
	wakeUntil(r, rm, 5*time.Second, func() bool { return rm.phase == phFlying })
	if rm.phase != phFlying {
		t.Fatal("countdown never launched")
	}
	for _, id := range []string{"p1", "p2"} {
		ps := rm.pilots[id]
		if !ps.alive || !ps.flew {
			t.Errorf("pilot %s alive=%v flew=%v after launch, want true/true", id, ps.alive, ps.flew)
		}
	}
}

func TestDiveBuildsSpeedClimbBleedsIt(t *testing.T) {
	rm := newRoom(kit.RoomConfig{}, kit.Services{})
	// Dive from a cruise-ish speed (launch sits near terminal, where there is
	// little headroom left to accelerate) to exercise that diving builds speed.
	const diveEntry = 16.0
	dive := &pilot{x: 1000, y: 10, v: diveEntry, pitch: -0.6, alive: true}
	climb := &pilot{x: 1000, y: 30, v: 20, pitch: 0.6, alive: true}
	for i := 0; i < 40; i++ { // one second in physics quanta
		rm.stepPilot(dive, 0.025)
		rm.stepPilot(climb, 0.025)
	}
	if dive.v <= diveEntry+6 {
		t.Errorf("after a 1s dive v = %.1f, want well above entry %v", dive.v, diveEntry)
	}
	if climb.v >= 20 {
		t.Errorf("after a 1s climb v = %.1f, want below the entry 20", climb.v)
	}
	if climb.y >= 30 {
		t.Errorf("a fast climb should gain height: y = %.1f, want < 30", climb.y)
	}
}

func TestStallMushesNoseDown(t *testing.T) {
	rm := newRoom(kit.RoomConfig{}, kit.Services{})
	ps := &pilot{x: 1000, y: 10, v: 4, pitch: 0.5, alive: true}
	rm.stepPilot(ps, 0.025)
	if !ps.stalled {
		t.Error("v=4 with nose-up trim should read stalled")
	}
	for i := 0; i < 40; i++ {
		rm.stepPilot(ps, 0.025)
	}
	if ps.y <= 10 {
		t.Errorf("a stalled glider must sink despite nose-up trim: y = %.1f, want > 10", ps.y)
	}
}

func TestThermalLifts(t *testing.T) {
	rm := newRoom(kit.RoomConfig{}, kit.Services{})
	rm.terr.reset()
	rm.terr.ensure(rand.New(rand.NewSource(1)), 100)
	rm.terr.therm[30] = true
	ps := &pilot{x: 30, y: 20, v: 14, alive: true}
	rm.stepPilot(ps, 0.025)
	if ps.y >= 20 {
		t.Errorf("a thermal must lift a level glider: y = %.2f, want < 20", ps.y)
	}
}

func TestSoloRoundCrashesSettlesAndPosts(t *testing.T) {
	r, rm := newFlight(t, "solo")
	wakeUntil(r, rm, 5*time.Second, func() bool { return rm.phase == phFlying })

	el := wakeUntil(r, rm, roundCap+time.Minute, func() bool { return rm.phase == phResults })
	if rm.phase != phResults {
		t.Fatal("an untrimmed glider never crashed; gates should make that impossible")
	}
	if len(r.Posted) != 1 {
		t.Fatalf("posted %d results, want 1", len(r.Posted))
	}
	pr := r.Posted[0].Rankings[0]
	if pr.Player.AccountID != "solo" || pr.Metric < 0 || pr.Status != kit.StatusFinished {
		t.Errorf("posted ranking = %+v, want solo / non-negative metric / finished", pr)
	}
	t.Logf("untrimmed solo flight: crashed after %v at %dm (smoke calibration)", el, pr.Metric)

	// After the results hold the room relaunches by itself.
	wakeUntil(r, rm, resultsDur+time.Second, func() bool { return rm.phase == phCountdown })
	if rm.phase != phCountdown || rm.roundNum != 2 {
		t.Errorf("after results: phase=%q round=%d, want countdown round 2", rm.phase, rm.roundNum)
	}
}

func TestTrimInputsSteerTheGlider(t *testing.T) {
	r, rm := newFlight(t, "p1")
	wakeUntil(r, rm, 5*time.Second, func() bool { return rm.phase == phFlying })
	ps := rm.pilots["p1"]
	for i := 0; i < 4; i++ {
		rm.OnInput(r, kittest.Player("p1"), key(kit.KeyUp))
	}
	if ps.pitch <= 0.3 {
		t.Errorf("four up-trims: pitch = %.3f, want > 0.3", ps.pitch)
	}
	for i := 0; i < 40; i++ {
		rm.OnInput(r, kittest.Player("p1"), key(kit.KeyDown))
	}
	if ps.pitch != -pitchMax {
		t.Errorf("held down-trim must clamp: pitch = %.3f, want %v", ps.pitch, -pitchMax)
	}
}

func TestLeaverRanksDNF(t *testing.T) {
	r, rm := newFlight(t, "p1", "p2")
	wakeUntil(r, rm, 5*time.Second, func() bool { return rm.phase == phFlying })
	rm.OnLeave(r, kittest.Player("p2"))
	r.Players = r.Players[:1]

	wakeUntil(r, rm, roundCap+time.Minute, func() bool { return rm.phase == phResults })
	if len(r.Posted) != 1 {
		t.Fatalf("posted %d results, want 1", len(r.Posted))
	}
	var saw bool
	for _, pr := range r.Posted[0].Rankings {
		if pr.Player.AccountID == "p2" {
			saw = true
			if pr.Status != kit.StatusDNF {
				t.Errorf("leaver status = %v, want DNF", pr.Status)
			}
		}
	}
	if !saw {
		t.Error("mid-flight leaver missing from the round rankings")
	}
}

func TestPersonalBestPersistsViaMergeMax(t *testing.T) {
	r, rm := newFlight(t, "p1")
	wakeUntil(r, rm, 5*time.Second, func() bool { return rm.phase == phFlying })
	wakeUntil(r, rm, roundCap+time.Minute, func() bool { return rm.phase == phResults })

	v, ok := r.KV["p1"]["best"], r.KVRules["p1"]["best"]
	if v == nil {
		t.Fatal("no best written to KV after the round")
	}
	if ok != kit.MergeMax {
		t.Errorf("best merge rule = %v, want MergeMax", ok)
	}
	if !rm.pilots["p1"].newPB {
		t.Error("first flight should flag a new personal best")
	}
}

func TestCorridorAlwaysOpenAndGatesPassable(t *testing.T) {
	var tr terrain
	tr.reset()
	tr.ensure(rand.New(rand.NewSource(7)), 6000)
	gates := 0
	for x := 0; x < 6000; x++ {
		ce, fl := int(tr.ceil[x]), int(tr.floor[x])
		if fl-ce < 6 {
			t.Fatalf("col %d: corridor %d rows, want >= 6", x, fl-ce)
		}
		if gT := int(tr.gTop[x]); gT >= 0 {
			gates++
			gB := int(tr.gBot[x])
			if gB-gT < 4 {
				t.Fatalf("col %d: gate gap %d rows, want >= 4", x, gB-gT+1)
			}
			if gT <= ce {
				t.Fatalf("col %d: gate gap starts at %d inside the ceiling band %d", x, gT, ce)
			}
		}
	}
	if gates < 50 {
		t.Errorf("only %d gate columns over 6000 - generation looks broken", gates)
	}
}

func TestSpawnSlotsHaveEqualEnergy(t *testing.T) {
	_, rm := newFlight(t, "p1", "p2", "p3", "p4")
	top := rm.pilots["p1"]
	for _, id := range []string{"p2", "p3", "p4"} {
		ps := rm.pilots[id]
		if ps.y <= top.y {
			t.Fatalf("%s spawned at y=%.2f, want below the top slot %.2f", id, ps.y, top.y)
		}
		if ps.v <= top.v {
			t.Errorf("%s starts lower but not faster: v=%.2f vs top %.2f", id, ps.v, top.v)
		}
		// ½v² − g·y (y grows downward) must match the top slot's total.
		got := ps.v*ps.v/2 - gravity*ps.y
		want := top.v*top.v/2 - gravity*top.y
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("%s spawn energy = %.3f, want %.3f (equal to the top slot)", id, got, want)
		}
	}
}

func TestLoneQuickPlayerWaitsThenAutoLaunches(t *testing.T) {
	r := kittest.NewRoom(kittest.Player("p1"))
	r.Cfg = kit.RoomConfig{Mode: kit.ModeQuick}
	rm := newRoom(r.Cfg, r.Services())
	rm.OnStart(r)
	rm.OnJoin(r, kittest.Player("p1"))
	if rm.phase != phLobby {
		t.Fatalf("lone quick join: phase = %q, want lobby (grace window)", rm.phase)
	}
	if r.LastFrame(kittest.Player("p1")) == nil {
		t.Fatal("the lobby never rendered")
	}
	wakeUntil(r, rm, lobbyGrace+launchCountdown+time.Second, func() bool { return rm.phase == phFlying })
	if rm.phase != phFlying {
		t.Fatal("the grace window never launched the lone player")
	}
}

func TestStormSweepsAHoverer(t *testing.T) {
	r, rm := newFlight(t, "solo")
	wakeUntil(r, rm, 5*time.Second, func() bool { return rm.phase == phFlying })
	// Trim hard up: the glider stall-hovers, which without the storm is a
	// stable limit cycle that would float in place until the round cap.
	for i := 0; i < 20; i++ {
		rm.OnInput(r, kittest.Player("solo"), key(kit.KeyUp))
	}
	el := wakeUntil(r, rm, time.Minute, func() bool { return rm.phase == phResults })
	if rm.phase != phResults {
		t.Fatal("the storm never swept a hovering glider; rounds must settle without the cap")
	}
	t.Logf("hoverer swept after %v at %dm", el, r.Posted[0].Rankings[0].Metric)
}

func TestRunsAreDeterministic(t *testing.T) {
	run := func() int {
		r, rm := newFlight(t, "solo")
		wakeUntil(r, rm, 5*time.Second, func() bool { return rm.phase == phFlying })
		wakeUntil(r, rm, roundCap+time.Minute, func() bool { return rm.phase == phResults })
		return r.Posted[0].Rankings[0].Metric
	}
	if a, b := run(), run(); a != b {
		t.Errorf("same seed, same wakes, different distances: %d vs %d", a, b)
	}
}
