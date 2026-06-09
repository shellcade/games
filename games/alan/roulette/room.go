package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// Round phases. These are INTERNAL state-machine markers held in guest memory —
// the lean ABI has no phase surface — so a phase only drives this game's own
// logic and rendering.
const (
	phBetting  = "betting"
	phSpinning = "spinning"
	phResults  = "results"
)

const (
	startBalance = 1000 // chips a first-ever player sits down with
	rebuyAmount  = 1000  // balance restored on a bust

	bettingDur  = 20 * time.Second // the open betting window
	spinAnimDur = 5 * time.Second  // wheel deceleration
	spinHoldDur = 3 * time.Second  // rest on the landed pocket before settling
	spinDur     = spinAnimDur + spinHoldDur
	resultsDur  = 7 * time.Second // payoff hold (board) before the next window
	gracePeriod = 2 * time.Second // beat between "all ready" and the spin

	historyLen = 12 // recent winning numbers kept for the marquee

	// wallet KV keys + merge rules, the casino pattern (balance: sum, the
	// carryable bankroll; peak: max, the high-water mark + leaderboard metric).
	keyBalance = "balance"
	keyPeak    = "peak"
)

// stakeTiers are the selectable chip denominations, lowest first.
var stakeTiers = []int{10, 25, 50, 100}

// placedBet is one chip a player has put down this round (kept in placement
// order so Backspace can undo the last one).
type placedBet struct {
	master int // index into masterBets
	stake  int
}

// player is one seat at the table, keyed by account id so it survives a
// hibernation freeze/thaw (connections change; accounts don't).
type player struct {
	p          kit.Player
	balance    int
	peak       int
	postedPeak int // last peak Posted to the board (post only on increase)
	stakeIdx   int // index into stakeTiers
	sel        selection
	bets       []placedBet
	ready      bool
	joinOrder  int
	colorIdx   int // index into the chip-colour palette (stable while seated)

	lastNet    int  // net chips from the last settled round (for the results panel)
	lastPlayed bool // had at least one bet in the last settled round
}

// staked is the total currently on the felt for this player.
func (pl *player) staked() int {
	t := 0
	for _, b := range pl.bets {
		t += b.stake
	}
	return t
}

// pending names the deferred one-shot the room is waiting on — each a deadline
// held in guest memory and landed in OnWake when r.Now() passes it (the wake
// idiom; no host timer survives a thaw).
type pending uint8

const (
	pendNone    pending = iota
	pendSpin            // betting window closed -> roll + spin the wheel
	pendSettle          // wheel finished decelerating -> pay out
	pendResults         // results hold elapsed -> reopen betting
)

type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	phase   string
	players map[string]*player
	order   []string // join order of account ids
	joinSeq int

	// deadline is the betting-window end (rendered as the countdown). what is the
	// armed one-shot and pendAt the instant it fires; the round is strictly
	// sequential so at most one is armed at a time. closing marks the short grace
	// beat after every seated player readies up.
	deadline time.Time
	what     pending
	pendAt   time.Time
	closing  bool

	// spin animation state. result is rolled once at spin start (from the seeded
	// RNG) so a seeded room reproduces every outcome and a later render never
	// re-rolls it.
	spinStart time.Time
	result    int
	spunOnce  bool
	history   []int // recent winning numbers, newest last

	lastNow  time.Time
	frame    *kit.Frame
	groupBuf []betGroup // reused per-render scratch for the "your chips" summary
	chipBits []uint8    // per master bet: bitmask of player colours with a chip there (reused)

	// viewer is the player a frame is currently being composed for, set
	// transiently by compose so the outside-box drawer can read this viewer's
	// overlays without threading it through every call.
	viewer *player
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:      cfg,
		svc:      svc,
		players:  map[string]*player{},
		frame:    kit.NewFrame(),
		chipBits: make([]uint8, len(masterBets)),
	}
}

// freeColorIdx returns the lowest chip-colour index not currently held by a
// seated player, so each player at the table has a distinct colour.
func (rm *room) freeColorIdx() int {
	for i := 0; i < numChipColors; i++ {
		taken := false
		for _, id := range rm.order {
			if p := rm.players[id]; p != nil && p.colorIdx == i {
				taken = true
				break
			}
		}
		if !taken {
			return i
		}
	}
	return 0 // more players than colours (capacity keeps this from happening)
}

func (rm *room) OnStart(r kit.Room) {
	rm.lastNow = r.Now()
	rm.enterBetting(r)
	rm.render(r)
}

func (rm *room) OnClose(r kit.Room) {
	for _, id := range rm.order {
		if pl := rm.players[id]; pl != nil {
			rm.persistWallet(r, pl)
		}
	}
}

// --- durable wallet (the casino pattern over kv) ---------------------------

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

// seedWallet returns the joining player's durable (balance, peak): balance
// defaults to startBalance for a first-ever player (or a non-positive stored
// balance), and peak is raised to at least the balance. A nil/guest account
// returns the defaults.
func (rm *room) seedWallet(r kit.Room, p kit.Player) (balance, peak int) {
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return startBalance, startBalance
	}
	store := acct.Store()
	bal, ok := kvInt(store, keyBalance)
	if !ok || bal <= 0 {
		bal = startBalance
	}
	pk, ok := kvInt(store, keyPeak)
	if !ok || pk < bal {
		pk = bal
	}
	return bal, pk
}

