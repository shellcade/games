package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// room hosts a cabinet of independent boards — one per seated player, keyed by
// account id so a board survives a reconnect. Everyone plays at once; the only
// thing shared is the high-score board and a glance at each other's score.
type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	boards map[string]*board
	names  map[string]kit.Player
	order  []string // join order (stable rivals strip)

	now     time.Time
	lastNow time.Time

	frame *kit.Frame // reused render buffer (Send copies it)

	// sk standardises the durable high-score KV write (PersistBest, MergeMax),
	// replacing the hand-rolled Store().Set in persistBest. The leaderboard Post
	// stays hand-rolled in OnWake because board.posted is seeded from the durable
	// best at join — so a returning player only posts on a NEW high, which
	// ScoreKeeper.Record (always posts the first observed value) would not
	// preserve.
	sk *kit.ScoreKeeper
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:    cfg,
		svc:    svc,
		boards: map[string]*board{},
		names:  map[string]kit.Player{},
		frame:  kit.NewFrame(),
		sk:     kit.NewScoreKeeper(kit.OnImprove),
	}
}

// --- lifecycle ---------------------------------------------------------------

func (rm *room) OnStart(r kit.Room) {
	r.SetInputContext(kit.CtxNav)
	rm.now = r.Now()
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	rm.names[p.AccountID] = p
	if _, ok := rm.boards[p.AccountID]; !ok {
		rm.boards[p.AccountID] = newBoard(rm.loadBest(r, p))
		rm.order = append(rm.order, p.AccountID)
	}
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	rm.now = r.Now()
	if b := rm.boards[p.AccountID]; b != nil {
		rm.persistBest(r, p, b)
		delete(rm.boards, p.AccountID)
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
	for id, b := range rm.boards {
		if p, ok := rm.names[id]; ok {
			rm.persistBest(r, p, b)
		}
	}
}

// --- input -------------------------------------------------------------------

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	rm.now = r.Now()
	b := rm.boards[p.AccountID]
	if b == nil {
		return
	}
	switch kit.Resolve(in, kit.CtxNav) {
	case kit.ActLeft:
		b.movePaddle(-1)
	case kit.ActRight:
		b.movePaddle(+1)
	case kit.ActConfirm: // Space / Enter: launch the bit, or replay after game over
		b.launch(r.Rand())
	}
	rm.render(r)
}

// --- the heartbeat -----------------------------------------------------------

func (rm *room) OnWake(r kit.Room) {
	rm.now = r.Now()
	dt := rm.step()
	for id, b := range rm.boards {
		b.step(dt, rm.now, r.Rand())
		if b.best > b.posted {
			b.posted = b.best
			if p, ok := rm.names[id]; ok {
				rm.persistBest(r, p, b)
				r.Post(kit.Result{Rankings: []kit.PlayerResult{{
					Player: p, Metric: b.best, Status: kit.StatusFinished,
				}}})
			}
		}
	}
	rm.render(r)
}

// step returns seconds since the last wake, clamped so a pause or hibernation
// can't teleport the bits across the board.
func (rm *room) step() float64 {
	dt := 0.05
	if !rm.lastNow.IsZero() {
		if d := rm.now.Sub(rm.lastNow).Seconds(); d > 0 && d < 0.2 {
			dt = d
		} else if d >= 0.2 {
			dt = 0.2
		}
	}
	rm.lastNow = rm.now
	return dt
}

// --- durable high score ------------------------------------------------------

func kvInt(store kit.KVStore, key string) (int, bool) {
	v, ok, err := store.Get(context.Background(), key)
	if err != nil || !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(v)))
	if err != nil {
		return 0, false
	}
	return n, true
}

func (rm *room) loadBest(r kit.Room, p kit.Player) int {
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return 0
	}
	best, _ := kvInt(acct.Store(), "best")
	return best
}

func (rm *room) persistBest(r kit.Room, p kit.Player, b *board) {
	rm.sk.PersistBest(r, p, "best", b.best)
}
