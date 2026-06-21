package main

import (
	"testing"
	"time"

	"github.com/shellcade/kit/v2/kittest"
)

func TestPawnMovementAndCollision(t *testing.T) {
	a, b := kittest.Player("a"), kittest.Player("b")
	rm, r := newGame(t, a, b)
	rm.OnJoin(r, a)
	rm.OnJoin(r, b)
	pa := rm.pawns[a.AccountID]
	pa.x, pa.y = 5, 5

	rm.tryMove(a.AccountID, 1, 0)
	if pa.x != 6 || pa.y != 5 {
		t.Fatalf("after east move pawn at (%d,%d), want (6,5)", pa.x, pa.y)
	}
	pa.x, pa.y = 1, 1
	rm.tryMove(a.AccountID, 0, -1) // into top wall (y=0)
	if pa.x != 1 || pa.y != 1 {
		t.Fatalf("wall move should be a no-op, pawn at (%d,%d)", pa.x, pa.y)
	}
	pb := rm.pawns[b.AccountID]
	pa.x, pa.y = 5, 5
	pb.x, pb.y = 6, 5
	rm.tryMove(a.AccountID, 1, 0)
	if pa.x != 5 {
		t.Fatalf("should not move onto another pawn, pawn at (%d,%d)", pa.x, pa.y)
	}
}

func TestSitAndStand(t *testing.T) {
	a, b := kittest.Player("a"), kittest.Player("b")
	rm, r := newGame(t, a, b)
	rm.OnJoin(r, a)
	rm.OnJoin(r, b)
	mc := rm.fmachines[0]

	pa := rm.pawns[a.AccountID]
	pa.x, pa.y = mc.ax, mc.ay
	rm.trySit(a.AccountID)
	if !pa.seated || pa.seat != mc.id {
		t.Fatalf("A should be seated at machine %d, got seated=%v seat=%d", mc.id, pa.seated, pa.seat)
	}
	if rm.occupied[mc.id] != a.AccountID {
		t.Fatalf("machine %d should be occupied by A", mc.id)
	}
	pb := rm.pawns[b.AccountID]
	pb.x, pb.y = mc.ax, mc.ay
	rm.trySit(b.AccountID)
	if pb.seated {
		t.Fatal("B must not sit at an occupied machine")
	}
	rm.standUp(a.AccountID)
	if pa.seated || (rm.occupied[mc.id] == a.AccountID) {
		t.Fatal("standing should free the seat")
	}
	if pa.x != mc.ax || pa.y != mc.ay {
		t.Fatalf("after standing pawn at (%d,%d), want approach (%d,%d)", pa.x, pa.y, mc.ax, mc.ay)
	}
	if rm.machines[a.AccountID].seatVar != nil {
		t.Error("standing should clear the seated variant binding")
	}
}

func TestInputRoutesByMode(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	pw := rm.pawns[p.AccountID]
	pw.x, pw.y = 5, 5

	rm.OnInput(r, p, keyRight()) // roaming: move east
	if pw.x != 6 {
		t.Fatalf("roaming right should move east, x=%d", pw.x)
	}
	m := rm.machines[p.AccountID]
	mc := rm.fmachines[0]
	pw.x, pw.y = mc.ax, mc.ay
	rm.trySit(p.AccountID)
	m.bet = betTiers[0]
	rm.OnInput(r, p, keyUp())
	if m.bet != betTiers[1] {
		t.Fatalf("seated up should raise bet to %d, got %d", betTiers[1], m.bet)
	}
}

func TestJoinSpawnsPawnAtEntrance(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	pw := rm.pawns[p.AccountID]
	if pw == nil {
		t.Fatal("join should create a pawn")
	}
	sx, sy := loungeSpawn()
	if pw.x != sx || pw.y != sy {
		t.Errorf("pawn at (%d,%d), want spawn (%d,%d)", pw.x, pw.y, sx, sy)
	}
	if pw.seated {
		t.Error("a fresh pawn should be roaming, not seated")
	}
}

// TestSmokeSequenceReachesAndSpins mirrors smoke.yaml's input (walk up to the
// front-centre machine, sit, pull) and asserts the player actually reaches a
// seat and starts a spin — so the smoke preview shows the game, not an empty
// floor, and the map stays navigable from the entrance.
func TestSmokeSequenceReachesAndSpins(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	pw := rm.pawns[p.AccountID]

	rm.OnInput(r, p, keyUp()) // step toward the machines
	rm.OnInput(r, p, keyUp()) // onto the front-centre approach tile
	if rm.machineAtApproach(pw.x, pw.y) == nil {
		t.Fatalf("after 2 up steps the player is at (%d,%d) — not on any machine's approach tile", pw.x, pw.y)
	}
	rm.OnInput(r, p, space()) // sit
	if !pw.seated {
		t.Fatalf("space on the approach tile should seat the player (at %d,%d)", pw.x, pw.y)
	}
	m := rm.machines[p.AccountID]
	m.bet = betTiers[0]
	rm.OnInput(r, p, space()) // pull
	if m.spin == nil {
		t.Fatal("seated, space should start a spin (the smoke would show an empty floor otherwise)")
	}
}

func TestWakeIgnoresRoamingMachines(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.freeSpins = 3
	m.freeBet, m.freeVar = 10, rm.variant
	for i := 0; i < 10; i++ {
		r.Advance(300 * time.Millisecond)
		rm.OnWake(r)
	}
	if m.freeSpins != 3 {
		t.Fatalf("roaming machine should not auto-play free spins, freeSpins=%d", m.freeSpins)
	}
}
