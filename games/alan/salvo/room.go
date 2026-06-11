package main

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// mphase is the match's state machine.
type mphase uint8

const (
	phLobby  mphase = iota // no battle yet (no players)
	phAim                  // the current tank lines up a shot
	phFlight               // a shell is in the air
	phImpact               // the blast is going off
	phSettle               // craters made — tanks tumble into them
	phOver                 // a winner; a beat, then a rematch
)

const (
	maxWind     = 13.0 // cells/sec^2, re-rolled each turn
	thinkDelay  = 1100 * time.Millisecond
	turnLimit   = 30 * time.Second // a human turn auto-fires if it stalls
	impactDur   = 480 * time.Millisecond
	settleHold  = 250 * time.Millisecond
	overDur     = 5 * time.Second
	lobbyWait   = 20 * time.Second // the lobby auto-starts after this if nobody hits SPACE
	fallSpeed   = 14.0             // rows/sec a tank tumbles into a crater
	soloTanks   = 3                // a lone human battles this many tanks total (rest are CPU)
	fallDmgPerR = 5                // health lost per row fallen (beyond a freebie)
)

// CPU difficulty scales how far the aimer lets itself miss: easy lobs wide,
// hard is nearly dead-on.
var difficultyNames = []string{"EASY", "NORMAL", "HARD"}
var difficultyMiss = []float64{2.4, 1.0, 0.3}

// boom is an expanding blast ring; particle is flung debris. Both are eye-candy.
type boom struct {
	x, y, radius float64
	bornAt       time.Time
	color        kit.Color
}

type particle struct {
	x, y, vx, vy float64
	until        time.Time
	color        kit.Color
	glyph        rune
}

type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	players map[string]kit.Player // current humans by account id
	order   []string              // human join order
	wins    map[string]int        // career wins, loaded from KV

	terrain []int
	tanks   []*tank
	turn    int
	phase   mphase
	shell   *shell
	booms   []boom
	parts   []particle
	wind    float64
	winner  *tank

	now, lastNow time.Time
	phaseUntil   time.Time
	lobbyUntil   time.Time // fallback auto-start while gathering in the lobby
	turnEndsAt   time.Time
	cpuActAt     time.Time
	msg          string
	msgUntil     time.Time
	cpuSeq       int
	cpuWanted    int    // CPU opponents to add, chosen in the lobby
	difficulty   int    // CPU difficulty, chosen in the lobby (index into difficultyNames)
	randState    uint32 // LCG state for cosmetic scatter + CPU jitter

	frame *kit.Frame
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:        cfg,
		svc:        svc,
		players:    map[string]kit.Player{},
		wins:       map[string]int{},
		phase:      phLobby,
		cpuWanted:  soloTanks - 1, // a lone player defaults to two CPU foes
		difficulty: 1,             // NORMAL
		frame:      kit.NewFrame(),
	}
}

// clampCpu keeps the chosen CPU count to a sane range: at least enough to make
// two tanks, and never more than the table holds.
func (rm *room) clampCpu() {
	lo := 2 - len(rm.order)
	if lo < 0 {
		lo = 0
	}
	hi := len(tankPalette) - len(rm.order)
	if hi < 0 {
		hi = 0
	}
	rm.cpuWanted = clampI(rm.cpuWanted, lo, hi)
}

// --- lifecycle ---------------------------------------------------------------

func (rm *room) OnStart(r kit.Room) {
	r.SetInputContext(kit.CtxNav)
	rm.now = r.Now()
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	rm.players[p.AccountID] = p
	if _, seen := rm.wins[p.AccountID]; !seen {
		rm.order = append(rm.order, p.AccountID)
	}
	rm.wins[p.AccountID] = rm.loadWins(r, p)
	// Gather in the lobby and start together (on SPACE, or the fallback timer) so
	// everyone arriving at once lands in the same battle, sized for all of them —
	// rather than the first arrival auto-starting and locking the rest out.
	if rm.phase == phLobby {
		rm.lobbyUntil = rm.now.Add(lobbyWait)
	}
	rm.clampCpu()
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	delete(rm.players, p.AccountID)
	for i, id := range rm.order {
		if id == p.AccountID {
			rm.order = append(rm.order[:i], rm.order[i+1:]...)
			break
		}
	}
	// Forfeit their tank if a battle is live.
	if t := rm.tankByID(p.AccountID); t != nil && t.alive {
		t.alive = false
		rm.kaboom(float64(t.col), t.y, t.color, 14)
		rm.checkMatchEnd(r)
	}
	rm.clampCpu()
	rm.render(r)
}

