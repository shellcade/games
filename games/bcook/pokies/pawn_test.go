package main

import (
	"testing"

	"github.com/shellcade/kit/v2/kittest"
)

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
