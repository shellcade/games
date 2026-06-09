package main

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// palette assigns each pilot a distinct bright color by join order.
var palette = []kit.Color{
	kit.RGB(0x4f, 0xd6, 0xff), // cyan
	kit.RGB(0xff, 0x8a, 0x4f), // orange
	kit.RGB(0x7d, 0xff, 0x6b), // green
	kit.RGB(0xff, 0x6b, 0xc7), // pink
	kit.RGB(0xb9, 0x8a, 0xff), // purple
	kit.RGB(0xff, 0xe1, 0x55), // yellow
}

var (
	craterColor = kit.RGB(0x9a, 0x86, 0x6a)
	bulletWhite = kit.RGB(0xff, 0xff, 0xff)
)

// room is the live game state. Per-pilot state lives in ships, keyed by account
// id; everything else is plain slices advanced on each wake.
type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	ships   map[string]*ship      // by account id (hibernation-safe)
	names   map[string]kit.Player // account id -> player (for handle/persist)
	order   []string              // join order of account ids (stable scoreboard)
	bullets []bullet
	craters []crater
	booms   []explosion
	stars   []star

	now     time.Time
	lastNow time.Time
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:   cfg,
		svc:   svc,
		ships: map[string]*ship{},
		names: map[string]kit.Player{},
	}
}

// --- lifecycle ---------------------------------------------------------------

func (rm *room) OnStart(r kit.Room) {
	r.SetInputContext(kit.CtxNav)
	rm.now = r.Now()
	rm.buildStarfield(r)
	for i := 0; i < initialRocks; i++ {
		rm.spawnCrater(r, 3)
	}
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	rm.names[p.AccountID] = p
	if _, ok := rm.ships[p.AccountID]; !ok {
		s := &ship{
			color: palette[len(rm.order)%len(palette)],
			best:  rm.loadBest(r, p),
		}
		rm.ships[p.AccountID] = s
		rm.order = append(rm.order, p.AccountID)
		rm.spawnShip(r, s)
	}
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	if s := rm.ships[p.AccountID]; s != nil {
		rm.persistBest(r, p.AccountID)
		delete(rm.ships, p.AccountID)
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
	for id := range rm.ships {
		rm.persistBest(r, id)
	}
}

// OnInput applies discrete control impulses. A terminal has no key-up events,
// so flight is impulse-based: each press nudges the ship and momentum carries
// it — which is exactly the asteroids drift we want.
func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	rm.now = r.Now()
	s := rm.ships[p.AccountID]
	if s == nil || !s.alive {
		return
	}
	switch kit.Resolve(in, kit.CtxNav) {
	case kit.ActLeft:
		s.heading -= rotStep
	case kit.ActRight:
		s.heading += rotStep
	case kit.ActUp:
		s.vx += math.Cos(s.heading) * thrustDV
		s.vy += math.Sin(s.heading) * thrustDV * aspect
	case kit.ActDown:
		s.vx *= brakeFactor
		s.vy *= brakeFactor
	case kit.ActConfirm:
		rm.fire(r, p, s)
	}
	rm.render(r)
}

// OnWake is the heartbeat: advance everything against elapsed time, resolve
// collisions, top up craters, respawn the dead, then render every view.
func (rm *room) OnWake(r kit.Room) {
	rm.now = r.Now()
	dt := rm.step()

	rm.advanceShips(dt)
	rm.advanceBullets(dt)
	rm.advanceCraters(dt)
	rm.resolveCollisions(r)
	rm.respawnDead(r)
	rm.pruneExplosions()

	for len(rm.craters) < craterTarget {
		rm.spawnCrater(r, 3)
	}
	rm.render(r)
}

// step returns the seconds elapsed since the last wake, clamped so a pause or
// hibernation can't teleport everything across the arena.
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

// --- physics -----------------------------------------------------------------