func (rm *room) OnClose(r kit.Room) {}

// --- input -------------------------------------------------------------------

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	rm.now = r.Now()
	if rm.phase == phLobby {
		switch kit.Resolve(in, kit.CtxNav) {
		case kit.ActConfirm:
			rm.startMatch(r) // anyone can start the battle
		case kit.ActUp:
			rm.cpuWanted++
			rm.clampCpu()
		case kit.ActDown:
			rm.cpuWanted--
			rm.clampCpu()
		case kit.ActLeft:
			if rm.difficulty > 0 {
				rm.difficulty--
			}
		case kit.ActRight:
			if rm.difficulty < len(difficultyNames)-1 {
				rm.difficulty++
			}
		}
		rm.render(r)
		return
	}
	if rm.phase == phOver {
		if kit.Resolve(in, kit.CtxNav) == kit.ActConfirm {
			rm.startMatch(r) // skip the wait, rematch now
			rm.render(r)
		}
		return
	}
	if rm.phase != phAim {
		return
	}
	t := rm.currentTank()
	if t == nil || t.cpu || t.id != p.AccountID {
		return // only the tank whose turn it is may act
	}
	switch kit.Resolve(in, kit.CtxNav) {
	case kit.ActLeft:
		t.adjustAngle(+2)
	case kit.ActRight:
		t.adjustAngle(-2)
	case kit.ActUp:
		t.adjustPower(+4)
	case kit.ActDown:
		t.adjustPower(-4)
	case kit.ActConfirm:
		rm.fire(r)
	default:
		if in.Kind == kit.InputRune && (in.Rune == 'w' || in.Rune == 'W') {
			t.cycleWeapon()
		}
	}
	rm.render(r)
}

// --- the heartbeat -----------------------------------------------------------

func (rm *room) OnWake(r kit.Room) {
	rm.now = r.Now()
	dt := rm.step()

	switch rm.phase {
	case phLobby:
		if !rm.lobbyUntil.IsZero() && rm.now.After(rm.lobbyUntil) {
			rm.startMatch(r)
		}
	case phAim:
		t := rm.currentTank()
		if t == nil {
			break
		}
		if t.cpu {
			if rm.now.After(rm.cpuActAt) {
				cpuAim(t, rm.tanks, rm.terrain, rm.wind, rm.frand, difficultyMiss[rm.difficulty])
				rm.fire(r)
			}
		} else if rm.now.After(rm.turnEndsAt) {
			rm.fire(r) // AFK auto-fire
		}
	case phFlight:
		rm.advanceShell(dt, r)
	case phImpact:
		rm.advanceParticles(dt)
		rm.advanceBooms()
		if rm.now.After(rm.phaseUntil) {
			rm.beginSettle()
		}
	case phSettle:
		rm.settleStep(dt, r)
		rm.advanceParticles(dt)
		rm.advanceBooms()
		if rm.settleDone() && rm.now.After(rm.phaseUntil) {
			rm.endTurn(r)
		}
	case phOver:
		rm.advanceParticles(dt)
		rm.advanceBooms()
		if rm.now.After(rm.phaseUntil) {
			rm.startMatch(r)
		}
	}
	rm.render(r)
}

func (rm *room) step() float64 {
	dt := 0.05
	if !rm.lastNow.IsZero() {
		if d := rm.now.Sub(rm.lastNow).Seconds(); d > 0 {
			dt = math.Min(d, 0.1)
		}
	}
	rm.lastNow = rm.now
	return dt
}

// --- match setup -------------------------------------------------------------

