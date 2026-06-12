package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// mphase is the run's state machine.
type mphase uint8

const (
	phLobby  mphase = iota // crew muster: difficulty + launch
	phSector               // live duty: orders, timers, anomalies
	phWarp                 // the between-sectors interstitial
	phOver                 // hull breach — debrief, SPACE back to the lobby
)

const (
	lobbyWait    = 20 * time.Second // the lobby auto-launches after this
	warpDur      = 4 * time.Second
	maxHull      = 10
	hullPatch    = 2 // restored on every warp jump
	hailCooldown = 2 * time.Second
	maxHails     = 3 // ticker depth (two render, one queued)
)

var difficultyNames = []string{"CADET", "CAPTAIN", "ADMIRAL"}

// crew is one human aboard: their panel, their current order, their stats.
type crew struct {
	id      string
	player  kit.Player
	boarded bool // has a panel in the current run (joiners wait for a warp)
	panel   []control
	ord     order
	sel     int // arrow-fallback selection index into panel

	hailReadyAt time.Time
	goodUntil   time.Time // order box flashes green on completion

	mashKey rune // meteor storm: this crew's assigned mash key
	mashN   int

	done, hailsSent, fumbles int
	best                     int // career best sectors, from KV
}

// hail is one comms-ticker entry: a crewmate broadcasting their order.
type hail struct {
	id   string // sender account (renders as "you" on their own frame)
	who  string // sender handle, pre-trimmed
	text string // the order text (shared string, built at issue time)
	at   time.Time
	seq  int // order serial, so resolution clears the entry
}

type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	crews      []*crew
	difficulty int // index into difficultyNames

	phase   mphase
	sector  int
	hull    int
	charges int
	need    int

	orderSeq int
	hails    []hail

	// anomalies (see anomaly.go)
	anKind   anomalyKind
	anStage  anStage
	anWarnAt time.Time // warn banner ends / effect lands
	anEndAt  time.Time // effect ends
	lastAn   anomalyKind
	schedule []int // charge counts that trigger the sector's anomalies

	now            time.Time
	lobbyUntil     time.Time
	warpUntil      time.Time
	hullFlashUntil time.Time
	fumbleText     string
	fumbleUntil    time.Time

	warpOrders int // sector summary shown during the jump
	score      int // sectors cleared, set at game over

	frame *kit.Frame
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:        cfg,
		svc:        svc,
		difficulty: 1, // CAPTAIN
		phase:      phLobby,
		hull:       maxHull,
		frame:      kit.NewFrame(),
	}
}

// --- lifecycle -----------------------------------------------------------

func (rm *room) OnStart(r kit.Room) {
	r.SetInputContext(kit.CtxNav)
	rm.now = r.Now()
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	if c := rm.crewByID(p.AccountID); c != nil {
		c.player = p // reconnect: keep their seat
		rm.render(r)
		return
	}
	c := &crew{id: p.AccountID, player: p, best: rm.loadBest(r, p)}
	rm.crews = append(rm.crews, c)
	// Gather in the lobby and launch together (SPACE, or the fallback timer)
	// so everyone arriving at once crews the same ship.
	if rm.phase == phLobby {
		rm.lobbyUntil = rm.now.Add(lobbyWait)
	}
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	for i, c := range rm.crews {
		if c.id == p.AccountID {
			rm.crews = append(rm.crews[:i], rm.crews[i+1:]...)
			break
		}
	}
	if rm.phase == phSector {
		// Orders aimed at the leaver's panel can never complete — reroll them.
		for _, cw := range rm.crews {
			if cw.ord.active && cw.ord.targetID == p.AccountID {
				cw.ord.active = false
				rm.removeHail(cw.ord.seq)
				rm.issueOrder(r.Rand(), cw, rm.now)
			}
		}
		if rm.boardedCount() == 0 && len(rm.crews) > 0 {
			rm.toLobby(r) // only spectators left — regroup
		}
	}
	rm.render(r)
}

func (rm *room) OnClose(r kit.Room) {}

// --- input -----------------------------------------------------------------

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	rm.now = r.Now()
	c := rm.crewByID(p.AccountID)
	if c == nil {
		return
	}
	switch rm.phase {
	case phLobby:
		switch kit.Resolve(in, kit.CtxNav) {
		case kit.ActConfirm:
			rm.launch(r) // anyone can launch
		case kit.ActLeft:
			if rm.difficulty > 0 {
				rm.difficulty--
			}
		case kit.ActRight:
			if rm.difficulty < len(difficultyNames)-1 {
				rm.difficulty++
			}
		}
	case phOver:
		if kit.Resolve(in, kit.CtxNav) == kit.ActConfirm {
			rm.toLobby(r)
		}
	case phSector:
		rm.sectorInput(r, c, in)
	}
	rm.render(r)
}

