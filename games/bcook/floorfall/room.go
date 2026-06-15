package main

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// palette assigns each contestant a distinct bright color by join order.
var palette = []kit.Color{
	kit.RGB(0x4f, 0xd6, 0xff), // cyan
	kit.RGB(0xff, 0x8a, 0x4f), // orange
	kit.RGB(0x7d, 0xff, 0x6b), // green
	kit.RGB(0xff, 0x6b, 0xc7), // pink
	kit.RGB(0xb9, 0x8a, 0xff), // purple
	kit.RGB(0xff, 0xe1, 0x55), // yellow
}

// room is the live game state. Per-player state lives in players, keyed by
// account id; the floors and pending decays are plain arrays/slices advanced on
// each wake.
type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	players map[string]*player    // by account id (hibernation-safe)
	names   map[string]kit.Player // account id -> player (handle/persist/character)
	order   []string              // join order of account ids (stable scoreboard)

	floors [layers][arenaH][arenaW]uint8 // tile decay stage per layer/row/col
	decay  []decayEvent                  // pending per-tile decay ticks

	roundStart time.Time // when the current round's arena went live
	playing    bool      // a round is in progress (false during intermission)
	resumeAt   time.Time // when the next round starts (during intermission)
	winnerID   string    // account id of the round winner ("" = none/solo)

	crumbleAcc float64 // fractional ambient-crumble budget carried between wakes

	now     time.Time
	lastNow time.Time

	frame *kit.Frame // long-lived render buffer, reused every frame (Send copies)
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:     cfg,
		svc:     svc,
		players: map[string]*player{},
		names:   map[string]kit.Player{},
		frame:   kit.NewFrame(),
	}
}

// --- lifecycle ---------------------------------------------------------------

func (rm *room) OnStart(r kit.Room) {
	r.SetInputContext(kit.CtxNav)
	rm.now = r.Now()
	rm.startRound(r)
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	rm.names[p.AccountID] = p
	if _, ok := rm.players[p.AccountID]; !ok {
		pl := &player{
			glyph: '@',
			color: palette[len(rm.order)%len(palette)],
		}
		pl.bestSecs = rm.loadBest(r, p)
		rm.players[p.AccountID] = pl
		rm.order = append(rm.order, p.AccountID)
		rm.spawn(r, pl)
	}
	// The contestant's character IS their glyph: their character becomes the
	// avatar and their character's BACKGROUND colour becomes their colour
	// everywhere it shows (avatar + scoreboard ● and name). A zero Character (a
	// host that doesn't declare the feature, or test doubles) reverts to the '@'
	// glyph and the join-order palette colour. Applied on every join so a player
	// reconnecting after hibernation carries their current look.
	pl := rm.players[p.AccountID]
	if c := p.Character; c.Glyph != "" {
		for _, ru := range c.Glyph {
			pl.glyph = ru
			break
		}
		pl.color = kit.RGB(c.BgR, c.BgG, c.BgB)
	} else {
		pl.glyph = '@'
		for i, id := range rm.order {
			if id == p.AccountID {
				pl.color = palette[i%len(palette)]
				break
			}
		}
	}
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	if pl := rm.players[p.AccountID]; pl != nil {
		rm.persistBest(r, p.AccountID)
		delete(rm.players, p.AccountID)
	}
	delete(rm.names, p.AccountID)
	for i, id := range rm.order {
		if id == p.AccountID {
			rm.order = append(rm.order[:i], rm.order[i+1:]...)
			break
		}
	}
	rm.render(r)
}

func (rm *room) OnClose(r kit.Room) {
	for id := range rm.players {
		rm.persistBest(r, id)
	}
}

// OnInput moves a contestant one cell. A terminal has no key-up events, so each
// arrow is a discrete step; a short per-player cooldown keeps held-key autorepeat
// from teleporting anyone across the floor. Stepping off a tile schedules it to
// crumble a beat later, and stepping onto a hole drops you a layer.
func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	rm.now = r.Now()
	pl := rm.players[p.AccountID]
	if pl == nil || !pl.alive || !rm.playing {
		return
	}
	if rm.now.Sub(pl.lastMove) < moveEvery {
		return
	}
	dc, dr := 0, 0
	switch kit.Resolve(in, kit.CtxNav) {
	case kit.ActUp:
		dr = -1
	case kit.ActDown:
		dr = 1
	case kit.ActLeft:
		dc = -1
	case kit.ActRight:
		dc = 1
	default:
		return
	}
	rm.move(r, pl, dc, dr)
	rm.render(r)
}