func (rm *room) startMatch(r kit.Room) {
	rm.lobbyUntil = time.Time{}
	if len(rm.order) == 0 {
		rm.phase = phLobby // nobody to fight — wait (an empty ephemeral room closes)
		return
	}
	rm.terrain = genTerrain(r.Rand())
	rm.shell = nil
	rm.booms = rm.booms[:0]
	rm.parts = rm.parts[:0]
	rm.winner = nil

	rm.clampCpu()
	total := clampI(len(rm.order)+rm.cpuWanted, 2, len(tankPalette))
	cols := spreadCols(total)

	rm.tanks = rm.tanks[:0]
	ci := 0
	for _, id := range rm.order {
		if ci >= total {
			break
		}
		p := rm.players[id]
		t := newTank(id, p, false, handleOf(p), cols[ci], ci)
		t.y = surfaceAt(rm.terrain, t.col)
		rm.tanks = append(rm.tanks, t)
		ci++
	}
	rm.cpuSeq = 0
	for ci < total {
		rm.cpuSeq++
		id := "cpu#" + strconv.Itoa(rm.cpuSeq)
		t := newTank(id, kit.Player{}, true, "CPU "+strconv.Itoa(rm.cpuSeq), cols[ci], ci)
		t.y = surfaceAt(rm.terrain, t.col)
		rm.tanks = append(rm.tanks, t)
		ci++
	}

	rm.turn = 0 // fresh tanks are all alive; the first one leads off
	rm.beginTurn(r)
	rm.setMsg("BATTLE!", 1400)
}

// spreadCols places n tanks evenly across the field, clear of the walls.
func spreadCols(n int) []int {
	const left, right = 7, scrW - 8
	cols := make([]int, n)
	if n == 1 {
		cols[0] = scrW / 2
		return cols
	}
	step := float64(right-left) / float64(n-1)
	for i := 0; i < n; i++ {
		cols[i] = left + int(math.Round(float64(i)*step))
	}
	return cols
}

// --- turns -------------------------------------------------------------------

func (rm *room) currentTank() *tank {
	if rm.turn < 0 || rm.turn >= len(rm.tanks) {
		return nil
	}
	return rm.tanks[rm.turn]
}

func (rm *room) aliveCount() int {
	n := 0
	for _, t := range rm.tanks {
		if t.alive {
			n++
		}
	}
	return n
}

func (rm *room) advanceTurn(r kit.Room) {
	if rm.checkMatchEnd(r) {
		return
	}
	for i := 1; i <= len(rm.tanks); i++ {
		idx := (rm.turn + i) % len(rm.tanks)
		if rm.tanks[idx].alive {
			rm.turn = idx
			break
		}
	}
	rm.beginTurn(r)
}

// beginTurn opens the aiming phase for the current tank: a fresh wind, the turn
// clock, and (for a CPU) its think timer.
func (rm *room) beginTurn(r kit.Room) {
	t := rm.currentTank()
	if t == nil {
		return
	}
	if t.ammo[t.weapon] == 0 {
		t.cycleWeapon()
	}
	rm.rollWind(r)
	rm.phase = phAim
	rm.turnEndsAt = rm.now.Add(turnLimit)
	if t.cpu {
		rm.cpuActAt = rm.now.Add(thinkDelay)
	}
}

func (rm *room) endTurn(r kit.Room) {
	if rm.checkMatchEnd(r) {
		return
	}
	rm.advanceTurn(r)
}

func (rm *room) checkMatchEnd(r kit.Room) bool {
	if rm.phase == phOver || len(rm.tanks) == 0 {
		return true
	}
	if rm.aliveCount() > 1 {
		return false
	}
	// 1 (or 0) left: the battle is over.
	rm.winner = nil
	for _, t := range rm.tanks {
		if t.alive {
			rm.winner = t
		}
	}
	if rm.winner != nil && !rm.winner.cpu {
		rm.awardWin(r, rm.winner)
	}
	rm.phase = phOver
	rm.phaseUntil = rm.now.Add(overDur)
	return true
}

func (rm *room) rollWind(r kit.Room) {
	rm.wind = (r.Rand().Float64()*2 - 1) * maxWind
}

// --- firing + flight ---------------------------------------------------------