func (rm *room) advanceShips(dt float64) {
	drag := math.Exp(-dragPerSec * dt)
	for _, s := range rm.ships {
		if !s.alive {
			continue
		}
		s.vx *= drag
		s.vy *= drag
		// Cap speed (convert vy back to horizontal units for an honest hypot).
		if sp := math.Hypot(s.vx, s.vy/aspect); sp > maxSpeed {
			k := maxSpeed / sp
			s.vx *= k
			s.vy *= k
		}
		s.x = wrapX(s.x + s.vx*dt)
		s.y = wrapY(s.y + s.vy*dt)
	}
}

func (rm *room) advanceBullets(dt float64) {
	keep := rm.bullets[:0]
	for _, b := range rm.bullets {
		if rm.now.After(b.dieAt) {
			continue
		}
		b.x = wrapX(b.x + b.vx*dt)
		b.y = wrapY(b.y + b.vy*dt)
		keep = append(keep, b)
	}
	rm.bullets = keep
}

func (rm *room) advanceCraters(dt float64) {
	for i := range rm.craters {
		rm.craters[i].x = wrapX(rm.craters[i].x + rm.craters[i].vx*dt)
		rm.craters[i].y = wrapY(rm.craters[i].y + rm.craters[i].vy*dt)
	}
}

// --- combat ------------------------------------------------------------------

func (rm *room) fire(r kit.Room, p kit.Player, s *ship) {
	if rm.now.Sub(s.lastShot) < fireCooldown {
		return
	}
	s.lastShot = rm.now
	// Spawn just ahead of the nose so you never shoot yourself.
	bx := wrapX(s.x + math.Cos(s.heading)*1.6)
	by := wrapY(s.y + math.Sin(s.heading)*1.6*aspect)
	rm.bullets = append(rm.bullets, bullet{
		x: bx, y: by,
		vx:    math.Cos(s.heading) * bulletSpeed,
		vy:    math.Sin(s.heading) * bulletSpeed * aspect,
		dieAt: rm.now.Add(bulletLife),
		owner: p.AccountID,
		color: s.color,
	})
}

func (rm *room) resolveCollisions(r kit.Room) {
	keep := rm.bullets[:0]
	for _, b := range rm.bullets {
		if rm.bulletHitsCrater(r, b) || rm.bulletHitsShip(r, b) {
			continue
		}
		keep = append(keep, b)
	}
	rm.bullets = keep

	// Ramming a crater is fatal (no credit) and shatters the rock.
	for id, s := range rm.ships {
		if !s.alive || rm.now.Before(s.invulnUntil) {
			continue
		}
		for ci := 0; ci < len(rm.craters); ci++ {
			c := rm.craters[ci]
			rad := craterRadius(c.size) + 0.6
			if dist2(s.x, s.y, c.x, c.y) <= rad*rad {
				rm.killShip(id)
				rm.addExplosion(c.x, c.y, craterColor)
				rm.splitCrater(r, ci)
				break
			}
		}
	}
}

func (rm *room) bulletHitsCrater(r kit.Room, b bullet) bool {
	for ci := 0; ci < len(rm.craters); ci++ {
		c := rm.craters[ci]
		rad := craterRadius(c.size) + 0.5
		if dist2(b.x, b.y, c.x, c.y) <= rad*rad {
			rm.awardKill(r, b.owner, killCrater)
			rm.addExplosion(c.x, c.y, craterColor)
			rm.splitCrater(r, ci)
			return true
		}
	}
	return false
}

func (rm *room) bulletHitsShip(r kit.Room, b bullet) bool {
	for id, s := range rm.ships {
		if id == b.owner || !s.alive || rm.now.Before(s.invulnUntil) {
			continue
		}
		if dist2(b.x, b.y, s.x, s.y) <= shipHit*shipHit {
			rm.killShip(id)
			rm.awardKill(r, b.owner, killPlayer)
			return true
		}
	}
	return false
}

func (rm *room) killShip(id string) {
	s := rm.ships[id]
	if s == nil || !s.alive {
		return
	}
	s.alive = false
	s.deaths++
	s.respawnAt = rm.now.Add(respawnDelay)
	rm.addExplosion(s.x, s.y, s.color)
}

