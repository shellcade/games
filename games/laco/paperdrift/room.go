package main

import (
	"context"
	"math"
	"sort"
	"strconv"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// Round flow: lobby → countdown → flying → results → countdown → … Every
// clock is a deadline held in room state and compared against r.Now() on the
// heartbeat (the wake idioms from the kit guide).
const (
	launchCountdown = 3 * time.Second  // Enter / capacity / next-round launch
	gatherCountdown = 6 * time.Second  // a second player arrived: brief gather
	lobbyGrace      = 12 * time.Second // lone quick player flies after this
	resultsDur      = 12 * time.Second // results hold before auto-relaunch
	roundCap        = 4 * time.Minute  // safety net; gates make crashes certain

	spawnX = 24.0 // launch column; distance is measured from here
)

const (
	phLobby     = "lobby"
	phCountdown = "countdown"
	phFlying    = "flying"
	phResults   = "results"
)

type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	phase   string
	pilots  map[string]*pilot     // account id → glider (hibernation-safe key)
	order   []string              // join order of account ids
	names   map[string]kit.Player // last-seen player tokens, kept for results
	joinSeq int

	terr       terrain
	roundNum   int
	roundStart time.Time
	lastPhys   time.Time
	simAccum   time.Duration

	graceDeadline     time.Time
	countdownDeadline time.Time
	capDeadline       time.Time
	resultsDeadline   time.Time

	lastSecShown int // render-on-change for lobby/results countdowns

	frame *kit.Frame // reused render scratch
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:          cfg,
		svc:          svc,
		phase:        phLobby,
		pilots:       map[string]*pilot{},
		names:        map[string]kit.Player{},
		lastSecShown: -1,
		frame:        kit.NewFrame(),
	}
}

func (rm *room) OnStart(r kit.Room) {
	r.SetInputContext(kit.CtxNav)
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.names[p.AccountID] = p
	ps := rm.pilots[p.AccountID]
	if ps == nil {
		ps = &pilot{joinOrder: rm.joinSeq}
		rm.joinSeq++
		rm.pilots[p.AccountID] = ps
		rm.order = append(rm.order, p.AccountID)
		rm.loadBest(p, ps)
	}
	ps.left = false

	switch rm.phase {
	case phFlying, phResults:
		// Late arrival spectates and flies the next round.
	case phCountdown:
		rm.placeAtSpawn() // slot the newcomer onto the launch fold
	default: // lobby
		switch rm.cfg.Mode {
		case kit.ModeSolo:
			rm.startCountdown(r, launchCountdown)
		default: // quick / private
			switch {
			case rm.cfg.Capacity > 0 && r.Count() >= rm.cfg.Capacity:
				rm.startCountdown(r, launchCountdown)
			case r.Count() >= 2:
				rm.startCountdown(r, gatherCountdown)
			case rm.cfg.Mode == kit.ModeQuick:
				rm.graceDeadline = r.Now().Add(lobbyGrace)
			}
		}
	}
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	ps := rm.pilots[p.AccountID]
	if ps == nil {
		return
	}
	if rm.phase == phFlying && ps.flew {
		// Mid-round departure: the glider folds where it was, ranked DNF.
		ps.left = true
		if ps.alive {
			ps.alive = false
			ps.dist = ps.liveDist()
		}
		if rm.allDown() {
			rm.enterResults(r)
		}
	} else {
		rm.dropPilot(p.AccountID)
	}
	rm.render(r)
}

// dropPilot removes a pilot entirely (outside a round there is nothing to rank).
func (rm *room) dropPilot(id string) {
	delete(rm.pilots, id)
	for i, oid := range rm.order {
		if oid == id {
			rm.order = append(rm.order[:i], rm.order[i+1:]...)
			break
		}
	}
}

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	switch rm.phase {
	case phFlying:
		ps := rm.pilots[p.AccountID]
		if ps == nil || !ps.alive {
			return
		}
		// Discrete trim nudges: a tap is 5°, holding the key rides terminal
		// auto-repeat into a continuous rotation. Resolve folds in k/j too.
		switch kit.Resolve(in, kit.CtxNav) {
		case kit.ActUp:
			ps.pitch += pitchStep
			if ps.pitch > pitchMax {
				ps.pitch = pitchMax
			}
		case kit.ActDown:
			ps.pitch -= pitchStep
			if ps.pitch < -pitchMax {
				ps.pitch = -pitchMax
			}
		}
	case phLobby:
		if kit.Resolve(in, kit.CtxNav) == kit.ActConfirm {
			rm.startCountdown(r, launchCountdown)
			rm.render(r)
		}
	case phResults:
		if kit.Resolve(in, kit.CtxNav) == kit.ActConfirm {
			rm.startCountdown(r, launchCountdown)
			rm.render(r)
		}
	}
}