func (rm *room) fire(r kit.Room) {
	t := rm.currentTank()
	if t == nil || rm.phase != phAim {
		return
	}
	if t.ammo[t.weapon] > 0 {
		t.ammo[t.weapon]--
	}
	x, y := t.barrelTip()
	vx, vy := t.launchVel()
	rm.shell = &shell{x: x, y: y, vx: vx, vy: vy, w: weapons[t.weapon], owner: t}
	if t.ammo[t.weapon] == 0 {
		t.cycleWeapon()
	}
	rm.phase = phFlight
}

func (rm *room) advanceShell(dt float64, r kit.Room) {
	s := rm.shell
	if s == nil {
		rm.endTurn(r)
		return
	}
	speed := math.Hypot(s.vx, s.vy)
	steps := int(math.Ceil(speed*dt/0.4)) + 1
	sub := dt / float64(steps)
	for i := 0; i < steps; i++ {
		s.trail = append(s.trail, pt{s.x, s.y})
		if len(s.trail) > maxTrail {
			s.trail = s.trail[len(s.trail)-maxTrail:]
		}
		s.x, s.y, s.vx, s.vy = integrate(s.x, s.y, s.vx, s.vy, rm.wind, sub)

		if s.x < 0 || s.x >= scrW || s.y > float64(groundBottom)+2 {
			rm.shell = nil // sailed off the field — a dud, no blast
			rm.beginImpactPause()
			return
		}
		if t := rm.tankHit(s); t != nil {
			rm.explode(s.x, s.y, s.w)
			return
		}
		if solidAt(rm.terrain, s.x, s.y) {
			rm.explode(s.x, s.y, s.w)
			return
		}
	}
}

func (rm *room) tankHit(s *shell) *tank {
	for _, t := range rm.tanks {
		if !t.alive {
			continue
		}
		if math.Hypot(float64(t.col)-s.x, t.y-s.y) < 1.2 {
			return t
		}
	}
	return nil
}

// --- explosions --------------------------------------------------------------

func (rm *room) explode(ex, ey float64, w weapon) {
	rm.shell = nil
	craterAt(rm.terrain, ex, ey, w.crater)
	for _, t := range rm.tanks {
		if !t.alive {
			continue
		}
		d := math.Hypot(float64(t.col)-ex, t.y-ey)
		if d >= w.radius {
			continue
		}
		dmg := int(math.Round(float64(w.damage) * (1 - d/w.radius)))
		rm.hurt(t, dmg)
	}
	rm.kaboom(ex, ey, w.color, w.radius)
	rm.phase = phImpact
	rm.phaseUntil = rm.now.Add(impactDur)
}

func (rm *room) hurt(t *tank, dmg int) {
	if dmg <= 0 || !t.alive {
		return
	}
	t.health -= dmg
	if t.health <= 0 {
		t.health = 0
		t.alive = false
		rm.kaboom(float64(t.col), t.y, t.color, 9)
		rm.setMsg(strings.ToUpper(t.name)+" DESTROYED", 1500)
	}
}

// kaboom spawns the blast ring + a spray of debris.
func (rm *room) kaboom(x, y float64, c kit.Color, radius float64) {
	rm.booms = append(rm.booms, boom{x: x, y: y, radius: radius, bornAt: rm.now, color: c})
	n := 6 + int(radius)
	glyphs := []rune{'*', '+', '.', '\'', '`'}
	for i := 0; i < n; i++ {
		ang := rm.frand() * 2 * math.Pi
		spd := 6 + rm.frand()*16
		rm.parts = append(rm.parts, particle{
			x: x, y: y,
			vx:    math.Cos(ang) * spd,
			vy:    math.Sin(ang)*spd/aspect - 4,
			until: rm.now.Add(time.Duration(350+int(rm.frand()*350)) * time.Millisecond),
			color: c,
			glyph: glyphs[int(rm.frand()*float64(len(glyphs)))%len(glyphs)],
		})
	}
}

func (rm *room) advanceBooms() {
	out := rm.booms[:0]
	for _, b := range rm.booms {
		if rm.now.Sub(b.bornAt) < impactDur {
			out = append(out, b)
		}
	}
	rm.booms = out
}

