package main

import (
	kit "github.com/shellcade/kit/v2"
)

// Items (design §3/§4): a compact catalog, seed-placed at floor gen from the
// band drop table, auto-equip-if-better gear, counted potions, a single relic
// slot. Bones-loot (and only bones-loot) carries the 1-in-3 curse roll.

type itemKind uint8

const (
	iGold itemKind = iota
	iWeapon
	iArmor
	iPotionHeal
	iPotionTorch
	iRelic
)

type itemDef struct {
	name  string
	kind  itemKind
	glyph rune
	style kit.Style
	power int // weapon: die faces; armor: AC; relic: id; potion: unused
	minB  int
	maxB  int
}

var catalog = []itemDef{
	{"rusted longsword", iWeapon, '†', kit.Style{FG: kit.White, Attr: kit.AttrBold}, 6, 1, 6},
	{"kobold shiv", iWeapon, '†', kit.Style{FG: kit.White, Attr: kit.AttrBold}, 4, 2, 8}, // fast: -60ms moveCD on attack ticks
	{"bone cleaver", iWeapon, '†', kit.Style{FG: kit.White, Attr: kit.AttrBold}, 8, 4, 9},
	{"tattered shroud", iArmor, '[', kit.Style{FG: kit.White}, 2, 1, 5},
	{"gilded reliquary mail", iArmor, '[', kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}, 7, 8, 9},
	{"healing draught", iPotionHeal, '!', kit.Style{FG: kit.RGB(0xd0, 0x60, 0xd0), Attr: kit.AttrBold}, 0, 1, 9},
	{"torch oil", iPotionTorch, '!', kit.Style{FG: kit.Yellow}, 0, 1, 9},
	{"amulet of the deep", iRelic, '=', kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}, 1, 7, 9}, // torch -30%
	{"ring of the graverobber", iRelic, '=', kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}, 2, 5, 9},
	{"ring of the unmourned", iRelic, '=', kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}, 3, 3, 9},
}

// drop is one placed floor item.
type drop struct {
	def   *itemDef
	floor int
	x, y  int
	gold  int // iGold value
	taken bool
}

// placeDrops seeds a floor's item slots (design §4: ~4-6 slots, band drop
// table) on the same domain-tagged stream discipline as monster spawns.
func (rm *room) placeDrops(f *floor) {
	g := newGenRNG(rm.world.seed, f.depth)
	g.s ^= 0x17E4 // item sub-stream tag

	slots := 4
	if f.depth >= 4 {
		slots = 5 + g.intn(2)
	}
	for i := 0; i < slots; i++ {
		x, y := rm.openTile(g, f)
		pct := g.intn(100)
		var def *itemDef
		switch band := f.depth; {
		case pct < 40: // gold (B1-3: 45%, belt 35% — split the difference)
			v := 10 + g.intn(25)
			if band >= 4 {
				v = 50 + g.intn(100)
			}
			rm.drops = append(rm.drops, &drop{floor: f.depth, x: x, y: y, gold: v})
			continue
		case pct < 65:
			def = pick(g, iPotionHeal, iPotionTorch, f.depth)
		case pct < 80:
			def = pick(g, iWeapon, iWeapon, f.depth)
		case pct < 92:
			def = pick(g, iArmor, iArmor, f.depth)
		default:
			def = pick(g, iRelic, iRelic, f.depth)
		}
		if def != nil {
			rm.drops = append(rm.drops, &drop{def: def, floor: f.depth, x: x, y: y})
		}
	}
}

// pick selects a band-legal item of kind a or b.
func pick(g *genRNG, a, b itemKind, depth int) *itemDef {
	var c []*itemDef
	for i := range catalog {
		d := &catalog[i]
		if (d.kind == a || d.kind == b) && depth >= d.minB && depth <= d.maxB {
			c = append(c, d)
		}
	}
	if len(c) == 0 {
		return nil
	}
	return c[g.intn(len(c))]
}

// dropAt returns the untaken drop on a tile.
func (rm *room) dropAt(floor, x, y int) *drop {
	for _, dr := range rm.drops {
		if !dr.taken && dr.floor == floor && dr.x == x && dr.y == y {
			return dr
		}
	}
	return nil
}

// pickup takes the drop underfoot: gold banks instantly; gear auto-equips
// when strictly better; potions are counted in the belt.
func (d *delver) pickup(rm *room, dr *drop) {
	dr.taken = true
	rm.dirtyWitnesses(dr.floor, dr.x, dr.y, nil)
	if dr.def == nil {
		d.gold += dr.gold
		d.say("You pocket " + itoa(dr.gold) + " gold.")
		return
	}
	switch dr.def.kind {
	case iWeapon:
		if d.weapon == nil || dr.def.power > d.weapon.power {
			d.weapon = dr.def
			d.say("You take up the " + dr.def.name + ".")
		} else {
			d.say("A " + dr.def.name + " — yours is better. Left for the next delver.")
			dr.taken = false
		}
	case iArmor:
		if d.armor == nil || dr.def.power > d.armor.power {
			d.armor = dr.def
			d.say("You strap on the " + dr.def.name + ".")
		} else {
			d.say("A " + dr.def.name + " — yours is better.")
			dr.taken = false
		}
	case iRelic:
		if d.relic == nil {
			d.relic = dr.def
			d.say("The " + dr.def.name + " hums on your hand.")
		} else {
			d.say("A " + dr.def.name + " — but your relic slot is taken.")
			dr.taken = false
		}
	case iPotionHeal:
		d.heals++
		d.say("A healing draught. [q] to quaff. (" + itoa(d.heals) + ")")
	case iPotionTorch:
		d.torch += 400
		if d.torch > 999 {
			d.torch = 999
		}
		d.say("Torch oil. The flame steadies. (+400t)")
	}
}

// quaff drinks a healing draught: 2d8 from the actor PRNG.
func (d *delver) quaff() {
	if d.heals == 0 {
		d.say("No draughts left.")
		return
	}
	d.heals--
	heal := roll(&d.rng, 8) + roll(&d.rng, 8)
	d.hp += heal
	if d.hp > d.maxHP {
		d.hp = d.maxHP
	}
	d.say("You quaff. +" + itoa(heal) + " HP.")
}

// ---- Kits (design: BLADE / LANTERN / FLASK; chosen at the Gate) ----

type kitDef struct {
	name     string
	str, dex int
	maxHP    int
	torch    int
	torchMul int // percent: 100 = baseline; LANTERN 60 (burns slower)
	weapon   *itemDef
	heals    int
}

var kits = []kitDef{
	{"BLADE", 16, 10, 34, 600, 100, &catalog[2], 0},  // d8 cleaver, glass cannon
	{"LANTERN", 14, 14, 36, 480, 60, &catalog[0], 0}, // 800t effective
	{"FLASK", 13, 15, 44, 600, 100, &catalog[0], 2},  // tanky, two draughts
}

// applyKit outfits a fresh run.
func (d *delver) applyKit(k *kitDef) {
	d.kit = k
	d.str, d.dex = k.str, k.dex
	d.maxHP, d.hp = k.maxHP, k.maxHP
	d.torch, d.torchMul = k.torch, k.torchMul
	d.weapon, d.armor, d.relic = k.weapon, nil, nil
	d.heals = k.heals
	d.say(k.name + " kit. The Gate opens.")
}