// startCountdown preps the round (fresh terrain, gliders on the launch fold)
// and arms the launch deadline.
func (rm *room) startCountdown(r kit.Room, d time.Duration) {
	if rm.phase == phCountdown {
		return
	}
	rm.phase = phCountdown
	rm.graceDeadline = time.Time{}
	rm.countdownDeadline = r.Now().Add(d)
	rm.roundNum++

	// Anyone who left between rounds is gone; everyone present flies.
	for _, id := range append([]string(nil), rm.order...) {
		if rm.pilots[id].left {
			rm.dropPilot(id)
		}
	}
	rm.terr.reset()
	rm.terr.ensure(r.Rand(), 400)
	rm.placeAtSpawn()
}

// placeAtSpawn stacks every pilot on the launch fold, lightly staggered so
// gliders don't overlap pixel-for-pixel. Altitude is stored energy in this
// flight model, so each lower slot gets the extra airspeed that makes its
// total energy (½v² − g·height) equal the top slot's — a zoom-climb converts
// the bonus back into exactly the missing height.
func (rm *room) placeAtSpawn() {
	mid := rm.terr.midY(int(spawnX))
	n := len(rm.order)
	for i, id := range rm.order {
		ps := rm.pilots[id]
		ps.x = spawnX
		ps.y = mid + (float64(i)-float64(n-1)/2)*1.6
		ps.v = math.Min(math.Sqrt(launchV*launchV+2*gravity*1.6*float64(i)), vMax)
		ps.pitch = 0
		ps.alive = false
		ps.flew = false
		ps.dist = 0
		ps.newPB = false
		ps.stalled = false
		ps.trailN = 0
		ps.lastTrail = time.Time{}
	}
}

func (rm *room) launch(r kit.Room) {
	rm.phase = phFlying
	rm.countdownDeadline = time.Time{}
	now := r.Now()
	rm.roundStart = now
	rm.lastPhys = now
	rm.simAccum = 0
	rm.capDeadline = now.Add(roundCap)
	for _, id := range rm.order {
		ps := rm.pilots[id]
		ps.alive = true
		ps.flew = true
	}
}

// OnWake is the host heartbeat: advance the phase clocks and the flight
// physics, then render.
func (rm *room) OnWake(r kit.Room) {
	now := r.Now()
	switch rm.phase {
	case phLobby:
		if !rm.graceDeadline.IsZero() && now.After(rm.graceDeadline) && r.Count() >= 1 {
			rm.startCountdown(r, launchCountdown)
		}
	case phCountdown:
		switch {
		case r.Count() == 0:
			// Everyone left before launch; an empty round would just idle
			// out the cap. Fall back to the lobby (ephemeral disposal will
			// collect a room nobody returns to).
			rm.phase = phLobby
			rm.countdownDeadline = time.Time{}
		case !rm.countdownDeadline.IsZero() && now.After(rm.countdownDeadline):
			rm.launch(r)
		}
	case phFlying:
		rm.advance(r, now)
		switch {
		case rm.allDown():
			rm.enterResults(r)
		case !rm.capDeadline.IsZero() && now.After(rm.capDeadline):
			rm.enterResults(r)
		}
	case phResults:
		if !rm.resultsDeadline.IsZero() && now.After(rm.resultsDeadline) && r.Count() > 0 {
			rm.startCountdown(r, launchCountdown)
		}
	}

	switch rm.phase {
	case phFlying, phCountdown:
		rm.render(r) // real-time phases repaint every heartbeat
	default:
		// Lobby/results only show a ticking second; repaint when it changes.
		if sec := rm.shownSecond(now); sec != rm.lastSecShown {
			rm.render(r)
		}
	}
}