// persistWallet writes the current balance (summed) and raises peak (max). The
// monotonic max-on-write means out-of-order or concurrent same-account writes
// can never regress the leaderboard metric.
func (rm *room) persistWallet(r kit.Room, pl *player) {
	acct := r.Services().Accounts.For(pl.p)
	if acct == nil {
		return
	}
	store := acct.Store()
	_ = store.Set(context.Background(), keyBalance, []byte(strconv.Itoa(pl.balance)), kit.MergeSum)
	_ = store.Set(context.Background(), keyPeak, []byte(strconv.Itoa(pl.peak)), kit.MergeMax)
}

// postPeak feeds the declared leaderboard with a new personal peak (the board
// keeps each account's best). KV is durable state; Post is what reaches the
// board.
func (rm *room) postPeak(r kit.Room, pl *player) {
	if pl.peak <= pl.postedPeak {
		return
	}
	pl.postedPeak = pl.peak
	r.Post(kit.Result{Rankings: []kit.PlayerResult{{
		Player: pl.p, Metric: pl.peak, Status: kit.StatusFinished,
	}}})
}

// --- roster ----------------------------------------------------------------

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	if pl := rm.players[p.AccountID]; pl != nil {
		pl.p = p // refresh the token (handle/conn may have changed on rejoin)
		rm.render(r)
		return
	}
	bal, peak := rm.seedWallet(r, p)
	rm.players[p.AccountID] = &player{
		p: p, balance: bal, peak: peak, postedPeak: peak,
		sel: newSelection(), joinOrder: rm.joinSeq, colorIdx: rm.freeColorIdx(),
	}
	rm.joinSeq++
	rm.order = append(rm.order, p.AccountID)
	rm.render(r)
}

// OnLeave persists the leaver's wallet and frees the seat. Bets still on the
// felt during the open betting window are refunded (the round hasn't resolved);
// bets locked in once the wheel is spinning are forfeit, as at a real table.
func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	pl := rm.players[p.AccountID]
	if pl == nil {
		return
	}
	if rm.phase == phBetting {
		rm.refundAll(pl)
	}
	rm.persistWallet(r, pl)
	delete(rm.players, p.AccountID)
	for i, id := range rm.order {
		if id == p.AccountID {
			rm.order = append(rm.order[:i], rm.order[i+1:]...)
			break
		}
	}
	// A departure can complete an "all ready" table.
	if rm.phase == phBetting {
		rm.maybeCloseEarly(r)
	}
	rm.render(r)
}

// --- the wake heartbeat ----------------------------------------------------

// OnWake advances the armed one-shot against CallContext time, then renders
// once. Each branch may re-arm `what`, so it is re-read on the next wake.
func (rm *room) OnWake(r kit.Room) {
	rm.lastNow = r.Now()
	if rm.what != pendNone && rm.lastNow.After(rm.pendAt) {
		switch rm.what {
		case pendSpin:
			rm.what = pendNone
			rm.onBettingClose(r)
		case pendSettle:
			rm.what = pendNone
			rm.settle(r)
		case pendResults:
			rm.what = pendNone
			rm.enterBetting(r)
		}
	}
	rm.render(r)
}

func (rm *room) arm(what pending, at time.Time) {
	rm.what = what
	rm.pendAt = at
}

// --- betting ---------------------------------------------------------------

func (rm *room) enterBetting(r kit.Room) {
	rm.phase = phBetting
	rm.closing = false
	for _, id := range rm.order {
		pl := rm.players[id]
		if pl == nil {
			continue
		}
		pl.bets = nil
		pl.ready = false
		rm.clampStake(pl)
	}
	rm.deadline = r.Now().Add(bettingDur)
	r.SetInputContext(kit.CtxNav)
	rm.arm(pendSpin, rm.deadline)
}

func (rm *room) onBettingClose(r kit.Room) {
	if rm.anyBet() {
		rm.startSpin(r)
		return
	}
	rm.enterBetting(r) // nobody staked anything — reopen the window
}

func (rm *room) anyBet() bool {
	for _, id := range rm.order {
		if pl := rm.players[id]; pl != nil && len(pl.bets) > 0 {
			return true
		}
	}
	return false
}

// allReady reports whether at least one seat is taken and every seated player
// has readied up.
func (rm *room) allReady() bool {
	seated := false
	for _, id := range rm.order {
		pl := rm.players[id]
		if pl == nil {
			continue
		}
		seated = true
		if !pl.ready {
			return false
		}
	}
	return seated
}

// maybeCloseEarly arms the short grace beat once every seated player is ready
// and at least one chip is down. A guard (closing) keeps a later toggle during
// the grace beat from re-arming and pushing the spin out.
func (rm *room) maybeCloseEarly(r kit.Room) {
	if rm.closing || !rm.allReady() || !rm.anyBet() {
		return
	}
	rm.closing = true
	rm.arm(pendSpin, r.Now().Add(gracePeriod))
}

