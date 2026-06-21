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
