package main

import (
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
	// maxPayoutMult is the declared per-seat gross ceiling (Meta.MaxPayoutMultiplier).
	// The richest single wager is a straight-up (35:1): a winning stake returns
	// stake*(35+1) = stake*36, and no combination of chips on the board can return
	// more than 36x the seat's total stake — so a whole-board gross is a
	// stake-weighted blend that is always <= 36x. This is the true top prize.
	maxPayoutMult = 36

	// minBet is the table minimum (the lowest chip tier); a seat whose account-wide
	// balance falls below it after a Settle is offered the platform Buyback.
	minBet = 5

	bettingDur  = 20 * time.Second // the open betting window
	spinAnimDur = 5 * time.Second  // wheel deceleration
	spinHoldDur = 3 * time.Second  // rest on the landed pocket before settling
	spinDur     = spinAnimDur + spinHoldDur
	resultsDur  = 7 * time.Second // payoff hold (board) before the next window
	gracePeriod = 2 * time.Second // beat between "all ready" and the spin

	// After the ball lands, the winning squares pause a beat, then flash until the
	// wheel disappears (settlement), where the results board shows them solid.
	flashDelay  = 1 * time.Second
	flashPeriod = 300 * time.Millisecond // half a blink cycle

	historyLen = 12 // recent winning numbers kept for the marquee
)

// stakeTiers are the selectable chip denominations, lowest first. The 5 chip is
// the floor so a player whittled down to a few dollars can still place a bet
// (and lose into the re-buy) rather than getting stuck unable to afford a chip.
var stakeTiers = []int{5, 10, 25, 50, 100}

// defaultStakeIdx is the chip a fresh seat arms — the 10, not the 5 floor.
const defaultStakeIdx = 1

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
	bal        int64 // cached account-wide Credits balance (refreshed at money events)
	postedPeak int64 // highest balance Posted to the board (post only on increase)
	stakeIdx   int   // index into stakeTiers
	sel        selection
	bets       []placedBet
	wagered    bool // an escrow is open for this seat (Wagered this spin, not yet Settled)
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

// avail is what this seat can still put on the felt: the cached account-wide
// balance less the chips already down (bets are LOCAL until the spin locks, so
// nothing is escrowed yet — affordability is purely bal minus staked).
func (pl *player) avail() int64 { return pl.bal - int64(pl.staked()) }

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

	// econOff is set when the host has no Credits economy (svc.Credits nil, or a
	// Balance call reports the economy disabled/unavailable). The table then
	// renders "credits offline" instead of a bankroll rather than trapping.
	econOff bool

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
// seated player, so each player at the table has a distinct colour (the palette
// covers MaxPlayers; see TestChipPaletteCoversTable).
func (rm *room) freeColorIdx() int {
	for i := 0; i < len(chipColors); i++ {
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

// OnClose closes any still-open escrow before the room disappears. The balances
// themselves are the platform's — nothing local to persist — but a seat that
// was mid-spin (Wagered, not yet Settled) when the room tears down must be
// Settled or its escrow leaks. The result is already rolled, so book the fair
// gross.
func (rm *room) OnClose(r kit.Room) {
	if rm.svc.Credits == nil {
		return
	}
	for _, id := range rm.order {
		if pl := rm.players[id]; pl != nil && pl.wagered {
			_ = rm.svc.Credits.Settle(pl.p, rm.grossFor(pl))
			pl.wagered = false
		}
	}
}

// --- account-wide Credits (the platform economy) ---------------------------

// refreshBal caches the seat's authoritative account-wide balance. Called only
// at money events (join, post-Wager, post-Settle, Buyback) — never per frame —
// so the hot render path reads the cached pl.bal. A nil economy or a
// disabled/unavailable host flips econOff and leaves the last cached value.
func (rm *room) refreshBal(pl *player) {
	if rm.svc.Credits == nil {
		rm.econOff = true
		return
	}
	b, err := rm.svc.Credits.Balance(pl.p)
	if err != nil {
		rm.econOff = true
		return
	}
	rm.econOff = false
	pl.bal = b
}

// grossFor is the seat's clamped gross return for the current result: the stake
// back plus winnings on every covered bet, capped at stake*maxPayoutMult so the
// game never books (or shows) more than the host will actually pay.
func (rm *room) grossFor(pl *player) int64 {
	var ret int64
	for _, b := range pl.bets {
		ret += int64(settleReturn(masterBets[b.master], b.stake, rm.result))
	}
	if cap := int64(pl.staked()) * maxPayoutMult; ret > cap {
		ret = cap
	}
	return ret
}

// postPeak feeds the declared leaderboard with the seat's account-wide balance
// whenever it sets a new personal high (the board keeps each account's best).
func (rm *room) postPeak(r kit.Room, pl *player) {
	if pl.bal <= pl.postedPeak {
		return
	}
	pl.postedPeak = pl.bal
	r.Post(kit.Result{Rankings: []kit.PlayerResult{{
		Player: pl.p, Metric: int(pl.bal), Status: kit.StatusFinished,
	}}})
}

// --- roster ----------------------------------------------------------------

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	if pl := rm.players[p.AccountID]; pl != nil {
		pl.p = p // refresh the token (handle/conn may have changed on rejoin)
		rm.refreshBal(pl)
		rm.render(r)
		return
	}
	pl := &player{
		p:        p,
		stakeIdx: defaultStakeIdx,
		sel:      newSelection(), joinOrder: rm.joinSeq, colorIdx: rm.freeColorIdx(),
	}
	rm.refreshBal(pl)   // read the account-wide balance the platform owns
	pl.postedPeak = pl.bal
	rm.clampStake(pl) // arm the highest chip this balance can cover
	rm.players[p.AccountID] = pl
	rm.joinSeq++
	rm.order = append(rm.order, p.AccountID)
	rm.render(r)
}

// OnLeave frees the seat. A seat that has an OPEN escrow (Wagered at spin lock,
// not yet Settled) must be Settled or the stake leaks: the result is already
// rolled, so book the fair gross. Chips merely resting on the felt during the
// open betting window are pre-Wager — a purely local refund, no escrow to close.
func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	pl := rm.players[p.AccountID]
	if pl == nil {
		return
	}
	if pl.wagered {
		if rm.svc.Credits != nil {
			_ = rm.svc.Credits.Settle(pl.p, rm.grossFor(pl))
		}
		pl.wagered = false
	} else if rm.phase == phBetting {
		rm.refundAll(pl)
	}
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
		rm.cancelEarlyClose()
		return
	}
	rm.maybeCloseEarly(r)
}

