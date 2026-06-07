package main

import (
	"time"

	kit "github.com/shellcade/kit/v2"
)

// The Gate, the collapse, and the memory of the dead (design §10): the week
// ends Monday 00:00 UTC — the room computes its doom at OnStart from the
// room clock, shows the countdown all week, auto-banks caught runs in the
// final minute, and at rollover ENDS the room. Ending a resident room IS the
// world reset: the platform births next week's room (new seed) on the next
// join. Ancestral bones seed the cold-start world so even the first delver
// of the week walks among the dead.

// nextMondayUTC returns the upcoming Monday 00:00 UTC strictly after t.
func nextMondayUTC(t time.Time) time.Time {
	t = t.UTC()
	day := t.Truncate(24 * time.Hour)
	daysAhead := (8 - int(day.Weekday())) % 7 // Monday=1
	if daysAhead == 0 {
		daysAhead = 7
	}
	return day.Add(time.Duration(daysAhead) * 24 * time.Hour)
}

// tickCollapse drives the doom timer: warnings in the final hour and minute,
// auto-bank in the final 30s, End at zero.
func (rm *room) tickCollapse(r kit.Room, now time.Time) {
	if rm.collapseAt.IsZero() || rm.collapsed {
		return
	}
	left := rm.collapseAt.Sub(now)
	switch {
	case left <= 0:
		rm.collapsed = true
		rm.rollOfTheDead(r)
		r.End(kit.Result{}) // the world reset primitive: next join births next week
	case left <= 30*time.Second && !rm.graceBanked:
		rm.graceBanked = true
		for _, d := range rm.delvers {
			if d.deepest > d.banked {
				d.banked = d.deepest // the collapse banks what the run carried
			}
			rm.settleRunScore(r, d)
			r.Post(kit.Result{Rankings: []kit.PlayerResult{{
				Player: d.p, Metric: d.banked, Rank: 1, Status: kit.StatusFinished,
			}}})
			d.say("THE OSSUARY IS COLLAPSING. Your depth is banked. Run.")
		}
	case left <= time.Minute && !rm.warnedMinute:
		rm.warnedMinute = true
		rm.broadcastSay("One minute. The water is at the stairs.")
	case left <= time.Hour && !rm.warnedHour:
		rm.warnedHour = true
		rm.broadcastSay("The Ossuary collapses within the hour.")
	}
}

func (rm *room) broadcastSay(s string) {
	for _, d := range rm.delvers {
		if d.online {
			d.say(s)
		}
	}
}

// rollOfTheDead settles the week's memorial into each fallen account's KV
// (permanent prestige feeds: deepest_ever_banked, lineage).
func (rm *room) rollOfTheDead(r kit.Room) {
	for _, d := range rm.delvers {
		kvMax(r, d.p, "deepest_ever_banked", d.banked)
		kvAdd(r, d.p, "lineage_total", d.kills)
	}
}

// countdown renders the doom timer for the HUD.
func (rm *room) countdown(now time.Time) string {
	if rm.collapseAt.IsZero() {
		return ""
	}
	left := rm.collapseAt.Sub(now)
	if left <= 0 {
		return "COLLAPSING"
	}
	if d := int(left.Hours()) / 24; d > 0 {
		return itoa(d) + "d " + itoa(int(left.Hours())%24) + "h"
	}
	if h := int(left.Hours()); h > 0 {
		return itoa(h) + "h " + itoa(int(left.Minutes())%60) + "m"
	}
	return itoa(int(left.Minutes())) + "m " + itoa(int(left.Seconds())%60) + "s"
}

// seedAncestralBones cold-starts the week: ~20 synthetic dead with
// closed-vocab epitaphs, spread across the band — the first delver of the
// week still walks among predecessors (design: cold-start at low pop).
func (rm *room) seedAncestralBones(r kit.Room) {
	names := []string{"the first delver", "a nameless porter", "the cartographer",
		"an old soldier", "the previous tenant", "a hopeful fool", "the bellringer",
		"a torch merchant", "the tax collector", "an apprentice", "the gravedigger"}
	killers := []string{"kobold", "jackal", "skeleton", "gelatinous cube", "the dark", "goblin"}
	g := newGenRNG(rm.world.seed, 0)
	g.s ^= 0xDEAD // ancestral sub-stream
	n := 20
	for i := 0; i < n; i++ {
		depth := 1 + g.intn(maxMVP)
		f := rm.floorAt(depth)
		x, y := rm.openTile(g, f)
		k := killers[g.intn(len(killers))]
		dir := []string{"east", "west", "north", "south"}[g.intn(4)]
		rm.bones = append(rm.bones, &corpse{
			handle: names[g.intn(len(names))],
			floor:  depth, x: x, y: y,
			killer: k, species: k,
			gold:    5 + g.intn(20),
			at:      r.Now(),
			gaspDir: dir,
			words:   panicScrawl(k, dir),
		})
	}
	for depth := 1; depth <= maxMVP; depth++ {
		rm.evictBones(depth)
	}
}
