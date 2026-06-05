package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	kit "github.com/shellcade/kit"
)

// phases of one round. These are INTERNAL state-machine markers held in guest
// memory — the lean ABI has no phase surface (SetPhase is gone; joinability is
// host-derived), so a phase only drives this game's own logic and rendering.
const (
	phBetting   = "betting"
	phInsurance = "insurance"
	phTurns     = "player turns"
	phResults   = "results"
)

const (
	startChips = 1000
	rebuyChips = 1000
	maxHands   = 4 // a seat may split up to four hands

	bettingDur   = 15 * time.Second
	insuranceDur = 10 * time.Second
	turnDur      = 20 * time.Second
	resultsDur   = 6 * time.Second

	// gracePeriod is the short beat between the last seated player placing a bet
	// and dealing, so an all-bets-in table deals early without feeling abrupt.
	gracePeriod = time.Second

	// wallet KV keys + the first-ever stack, mirroring the native casino package
	// (balance: merge sum, the carryable bankroll; peak: merge max, the
	// high-water mark and the leaderboard metric).
	keyBalance = "balance"
	keyPeak    = "peak"
)

// betTiers are the selectable stakes, lowest first.
var betTiers = []int{10, 25, 50, 100}

// phand is one hand a seat plays (a seat holds more than one after a split).
type phand struct {
	cards       hand
	bet         int
	resolved    bool // stood / busted / blackjack / doubled / surrendered
	doubled     bool
	surrendered bool
	fromSplit   bool // a split hand: a two-card 21 is a plain 21, not a blackjack
}

// seat is one player's place at the table. Keyed by account id in the room map
// so it survives a hibernation freeze/thaw (connections change; accounts don't).
type seat struct {
	p                kit.Player
	chips            int
	highScore        int
	postedPeak       int // last peak Posted to the board (post only on increase)
	bet              int // currently selected/placed stake
	placed           bool
	insurance        int
	insuranceDecided bool
	hands            []*phand
	joinOrder        int
	result           string // settlement summary for the results phase
}

// pending names the deferred one-shot the room is waiting on, replacing the
// native engine timers: each is a deadline held in guest memory and landed in
// OnWake when r.Now() passes it (the wake idiom — no host timer survives a thaw).
type pending uint8

const (
	pendNone        pending = iota
	pendBettingClose         // betting window closed (or grace beat elapsed)
	pendInsurance            // insurance window closed -> resolve
	pendTurn                 // active turn timed out -> auto-stand
	pendResults              // results flash elapsed -> next round
	pendSettle               // dealer reveal/draw animation done -> settle
)

type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	sh         *shoe
	phase      string
	seats      map[string]*seat // keyed by account id (hibernation-safe)
	order      []string         // join order of account ids
	dealer     hand
	dealerHole bool // hole card concealed
	joinSeq    int

	// deadline is the current phase deadline (rendered as the countdown) and
	// what is the active pending one-shot. pendAt is the instant `what` fires;
	// for most phases it equals deadline, but pendSettle/pendBettingClose-grace
	// carry their own instant. A single (what, pendAt) is enough because the
	// round is strictly sequential: at most one one-shot is armed at a time.
	deadline       time.Time
	what           pending
	pendAt         time.Time
	bettingClosing bool // grace timer armed after an all-bets-in early close

	lastNow time.Time

	// sched is the in-flight card animation schedule; schedEnd is the room-clock
	// instant the last card settles. A frame composed at or after schedEnd renders
	// every card settled, so the schedule is read-only cosmetic state derived from
	// authoritative hands. Cleared once schedEnd passes (in OnWake).
	sched    []cardAnim
	schedEnd time.Time

	frame *kit.Frame // reused render scratch (allocation-light steady state)
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{cfg: cfg, svc: svc, seats: map[string]*seat{}, frame: kit.NewFrame()}
}