// cancelEarlyClose backs out of the armed grace beat (someone un-readied or
// went back to betting) and restores the full window deadline.
func (rm *room) cancelEarlyClose() {
	if !rm.closing {
		return
	}
	rm.closing = false
	rm.arm(pendSpin, rm.deadline)
}

// --- stakes & chips --------------------------------------------------------

func (rm *room) clampStake(pl *player) {
	// Drop to the highest tier the still-available balance can cover (at least the
	// lowest index). Bets are local until spin lock, so "available" is bal - staked.
	for pl.stakeIdx > 0 && int64(stakeTiers[pl.stakeIdx]) > pl.avail() {
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

// placeBet puts the current stake on the armed bet. This is LOCAL bookkeeping
// only — nothing is escrowed until the spin locks (the ABI has no un-wager, so
// chips a player might still undo/clear cannot be Wagered yet); affordability
// gates on the cached balance less what is already down.
func (rm *room) placeBet(pl *player) {
	mi := pl.sel.betIndex()
	if mi < 0 {
		return
	}
	stake := stakeTiers[pl.stakeIdx]
	if int64(stake) > pl.avail() {
		return // can't cover it
	}
	pl.bets = append(pl.bets, placedBet{master: mi, stake: stake})
	// Placing a chip un-readies you — and if the table was already in the grace
	// beat, backs out of the early close too (same as un-readying with r), so a
	// player still betting can never be spun out from under.
	pl.ready = false
	rm.cancelEarlyClose()
}

// undoBet removes the last chip placed (local only — pre-Wager).
func (rm *room) undoBet(pl *player) {
	n := len(pl.bets)
	if n == 0 {
		return
	}
	pl.bets = pl.bets[:n-1]
}

// clearBets drops every chip on the felt (local only — pre-Wager).
func (rm *room) clearBets(pl *player) { rm.refundAll(pl) }

func (rm *room) refundAll(pl *player) { pl.bets = nil }

// --- spinning & settlement -------------------------------------------------

func (rm *room) startSpin(r kit.Room) {
	// The outcome is rolled once, up front, from the seeded RNG so a seeded room
	// reproduces every result and a later render never re-rolls it.
	rm.result = r.Rand().Intn(pockets)
	rm.spunOnce = true

	// Lock in ONE escrow per seat: the whole board is a single Wager, taken now.
	// A seat is only in the spin if its Wager succeeds; the ABI has no un-wager,
	// so this is the first and only escrow of the round.
	n := 0
	for _, id := range rm.order {
		pl := rm.players[id]
		if pl == nil || len(pl.bets) == 0 {
			continue
		}
		amt := int64(pl.staked())
		if rm.svc.Credits == nil {
			pl.bets = nil // economy off: cannot take the bet, drop the chips
			continue
		}
		if err := rm.svc.Credits.Wager(pl.p, amt); err != nil {
			pl.bets = nil // couldn't escrow (insufficient/disabled) — sit them out
			continue
		}
		pl.wagered = true
		pl.bal -= amt // reflect the escrow in the cached HUD balance
		n++
	}
	if n == 0 {
		rm.enterBetting(r) // nobody's stake could be taken — reopen the window
		return
	}
	rm.phase = phSpinning
	rm.closing = false
	rm.spinStart = r.Now()
	rm.deadline = rm.spinStart.Add(spinDur)
	rm.arm(pendSettle, rm.deadline)
}

func (rm *room) settle(r kit.Room) {
	for _, id := range rm.order {
		pl := rm.players[id]
		if pl == nil {
			continue
		}
		if !pl.wagered { // no escrow this round (sat out or Wager was refused)
			pl.lastPlayed = false
			continue
		}
		staked := pl.staked()
		ret := rm.grossFor(pl) // clamped gross (0 on a total loss)
		// Close the ONE open stake with its gross exactly once.
		if rm.svc.Credits != nil {
			_ = rm.svc.Credits.Settle(pl.p, ret)
		}
		pl.wagered = false
		pl.lastPlayed = staked > 0
		pl.lastNet = int(ret) - staked
		// The chips stay on the felt through the results screen so players can
		// see them against the winning number; enterBetting clears them when the
		// next window opens.
		rm.refreshBal(pl) // authoritative post-settle account-wide balance
		// Broke-relief: a seat wiped below the table minimum tops up from the
		// platform Buyback (solvent/limit-reached returns ErrInsufficientCredits,
		// which we surface by simply leaving the balance as-is — no retry).
		if pl.bal < minBet && rm.svc.Credits != nil {
			if nb, err := rm.svc.Credits.Buyback(pl.p); err == nil {
				pl.bal = nb
			}
		}
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
