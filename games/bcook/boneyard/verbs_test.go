package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// The full social-verb suite on one corpse: carve last words from the closed
// templates, read the gasp, arm and settle the avenge vow, devour the
// unmourned, and watch the well-mourned protect themselves.
func TestVerbsOnTheFallen(t *testing.T) {
	a, b := bp("ada"), bp("bob")
	tr := kittest.NewRoom(a, b)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	ada := rm.delvers[a.AccountID]
	f2 := rm.floorAt(2)
	ada.floor, ada.x, ada.y = 2, f2.upX+1, f2.upY
	ada.lastDX, ada.lastDY = 1, 0 // fled east
	rm.die(tr, ada, "kobold")
	ada.deathCard = nil // the YOU DIED card is dismissed; carry on testing verbs

	var c *corpse
	for _, bc := range rm.bones {
		if bc.handle == "ada" {
			c = bc
		}
	}
	if c == nil || c.gaspDir != "east" || c.species != "kobold" {
		t.Fatalf("corpse = %+v", c)
	}

	// The dying breath: [5] carves the Dark template from closed vocab.
	rm.OnInput(tr, a, kit.Input{Kind: kit.InputRune, Rune: '5'})
	if c.words != "all-in on the kobold. no regrets." {
		t.Fatalf("carved words = %q", c.words)
	}

	// Bob reads the bones (vow armed), kills a kobold on the floor: avenged.
	bob := rm.delvers[b.AccountID]
	bob.floor = 2
	bob.inspectBones(c)
	if bob.vow != c {
		t.Fatal("reading the bones did not arm the vow")
	}
	bob.creditAvenge(rm, "kobold")
	if c.avenged != 1 || bob.avenges != 1 || bob.vow != nil {
		t.Fatalf("avenge: corpse=%d bob=%d vow=%v", c.avenged, bob.avenges, bob.vow)
	}

	// Devour heals min(8+floor/2, 20) — then the marrow is gone.
	bob.hp = 5
	bob.devourBones(rm, c)
	if !c.devoured || bob.hp != 5+9 || bob.devours != 1 {
		t.Fatalf("devour: devoured=%v hp=%d", c.devoured, bob.hp)
	}

	// A well-mourned corpse is protected.
	c2 := &corpse{handle: "saint", floor: 2, x: 1, y: 1, respects: 3}
	bob.devourBones(rm, c2)
	if c2.devoured {
		t.Fatal("devoured a corpse with 3 respects — the flowers failed")
	}
}

// One flower per account per corpse — repeat respects neither farm luck nor
// inflate the boards; and you cannot mourn your own bones.
func TestRespectIsOncePerAccount(t *testing.T) {
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	ada := rm.delvers[a.AccountID]
	c := &corpse{handle: "bob", floor: 1, x: 1, y: 1}
	ada.respectBones(rm, tr, c)
	ada.respectBones(rm, tr, c)
	ada.respectBones(rm, tr, c)
	if c.respects != 1 || ada.luck != 1 || ada.respects != 1 {
		t.Fatalf("respect farmed: corpse=%d luck=%d", c.respects, ada.luck)
	}
	own := &corpse{handle: "ada", floor: 1, x: 2, y: 1}
	ada.respectBones(rm, tr, own)
	if own.respects != 0 {
		t.Fatal("self-mourning counted")
	}
}

// Bones now carry the dead's gear, and bones-loot can curse it — the greed tax.
func TestBonesLootGearAndCurse(t *testing.T) {
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	d := rm.delvers[a.AccountID]
	d.weapon = nil
	c := &corpse{handle: "bob", floor: 1, x: 1, y: 1, gold: 50, weapon: &catalog[2]} // bone cleaver
	d.lootBones(rm, c)
	if d.weapon == nil || d.gold != 50 || !c.looted {
		t.Fatalf("loot: weapon=%v gold=%d", d.weapon, d.gold)
	}
	// Cleanse only works on a shrine and clears curses + spends tokens/gold.
	d.cursedW, d.tokens = true, 3
	f3 := rm.floorAt(3)
	d.floor, d.x, d.y = 3, f3.shrineX, f3.shrineY
	d.cleanse(rm)
	if d.cursedW || d.tokens != 0 {
		t.Fatalf("cleanse: cursed=%v tokens=%d", d.cursedW, d.tokens)
	}
}

// Recall returns you to the last banked shrine; necromancer raises an ally.
func TestScrolls(t *testing.T) {
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	d := rm.delvers[a.AccountID]

	f3 := rm.floorAt(3)
	d.floor, d.x, d.y = 3, f3.shrineX, f3.shrineY
	d.bank(rm, tr) // sets the recall anchor
	f6 := rm.floorAt(6)
	d.floor, d.x, d.y = 6, f6.upX, f6.upY
	d.recalls = 1
	d.readRecall(rm, tr)
	if d.floor != 3 || d.recalls != 0 {
		t.Fatalf("recall: floor=B%d recalls=%d", d.floor, d.recalls)
	}

	// Necromancer raises the nearest bones as an ally.
	rm.bones = append(rm.bones, &corpse{handle: "x", floor: 3, x: d.x + 1, y: d.y})
	d.necros = 1
	before := len(rm.monsters)
	d.readNecro(rm, tr)
	if len(rm.monsters) != before+1 || !rm.monsters[len(rm.monsters)-1].ally {
		t.Fatal("necromancer raised no ally")
	}
}