func (rm *room) sectorInput(r kit.Room, c *crew, in kit.Input) {
	if !c.boarded {
		return // spectating until the next warp
	}
	key := lowerRune(in)

	// A meteor storm swallows the panel: only the assigned mash key counts.
	if rm.meteorActive() {
		if key == c.mashKey {
			c.mashN++
		}
		return
	}

	switch kit.Resolve(in, kit.CtxCommand) {
	case kit.ActLeft:
		rm.moveSel(c, -1, 0)
		return
	case kit.ActRight:
		rm.moveSel(c, +1, 0)
		return
	case kit.ActUp, kit.ActDown:
		rm.moveSel(c, 0, 1)
		return
	case kit.ActConfirm:
		rm.actuate(r, c, c.sel)
		return
	}

	if key == 'h' {
		rm.sendHail(c)
		return
	}
	for i := range c.panel {
		if c.panel[i].key == key {
			rm.actuate(r, c, i)
			return
		}
	}
}

func lowerRune(in kit.Input) rune {
	if in.Kind != kit.InputRune {
		return 0
	}
	r := in.Rune
	if r >= 'A' && r <= 'Z' {
		r += 'a' - 'A'
	}
	return r
}

// moveSel is the arrow-key fallback: a selection ring over the hotkey grid.
func (rm *room) moveSel(c *crew, dx, dy int) {
	per := len(c.panel) / 2
	row, col := c.sel/per, c.sel%per
	col = (col + dx + per) % per
	if dy != 0 {
		row = 1 - row
	}
	c.sel = row*per + col
}

// actuate presses one control on c's own panel and resolves any order that
// the new state satisfies — whoever's order it is, even by accident.
func (rm *room) actuate(r kit.Room, c *crew, idx int) {
	ctrl := &c.panel[idx]
	if !ctrl.actuate(rm.now) {
		return // the press went to wiping fog
	}
	for _, cw := range rm.crews {
		o := &cw.ord
		if !o.active || o.targetID != c.id || o.ctrlIdx != idx {
			continue
		}
		if o.want == -1 || ctrl.state == o.want {
			rm.completeOrder(r, cw, ctrl)
		}
	}
}

func (rm *room) completeOrder(r kit.Room, owner *crew, ctrl *control) {
	owner.ord.active = false
	owner.done++
	owner.goodUntil = rm.now.Add(400 * time.Millisecond)
	ctrl.litUntil = rm.now.Add(600 * time.Millisecond)
	rm.removeHail(owner.ord.seq)
	rm.charges++
	if rm.charges >= rm.need {
		rm.beginWarp(r)
		return
	}
	rm.issueOrder(r.Rand(), owner, rm.now)
}

// sendHail broadcasts c's current order to the comms ticker on every frame.
func (rm *room) sendHail(c *crew) {
	if rm.boardedCount() < 2 || !c.ord.active || rm.now.Before(c.hailReadyAt) {
		return
	}
	c.hailReadyAt = rm.now.Add(hailCooldown)
	c.hailsSent++
	if len(rm.hails) >= maxHails {
		copy(rm.hails, rm.hails[1:])
		rm.hails = rm.hails[:maxHails-1]
	}
	rm.hails = append(rm.hails, hail{
		id: c.id, who: handleOf(c.player), text: c.ord.text, at: rm.now, seq: c.ord.seq,
	})
}

func (rm *room) removeHail(seq int) {
	out := rm.hails[:0]
	for _, h := range rm.hails {
		if h.seq != seq {
			out = append(out, h)
		}
	}
	rm.hails = out
}

// --- the heartbeat ----------------------------------------------------------

func (rm *room) OnWake(r kit.Room) {
	rm.now = r.Now()
	switch rm.phase {
	case phLobby:
		if !rm.lobbyUntil.IsZero() && rm.now.After(rm.lobbyUntil) && len(rm.crews) > 0 {
			rm.launch(r)
		}
	case phSector:
		rm.stepAnomaly(r)
		if rm.phase != phSector { // an anomaly penalty can end the run
			break
		}
		if !rm.meteorActive() {
			rm.expireOrders(r)
		}
	case phWarp:
		if rm.now.After(rm.warpUntil) {
			rm.beginSector(r, rm.sector+1)
		}
	}
	rm.render(r)
}

func (rm *room) expireOrders(r kit.Room) {
	for _, c := range rm.crews {
		if !c.boarded || !c.ord.active || rm.now.Before(c.ord.expires) {
			continue
		}
		c.ord.active = false
		c.fumbles++
		rm.removeHail(c.ord.seq)
		rm.fumbleText = c.ord.text
		rm.fumbleUntil = rm.now.Add(1200 * time.Millisecond)
		if rm.loseHull(r, 1) {
			return
		}
		rm.issueOrder(r.Rand(), c, rm.now)
	}
}