func (rm *room) awardKill(r kit.Room, ownerID string, amount int) {
	s := rm.ships[ownerID]
	if s == nil {
		return
	}
	s.kills += amount
	if s.kills > s.best {
		s.best = s.kills
		rm.persistBest(r, ownerID)
	}
}

// splitCrater removes crater ci and, if it was large or medium, spawns two
// smaller fragments flying apart from where it broke.
func (rm *room) splitCrater(r kit.Room, idx int) {
	c := rm.craters[idx]
	rm.craters = append(rm.craters[:idx], rm.craters[idx+1:]...)
	if c.size <= 1 {
		return
	}
	rng := r.Rand()
	for k := 0; k < 2; k++ {
		ang := rng.Float64() * 2 * math.Pi
		spd := 2 + rng.Float64()*3
		rm.craters = append(rm.craters, crater{
			x: c.x, y: c.y,
			vx:   math.Cos(ang) * spd,
			vy:   math.Sin(ang) * spd * aspect,
			size: c.size - 1,
		})
	}
}

func (rm *room) respawnDead(r kit.Room) {
	for _, s := range rm.ships {
		if !s.alive && rm.now.After(s.respawnAt) {
			rm.spawnShip(r, s)
		}
	}
}

// --- spawning ----------------------------------------------------------------

func (rm *room) spawnShip(r kit.Room, s *ship) {
	rng := r.Rand()
	s.x, s.y = rm.safeSpot(rng, 9)
	s.vx, s.vy = 0, 0
	s.heading = rng.Float64() * 2 * math.Pi
	s.alive = true
	s.invulnUntil = rm.now.Add(invulnDur)
}

func (rm *room) spawnCrater(r kit.Room, size int) {
	rng := r.Rand()
	x, y := rm.safeSpot(rng, 8)
	ang := rng.Float64() * 2 * math.Pi
	spd := 1.5 + rng.Float64()*2.5
	rm.craters = append(rm.craters, crater{
		x: x, y: y,
		vx:   math.Cos(ang) * spd,
		vy:   math.Sin(ang) * spd * aspect,
		size: size,
	})
}

// safeSpot finds a random point at least minDist (horizontal cells) from every
// living ship, falling back to a plain random point after a few tries.
func (rm *room) safeSpot(rng interface{ Float64() float64 }, minDist float64) (float64, float64) {
	var x, y float64
	for try := 0; try < 12; try++ {
		x = rng.Float64() * cols
		y = top + rng.Float64()*float64(playH)
		clear := true
		for _, s := range rm.ships {
			if s.alive && dist2(x, y, s.x, s.y) < minDist*minDist {
				clear = false
				break
			}
		}
		if clear {
			break
		}
	}
	return wrapX(x), wrapY(y)
}

func (rm *room) buildStarfield(r kit.Room) {
	rng := r.Rand()
	n := 34
	rm.stars = make([]star, 0, n)
	for i := 0; i < n; i++ {
		rm.stars = append(rm.stars, star{
			x:      rng.Intn(cols),
			y:      top + rng.Intn(playH),
			bright: rng.Intn(4) == 0,
		})
	}
}

func (rm *room) addExplosion(x, y float64, c kit.Color) {
	rm.booms = append(rm.booms, explosion{x: x, y: y, start: rm.now, color: c})
}

func (rm *room) pruneExplosions() {
	keep := rm.booms[:0]
	for _, e := range rm.booms {
		if rm.now.Sub(e.start) < explodeDur {
			keep = append(keep, e)
		}
	}
	rm.booms = keep
}

// --- durable high score ------------------------------------------------------

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

func (rm *room) persistBest(r kit.Room, id string) {
	s := rm.ships[id]
	p, ok := rm.names[id]
	if s == nil || !ok {
		return
	}
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return
	}
	_ = acct.Store().Set(context.Background(), "best", []byte(strconv.Itoa(s.best)), kit.MergeMax)
}
