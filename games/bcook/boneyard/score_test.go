package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// The design §8 worked examples, pinned: the bones-engager beats the
// stair-diver by design.
func TestSoulScoreWorkedExamples(t *testing.T) {
	engager := &delver{banked: 9, kills: 18, gold: 900, respects: 6, avenges: 3,
		looted: 4, devours: 1, firstBankTurn: 220}
	if got := engager.soulScore(); got != 900+250+144+90+1200+450+240-40+38 {
		t.Fatalf("bones-engager = %d, want 3272 (design example minus the unbuilt gasps term)", got)
	}
	diver := &delver{banked: 11, kills: 12, gold: 300, firstBankTurn: 540}
	if got := diver.soulScore(); got != 1100+250+96+30+6 {
		t.Fatalf("stair-diver = %d", got)
	}
	if engager.soulScore() <= diver.soulScore() {
		t.Fatal("the stair-diver out-scored the bones-engager — the design inverted")
	}
}

// Banking is the Greed Engine: depth posts ONLY at shrines, never twice for
// the same floor, and the KV counters land for the weekly boards.
func TestBankingAtTheShrine(t *testing.T) {
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	d := rm.delvers[a.AccountID]

	// Not on a shrine: nothing banks.
	d.bank(rm, tr)
	if d.banked != 0 || len(tr.Posted) != 0 {
		t.Fatal("banked off-shrine")
	}

	// On B3's shrine: the depth banks and posts.
	f3 := rm.floorAt(3)
	d.floor, d.x, d.y = 3, f3.shrineX, f3.shrineY
	d.turns = 100
	d.bank(rm, tr)
	if d.banked != 3 || d.firstBankTurn != 100 {
		t.Fatalf("bank: banked=%d first=%d", d.banked, d.firstBankTurn)
	}
	if len(tr.Posted) != 1 || tr.Posted[0].Rankings[0].Metric != 3 {
		t.Fatalf("post = %+v", tr.Posted)
	}
	d.bank(rm, tr) // same floor: no double bank
	if len(tr.Posted) != 1 {
		t.Fatal("re-banked the same floor")
	}

	// Death now posts the BANKED depth and settles the score KVs.
	d.respects = 2
	rm.die(tr, d, "kobold")
	if len(tr.Posted) != 2 || tr.Posted[1].Rankings[0].Metric != 3 {
		t.Fatalf("death post = %+v", tr.Posted[len(tr.Posted)-1])
	}
	if string(tr.KV[a.AccountID]["soulscore_best_wk"]) == "" {
		t.Fatal("soul score never landed in the KV")
	}
}

var _ = kit.MergeMax // the boards rely on max-merge being available

// The shrine shop is the gold sink: prices scale with depth, purchases land.
func TestShrineShop(t *testing.T) {
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	d := rm.delvers[a.AccountID]
	f3 := rm.floorAt(3)
	d.floor, d.x, d.y = 3, f3.shrineX, f3.shrineY
	d.gold, d.heals = 200, 0
	d.shop(rm, tr, '8') // draught at B3: 120g
	if d.heals != 1 || d.gold != 80 {
		t.Fatalf("draught: heals=%d gold=%d", d.heals, d.gold)
	}
	d.shop(rm, tr, '8') // can't afford the second
	if d.heals != 1 || d.gold != 80 {
		t.Fatal("sold on credit")
	}
	d.x = 1 // off the shrine: shop is closed
	d.gold = 999
	d.shop(rm, tr, '7')
	if d.gold != 999 {
		t.Fatal("bought off-shrine")
	}
}
