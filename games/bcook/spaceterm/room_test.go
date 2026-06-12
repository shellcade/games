package main

import (
	"context"
	"testing"
	"time"

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
	rm.launch(r) // skip the lobby wait — most tests want a live sector
	return r, rm
}

// fulfil drives the targeted control until the order resolves (a dial may
// need several presses to cycle to the demanded position).
func fulfil(t *testing.T, r *kittest.Room, rm *room, o order) {
	t.Helper()
	cw := rm.crewByID(o.targetID)
	if cw == nil {
		t.Fatalf("order targets unknown crew %q", o.targetID)
	}
	for i := 0; i < dialMax+2; i++ {
		rm.actuate(r, cw, o.ctrlIdx)
		if owner := rm.ownerOf(o.seq); owner == nil {
			return // resolved
		}
	}
	t.Fatalf("order %q did not resolve after cycling its control", o.text)
}

func (rm *room) ownerOf(seq int) *crew {
	for _, c := range rm.crews {
		if c.ord.active && c.ord.seq == seq {
			return c
		}
	}
	return nil
}

func TestLobbyGathersBeforeLaunch(t *testing.T) {
	r := kittest.NewRoom(kittest.Player("p1"), kittest.Player("p2"))
	rm := (Game{}).NewRoom(r.Config(), r.Services()).(*room)
	rm.OnStart(r)
	rm.OnJoin(r, kittest.Player("p1"))
	rm.OnJoin(r, kittest.Player("p2"))
	if rm.phase != phLobby {
		t.Fatalf("phase = %v, want lobby — joining must not auto-launch", rm.phase)
	}
	// The fallback timer launches for everyone at once.
	r.Advance(lobbyWait + time.Second)
	rm.OnWake(r)
	if rm.phase != phSector || rm.sector != 1 {
		t.Fatalf("lobby did not launch: phase=%v sector=%d", rm.phase, rm.sector)
	}
	for _, c := range rm.crews {
		if !c.boarded || len(c.panel) != 6 || !c.ord.active {
			t.Errorf("crew %s not fully dealt in: boarded=%v panel=%d order=%v",
				c.id, c.boarded, len(c.panel), c.ord.active)
		}
	}
}

func TestShipNamesUnique(t *testing.T) {
	_, rm := newGame(t, "p1", "p2", "p3", "p4", "p5", "p6")
	seen := map[string]bool{}
	for _, c := range rm.crews {
		for _, ctrl := range c.panel {
			full := ctrl.adj + " " + ctrl.jot
			if seen[full] {
				t.Errorf("duplicate control name across the ship: %s", full)
			}
			seen[full] = true
		}
	}
}

func TestOrdersNeverPreSatisfiedAndUnique(t *testing.T) {
	_, rm := newGame(t, "p1", "p2", "p3")
	targets := map[[2]interface{}]bool{}
	for _, c := range rm.crews {
		o := c.ord
		if !o.active {
			t.Fatalf("crew %s has no order at sector start", c.id)
		}
		tc := rm.controlAt(o.targetID, o.ctrlIdx)
		if o.want != -1 && tc.state == o.want {
			t.Errorf("order %q already satisfied at issue", o.text)
		}
		k := [2]interface{}{o.targetID, o.ctrlIdx}
		if targets[k] {
			t.Errorf("two orders demand the same control: %v", k)
		}
		targets[k] = true
	}
}

func TestSoloOrdersTargetOwnPanel(t *testing.T) {
	r, rm := newGame(t, "solo")
	for i := 0; i < 8; i++ {
		o := rm.crews[0].ord
		if o.targetID != "solo" {
			t.Fatalf("solo order %d targets %q, want own panel", i, o.targetID)
		}
		fulfil(t, r, rm, o)
		if rm.phase != phSector {
			break // warped — fine
		}
	}
}

func TestCompleteOrderChargesAndReissues(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	c := rm.crews[0]
	first := c.ord.seq
	fulfil(t, r, rm, c.ord)
	if rm.charges != 1 {
		t.Errorf("charges = %d, want 1", rm.charges)
	}
	if c.done != 1 {
		t.Errorf("done = %d, want 1", c.done)
	}
	if !c.ord.active || c.ord.seq == first {
		t.Error("a fresh order was not issued after completion")
	}
}

func TestExpiryCostsHullAndReissues(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	c := rm.crews[0]
	first := c.ord.seq
	r.Advance(rm.orderDur() + time.Second)
	rm.OnWake(r)
	if rm.hull >= maxHull {
		t.Errorf("hull = %d, want damage after expiry", rm.hull)
	}
	if c.fumbles == 0 {
		t.Error("fumble not tallied")
	}
	if !c.ord.active || c.ord.seq == first {
		t.Error("a fresh order was not issued after the fumble")
	}
}

