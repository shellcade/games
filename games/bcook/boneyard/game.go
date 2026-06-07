package main

import (
	kit "github.com/shellcade/kit/v2"
)

// Game is the catalog registry entry. The meta declares the full
// persistent-world platform surface: ONE resident room (granted by the
// platform), large-room roster-epoch callbacks, and the gentle 100ms tick a
// real-time roguelike actually needs.
type Game struct{}

func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "boneyard",
		Name:             "Boneyard",
		ShortDescription: "Delve the week's shared dungeon — and read the bones of everyone who died before you.",
		MinPlayers:       1,
		MaxPlayers:       100,
		Tags:             []string{"roguelike", "persistent", "multiplayer"},

		QuickModeLabel: "Descend",

		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Depth",
			Direction:   kit.HigherBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Integer,
		},

		CtxFeatures: kit.CtxFeatRosterEpoch,
		HeartbeatMS: 100,
		Lifecycle:   kit.LifecycleResident,
	}
}

func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return &room{}
}

// room is the weekly world: the dungeon (lazily generated per floor from the
// week seed), every delver's run state, and the bones of the fallen. One
// handler instance == one week of the Boneyard.
type room struct {
	kit.Base

	world    *world
	delvers  map[string]*delver // by AccountID (rejoin = same run)
	roster   []kit.Player       // join-ordered, for the send loop
	monsters []*monster         // every live spawn, all floors
	bones    []*corpse          // the week's fallen (the point of the game)

	frame *kit.Frame // reused for every per-player send
	wakes int
}

func (rm *room) OnStart(r kit.Room) {
	rm.world = newWorld(r.Config().Seed)
	rm.delvers = map[string]*delver{}
	rm.frame = kit.NewFrame()
}

// floorAt returns B<depth>, generating AND populating it on first entry.
func (rm *room) floorAt(depth int) *floor {
	if _, ok := rm.world.floors[depth]; !ok {
		f := rm.world.at(depth)
		rm.spawnFloor(f)
		return f
	}
	return rm.world.at(depth)
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	if d, ok := rm.delvers[p.AccountID]; ok {
		// Rejoin (same seat, post-restore re-seat included): the run resumes
		// where it stood; the viewer needs a frame (baselines were reset).
		d.dirty = true
		return
	}
	rm.floorAt(1) // first join generates + populates B1
	d := newDelver(p, rm.world, r)
	rm.delvers[p.AccountID] = d
	rm.roster = append(rm.roster, p)
	rm.dirtyFloor(d.floor) // a new @ appeared on the floor
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	// The RUN persists across a disconnect (this is a persistent world); the
	// delver simply stops being rendered to. Roster bookkeeping only.
	for i := range rm.roster {
		if rm.roster[i].AccountID == p.AccountID {
			rm.roster = append(rm.roster[:i], rm.roster[i+1:]...)
			break
		}
	}
	if d, ok := rm.delvers[p.AccountID]; ok {
		rm.dirtyFloor(d.floor) // their @ vanishes from witnesses' views
	}
}

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	d, ok := rm.delvers[p.AccountID]
	if !ok {
		return
	}
	d.handleInput(rm, r, in)
}

// OnWake is the 100ms world tick: passive torch burn, monster turns (stage
// 2), then per-player viewport sends — DIRTY VIEWS ONLY (GUIDE.md "Large
// rooms"): a view re-renders when its owner acted, something moved inside
// its window, or its HUD changed.
func (rm *room) OnWake(r kit.Room) {
	rm.wakes++
	now := r.Now()

	for _, d := range rm.delvers {
		d.tick(rm, now)
	}
	rm.tickMonsters(r, now)

	for _, p := range rm.roster {
		d, ok := rm.delvers[p.AccountID]
		if !ok || !d.dirty {
			continue
		}
		d.dirty = false
		rm.compose(d)
		r.Send(p, rm.frame)
	}
}

// dirtyFloor marks every connected delver on floor f dirty (something
// floor-visible changed: a join, a leave, a death, a bones change).
func (rm *room) dirtyFloor(f int) {
	for _, d := range rm.delvers {
		if d.floor == f {
			d.dirty = true
		}
	}
}

// dirtyWitnesses marks delvers on floor f dirty when world cell (x,y) is
// inside their (clamped, centered) viewport — render-on-change rule 2.
func (rm *room) dirtyWitnesses(f, x, y int, except *delver) {
	for _, o := range rm.delvers {
		if o == except || o.dirty || o.floor != f {
			continue
		}
		ox, oy := o.camera()
		if x >= ox && x < ox+kit.Cols && y >= oy && y < oy+mapRows {
			o.dirty = true
		}
	}
}
