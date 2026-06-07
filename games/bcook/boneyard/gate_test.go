package main

import (
	"testing"
	"time"

	"github.com/shellcade/kit/v2/kittest"
)

// The week's arc: ancestral bones at birth, the doom timer, the final-minute
// grace bank, and End() at rollover — the resident world-reset primitive.
func TestCollapseEndsTheWeek(t *testing.T) {
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)

	if len(rm.bones) == 0 {
		t.Fatal("the week was born without ancestral bones")
	}
	if rm.collapseAt.Weekday() != time.Monday || rm.collapseAt.Hour() != 0 {
		t.Fatalf("collapse at %v, want Monday 00:00 UTC", rm.collapseAt)
	}

	// Ada is deep and unbanked when the final 30s hit: the grace banks her.
	d := rm.delvers[a.AccountID]
	rm.floorAt(5)
	d.floor, d.deepest = 5, 5
	tr.Clock = rm.collapseAt.Add(-20 * time.Second)
	rm.OnWake(tr)
	if d.banked != 5 {
		t.Fatalf("grace bank: banked=%d, want 5", d.banked)
	}
	if len(tr.Posted) == 0 {
		t.Fatal("grace bank posted nothing")
	}

	// Rollover: the room ENDS (the platform births next week on next join).
	tr.Clock = rm.collapseAt.Add(time.Second)
	rm.OnWake(tr)
	if tr.Ended == nil {
		t.Fatal("the collapse did not End the room")
	}
	if string(tr.KV[a.AccountID]["deepest_ever_banked"]) != "5" {
		t.Fatalf("prestige KV = %q", tr.KV[a.AccountID]["deepest_ever_banked"])
	}
}
