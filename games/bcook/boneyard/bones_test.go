package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// The soul of the game, end to end: a delver dies, their banked depth posts
// to the board, their corpse joins the world with the panic-scrawl — and the
// NEXT delver finds the bones, reads them, robs them or honors them.
func TestDeathLeavesBonesForTheNextDelver(t *testing.T) {
	a, b := bp("ada"), bp("bob")
	tr := kittest.NewRoom(a, b)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	// Ada descends to B2, earns some gold, banks nothing, and dies.
	ada := rm.delvers[a.AccountID]
	f2 := rm.floorAt(2)
	ada.floor, ada.gold = 2, 120
	ada.x, ada.y = f2.upX, f2.upY // dies ON the stairs: the corpse must jitter off
	rm.die(tr, ada, "a gelatinous cube")

	// The death posted her BANKED depth (0 — unbanked progress is tuition).
	if len(tr.Posted) != 1 {
		t.Fatalf("death posted %d results, want 1", len(tr.Posted))
	}
	pr := tr.Posted[0].Rankings[0]
	if pr.Player != a || pr.Metric != 0 || pr.Status != kit.StatusFinished {
		t.Fatalf("posted %+v, want ada banked=0 finished", pr)
	}

	// The corpse is in the world near where she fell — jittered OFF the
	// stairs (the staircase contract), words and all.
	var c *corpse
	for _, b := range rm.bones {
		if b.handle == "ada" {
			c = b
		}
	}
	if c == nil || c.floor != 2 || c.gold != 120 {
		t.Fatalf("corpse = %+v", c)
	}
	if t2 := f2.tiles[c.y][c.x]; t2 != tFloor && t2 != tWater {
		t.Fatalf("corpse landed on %q — stairs/shrine exclusion violated", string(t2))
	}
	if cheb(c.x-f2.upX, c.y-f2.upY) > 4 {
		t.Fatal("corpse jittered too far from the death tile")
	}

	// Ada herself is a fresh run at the Gate.
	ada2 := rm.delvers[a.AccountID]
	if ada2.floor != 1 || ada2.hp != ada2.maxHP || ada2.gold != 0 {
		t.Fatalf("respawn: floor=B%d hp=%d gold=%d", ada2.floor, ada2.hp, ada2.gold)
	}

	// Bob finds the bones: respects them (+1 luck), then robs them anyway.
	bob := rm.delvers[b.AccountID]
	bob.floor = 2
	bob.x, bob.y = c.x, c.y
	bob.respectBones(rm, tr, c)
	if c.respects != 1 || bob.luck != 1 {
		t.Fatalf("respect: corpse=%d luck=%d", c.respects, bob.luck)
	}
	bob.lootBones(rm, c)
	if bob.gold != 120 || !c.looted {
		t.Fatalf("loot: gold=%d looted=%v", bob.gold, c.looted)
	}
	bob.lootBones(rm, c) // picked clean
	if bob.gold != 120 {
		t.Fatal("double-loot duplicated gold")
	}
}

// The render cap: a 13th corpse on a floor evicts the oldest to dust.
func TestBonesRenderCap(t *testing.T) {
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	rm.floorAt(2)
	for i := 0; i < bonesRenderCap+1; i++ {
		rm.bones = append(rm.bones, &corpse{handle: "d" + itoa(i), floor: 2, x: 2 + i, y: 2, at: tr.Now()})
	}
	rm.evictBones(2)
	rendered, dusted := 0, 0
	for _, c := range rm.bones {
		if c.dust() {
			dusted++
		} else {
			rendered++
		}
	}
	if rendered != bonesRenderCap || dusted != 1 {
		t.Fatalf("rendered=%d dusted=%d, want %d/1", rendered, dusted, bonesRenderCap)
	}
	if !rm.bones[0].dust() {
		t.Fatal("eviction took a newer corpse than the oldest")
	}
}

// Combat smoke: bump-attacking a cornered cave rat kills it within a few
// swings (d20 + Str mod vs Armor 10 — misses happen; persistence wins).
func TestBumpAttackKillsTheRat(t *testing.T) {
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	d := rm.delvers[a.AccountID]
	f := rm.world.at(1)

	// Plant a rat next to ada on a known-open tile.
	var rx, ry int
	for _, try := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		if f.tiles[d.y+try[1]][d.x+try[0]] == tFloor {
			rx, ry = d.x+try[0], d.y+try[1]
			break
		}
	}
	rat := &monster{sp: &bestiary[0], floor: 1, x: rx, y: ry, hp: scaled(bestiary[0].hp, hpScalar(1))}
	rm.monsters = append(rm.monsters, rat)

	for i := 0; i < 40 && rat.hp > 0; i++ {
		tr.Advance(d.moveCD() + time.Millisecond)
		rm.OnInput(tr, a, kit.Input{Kind: kit.InputRune, Rune: dirRune(rx-d.x, ry-d.y)})
	}
	if rat.hp > 0 {
		t.Fatal("40 swings did not kill a 3-HP rat")
	}
	if d.kills != 1 {
		t.Fatalf("kills = %d", d.kills)
	}
}

func dirRune(dx, dy int) rune {
	switch {
	case dx == 1:
		return 'l'
	case dx == -1:
		return 'h'
	case dy == 1:
		return 'j'
	default:
		return 'k'
	}
}
