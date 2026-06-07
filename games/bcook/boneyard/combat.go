package main

import (
	kit "github.com/shellcade/kit/v2"
)

// Combat (design §5): d20 + AttackBonus >= Armor; nat 20 crits (damage twice),
// nat 1 misses; damage = weapon dice + Str mod, no soak. Every roll draws
// from the ATTACKER'S week-seed-derived PRNG (rng.go) — never wall-clock —
// the determinism the stage-5 replay anti-cheat depends on. The dark
// (torch 0) is worth ±4 on the d20: -4 to a delver's swings, +4 to swings
// AGAINST a delver in the dark.

func strMod(str int) int { return (str - 10) / 2 }

func darkPenalty(d *delver) int {
	if d.torch <= 0 {
		return 4
	}
	return 0
}

// playerArmor: 10 + AC + Dex mod (AC 0 until armor lands in stage 3).
func (d *delver) armorClass() int { return 10 + 0 + (d.dex-10)/2 }

// attackMonster is the player's bump attack: 1d6 baseline weapon until the
// item catalog lands.
func (d *delver) attackMonster(rm *room, r kit.Room, m *monster) {
	dieRoll := roll(&d.rng, 20)
	hit := dieRoll == 20 || (dieRoll != 1 && dieRoll+strMod(d.str)-darkPenalty(d) >= m.sp.armor)
	if !hit {
		d.say("You miss the " + m.sp.name + ".")
		return
	}
	dmg := roll(&d.rng, 6) + strMod(d.str)
	if dieRoll == 20 {
		dmg += roll(&d.rng, 6) + strMod(d.str)
	}
	if dmg < 1 {
		dmg = 1
	}
	m.hp -= dmg
	if m.hp <= 0 {
		d.kills++
		d.say("The " + m.sp.name + " dies.")
		rm.dirtyWitnesses(m.floor, m.x, m.y, nil)
		if m.sp.burst {
			rm.burst(r, m)
		}
		return
	}
	if dieRoll == 20 {
		d.say("Crit! The " + m.sp.name + " takes " + itoa(dmg) + ".")
	} else {
		d.say("You hit the " + m.sp.name + " for " + itoa(dmg) + ".")
	}
}

// monsterAttack rolls the monster's swing, scaling the ROLLED damage total by
// the floor's lethality (design §1: dice scale by scaling the rolled total).
func (rm *room) monsterAttack(r kit.Room, m *monster, d *delver) {
	dieRoll := roll(&m.rng, 20)
	hit := dieRoll == 20 || (dieRoll != 1 && dieRoll+m.sp.atk+darkPenalty(d) >= d.armorClass())
	d.dirty = true
	if !hit {
		d.say("The " + m.sp.name + " misses you.")
		return
	}
	dmg := 0
	for i := 0; i < m.sp.dmgN; i++ {
		dmg += roll(&m.rng, m.sp.dmgD)
	}
	if dieRoll == 20 {
		for i := 0; i < m.sp.dmgN; i++ {
			dmg += roll(&m.rng, m.sp.dmgD)
		}
	}
	dmg = scaled(dmg, dmgScalar(m.floor))
	if dmg < 1 {
		dmg = 1
	}
	d.hp -= dmg
	d.say("The " + m.sp.name + " hits you for " + itoa(dmg) + ".")
	if d.hp <= 0 {
		rm.die(r, d, m.sp.name)
	}
}

// burst is the bloat's death explosion: 2d4 (floor-scaled) to every delver
// and monster in the 8 neighbors.
func (rm *room) burst(r kit.Room, m *monster) {
	for _, d := range rm.delvers {
		if d.floor == m.floor && d.hp > 0 && cheb(d.x-m.x, d.y-m.y) == 1 {
			dmg := scaled(roll(&m.rng, 4)+roll(&m.rng, 4), dmgScalar(m.floor))
			d.hp -= dmg
			d.say("The bloat bursts! " + itoa(dmg) + " damage.")
			if d.hp <= 0 {
				rm.die(r, d, "a bursting bloat")
			}
		}
	}
	for _, o := range rm.monsters {
		if o != m && o.hp > 0 && o.floor == m.floor && cheb(o.x-m.x, o.y-m.y) == 1 {
			o.hp -= roll(&m.rng, 4) + roll(&m.rng, 4)
		}
	}
}