func (rm *room) toggleReady(r kit.Room, pl *player) {
	pl.ready = !pl.ready
	if rm.closing && !rm.allReady() {
		// Someone backed out during the grace beat: cancel the early close and
		// restore the full window deadline.
		rm.closing = false
		rm.arm(pendSpin, rm.deadline)
		return
	}
	rm.maybeCloseEarly(r)
}

// --- stakes & chips --------------------------------------------------------

func (rm *room) clampStake(pl *player) {
	// Drop to the highest tier the balance can cover (at least the lowest index).
	for pl.stakeIdx > 0 && stakeTiers[pl.stakeIdx] > pl.balance {
		pl.stakeIdx--
	}
}

func (rm *room) adjustStake(pl *player, dir int) {
	i := pl.stakeIdx + dir
	if i < 0 {
		i = 0
	}
	if i >= len(stakeTiers) {
		i = len(stakeTiers) - 1
	}
	pl.stakeIdx = i
	rm.clampStake(pl)
}

// placeBet puts the current stake on the armed bet, deducting immediately.
func (rm *room) placeBet(pl *player) {
	mi := pl.sel.betIndex()
	if mi < 0 {
		return
	}
	stake := stakeTiers[pl.stakeIdx]
	if stake > pl.balance {
		return // can't cover it
	}
	pl.balance -= stake
	pl.bets = append(pl.bets, placedBet{master: mi, stake: stake})
	pl.ready = false // placing a chip un-readies you
}

// undoBet removes and refunds the last chip placed.
func (rm *room) undoBet(pl *player) {
	n := len(pl.bets)
	if n == 0 {
		return
	}
	pl.balance += pl.bets[n-1].stake
	pl.bets = pl.bets[:n-1]
}

// clearBets refunds every chip on the felt.
func (rm *room) clearBets(pl *player) { rm.refundAll(pl) }

func (rm *room) refundAll(pl *player) {
	pl.balance += pl.staked()
	pl.bets = nil
}

// --- spinning & settlement -------------------------------------------------

func (rm *room) startSpin(r kit.Room) {
	rm.phase = phSpinning
	rm.closing = false
	rm.spinStart = r.Now()
	rm.result = r.Rand().Intn(pockets) // the outcome, fixed up front
	rm.spunOnce = true
	rm.deadline = rm.spinStart.Add(spinDur)
	rm.arm(pendSettle, rm.deadline)
}

func (rm *room) settle(r kit.Room) {
	for _, id := range rm.order {
		pl := rm.players[id]
		if pl == nil {
			continue
		}
		staked := pl.staked()
		ret := 0
		for _, b := range pl.bets {
			ret += settleReturn(masterBets[b.master], b.stake, rm.result)
		}
		pl.balance += ret
		pl.lastPlayed = len(pl.bets) > 0
		pl.lastNet = ret - staked
		// The chips stay on the felt through the results screen so players can
		// see them against the winning number; enterBetting clears them when the
		// next window opens.
		if pl.balance <= 0 {
			pl.balance = rebuyAmount
		}
		if pl.balance > pl.peak {
			pl.peak = pl.balance
		}
		rm.persistWallet(r, pl)
		rm.postPeak(r, pl)
	}
	// Record the winning number for the marquee.
	rm.history = append(rm.history, rm.result)
	if len(rm.history) > historyLen {
		rm.history = rm.history[len(rm.history)-historyLen:]
	}
	rm.phase = phResults
	rm.deadline = r.Now().Add(resultsDur)
	rm.arm(pendResults, rm.deadline)
}

// --- input -----------------------------------------------------------------

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	pl := rm.players[p.AccountID]
	if pl == nil {
		return
	}
	if rm.phase == phBetting {
		rm.handleBetInput(r, pl, in)
	}
	// Spinning and results are watch-only; the round advances on the wake clock.
	rm.render(r)
}

func (rm *room) handleBetInput(r kit.Room, pl *player, in kit.Input) {
	switch kit.Resolve(in, kit.CtxNav) {
	case kit.ActUp:
		pl.sel.move(-1, 0)
		return
	case kit.ActDown:
		pl.sel.move(1, 0)
		return
	case kit.ActLeft:
		pl.sel.move(0, -1)
		return
	case kit.ActRight:
		pl.sel.move(0, 1)
		return
	case kit.ActConfirm:
		rm.placeBet(pl)
		return
	}
	if in.Kind == kit.InputKey && in.Key == kit.KeyBackspace {
		rm.undoBet(pl)
		return
	}
	if in.Kind == kit.InputRune {
		switch in.Rune {
		case '-', '_':
			rm.adjustStake(pl, -1)
		case '+', '=':
			rm.adjustStake(pl, +1)
		case 'c', 'C':
			rm.clearBets(pl)
		case 'r', 'R':
			rm.toggleReady(r, pl)
		}
	}
}

// remaining is the seconds left on the current phase deadline, never negative.
func (rm *room) remaining(now time.Time) int {
	d := rm.deadline.Sub(now)
	if d < 0 {
		d = 0
	}
	return int((d + time.Second - 1) / time.Second) // ceil
}