// loseHull applies damage; reports true when the ship is gone.
func (rm *room) loseHull(r kit.Room, n int) bool {
	rm.hull -= n
	rm.hullFlashUntil = rm.now.Add(300 * time.Millisecond)
	if rm.hull <= 0 {
		rm.hull = 0
		rm.gameOver(r)
		return true
	}
	return false
}

// --- run setup / teardown -----------------------------------------------------

func (rm *room) launch(r kit.Room) {
	if len(rm.crews) == 0 {
		return
	}
	rm.lobbyUntil = time.Time{}
	rm.hull = maxHull
	for _, c := range rm.crews {
		c.done, c.hailsSent, c.fumbles = 0, 0, 0
	}
	rm.beginSector(r, 1)
}

// beginSector deals fresh panels to everyone aboard (mid-run joiners board
// here) and opens the order flow.
func (rm *room) beginSector(r kit.Room, sector int) {
	rm.sector = sector
	rm.charges = 0
	rm.hails = rm.hails[:0]
	rm.anKind, rm.anStage = anNone, asNone

	n := 8
	if sector <= 2 {
		n = 6
	}
	rng := r.Rand()
	used := make(map[string]bool, len(rm.crews)*n)
	for _, c := range rm.crews {
		c.boarded = true
		c.panel = genPanel(rng, used, n, sector)
		c.sel = 0
		c.ord.active = false
	}
	rm.need = rm.chargesNeed()
	rm.scheduleAnomalies(rng)
	for _, c := range rm.crews {
		rm.issueOrder(rng, c, rm.now)
	}
	rm.phase = phSector
	r.SetInputContext(kit.CtxCommand)
}

// chargesNeed is the sector's warp-bar size (provisional tuning, spec §7).
func (rm *room) chargesNeed() int {
	c := rm.boardedCount()
	var need int
	switch {
	case rm.sector == 1:
		need = 4 + 3*c
	case rm.sector == 2:
		need = 5 + 3*c
	case rm.sector == 3:
		need = 5 + 4*c
	default:
		need = 6 + 4*c
	}
	if c == 1 {
		need += 4 // a solo shift earns its warp
	}
	return need
}

func (rm *room) beginWarp(r kit.Room) {
	rm.warpOrders = rm.charges
	rm.hull += hullPatch
	if rm.hull > maxHull {
		rm.hull = maxHull
	}
	for _, c := range rm.crews {
		c.ord.active = false
	}
	rm.hails = rm.hails[:0]
	rm.anKind, rm.anStage = anNone, asNone
	rm.phase = phWarp
	rm.warpUntil = rm.now.Add(warpDur)
}

func (rm *room) gameOver(r kit.Room) {
	rm.score = rm.sector - 1
	rm.phase = phOver
	r.SetInputContext(kit.CtxNav)
	for _, c := range rm.crews {
		c.ord.active = false
	}

	// The whole crew banks the same number — co-op means one score.
	rankings := make([]kit.PlayerResult, 0, len(rm.crews))
	for _, c := range rm.crews {
		if !c.boarded {
			continue
		}
		rankings = append(rankings, kit.PlayerResult{
			Player: c.player, Metric: rm.score, Status: kit.StatusFinished,
		})
		if rm.score > c.best {
			c.best = rm.score
			if acct := r.Services().Accounts.For(c.player); acct != nil {
				_ = acct.Store().Set(context.Background(), "best",
					[]byte(strconv.Itoa(rm.score)), kit.MergeMax)
			}
		}
	}
	if len(rankings) > 0 {
		r.Post(kit.Result{Rankings: rankings})
	}
}

func (rm *room) toLobby(r kit.Room) {
	rm.phase = phLobby
	r.SetInputContext(kit.CtxNav)
	rm.lobbyUntil = rm.now.Add(lobbyWait)
}

// --- small helpers -----------------------------------------------------------

func (rm *room) crewByID(id string) *crew {
	for _, c := range rm.crews {
		if c.id == id {
			return c
		}
	}
	return nil
}

func (rm *room) boardedCount() int {
	n := 0
	for _, c := range rm.crews {
		if c.boarded {
			n++
		}
	}
	return n
}

func (rm *room) loadBest(r kit.Room, p kit.Player) int {
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return 0
	}
	v, ok, err := acct.Store().Get(context.Background(), "best")
	if err != nil || !ok {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(v)))
	if err != nil {
		return 0
	}
	return n
}

func handleOf(p kit.Player) string {
	h := p.Handle
	if h == "" {
		h = "crew"
	}
	if len(h) > 8 {
		h = h[:8]
	}
	return h
}