// shownSecond is the whole second currently displayed by the lobby grace or
// results countdown (-1 when none is showing).
func (rm *room) shownSecond(now time.Time) int {
	var dl time.Time
	switch rm.phase {
	case phLobby:
		dl = rm.graceDeadline
	case phResults:
		dl = rm.resultsDeadline
	}
	if dl.IsZero() {
		return -1
	}
	s := int(dl.Sub(now).Seconds())
	if s < 0 {
		s = 0
	}
	return s
}

// allDown reports whether nobody who launched is still flying.
func (rm *room) allDown() bool {
	flew := 0
	for _, id := range rm.order {
		ps := rm.pilots[id]
		if !ps.flew {
			continue
		}
		flew++
		if ps.alive {
			return false
		}
	}
	return flew > 0
}

func (rm *room) enterResults(r kit.Room) {
	if rm.phase == phResults {
		return
	}
	rm.phase = phResults
	rm.capDeadline = time.Time{}
	now := r.Now()

	// Round-cap survivors land where they are with their distance.
	for _, id := range rm.order {
		ps := rm.pilots[id]
		if ps.alive {
			ps.alive = false
			ps.dist = ps.liveDist()
		}
	}

	res := rm.buildResult()
	if len(res.Rankings) > 0 {
		r.Post(res)
	}
	rm.persistBests()
	rm.resultsDeadline = now.Add(resultsDur)
}

// buildResult ranks this round's fliers by distance (join order tiebreak);
// mid-round leavers rank below fliers of equal distance via status DNF.
func (rm *room) buildResult() kit.Result {
	ids := make([]string, 0, len(rm.order))
	for _, id := range rm.order {
		if rm.pilots[id].flew {
			ids = append(ids, id)
		}
	}
	sort.SliceStable(ids, func(i, j int) bool {
		pi, pj := rm.pilots[ids[i]], rm.pilots[ids[j]]
		if pi.dist != pj.dist {
			return pi.dist > pj.dist
		}
		return pi.joinOrder < pj.joinOrder
	})
	res := kit.Result{}
	for i, id := range ids {
		ps := rm.pilots[id]
		status := kit.StatusFinished
		if ps.left {
			status = kit.StatusDNF
		}
		res.Rankings = append(res.Rankings, kit.PlayerResult{
			Player: rm.playerFor(id),
			Metric: ps.dist,
			Rank:   i + 1,
			Status: status,
		})
	}
	return res
}

// persistBests banks any new personal bests to the per-player KV. MergeMax
// keeps the durable value monotonic even if a store blip loses a read.
func (rm *room) persistBests() {
	for _, id := range rm.order {
		ps := rm.pilots[id]
		if !ps.flew || ps.dist <= ps.best {
			continue
		}
		ps.best = ps.dist
		ps.newPB = true
		if rm.svc.Accounts == nil {
			continue
		}
		if acct := rm.svc.Accounts.For(rm.playerFor(id)); acct != nil {
			_ = acct.Store().Set(context.Background(), "best",
				[]byte(strconv.Itoa(ps.dist)), kit.MergeMax)
		}
	}
}

func (rm *room) loadBest(p kit.Player, ps *pilot) {
	if rm.svc.Accounts == nil {
		return
	}
	acct := rm.svc.Accounts.For(p)
	if acct == nil {
		return
	}
	if v, ok, err := acct.Store().Get(context.Background(), "best"); err == nil && ok {
		ps.best, _ = strconv.Atoi(string(v))
	}
}

// playerFor reconstructs a Player for an account id; a departed player falls
// back to a token built from the id so rankings still name them.
func (rm *room) playerFor(id string) kit.Player {
	if p, ok := rm.names[id]; ok {
		return p
	}
	return kit.Player{AccountID: id, Handle: id, Kind: kit.KindMember}
}

// leaderID is the furthest still-flying pilot (crash spectators follow them).
func (rm *room) leaderID() string {
	best, bx := "", -1.0
	for _, id := range rm.order {
		ps := rm.pilots[id]
		if ps.alive && ps.x > bx {
			best, bx = id, ps.x
		}
	}
	return best
}