func TestWarpPatchesHullAndDealsNextSector(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	rm.hull = 5
	rm.charges = rm.need - 1
	fulfil(t, r, rm, rm.crews[0].ord)
	if rm.phase != phWarp {
		t.Fatalf("phase = %v, want warp once the bar fills", rm.phase)
	}
	if rm.hull != 5+hullPatch {
		t.Errorf("hull = %d, want %d after the patch", rm.hull, 5+hullPatch)
	}
	r.Advance(warpDur + time.Second)
	rm.OnWake(r)
	if rm.phase != phSector || rm.sector != 2 || rm.charges != 0 {
		t.Fatalf("sector 2 not dealt: phase=%v sector=%d charges=%d", rm.phase, rm.sector, rm.charges)
	}
	for _, c := range rm.crews {
		if !c.ord.active {
			t.Errorf("crew %s has no order in sector 2", c.id)
		}
	}
}

func TestHailTickerAndCooldown(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	c := rm.crews[0]
	rm.sendHail(c)
	if len(rm.hails) != 1 {
		t.Fatalf("hails = %d, want 1", len(rm.hails))
	}
	rm.sendHail(c) // inside the cooldown — swallowed
	if len(rm.hails) != 1 {
		t.Errorf("cooldown not enforced: %d hails", len(rm.hails))
	}
	r.Advance(hailCooldown + time.Second)
	rm.now = r.Now()
	rm.sendHail(c)
	if len(rm.hails) != 2 {
		t.Errorf("hail after cooldown swallowed: %d hails", len(rm.hails))
	}
	// Resolving the order clears its hail from the ticker.
	fulfil(t, r, rm, c.ord)
	for _, h := range rm.hails {
		if h.id == "p1" {
			t.Error("resolved order's hail still in the ticker")
		}
	}
}

func TestSoloHailSwallowed(t *testing.T) {
	_, rm := newGame(t, "solo")
	rm.sendHail(rm.crews[0])
	if len(rm.hails) != 0 {
		t.Error("a solo shift has nobody to hail")
	}
}

func TestMeteorPausesOrdersAndSettlesTheBill(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	rm.anKind, rm.anStage = anMeteor, asWarn
	rm.activateAnomaly(r)
	if !rm.meteorActive() {
		t.Fatal("storm did not activate")
	}
	for _, c := range rm.crews {
		if c.mashKey == 0 {
			t.Errorf("crew %s has no mash key", c.id)
		}
		if c.ord.paused <= 0 {
			t.Errorf("crew %s order clock not banked", c.id)
		}
	}
	// p1 rides it out, p2 freezes.
	rm.crews[0].mashN = mashNeed
	hull := rm.hull
	r.Advance(meteorDur + time.Second)
	rm.now = r.Now()
	rm.finishAnomaly(r)
	if rm.hull != hull-1 {
		t.Errorf("hull = %d, want %d (one crew missed)", rm.hull, hull-1)
	}
	for _, c := range rm.crews {
		if c.ord.active && !c.ord.expires.After(rm.now) {
			t.Errorf("crew %s order resumed already expired", c.id)
		}
	}
}

func TestMeteorPenaltyCapped(t *testing.T) {
	r, rm := newGame(t, "p1", "p2", "p3", "p4")
	rm.anKind, rm.anStage = anMeteor, asWarn
	rm.activateAnomaly(r)
	hull := rm.hull // nobody mashes
	r.Advance(meteorDur + time.Second)
	rm.now = r.Now()
	rm.finishAnomaly(r)
	if rm.hull != hull-meteorCap {
		t.Errorf("hull = %d, want %d (penalty capped at %d)", rm.hull, hull-meteorCap, meteorCap)
	}
}

func TestGameOverPostsOneCoopScore(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	rm.sector = 5
	rm.hull = 1
	r.Advance(15 * time.Second) // past any sector's order timer
	rm.OnWake(r)
	if rm.phase != phOver || rm.score != 4 {
		t.Fatalf("phase=%v score=%d, want debrief with 4 sectors", rm.phase, rm.score)
	}
	if len(r.Posted) == 0 {
		t.Fatal("score did not reach the leaderboard")
	}
	res := r.Posted[len(r.Posted)-1]
	if len(res.Rankings) != 2 {
		t.Fatalf("rankings = %d, want the whole crew", len(res.Rankings))
	}
	for _, pr := range res.Rankings {
		if pr.Metric != 4 {
			t.Errorf("metric = %d, want the shared 4", pr.Metric)
		}
	}
	v, ok, _ := r.Services().Accounts.For(kittest.Player("p1")).Store().Get(context.Background(), "best")
	if !ok || string(v) != "4" {
		t.Errorf("persisted best = %q, want 4", v)
	}
}

