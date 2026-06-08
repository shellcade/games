package main

import (
	"strings"
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
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

// The three overlays render: the Gate hub on spawn, the YOU DIED card on
// death, and the lineage badge on the memorial.
func TestUIScreens(t *testing.T) {
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	d := rm.delvers[a.AccountID]

	// Spawned on the Gate: the hub renders (the title band is present).
	rm.OnWake(tr)
	gate := tr.LastFrame(a)
	if gate == nil || !frameContains(gate, "Sunken Ossuary") {
		t.Fatal("Gate hub did not render on spawn")
	}

	// Step off, descend, die — the YOU DIED card renders with the killer.
	f := rm.world.at(1)
	d.x, d.y = f.downX, f.downY
	d.floor = 2
	rm.floorAt(2)
	rm.die(tr, d, "a tomb mimic")
	if d.deathCard == nil {
		t.Fatal("no death card after dying")
	}
	rm.compose(d) // sanity: card path is reached via dispatch in OnWake
	rm.deathCardScreen(d)
	if !frameContains(rm.frame, "slain by a tomb mimic") {
		t.Fatal("YOU DIED card missing")
	}

	// The memorial carries the delver's lineage badge.
	d.banked = 12
	rm.memorial(d)
	if !frameContains(rm.frame, "Brutalist") {
		t.Fatal("memorial badge missing for a B12 delver")
	}
}

func frameContains(f *kit.Frame, want string) bool {
	for r := 0; r < kit.Rows; r++ {
		var row []rune
		for c := 0; c < kit.Cols; c++ {
			if f.Cells[r][c].Rune == 0 {
				row = append(row, ' ')
			} else {
				row = append(row, f.Cells[r][c].Rune)
			}
		}
		if strings.Contains(string(row), want) {
			return true
		}
	}
	return false
}