// move steps a player by (dc,dr) if the destination is inside the arena. The
// tile they leave is scheduled to crumble after the grace delay; the tile they
// arrive on is resolved (a hole drops them a layer).
func (rm *room) move(r kit.Room, pl *player, dc, dr int) {
	nc, nr := pl.col+dc, pl.row+dr
	if nc < 0 || nc >= arenaW || nr < top || nr > bottom {
		return // arena edge — no wrap, edges are a hard boundary
	}
	pl.lastMove = rm.now
	// Schedule the tile we are leaving to begin crumbling after the grace beat.
	rm.scheduleDecay(pl.layer, pl.row, pl.col)
	pl.col, pl.row = nc, nr
	rm.resolveCell(r, pl)
}

// resolveCell drops a player through any holes under their feet, layer by layer,
// until they land on intact footing or fall out the bottom (elimination).
func (rm *room) resolveCell(r kit.Room, pl *player) {
	for rm.alive(pl) && rm.tileAt(pl.layer, pl.row, pl.col) == tileGone {
		pl.layer++
		pl.fellAt = rm.now
		if pl.layer >= layers {
			rm.eliminate(r, pl)
			return
		}
	}
}

func (rm *room) alive(pl *player) bool { return pl.alive }

// scheduleDecay queues a tile to start aging after the grace delay, unless it is
// already gone or already pending an earlier tick.
func (rm *room) scheduleDecay(layer, row, col int) {
	if rm.tileAt(layer, row, col) >= tileGone {
		return
	}
	at := rm.now.Add(decayDelay)
	for i := range rm.decay {
		d := rm.decay[i]
		if d.layer == layer && d.row == row && d.col == col {
			if at.Before(d.at) {
				rm.decay[i].at = at
			}
			return
		}
	}
	rm.decay = append(rm.decay, decayEvent{layer: layer, row: row, col: col, at: at})
}

// --- heartbeat ---------------------------------------------------------------

// OnWake is the heartbeat: age scheduled tiles, run the ambient crumble wave,
// drop anyone now standing on a hole, then check for a round end. Renders every
// view at the end.
func (rm *room) OnWake(r kit.Room) {
	rm.now = r.Now()
	dt := rm.step()

	if rm.playing {
		rm.advanceDecay()
		rm.ambientCrumble(r, dt)
		rm.dropStanders(r)
		rm.checkRoundEnd(r)
	} else if rm.now.After(rm.resumeAt) {
		rm.startRound(r)
	}
	rm.render(r)
}

// step advances the clock bookkeeping (clamped so a hibernation pause can't
// crumble the whole arena in one tick).
func (rm *room) step() float64 {
	dt := 0.05
	if !rm.lastNow.IsZero() {
		if d := rm.now.Sub(rm.lastNow).Seconds(); d > 0 {
			dt = math.Min(d, 0.2)
		}
	}
	rm.lastNow = rm.now
	return dt
}

// advanceDecay walks every due tile one decay stage and, if it has not reached
// the hole stage, re-queues its next tick.
func (rm *room) advanceDecay() {
	keep := rm.decay[:0]
	for _, d := range rm.decay {
		if rm.now.Before(d.at) {
			keep = append(keep, d)
			continue
		}
		stage := rm.tileAt(d.layer, d.row, d.col)
		if stage >= tileGone {
			continue // already a hole — drop the event
		}
		stage++
		rm.floors[d.layer][d.row-top][d.col] = stage
		if stage < tileGone {
			d.at = rm.now.Add(decayStep)
			keep = append(keep, d)
		}
	}
	rm.decay = keep
}

// ambientCrumble is the autonomous "crumble wave": it picks random solid-ish
// tiles and starts them aging. Solo, the rate accelerates over the round so a
// run is survive-as-long-as-you-can; with 2+ players it stays gentle and player
// footsteps are the main destroyer. This is Floorfall's analogue of the
// exemplar's solo/pvp split.
func (rm *room) ambientCrumble(r kit.Room, dt float64) {
	elapsed := rm.now.Sub(rm.roundStart).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	var rate float64
	if rm.solo() {
		rate = math.Min(soloCrumbleBase+soloCrumbleGrow*elapsed, soloCrumbleMax)
	} else {
		rate = math.Min(pvpCrumbleBase+pvpCrumbleGrow*elapsed, pvpCrumbleMax)
	}
	rm.crumbleAcc += rate * dt
	rng := r.Rand()
	for rm.crumbleAcc >= 1 {
		rm.crumbleAcc--
		layer := rng.Intn(layers)
		row := top + rng.Intn(arenaH)
		col := rng.Intn(arenaW)
		// The wave can target any solid tile — including the one under a player's
		// feet, so standing still is never safe (solo always has a failure state).
		// Decay still takes the grace beat plus two stages (~1.2s) to fully open,
		// so a contestant who sees their tile start to crack has a moment to move.
		if rm.tileAt(layer, row, col) == tileSolid {
			rm.scheduleDecay(layer, row, col)
		}
	}
}