func TestJoinMidSectorSpectatesThenBoards(t *testing.T) {
	r, rm := newGame(t, "p1")
	rm.OnJoin(r, kittest.Player("late"))
	late := rm.crewByID("late")
	if late.boarded {
		t.Fatal("a mid-sector joiner must spectate until the warp")
	}
	rm.charges = rm.need - 1
	fulfil(t, r, rm, rm.crews[0].ord)
	r.Advance(warpDur + time.Second)
	rm.OnWake(r)
	if !late.boarded || len(late.panel) == 0 || !late.ord.active {
		t.Error("the joiner did not board at the warp")
	}
}

func TestLeaveRerollsOrdersAimedAtTheirPanel(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	c1 := rm.crewByID("p1")
	// Surgically aim p1's order at p2's panel, then have p2 leave.
	c1.ord.targetID = "p2"
	c1.ord.ctrlIdx = 0
	rm.OnLeave(r, kittest.Player("p2"))
	if !c1.ord.active {
		t.Fatal("p1 lost their order entirely")
	}
	if c1.ord.targetID == "p2" {
		t.Error("p1's order still targets the departed crew's panel")
	}
}

func TestWormholeMirrorsDrawOnly(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	c := rm.crews[0]
	key := c.panel[0].key
	st0 := c.panel[0].state
	rm.anKind, rm.anStage = anWormhole, asWarn
	rm.activateAnomaly(r)
	// The hotkey still actuates the same control while mirrored.
	rm.sectorInput(r, c, kit.Input{Kind: kit.InputRune, Rune: key})
	if c.panel[0].kind != ckButton && c.panel[0].state == st0 {
		t.Error("mirrored panel broke hotkey binding")
	}
	rm.render(r) // and the mirrored draw must not explode
}

func TestCoolantFogEatsPressesThenWipes(t *testing.T) {
	r, rm := newGame(t, "p1")
	c := rm.crews[0]
	ctrl := &c.panel[0]
	ctrl.fog = fogWipes
	st0 := ctrl.state
	for i := 0; i < fogWipes; i++ {
		rm.actuate(r, c, 0)
	}
	if ctrl.fog != 0 {
		t.Fatalf("fog = %d after %d wipes, want clear", ctrl.fog, fogWipes)
	}
	if ctrl.kind != ckButton && ctrl.state != st0 {
		t.Error("wiping presses must not actuate the control")
	}
	rm.actuate(r, c, 0)
	if ctrl.kind != ckButton && ctrl.state == st0 {
		t.Error("control dead after the fog cleared")
	}
}

// compose runs every wake, per viewer. Production builds use -gc=leaking, so
// any per-frame allocation is a permanent leak over a long shift.
func TestComposeAllocFree(t *testing.T) {
	r, rm := newGame(t, "p1", "p2")
	c := rm.crews[0]
	rm.sendHail(c)
	rm.sendHail(rm.crews[1])
	rm.anKind, rm.anStage = anFlare, asLive
	rm.anEndAt = rm.now.Add(flareDur)
	c.panel[1].fog = 2
	rm.fumbleText = "SET THE GYROSCOPIC PLURALIZER TO 4"
	rm.fumbleUntil = rm.now.Add(time.Second)

	f := kit.NewFrame()
	v := kittest.Player("p1")
	if a := testing.AllocsPerRun(50, func() { rm.compose(f, v) }); a != 0 {
		t.Errorf("sector compose allocates %.1f/frame — must stay alloc-free under -gc=leaking", a)
	}

	rm.phase = phLobby
	if a := testing.AllocsPerRun(50, func() { rm.compose(f, v) }); a != 0 {
		t.Errorf("lobby compose allocates %.1f/frame", a)
	}
	rm.phase = phWarp
	rm.warpUntil = rm.now.Add(warpDur)
	if a := testing.AllocsPerRun(50, func() { rm.compose(f, v) }); a != 0 {
		t.Errorf("warp compose allocates %.1f/frame", a)
	}
	rm.phase = phOver
	if a := testing.AllocsPerRun(50, func() { rm.compose(f, v) }); a != 0 {
		t.Errorf("debrief compose allocates %.1f/frame", a)
	}
	_ = r
}
