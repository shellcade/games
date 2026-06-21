package main

import (
	"testing"

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