// dropStanders drops any living player whose tile has crumbled out from under
// them since the last wake (the ambient wave can open a hole beneath a player
// who is standing still).
func (rm *room) dropStanders(r kit.Room) {
	for _, pl := range rm.players {
		if pl.alive && rm.tileAt(pl.layer, pl.row, pl.col) == tileGone {
			rm.resolveCell(r, pl)
		}
	}
}

// checkRoundEnd resolves a finished round. Solo ends when the lone player is
// eliminated; multiplayer ends when one (or zero) remain. The winner is held on
// a banner through a short intermission, then a fresh arena begins.
func (rm *room) checkRoundEnd(r kit.Room) {
	living := 0
	var lastID string
	for id, pl := range rm.players {
		if pl.alive {
			living++
			lastID = id
		}
	}
	if len(rm.players) == 0 {
		return // empty room: keep the arena warm, nothing to resolve
	}
	if rm.solo() {
		if living == 0 {
			rm.endRound(r, "")
		}
		return
	}
	// Multiplayer: round is over when at most one contestant is left standing.
	if living <= 1 {
		rm.endRound(r, lastID)
	}
}

func (rm *room) endRound(r kit.Room, winnerID string) {
	rm.playing = false
	rm.winnerID = winnerID
	rm.resumeAt = rm.now.Add(intermission)
	if winnerID != "" {
		if pl := rm.players[winnerID]; pl != nil {
			rm.bankRun(r, winnerID, pl) // the survivor banks their time too
		}
	}
}

// --- rounds & spawning -------------------------------------------------------

func (rm *room) startRound(r kit.Room) {
	rm.playing = true
	rm.winnerID = ""
	rm.crumbleAcc = 0
	rm.decay = rm.decay[:0]
	rm.roundStart = rm.now
	// Fresh, fully-solid arena on every layer.
	for l := 0; l < layers; l++ {
		for row := 0; row < arenaH; row++ {
			for col := 0; col < arenaW; col++ {
				rm.floors[l][row][col] = tileSolid
			}
		}
	}
	for _, pl := range rm.players {
		rm.spawn(r, pl)
	}
	rm.render(r)
}

// spawn places a player on the top layer at a random intact, unoccupied cell and
// resets their per-run state.
func (rm *room) spawn(r kit.Room, pl *player) {
	rng := r.Rand()
	pl.layer = 0
	pl.alive = true
	pl.fellAt = time.Time{}
	pl.roundStart = rm.now
	col, row := arenaW/2, top+arenaH/2
	for try := 0; try < 24; try++ {
		c := rng.Intn(arenaW)
		rw := top + rng.Intn(arenaH)
		if !rm.occupied(0, rw, c) {
			col, row = c, rw
			break
		}
	}
	pl.col, pl.row = col, row
}

// eliminate marks a fallen player out and banks their survival time.
func (rm *room) eliminate(r kit.Room, pl *player) {
	pl.alive = false
	for id, p2 := range rm.players {
		if p2 == pl {
			rm.bankRun(r, id, pl)
			break
		}
	}
}

// bankRun records the player's survival time for the run that just ended: it
// posts the seconds to the leaderboard (best-of) and keeps a durable copy.
func (rm *room) bankRun(r kit.Room, id string, pl *player) {
	secs := int(rm.now.Sub(pl.roundStart).Seconds())
	if secs < 0 {
		secs = 0
	}
	pl.lastSecs = secs
	if secs > pl.bestSecs {
		pl.bestSecs = secs
		rm.persistBest(r, id)
	}
	if p, ok := rm.names[id]; ok {
		r.Post(kit.Result{Rankings: []kit.PlayerResult{{
			Player: p, Metric: secs, Status: kit.StatusFinished,
		}}})
	}
}

// --- helpers -----------------------------------------------------------------

func (rm *room) solo() bool { return len(rm.players) <= 1 }

func (rm *room) tileAt(layer, row, col int) uint8 {
	if layer < 0 || layer >= layers || row < top || row > bottom || col < 0 || col >= arenaW {
		return tileGone
	}
	return rm.floors[layer][row-top][col]
}

// occupied reports whether a living player currently stands on a given cell.
func (rm *room) occupied(layer, row, col int) bool {
	for _, pl := range rm.players {
		if pl.alive && pl.layer == layer && pl.row == row && pl.col == col {
			return true
		}
	}
	return false
}

// --- durable best survival ---------------------------------------------------

func (rm *room) loadBest(r kit.Room, p kit.Player) int {
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return 0
	}
	v, ok, err := acct.Store().Get(context.Background(), "best_secs")
	if err != nil || !ok {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(v)))
	if err != nil {
		return 0
	}
	return n
}

func (rm *room) persistBest(r kit.Room, id string) {
	pl := rm.players[id]
	p, ok := rm.names[id]
	if pl == nil || !ok {
		return
	}
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return
	}
	_ = acct.Store().Set(context.Background(), "best_secs", []byte(strconv.Itoa(pl.bestSecs)), kit.MergeMax)
}