func (rm *room) advanceParticles(dt float64) {
	out := rm.parts[:0]
	for _, p := range rm.parts {
		if rm.now.After(p.until) {
			continue
		}
		p.x += p.vx * dt
		p.y += p.vy * dt
		p.vy += gravity * 0.6 * dt
		out = append(out, p)
	}
	rm.parts = out
}

// --- settling (tanks fall into fresh craters) --------------------------------

func (rm *room) beginImpactPause() {
	rm.phase = phImpact
	rm.phaseUntil = rm.now.Add(impactDur)
}

func (rm *room) beginSettle() {
	// Apply fall damage up front for any tank now hanging over a crater, then let
	// the visuals catch up over the settle phase.
	for _, t := range rm.tanks {
		if !t.alive {
			continue
		}
		if !grounded(rm.terrain, t.col) {
			continue // will tumble into the pit during settleStep
		}
		drop := surfaceAt(rm.terrain, t.col) - t.y
		if drop > 2 {
			rm.hurt(t, int(drop-2)*fallDmgPerR)
		}
	}
	rm.phase = phSettle
	rm.phaseUntil = rm.now.Add(settleHold)
}

func (rm *room) settleStep(dt float64, r kit.Room) {
	for _, t := range rm.tanks {
		if !t.alive {
			continue
		}
		if !grounded(rm.terrain, t.col) {
			t.y += fallSpeed * dt
			if t.y > float64(groundBottom)+2 {
				t.alive = false
				rm.kaboom(float64(t.col), float64(groundBottom)+1, t.color, 6)
				rm.setMsg(strings.ToUpper(t.name)+" FELL OFF", 1500)
			}
			continue
		}
		target := surfaceAt(rm.terrain, t.col)
		if t.y < target-0.05 {
			t.y += fallSpeed * dt
			if t.y > target {
				t.y = target
			}
		} else if t.y > target {
			t.y = target
		}
	}
}

func (rm *room) settleDone() bool {
	for _, t := range rm.tanks {
		if !t.alive {
			continue
		}
		if !grounded(rm.terrain, t.col) {
			return false
		}
		if t.y < surfaceAt(rm.terrain, t.col)-0.05 {
			return false
		}
	}
	return true
}

// --- durable wins ------------------------------------------------------------

func (rm *room) awardWin(r kit.Room, t *tank) {
	rm.wins[t.id]++
	total := rm.wins[t.id]
	acct := r.Services().Accounts.For(t.player)
	if acct != nil {
		_ = acct.Store().Set(context.Background(), "wins", []byte(strconv.Itoa(total)), kit.MergeMax)
	}
	r.Post(kit.Result{Rankings: []kit.PlayerResult{{
		Player: t.player, Metric: total, Status: kit.StatusFinished,
	}}})
	rm.setMsg(strings.ToUpper(t.name)+" WINS!", 4000)
}

func (rm *room) loadWins(r kit.Room, p kit.Player) int {
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return 0
	}
	v, ok, err := acct.Store().Get(context.Background(), "wins")
	if err != nil || !ok {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(v)))
	if err != nil {
		return 0
	}
	return n
}

// --- small helpers -----------------------------------------------------------

func (rm *room) tankByID(id string) *tank {
	for _, t := range rm.tanks {
		if t.id == id {
			return t
		}
	}
	return nil
}

func (rm *room) setMsg(s string, ms int) {
	rm.msg = s
	rm.msgUntil = rm.now.Add(time.Duration(ms) * time.Millisecond)
}

// frand is a small deterministic float source (0..1) for particle scatter and
// CPU aim jitter — an LCG so replays stay reproducible without threading the
// room RNG through every call site.
func (rm *room) frand() float64 {
	rm.randState = rm.randState*1664525 + 1013904223
	return float64(rm.randState>>8) / float64(1<<24)
}

func handleOf(p kit.Player) string {
	h := p.Handle
	if h == "" {
		h = "Player"
	}
	if len(h) > 8 {
		h = h[:8]
	}
	return h
}
