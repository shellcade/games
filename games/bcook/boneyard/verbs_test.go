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
