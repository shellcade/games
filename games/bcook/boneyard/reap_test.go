package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// A resident world runs for a week; spawner monsters (lich acolytes, ossuary
// tyrants) append to rm.monsters every turn and dead monsters used to linger
// forever, so the slice grew without bound and every wake's O(monsters) work
// crept toward starving the 100ms tick. The reap must keep the live slice
// bounded under sustained play on a spawner floor.
func TestDeadMonstersAreReaped(t *testing.T) {
	p := kit.Player{AccountID: "a", Handle: "a", Kind: kit.KindMember, Conn: "c1"}
	tr := kittest.NewRoom(p)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, p)

	d := rm.delvers["a"]
	for d.floor < 17 { // descend into the lich/tyrant spawner band
		f := rm.world.at(d.floor)
		d.x, d.y = f.downX, f.downY
		d.descend(rm, tr)
	}
	d.maxHP, d.hp = 1_000_000, 1_000_000 // immortal: keep the floor active

	tick := func(n int) {
		for i := 0; i < n; i++ {
			tr.Advance(120 * time.Millisecond)
			rm.OnWake(tr)
		}
	}
	tick(20000)
	early := len(rm.monsters)
	tick(40000)
	late := len(rm.monsters)

	// Dead monsters never linger in the live slice.
	for _, m := range rm.monsters {
		if m.hp <= 0 {
			t.Fatalf("dead monster left in the live slice (reap failed)")
		}
	}
	// The population is BOUNDED: it must plateau, not climb. Pre-fix this grew
	// monotonically (~+250 over this window) toward starving the tick; with the
	// per-floor cap + reap it stabilizes.
	if late > early+20 {
		t.Fatalf("live monsters grew %d -> %d over 40k ticks - still unbounded", early, late)
	}
	t.Logf("bounded: %d monsters at 20k ticks, %d at 60k ticks", early, late)
}