func (rm *room) OnStart(r kit.Room) {
	rm.sh = newShoe(r.Rand())
	rm.enterBetting(r)
	rm.render(r)
}

func (rm *room) OnClose(r kit.Room) {
	// Persist every seat's durable wallet on the way out. Synchronous: the wasm
	// sandbox has no goroutines, and the host bounds the KV call.
	for _, id := range rm.order {
		if s := rm.seats[id]; s != nil {
			rm.persistWallet(r, s)
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
// defaults to startChips for a first-ever player, and peak is raised to at least
// the balance. A nil account (no durable storage) returns the defaults.
func (rm *room) seedWallet(r kit.Room, p kit.Player) (balance, peak int) {
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return startChips, startChips
	}
	store := acct.Store()
	bal, ok := kvInt(store, keyBalance)
	if !ok {
		bal = startChips
		_ = store.Set(context.Background(), keyBalance, []byte(strconv.Itoa(bal)), kit.MergeSum)
	}
	pk, _ := kvInt(store, keyPeak)
	if pk < bal {
		pk = bal
		_ = store.Set(context.Background(), keyPeak, []byte(strconv.Itoa(pk)), kit.MergeMax)
	}
	return bal, pk
}

// persistWallet writes the seat's balance and raises peak to >= balance, with
// the same keys/merge rules the native casino package used (sum for balance, max
// for peak — out-of-order or concurrent writes can never regress the metric).
func (rm *room) persistWallet(r kit.Room, s *seat) {
	acct := r.Services().Accounts.For(s.p)
	if acct == nil {
		return
	}
	store := acct.Store()
	_ = store.Set(context.Background(), keyBalance, []byte(strconv.Itoa(s.chips)), kit.MergeSum)
	_ = store.Set(context.Background(), keyPeak, []byte(strconv.Itoa(s.highScore)), kit.MergeMax)
}

// postPeak feeds the declared leaderboard with a new personal peak (the board
// keeps each account's best). KV is durable state; Post is what reaches the
// board. The native game surfaced the same peak via a custom KV provider.
func (rm *room) postPeak(r kit.Room, s *seat) {
	if s.highScore <= s.postedPeak {
		return
	}
	s.postedPeak = s.highScore
	r.Post(kit.Result{Rankings: []kit.PlayerResult{{
		Player: s.p, Metric: s.highScore, Status: kit.StatusFinished,
	}}})
}

// --- roster ----------------------------------------------------------------

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	if _, ok := rm.seats[p.AccountID]; ok {
		rm.render(r)
		return
	}
	bal, peak := rm.seedWallet(r, p)
	rm.seats[p.AccountID] = &seat{p: p, chips: bal, highScore: peak, postedPeak: peak, bet: betTiers[1], joinOrder: rm.joinSeq}
	rm.joinSeq++
	rm.order = append(rm.order, p.AccountID)
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	active, _ := rm.firstUnresolved()
	if s := rm.seats[p.AccountID]; s != nil {
		rm.persistWallet(r, s)
	}
	delete(rm.seats, p.AccountID)
	for i, id := range rm.order {
		if id == p.AccountID {
			rm.order = append(rm.order[:i], rm.order[i+1:]...)
			break
		}
	}
	// If the player who just left was the one on turn, advance the table.
	if rm.phase == phTurns && active != nil && active.p.AccountID == p.AccountID {
		rm.beginTurn(r)
	}
	rm.render(r)
}

// --- the wake heartbeat ----------------------------------------------------

// OnWake advances every time-driven element against CallContext time, then
// renders. It clears a finished animation, lands the active phase one-shot when
// its deadline has passed, and (for settlement) waits on the reveal schedule —
// all the native engine timers, re-expressed as deadline comparisons.
func (rm *room) OnWake(r kit.Room) {
	now := r.Now()
	rm.lastNow = now

	// Drop a fully-played animation schedule so the renderer stops consulting it.
	if len(rm.sched) > 0 && !rm.schedEnd.IsZero() && !now.Before(rm.schedEnd) {
		rm.sched = nil
		rm.schedEnd = time.Time{}
	}

	// Land the armed one-shot if its deadline has passed. Each branch may re-arm
	// `what` (e.g. beginTurn after an auto-stand), so re-read after handling.
	if rm.what != pendNone && now.After(rm.pendAt) {
		switch rm.what {
		case pendBettingClose:
			rm.what = pendNone
			rm.onBettingClose(r)
		case pendInsurance:
			rm.what = pendNone
			rm.resolveInsurance(r)
		case pendTurn:
			rm.what = pendNone
			rm.autoStand(r)
		case pendResults:
			rm.what = pendNone
			rm.enterBetting(r)
		case pendSettle:
			rm.what = pendNone
			rm.settle(r)
		}
	}
	rm.render(r)
}

// arm sets the active deferred one-shot (deadline checked in OnWake).
func (rm *room) arm(what pending, at time.Time) {
	rm.what = what
	rm.pendAt = at
}

// --- betting ---------------------------------------------------------------

func (rm *room) enterBetting(r kit.Room) {
	rm.phase = phBetting
	rm.dealer = nil
	rm.dealerHole = false
	rm.bettingClosing = false
	rm.clearSchedule()
	for _, s := range rm.seats {
		s.hands = nil
		s.placed = false
		s.insurance = 0
		s.insuranceDecided = false
		s.result = ""
		if s.bet > s.chips {
			s.bet = clampBet(s.chips)
		}
	}
	rm.deadline = r.Now().Add(bettingDur)
	r.SetInputContext(kit.CtxNav) // bet up/down + confirm
	rm.arm(pendBettingClose, rm.deadline)
}

func (rm *room) onBettingClose(r kit.Room) {
	if rm.anyPlaced() {
		rm.deal(r)
		return
	}
	rm.enterBetting(r) // nobody bet — reopen
}

func (rm *room) anyPlaced() bool {
	for _, s := range rm.seats {
		if s.placed {
			return true
		}
	}
	return false
}

// allSeatedPlaced reports whether at least one seat is taken and every seated
// player has placed a bet — the trigger to deal early after a short grace beat.
func (rm *room) allSeatedPlaced() bool {
	seated := false
	for _, s := range rm.seats {
		seated = true
		if !s.placed {
			return false
		}
	}
	return seated
}

// maybeCloseEarly arms the grace timer once every seated player has placed. It
// re-points the betting-close one-shot at a short grace deadline; a guard
// (bettingClosing) keeps a second confirm during the grace beat from re-arming
// and pushing the deal out. The empty-betting reopen path in onBettingClose is
// untouched (it always re-checks anyPlaced).
func (rm *room) maybeCloseEarly(r kit.Room) {
	if rm.bettingClosing || !rm.allSeatedPlaced() {
		return
	}
	rm.bettingClosing = true
	rm.deadline = r.Now().Add(gracePeriod)
	rm.arm(pendBettingClose, rm.deadline)
}

// clampBet returns the highest tier the chips can cover (at least the lowest).
func clampBet(chips int) int {
	best := betTiers[0]
	for _, t := range betTiers {
		if t <= chips {
			best = t
		}
	}
	return best
}

func (rm *room) adjustBet(s *seat, dir int) {
	i := tierIndex(s.bet) + dir
	if i < 0 {
		i = 0
	}
	if i >= len(betTiers) {
		i = len(betTiers) - 1
	}
	s.bet = betTiers[i]
	if s.bet > s.chips {
		s.bet = clampBet(s.chips)
	}
}

func tierIndex(bet int) int {
	for i, t := range betTiers {
		if t == bet {
			return i
		}
	}
	return 0
}

// --- dealing ---------------------------------------------------------------

func (rm *room) deal(r kit.Room) {
	if rm.sh.needsReshuffle() {
		rm.sh.shuffle(r.Rand())
	}
	rm.dealer = hand{rm.sh.draw(), rm.sh.draw()} // [up, hole]
	rm.dealerHole = true
	// Range the join-ordered slice (not the map) so dealing order is
	// deterministic — never depends on Go's map iteration order.
	for _, id := range rm.order {
		s := rm.seats[id]
		if s == nil || !s.placed {
			continue
		}
		s.chips -= s.bet
		h := &phand{cards: hand{rm.sh.draw(), rm.sh.draw()}, bet: s.bet}
		if h.cards.isBlackjack() {
			h.resolved = true
		}
		s.hands = []*phand{h}
	}

	rm.recordDeal(r)

	up := rm.dealer[0]
	switch {
	case up.r == rankAce:
		rm.enterInsurance(r)
	case up.r.points() == 10:
		if rm.dealer.isBlackjack() {
			rm.revealAndSettle(r)
		} else {
			rm.enterTurns(r)
		}
	default:
		rm.enterTurns(r)
	}
}

// --- animation schedule ----------------------------------------------------

// clearSchedule drops any pending card animation. Safe to call when none is
// active. (No frame-rate control exists in the ABI: the host heartbeat drives
// wakes; the schedule is read off room-clock timestamps either way.)
func (rm *room) clearSchedule() {
	rm.sched = nil
	rm.schedEnd = time.Time{}
}

// dealingActive reports whether a dealing/reveal animation is still in flight at
// the latest composed instant, so hand-action input can be ignored until it ends
// (betting-phase input is unaffected). It reads only authoritative timestamps.
func (rm *room) dealingActive() bool {
	if len(rm.sched) == 0 || rm.schedEnd.IsZero() || rm.lastNow.IsZero() {
		return false
	}
	return rm.lastNow.Before(rm.schedEnd)
}

// computeSchedEnd recomputes schedEnd as the latest settle instant across the
// recorded schedule.
func (rm *room) computeSchedEnd() {
	if len(rm.sched) == 0 {
		rm.schedEnd = time.Time{}
		return
	}
	rm.schedEnd = rm.sched[0].endsAt()
	for _, a := range rm.sched[1:] {
		if e := a.endsAt(); e.After(rm.schedEnd) {
			rm.schedEnd = e
		}
	}
}

// recordDeal lays out the initial two-card deal as a staggered slide-and-flip
// sweep: each card slides from the right felt edge to its slot and then flips
// face up, except the dealer hole card, which slides in but stays concealed
// until the reveal turns it over. Card identities are already fixed; this only
// records cosmetic timings.
func (rm *room) recordDeal(r kit.Room) {
	now := r.Now()
	rm.sched = nil
	step := 0
	add := func(a cardAnim) {
		a.slideStart = now.Add(time.Duration(step) * dealStagger)
		if !a.flipStart.IsZero() { // flip begins as the slide lands
			a.flipStart = a.slideStart.Add(slideDur)
		}
		rm.sched = append(rm.sched, a)
		step++
	}
	// Two passes around the table mirror a real deal: first card to every seat
	// then the dealer up card; second card to every seat then the dealer hole.
	for round := 0; round < 2; round++ {
		for _, id := range rm.order {
			s := rm.seats[id]
			if s == nil || !s.placed || len(s.hands) == 0 {
				continue
			}
			add(cardAnim{kind: animSeat, player: s.p, cardIdx: round, flipStart: now})
		}
		if round == 0 {
			add(cardAnim{kind: animDealer, cardIdx: 0, flipStart: now}) // up card flips
		} else {
			add(cardAnim{kind: animDealer, cardIdx: 1}) // hole card stays face down
		}
	}
	rm.computeSchedEnd()
}

// recordDraw schedules a single drawn card (hit/double/split) for the given seat
// hand: it slides in from the right edge and flips face up.
func (rm *room) recordDraw(r kit.Room, p kit.Player, handIdx, cardIdx int) {
	now := r.Now()
	rm.sched = []cardAnim{{
		kind:       animSeat,
		player:     p,
		handIdx:    handIdx,
		cardIdx:    cardIdx,
		slideStart: now,
		flipStart:  now.Add(slideDur),
	}}
	rm.computeSchedEnd()
}

// recordHoleReveal schedules the dealer hole card flipping over in place (no
// slide) when the dealer turns it up, and returns the room-clock instant the
// flip completes so dealer play can pause one beat for it.
func (rm *room) recordHoleReveal(r kit.Room) time.Time {
	now := r.Now()
	rm.sched = []cardAnim{{
		kind:      animDealer,
		cardIdx:   1,
		flipStart: now,
	}}
	rm.computeSchedEnd()
	return now.Add(flipDur)
}

// recordDealerDraw appends a dealer hit card sliding in and flipping face up.
func (rm *room) recordDealerDraw(start time.Time, cardIdx int) {
	rm.sched = append(rm.sched, cardAnim{
		kind:       animDealer,
		cardIdx:    cardIdx,
		slideStart: start,
		flipStart:  start.Add(slideDur),
	})
}

// --- insurance -------------------------------------------------------------

func (rm *room) enterInsurance(r kit.Room) {
	rm.phase = phInsurance
	rm.deadline = r.Now().Add(insuranceDur)
	r.SetInputContext(kit.CtxCommand) // y/n are domain commands
	rm.arm(pendInsurance, rm.deadline)
}

func (rm *room) takeInsurance(s *seat, yes bool) {
	if s == nil || !s.placed || s.insuranceDecided {
		return
	}
	s.insuranceDecided = true
	if yes {
		ins := s.bet / 2
		if ins > s.chips {
			ins = s.chips
		}
		s.chips -= ins
		s.insurance = ins
	}
}

func (rm *room) resolveInsurance(r kit.Room) {
	dbj := rm.dealer.isBlackjack()
	// Order the credit loop by join order for determinism (chips are
	// per-seat so order is harmless, but range-over-map is avoided on principle).
	for _, id := range rm.order {
		s := rm.seats[id]
		if s == nil || s.insurance <= 0 {
			continue
		}
		s.chips += insuranceCredit(dbj, s.insurance)
	}
	if dbj {
		rm.revealAndSettle(r)
		return
	}
	rm.enterTurns(r)
}

// --- player turns ----------------------------------------------------------

func (rm *room) enterTurns(r kit.Room) {
	rm.phase = phTurns
	rm.beginTurn(r)
}

// firstUnresolved returns the seat and hand currently on turn (the first
// unresolved hand of the first placed seat, in join order), or nil/nil.
func (rm *room) firstUnresolved() (*seat, *phand) {
	for _, id := range rm.order {
		s := rm.seats[id]
		if s == nil || !s.placed {
			continue
		}
		for _, h := range s.hands {
			if !h.resolved {
				return s, h
			}
		}
	}
	return nil, nil
}

func (rm *room) beginTurn(r kit.Room) {
	s, _ := rm.firstUnresolved()
	if s == nil {
		rm.enterDealer(r)
		return
	}
	rm.deadline = r.Now().Add(turnDur)
	r.SetInputContext(kit.CtxCommand) // h/s/d/p/r are domain commands
	rm.arm(pendTurn, rm.deadline)
}

func (rm *room) autoStand(r kit.Room) {
	if rm.phase != phTurns {
		return
	}
	if _, h := rm.firstUnresolved(); h != nil {
		h.resolved = true
	}
	rm.beginTurn(r)
}

func (rm *room) act(r kit.Room, p kit.Player, a rune) {
	s, h := rm.firstUnresolved()
	if s == nil || s.p.AccountID != p.AccountID {
		return // not this player's turn
	}
	hi := rm.handIndex(s, h)
	first := len(h.cards) == 2 && !h.doubled // first decision on this hand
	switch a {
	case 'h':
		h.cards = append(h.cards, rm.sh.draw())
		rm.recordDraw(r, p, hi, len(h.cards)-1)
		if h.cards.isBust() || h.cards.total() == 21 {
			h.resolved = true
		}
		rm.beginTurn(r)
	case 's':
		h.resolved = true
		rm.beginTurn(r)
	case 'd':
		if first && s.chips >= h.bet {
			s.chips -= h.bet
			h.bet *= 2
			h.doubled = true
			h.cards = append(h.cards, rm.sh.draw())
			rm.recordDraw(r, p, hi, len(h.cards)-1)
			h.resolved = true
		}
		rm.beginTurn(r)
	case 'p':
		rm.split(r, s, h)
		rm.beginTurn(r)
	case 'r':
		if first && len(s.hands) == 1 {
			h.surrendered = true
			h.resolved = true
			s.chips += h.bet / 2 // return half; the bet was deducted at deal
		}
		rm.beginTurn(r)
	}
}

// split turns a two-card equal-rank pair into two hands, each taking a new card.
// Split aces take one card and stand.
func (rm *room) split(r kit.Room, s *seat, h *phand) {
	if len(h.cards) != 2 || h.cards[0].r != h.cards[1].r || s.chips < h.bet || len(s.hands) >= maxHands {
		return
	}
	c0, c1 := h.cards[0], h.cards[1]
	s.chips -= h.bet
	nh := &phand{cards: hand{c1, rm.sh.draw()}, bet: h.bet, fromSplit: true}
	h.cards = hand{c0, rm.sh.draw()}
	h.fromSplit = true
	if c0.r == rankAce {
		h.resolved = true
		nh.resolved = true
	}
	// insert nh directly after h
	idx := 0
	for i, x := range s.hands {
		if x == h {
			idx = i
			break
		}
	}
	s.hands = append(s.hands[:idx+1], append([]*phand{nh}, s.hands[idx+1:]...)...)
	// Animate the freshly drawn card of each split hand sliding in.
	rm.sched = nil
	now := r.Now()
	for i := 0; i < 2; i++ {
		rm.sched = append(rm.sched, cardAnim{
			kind:       animSeat,
			player:     s.p,
			handIdx:    idx + i,
			cardIdx:    1,
			slideStart: now.Add(time.Duration(i) * dealStagger),
			flipStart:  now.Add(time.Duration(i)*dealStagger + slideDur),
		})
	}
	rm.computeSchedEnd()
}

// handIndex returns h's position within s.hands (0 if not found).
func (rm *room) handIndex(s *seat, h *phand) int {
	for i, x := range s.hands {
		if x == h {
			return i
		}
	}
	return 0
}

// --- dealer & settlement ---------------------------------------------------

func (rm *room) revealAndSettle(r kit.Room) {
	rm.dealerHole = false
	// Flip the hole card over, then settle once the flip has played out.
	done := rm.recordHoleReveal(r)
	rm.settleAt(r, done)
}

func (rm *room) enterDealer(r kit.Room) {
	rm.dealerHole = false
	// Outcome is determined up front from the seeded shoe; the schedule only
	// paces the reveal. Flip the hole card (one beat), then slide in each card
	// the dealer draws, and settle once the last one lands.
	done := rm.recordHoleReveal(r)
	if rm.anyLive() {
		before := len(rm.dealer)
		rm.dealer = dealerPlay(rm.dealer, rm.sh)
		start := done.Add(holePause - flipDur) // a beat after the flip finishes
		for i := before; i < len(rm.dealer); i++ {
			rm.recordDealerDraw(start, i)
			start = start.Add(dealStagger)
		}
		rm.computeSchedEnd() // across the appended dealer cards
		if len(rm.dealer) > before {
			done = rm.schedEnd
		}
	}
	rm.settleAt(r, done)
}

// settleAt defers settlement until the dealer reveal/draw schedule has played
// out (settlement timing keys off the schedule, never the renderer). A deadline
// already in the past settles immediately.
func (rm *room) settleAt(r kit.Room, at time.Time) {
	if !at.After(r.Now()) {
		rm.settle(r)
		return
	}
	rm.deadline = at
	rm.arm(pendSettle, at)
}

// anyLive reports whether any player hand can still beat the dealer.
func (rm *room) anyLive() bool {
	for _, id := range rm.order {
		s := rm.seats[id]
		if s == nil {
			continue
		}
		for _, h := range s.hands {
			if !h.surrendered && !h.cards.isBust() {
				return true
			}
		}
	}
	return false
}

func (rm *room) settle(r kit.Room) {
	dbj := rm.dealer.isBlackjack()
	for _, id := range rm.order {
		s := rm.seats[id]
		if s == nil || !s.placed {
			continue
		}
		net := 0
		for _, h := range s.hands {
			if h.surrendered {
				net -= h.bet - h.bet/2 // lost half
				continue
			}
			pbj := h.cards.isBlackjack() && !h.fromSplit
			o := settleHandEx(h.cards, pbj, rm.dealer, dbj)
			credit := creditFor(o, h.bet)
			s.chips += credit
			net += credit - h.bet
		}
		s.result = resultText(net)
		if s.chips <= 0 {
			s.chips = rebuyChips
			s.result = "BUST - re-buy"
		}
		if s.chips > s.highScore {
			s.highScore = s.chips
		}
		// Persist the durable wallet after each settled round, then feed the
		// board on a new peak. Synchronous (no goroutines in wasm; KV is
		// host-bounded).
		rm.persistWallet(r, s)
		rm.postPeak(r, s)
		s.placed = false
	}
	rm.enterResults(r)
}

func (rm *room) enterResults(r kit.Room) {
	rm.phase = phResults
	rm.deadline = r.Now().Add(resultsDur)
	r.SetInputContext(kit.CtxNav) // results screen: no domain letters, q/Esc backs out
	rm.arm(pendResults, rm.deadline)
}

// --- input -----------------------------------------------------------------

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	s := rm.seats[p.AccountID]
	if s == nil {
		return
	}
	switch rm.phase {
	case phBetting:
		switch kit.Resolve(in, kit.CtxNav) {
		case kit.ActUp:
			rm.adjustBet(s, +1)
		case kit.ActDown:
			rm.adjustBet(s, -1)
		case kit.ActConfirm:
			if s.chips >= betTiers[0] {
				if s.bet > s.chips {
					s.bet = clampBet(s.chips)
				}
				s.placed = true
				rm.maybeCloseEarly(r) // deal early once every seat has bet
			}
		}
	case phInsurance:
		if in.Kind == kit.InputRune {
			switch in.Rune {
			case 'y', 'Y':
				rm.takeInsurance(s, true)
			case 'n', 'N':
				rm.takeInsurance(s, false)
			}
		}
	case phTurns:
		// A hand-action key is ignored while a dealing/draw animation is in
		// flight, so a card can't be acted on before it has landed (the
		// betting-phase keys above are unaffected).
		if rm.dealingActive() {
			return
		}
		if in.Kind == kit.InputRune {
			switch in.Rune {
			case 'h', 'H':
				rm.act(r, p, 'h')
			case 's', 'S':
				rm.act(r, p, 's')
			case 'd', 'D':
				rm.act(r, p, 'd')
			case 'p', 'P':
				rm.act(r, p, 'p')
			case 'r', 'R':
				rm.act(r, p, 'r')
			}
		}
	}
	rm.render(r)
}

func resultText(net int) string {
	switch {
	case net > 0:
		return "WIN +" + strconv.Itoa(net)
	case net < 0:
		return "LOSE " + strconv.Itoa(net)
	default:
		return "PUSH"
	}
}
