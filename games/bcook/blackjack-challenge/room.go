package main

import (
	"sort"
	"strconv"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// phases of one round. These are INTERNAL state-machine markers held in guest
// memory — the lean ABI has no phase surface (SetPhase is gone; joinability is
// host-derived), so a phase only drives this game's own logic and rendering.
const (
	phBetting = "betting"
	phTurns   = "player turns"
	phResults = "results"
)

const (
	maxHands = 3 // split up to twice per seat, forming three hands (table rule)

	bettingDur = 15 * time.Second
	turnDur    = 20 * time.Second
	resultsDur = 6 * time.Second

	// gracePeriod is the short beat between the last seated player placing a bet
	// and dealing, so an all-bets-in table deals early without feeling abrupt.
	gracePeriod = time.Second

	// maxPayoutMult mirrors Meta().MaxPayoutMultiplier: the host clamps every
	// Settle to the seat's open stake × this multiplier, so the game clamps its
	// own displayed/settled gross to the same ceiling before Settle. 31 covers a
	// Star Pairs pair of aces (30:1 -> stake×31); the mandatory main bet only
	// dilutes the per-seat aggregate, so it is never reached in practice.
	maxPayoutMult = 31
)

// betTiers are the selectable stakes, lowest first. The ×2.5/×2/×2 climb repeats
// each decade (10→100, 100→1000, 1000→10000) so the ladder runs from a 10-chip
// minimum up to a 10k high roller.
var betTiers = []int{10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

// pairsTiers are the Star Pairs / behind side-bet stakes, lowest first; index
// 0 is "off" (no side bet). Adjusted on the Left/Right axis during betting, and
// mirrors betTiers' upper reaches so side action can scale with the main bet.
var pairsTiers = []int{0, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

// phand is one hand a seat plays (a seat holds more than one after a split).
type phand struct {
	cards    hand
	bet      int
	resolved bool // stood / busted / blackjack / doubled / auto-won
	doubled  bool
	autoWon  bool // Player 21 or Five Card Trick: instant even-money win
	// bjMult is the X in this blackjack's X:1 ranked payout (blackjackMult),
	// 0 until settle fixes it. It does not change the payout math — grossMult
	// computes that independently — it only lets the felt name the paid tier
	// (valueLabel appends " X:1" to "BJ" once this is set).
	bjMult int
}

// backBet is one seat's wager ON ANOTHER seat: a "behind" bet that rides the
// backed player's first hand vs the dealer, and/or a Star Pairs bet on the
// backed player's first two cards. Stakes are chosen during betting; the result
// fields are filled at settlement (their-pairs at the deal, behind at settle).
type backBet struct {
	behind    int    // behind-bet stake (0 = none)
	pairs     int    // their-Star-Pairs stake (0 = none)
	pairsKind string // resolved at deal: "" | "mixed" | "colored" | "perfect" | "aces"
	pairsWin  int    // their-pairs chips credited at deal (0 = lost/none)
	behindWin int    // behind chips credited at settle (0 = lost)
}

// seat is one player's place at the table. Keyed by account id in the room map
// so it survives a hibernation freeze/thaw (connections change; accounts don't).
type seat struct {
	p kit.Player
	// bal is the cached account-wide credits balance (svc.Credits.Balance),
	// refreshed at join, after each Wager (decremented), and after each Settle /
	// Buyback (re-read). It drives the HUD and every betting-affordability check;
	// the platform owns the authoritative balance — this is only a per-tick cache
	// so the render/clamp paths never call Balance in a hot loop.
	bal        int
	highScore  int // peak balance observed this session (the leaderboard metric)
	postedPeak int // last peak Posted to the board (post only on increase)
	// grossThisRound accumulates the seat's single open stake's GROSS payout
	// (stake-inclusive): side-bet wins fold in as they resolve, hand credits at
	// settle, and the whole thing Settles exactly once. roundStake is the total
	// escrowed onto that open stake (sum of every Wager this round); >0 marks an
	// open, unsettled stake and bounds the payout ceiling (roundStake×maxPayoutMult).
	grossThisRound int64
	roundStake     int64
	bet            int // currently selected/placed stake
	placed         bool
	pairsBet       int                 // Star Pairs side stake (0 = off), carried between rounds like bet
	pairsKind      string              // this round's pairs result: "" | "mixed" | "colored" | "perfect" | "aces"
	pairsWin       int                 // chips credited on the pairs side bet this round (0 = lost/none)
	focus          string              // betting UI: "" edits own bet, else the account id whose backs are being edited
	backs          map[string]*backBet // wagers on other seats, keyed by target account id (iterate via rm.order)
	hands          []*phand
	joinOrder      int
	result         string // settlement summary for the results phase
	ready          bool   // readied up during results to skip the wait
}

// pending names the deferred one-shot the room is waiting on, replacing the
// native engine timers: each is a deadline held in guest memory and landed in
// OnWake when r.Now() passes it (the wake idiom — no host timer survives a thaw).
type pending uint8

const (
	pendNone         pending = iota
	pendBettingClose         // betting window closed (or grace beat elapsed)
	pendTurn                 // active turn timed out -> auto-stand
	pendResults              // results flash elapsed -> next round
	pendSettle               // dealer reveal/draw animation done -> settle
)

type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	sh      *shoe
	phase   string
	seats   map[string]*seat // keyed by account id (hibernation-safe)
	order   []string         // join order of account ids
	dealer  hand
	joinSeq int

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

// economyOff reports whether the host has no credits economy (svc.Credits is
// nil). A casino game must render out-of-service rather than trap when the
// economy is disabled — every money path checks this first.
func (rm *room) economyOff() bool { return rm.svc.Credits == nil }

func (rm *room) OnStart(r kit.Room) {
	rm.sh = newShoe(r.Rand())
	if rm.economyOff() {
		rm.render(r) // out-of-service: no economy, no betting
		return
	}
	rm.enterBetting(r)
	rm.render(r)
}

func (rm *room) OnClose(r kit.Room) {
	// Book every seat's open stake on the way out so no escrow leaks: a seat with
	// an unsettled stake at close Settles its accumulated gross (forfeiting any
	// still-unresolved hand as a loss). Synchronous — the wasm sandbox has no
	// goroutines and the host bounds the call.
	for _, id := range rm.order {
		if s := rm.seats[id]; s != nil {
			rm.settleOpenStake(s)
		}
	}
}

// --- platform credits (the account-wide casino bankroll) -------------------

// wager escrows amt onto the seat's SINGLE open stake via svc.Credits, updating
// the cached balance and the round's total stake. A non-positive amt is a no-op
// success (nothing to escrow). Returns false — the caller MUST reject the action
// and never proceed — when the economy is off or the balance can't cover it
// (mirrors the old `chips < bet` guards). Wager accumulates: repeated calls
// before one Settle all land on the same open stake.
func (rm *room) wager(s *seat, amt int) bool {
	if amt <= 0 {
		return true
	}
	if rm.economyOff() {
		return false
	}
	if err := rm.svc.Credits.Wager(s.p, int64(amt)); err != nil {
		return false // ErrInsufficientCredits (or disabled): reject cleanly
	}
	s.roundStake += int64(amt)
	s.bal -= amt
	return true
}

// refreshBal re-reads the seat's authoritative account-wide balance into the
// cache (after a Settle or Buyback changes it). A disabled/erroring economy
// leaves the cache untouched.
func (rm *room) refreshBal(s *seat) {
	if rm.economyOff() {
		return
	}
	if b, err := rm.svc.Credits.Balance(s.p); err == nil {
		s.bal = int(b)
	}
}

// settleOpenStake closes the seat's single open stake EXACTLY ONCE: it clamps
// the accumulated gross to the payout ceiling (roundStake×maxPayoutMult) and
// Settles it, then clears the round's stake/gross and re-reads the balance.
// Returns the round's net chip change (gross-stake) for the results summary. A
// no-op with no open stake (roundStake==0), so it is safe on the mid-round exit
// paths (OnLeave/OnClose) — a committed-but-unresolved bet Settles its resolved
// gross (0 for a bare main bet), never leaking escrow and never double-settling.
func (rm *room) settleOpenStake(s *seat) int {
	if s.roundStake <= 0 {
		s.grossThisRound = 0
		return 0
	}
	gross := s.grossThisRound
	if lim := s.roundStake * maxPayoutMult; gross > lim {
		gross = lim // never settle (or show) more than the host will pay
	}
	net := int(gross - s.roundStake)
	if !rm.economyOff() {
		_ = rm.svc.Credits.Settle(s.p, gross)
	}
	s.grossThisRound = 0
	s.roundStake = 0
	rm.refreshBal(s)
	return net
}

// buyback triggers the platform broke-relief rebuy for a busted seat, updating
// the cached balance on success. Returns false when the host refuses (solvent or
// the daily rebuy limit reached — render it, do not retry) or the economy is off.
func (rm *room) buyback(s *seat) bool {
	if rm.economyOff() {
		return false
	}
	nb, err := rm.svc.Credits.Buyback(s.p)
	if err != nil {
		return false
	}
	s.bal = int(nb)
	if s.bal > s.highScore {
		s.highScore = s.bal
	}
	return true
}

// postPeak feeds the declared leaderboard with a new personal peak (the board
// keeps each account's best). highScore tracks the peak account-wide credits
// balance observed this session (sourced from svc.Credits.Balance after each
// Settle); Post is what reaches the board, only on a new personal best.
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
	if s, ok := rm.seats[p.AccountID]; ok {
		// A re-delivered join is a fresh kit.Player (new connection): adopt
		// it so the seat renders the current handle and character tile.
		s.p = p
		rm.render(r)
		return
	}
	s := &seat{p: p, bet: betTiers[1], joinOrder: rm.joinSeq}
	rm.refreshBal(s) // seed the cached balance from the platform bankroll
	s.highScore = s.bal
	s.postedPeak = s.bal
	rm.seats[p.AccountID] = s
	rm.joinSeq++
	rm.order = append(rm.order, p.AccountID)
	rm.render(r)
}

// OnLeave books any open stake and frees the seat. Leaving mid-round Settles the
// seat's single open stake exactly once with its accumulated gross (0 for a bare,
// still-unresolved main bet) — abandoning a dealt hand forfeits it as a loss, as
// at a real table, and the escrow is released rather than leaked.
func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	active, _ := rm.firstUnresolved()
	if s := rm.seats[p.AccountID]; s != nil {
		rm.settleOpenStake(s)
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
	// If the leaver was the last seat the table was waiting on to ready up, the
	// remaining players are all ready — deal the next hand now.
	if rm.phase == phResults && rm.allSeatedReady() {
		rm.enterBetting(r)
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
	rm.bettingClosing = false
	rm.clearSchedule()
	for _, s := range rm.seats {
		s.hands = nil
		s.placed = false
		s.result = ""
		s.pairsKind = ""
		s.pairsWin = 0
		s.focus = ""         // re-open editing on the seat's own bet
		s.grossThisRound = 0 // no open stake carries into a fresh betting window
		s.roundStake = 0     //
		rm.refreshBal(s)     // re-sync the cached balance before affordability clamps
		rm.carryBacks(s)     // behind/their-pairs stakes are sticky between rounds
		if s.bet > s.bal {
			s.bet = clampBet(s.bal)
		}
		rm.clampPairs(s) // a thinned stack may no longer afford the carried side bet
		rm.clampBacks(s) // nor its carried backs
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
	if s.bet > s.bal {
		s.bet = clampBet(s.bal)
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

// cycleOwnPairs advances the seat's own Star Pairs stake one tier, wrapping
// back to 0 past the top (and resetting to 0 if the next tier is unaffordable).
func (rm *room) cycleOwnPairs(s *seat) {
	s.pairsBet = loopTier(pairsTiers, s.pairsBet, s.bal-(s.committed()-s.pairsBet))
}

// clampPairs lowers the side bet to the highest tier the seat can still afford
// alongside its main bet (down to off), so a raised main bet or a thin stack can
// never leave an unaffordable side bet placed.
func (rm *room) clampPairs(s *seat) {
	for s.pairsBet > 0 && s.bet+s.pairsBet > s.bal {
		s.pairsBet = pairsTiers[pairsTierIndex(s.pairsBet)-1]
	}
}

// carryBacks makes a seat's backs sticky between rounds: it prunes any back on a
// target that has left the table (keying by account id, so a still-seated target
// is kept even if it sits the round out — resolveBackPairs voids that round's
// stakes) and clears last round's per-back result fields so they re-settle fresh.
func (rm *room) carryBacks(s *seat) {
	for tid, b := range s.backs {
		if rm.seats[tid] == nil {
			delete(s.backs, tid)
			continue
		}
		b.pairsKind, b.pairsWin, b.behindWin = "", 0, 0
	}
}

// clampBacks lowers each carried back's stakes to the highest tiers the seat can
// still afford alongside its main bet, own pairs, and the other backs (down to
// off). Visits backs in a deterministic order so the clamp never depends on Go's
// map iteration order.
func (rm *room) clampBacks(s *seat) {
	for _, tid := range sortedBackIDs(s) {
		b := s.backs[tid]
		b.pairs = affordTier(pairsTiers, b.pairs, s.bal-(s.committed()-b.pairs))
		b.behind = affordTier(pairsTiers, b.behind, s.bal-(s.committed()-b.behind))
	}
}

// affordTier returns the highest tier not exceeding `want` that still fits
// `budget` (the chips left for this stake after the seat's other commitments).
// tiers must be ascending and start at 0, so a zero/negative budget yields the
// "off" tier rather than panicking.
func affordTier(tiers []int, want, budget int) int {
	best := 0
	for _, t := range tiers {
		if t > want || t > budget {
			break
		}
		best = t
	}
	return best
}

func pairsTierIndex(bet int) int {
	for i, t := range pairsTiers {
		if t == bet {
			return i
		}
	}
	return 0
}

// committed totals every chip the seat has wagered this betting window: its main
// bet, its own Star Pairs, and every behind/their-pairs stake across backs.
func (s *seat) committed() int {
	total := s.bet + s.pairsBet
	for _, b := range s.backs {
		total += b.behind + b.pairs
	}
	return total
}

// backTargets is the account ids a seat may back: every OTHER occupied seat, in
// join order (never map order).
func (rm *room) backTargets(self *seat) []string {
	var ids []string
	for _, id := range rm.order {
		if rm.seats[id] != nil && id != self.p.AccountID {
			ids = append(ids, id)
		}
	}
	return ids
}

// cycleFocus moves the seat's betting focus `dir` steps (Right = +1, Left = -1)
// around ["" (self), t1, t2, …] where t1… are the other occupied seats, wrapping
// at both ends. With no other seats it stays on self.
func (rm *room) cycleFocus(s *seat, dir int) {
	targets := rm.backTargets(s)
	if len(targets) == 0 {
		s.focus = ""
		return
	}
	list := append([]string{""}, targets...) // index 0 = self
	cur := 0
	for i, id := range list {
		if id == s.focus {
			cur = i
			break
		}
	}
	n := len(list)
	s.focus = list[((cur+dir)%n+n)%n]
}

// backOn returns the seat's backBet on target (creating it on first use).
func (s *seat) backOn(target string) *backBet {
	if s.backs == nil {
		s.backs = map[string]*backBet{}
	}
	b := s.backs[target]
	if b == nil {
		b = &backBet{}
		s.backs[target] = b
	}
	return b
}

// cycleBackPairs / cycleBackBehind advance the focused back's pairs / behind
// stake one tier, wrapping to 0 past the top, budget-clamped against the seat's
// other commitments (an unaffordable next tier also resets to 0).
func (rm *room) cycleBackPairs(s *seat) {
	b := s.backOn(s.focus)
	b.pairs = loopTier(pairsTiers, b.pairs, s.bal-(s.committed()-b.pairs))
}

func (rm *room) cycleBackBehind(s *seat) {
	b := s.backOn(s.focus)
	b.behind = loopTier(pairsTiers, b.behind, s.bal-(s.committed()-b.behind))
}

// loopTier returns the tier one step above cur, wrapping back to 0 (the "off"
// tier) past the top — so a side bet cycles 0 -> 10 -> … -> top -> 0. A next tier
// the budget can't cover also resets to 0 (higher tiers are unaffordable too).
func loopTier(tiers []int, cur, budget int) int {
	next := tiers[(pairsTierIndex(cur)+1)%len(tiers)]
	if next > budget {
		return 0
	}
	return next
}

// --- dealing ---------------------------------------------------------------

func (rm *room) deal(r kit.Room) {
	if rm.sh.needsReshuffle() {
		rm.sh.shuffle(r.Rand())
	}
	rm.sh.beginRound() // everything dealt before this point is recyclable discards
	rng := r.Rand()
	rm.dealer = hand{rm.sh.draw(rng)} // ONE face-up card; the rest draw at the dealer's turn
	// Range the join-ordered slice (not the map) so dealing order is
	// deterministic — never depends on Go's map iteration order.
	for _, id := range rm.order {
		s := rm.seats[id]
		if s == nil || !s.placed {
			continue
		}
		// Open the seat's single stake with the main bet. A failed Wager
		// (balance can't cover it — should not happen after the betting clamps)
		// sits the seat out this round rather than dealing an unbacked hand.
		if !rm.wager(s, s.bet) {
			s.placed = false
			continue
		}
		h := &phand{cards: hand{rm.sh.draw(rng), rm.sh.draw(rng)}, bet: s.bet}
		if h.cards.isBlackjack() {
			h.resolved = true
		}
		s.hands = []*phand{h}
		rm.resolvePairs(s, h.cards)
	}

	rm.resolveBackPairs() // every hand is now dealt; settle the their-pairs side of each back

	rm.recordDeal(r)

	rm.enterTurns(r)
}

// resolvePairs settles a seat's Star Pairs side bet against its dealt cards:
// the stake is Wagered onto the seat's open stake and any winning pair folds its
// gross into grossThisRound immediately — the casino way, where the side bet
// stands apart from how the hand goes on to play out. A stake the balance can't
// cover is dropped (no side bet this round) rather than proceeding unpaid.
func (rm *room) resolvePairs(s *seat, dealt hand) {
	if s.pairsBet <= 0 || len(dealt) < 2 {
		return
	}
	if !rm.wager(s, s.pairsBet) {
		s.pairsBet = 0
		return
	}
	kind, mult := starPairsOutcome(dealt[0], dealt[1])
	s.pairsKind = kind
	s.pairsWin = pairsCreditFor(mult, s.pairsBet)
	s.grossThisRound += int64(s.pairsWin)
}

// sortedBackIDs returns a seat's back target ids in a deterministic (sorted)
// order. Backs are keyed by account id in a map; the targets are independent
// (settlement is purely additive), so a stable sort is enough to avoid relying
// on Go's map iteration order — and it still visits a target that has since left
// the table (which join order no longer lists).
func sortedBackIDs(s *seat) []string {
	if len(s.backs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(s.backs))
	for id := range s.backs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// resolveBackPairs settles the their-Star-Pairs side of every seat's backs,
// once all hands are dealt: a back on a seat that didn't get dealt in is voided
// (never charged); otherwise each stake is Wagered onto the backer's open stake
// and any winning pair on the target's first two cards folds its gross into the
// backer's grossThisRound immediately. The behind bet is Wagered here too
// (committed, held) and settled later against the dealer. A stake the balance
// can't cover is dropped (voided) rather than proceeding unpaid.
func (rm *room) resolveBackPairs() {
	for _, id := range rm.order {
		s := rm.seats[id]
		if s == nil || !s.placed {
			continue
		}
		for _, tid := range sortedBackIDs(s) {
			b := s.backs[tid]
			t := rm.seats[tid]
			if t == nil || !t.placed || len(t.hands) == 0 {
				b.behind, b.pairs = 0, 0 // target sat out / gone -> void the back
				continue
			}
			if b.pairs > 0 {
				if rm.wager(s, b.pairs) {
					kind, mult := starPairsOutcome(t.hands[0].cards[0], t.hands[0].cards[1])
					b.pairsKind = kind
					b.pairsWin = pairsCreditFor(mult, b.pairs)
					s.grossThisRound += int64(b.pairsWin)
				} else {
					b.pairs = 0
				}
			}
			if b.behind > 0 && !rm.wager(s, b.behind) {
				b.behind = 0 // committed now; the dealer comparison happens at settle
			}
		}
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

// recordDeal lays out the initial deal as a staggered slide-and-flip sweep:
// each seat's two cards and the dealer's single card slide from the right felt
// edge to their slot and then flip face up. Card identities are already fixed;
// this only records cosmetic timings.
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
	// then the dealer's only dealt card; second card to every seat.
	for round := 0; round < 2; round++ {
		for _, id := range rm.order {
			s := rm.seats[id]
			if s == nil || !s.placed || len(s.hands) == 0 {
				continue
			}
			add(cardAnim{kind: animSeat, player: s.p, cardIdx: round, flipStart: now})
		}
		if round == 0 {
			add(cardAnim{kind: animDealer, cardIdx: 0, flipStart: now}) // the dealer's only dealt card
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

// recordDealerDraw appends a dealer hit card sliding in and flipping face up.
func (rm *room) recordDealerDraw(start time.Time, cardIdx int) {
	rm.sched = append(rm.sched, cardAnim{
		kind:       animDealer,
		cardIdx:    cardIdx,
		slideStart: start,
		flipStart:  start.Add(slideDur),
	})
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
	r.SetInputContext(kit.CtxCommand) // h/s/d/p are domain commands
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

// autoWin marks h an automatic even-money winner — Player 21 (any
// non-blackjack 21) or Five Card Trick (five cards without busting) — and
// reports whether it fired. A blackjack is NOT an auto-win: it settles at
// its own ranked odds once the dealer's hand is known.
func autoWin(h *phand) bool {
	if h.cards.isBust() || h.cards.isBlackjack() {
		return false
	}
	if h.cards.total() == 21 || len(h.cards) == 5 {
		h.autoWon = true
		h.resolved = true
		return true
	}
	return false
}

func (rm *room) act(r kit.Room, p kit.Player, a rune) {
	s, h := rm.firstUnresolved()
	if s == nil || s.p.AccountID != p.AccountID {
		return // not this player's turn
	}
	hi := rm.handIndex(s, h)
	// A REJECTED action returns without beginTurn: re-arming there would reset
	// the turn deadline, letting a player stall their own clock with no-ops.
	switch a {
	case 'h':
		h.cards = append(h.cards, rm.sh.draw(r.Rand()))
		rm.recordDraw(r, p, hi, len(h.cards)-1)
		if !autoWin(h) && h.cards.isBust() {
			h.resolved = true
		}
		rm.beginTurn(r)
	case 's':
		h.resolved = true
		rm.beginTurn(r)
	case 'd':
		// Doubling is open on a two- OR three-card hand (original or split) that
		// hasn't already doubled — the Challenge extension of the usual gate.
		if h.doubled || len(h.cards) > 3 {
			return
		}
		if !rm.wager(s, h.bet) {
			return
		}
		h.bet *= 2
		h.doubled = true
		h.cards = append(h.cards, rm.sh.draw(r.Rand()))
		rm.recordDraw(r, p, hi, len(h.cards)-1)
		autoWin(h) // a doubled 21 is a Player 21: instant even money
		h.resolved = true
		rm.beginTurn(r)
	case 'p':
		if !rm.split(r, s, h) {
			return
		}
		rm.beginTurn(r)
	}
}

// split turns a two-card pair of equal POINT VALUE (a K and a ten split on
// this table) into two hands, each taking a new card, reporting whether the
// split happened. Split hands play on in full — and an Ace + ten-card drawn
// to one IS a Challenge blackjack, resolved on the spot at its ranked odds.
func (rm *room) split(r kit.Room, s *seat, h *phand) bool {
	if len(h.cards) != 2 || h.cards[0].r.points() != h.cards[1].r.points() || len(s.hands) >= maxHands {
		return false
	}
	// The new hand's stake Wagers onto the seat's open stake; a failed Wager
	// (can't cover it) rejects the split, as the old `chips < bet` guard did.
	if !rm.wager(s, h.bet) {
		return false
	}
	c0, c1 := h.cards[0], h.cards[1]
	rng := r.Rand()
	nh := &phand{cards: hand{c1, rm.sh.draw(rng)}, bet: h.bet}
	h.cards = hand{c0, rm.sh.draw(rng)}
	for _, sp := range []*phand{h, nh} {
		if sp.cards.isBlackjack() {
			sp.resolved = true
		}
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
	return true
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

func (rm *room) enterDealer(r kit.Room) {
	// No hole card exists on this table: the dealer's turn is a lead-in beat,
	// then each drawn card sliding in one unhurried card at a time. Pace off
	// any animation still in flight (a table of instant naturals arrives here
	// straight from the deal sweep) so the draw-out never overlaps it.
	done := r.Now().Add(dealerLeadIn)
	if rm.schedEnd.After(done) {
		done = rm.schedEnd
	}
	if rm.anyLive() {
		before := len(rm.dealer)
		rm.dealer = dealerPlay(rm.dealer, rm.sh, r.Rand())
		start := done
		for i := before; i < len(rm.dealer); i++ {
			rm.recordDealerDraw(start, i)
			start = start.Add(slideDur + flipDur + dealerDrawGap)
		}
		rm.computeSchedEnd()
		if len(rm.dealer) > before {
			done = rm.schedEnd
		}
	}
	rm.settleAt(r, done.Add(dealerDoneHold))
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

// anyLive reports whether any hand's payout still depends on the dealer's
// draw-out: busts are already lost and auto-wins already fixed at even
// money, but every other hand — a blackjack's ranked odds included — needs
// the dealer's final hand.
func (rm *room) anyLive() bool {
	for _, id := range rm.order {
		s := rm.seats[id]
		if s == nil {
			continue
		}
		for _, h := range s.hands {
			if !h.cards.isBust() && !h.autoWon {
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
			continue // skip seats that never opened a stake this round
		}
		// Fold every hand's gross into the seat's single open-stake gross, and
		// track the stake dealer-blackjack losses would collect: the house only
		// keeps the seat's ORIGINAL bet when its blackjack lands after doubles
		// and splits — the excess is refunded (the Challenge clawback). Busted
		// hands lost before the dealer's hand existed and stay lost.
		lostToBJ := 0
		for _, h := range s.hands {
			if h.cards.isBlackjack() {
				h.bjMult = blackjackMult(bjTen(h.cards), bjTen(rm.dealer), dbj)
			}
			m := grossMult(h, rm.dealer, dbj)
			if dbj && m == 0 && !h.cards.isBust() {
				lostToBJ += h.bet
			}
			s.grossThisRound += int64(m * h.bet)
		}
		if lostToBJ > s.bet {
			s.grossThisRound += int64(lostToBJ - s.bet)
		}
		rm.settleBacks(s, dbj)
		// Close the seat's open stake with ONE Settle of the accumulated gross
		// (clamped to the payout ceiling), then feed the board on a new peak.
		net := rm.settleOpenStake(s)
		s.result = resultText(net)
		if s.bal > s.highScore {
			s.highScore = s.bal
		}
		rm.postPeak(r, s)
		// A seat that can no longer cover the minimum stake would be soft-locked in
		// betting (Confirm requires bal >= betTiers[0]); trigger the platform
		// broke-relief rebuy. A refusal (solvent/daily limit) leaves the LOSE
		// summary and lets the seat sit out until it recovers.
		if s.bal < betTiers[0] && rm.buyback(s) {
			s.result = "BUST - re-buy"
		}
		s.placed = false
	}
	rm.enterResults(r)
}

// settleBacks settles seat s's behind bets, folding each behind win into the
// seat's open-stake gross. Each behind stake rides the target's FIRST hand at
// the table odds — even money, an auto-win's even money, or the ranked
// blackjack payout — and is refunded if the target has left. A behind stake
// is its own original bet, so a dealer blackjack collects it in full (the
// clawback shields only stakes ADDED to a hand by doubling and splitting).
func (rm *room) settleBacks(s *seat, dealerBJ bool) {
	for _, tid := range sortedBackIDs(s) {
		b := s.backs[tid]
		if b.behind <= 0 {
			continue
		}
		t := rm.seats[tid]
		if t == nil || len(t.hands) == 0 {
			b.behindWin = b.behind // target left: refund the behind stake
		} else {
			b.behindWin = grossMult(t.hands[0], rm.dealer, dealerBJ) * b.behind
		}
		s.grossThisRound += int64(b.behindWin)
	}
}

func (rm *room) enterResults(r kit.Room) {
	rm.phase = phResults
	for _, s := range rm.seats {
		s.ready = false // a fresh round of ready-ups
	}
	rm.deadline = r.Now().Add(resultsDur)
	r.SetInputContext(kit.CtxNav) // results screen: Up/Down idle, Confirm readies up
	rm.arm(pendResults, rm.deadline)
}

// allSeatedReady reports whether at least one seat is taken and every seated
// player has readied up — the trigger to start the next round without waiting
// out the results flash.
func (rm *room) allSeatedReady() bool {
	seated := false
	for _, s := range rm.seats {
		seated = true
		if !s.ready {
			return false
		}
	}
	return seated
}

// unreadyCount is how many seated players have not yet readied up.
func (rm *room) unreadyCount() int {
	n := 0
	for _, s := range rm.seats {
		if !s.ready {
			n++
		}
	}
	return n
}

// --- input -----------------------------------------------------------------

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	s := rm.seats[p.AccountID]
	if s == nil {
		return
	}
	switch rm.phase {
	case phBetting:
		// Up/Down set your own main stake; Left/Right change which seat you're
		// betting on (self, then each other seat). P loops the pairs side bet and
		// B loops the behind bet for the focused seat — each cycles up a tier and
		// resets to 0 past the top. Both runes are unmapped in CtxNav, read raw.
		switch kit.Resolve(in, kit.CtxNav) {
		case kit.ActUp:
			rm.adjustBet(s, +1)
			rm.clampPairs(s) // a raised main bet may crowd out the side bet
		case kit.ActDown:
			rm.adjustBet(s, -1)
		case kit.ActLeft:
			rm.cycleFocus(s, -1)
		case kit.ActRight:
			rm.cycleFocus(s, +1)
		case kit.ActConfirm:
			if s.bal >= betTiers[0] {
				if s.bet > s.bal {
					s.bet = clampBet(s.bal)
				}
				rm.clampPairs(s)
				s.placed = true
				rm.maybeCloseEarly(r) // deal early once every seat has bet
			}
		}
		if in.Kind == kit.InputRune {
			switch in.Rune {
			case 'p', 'P': // loop the focused seat's pairs side bet (yours, or theirs)
				if s.focus == "" {
					rm.cycleOwnPairs(s)
				} else {
					rm.cycleBackPairs(s)
				}
			case 'b', 'B': // loop the behind bet on the focused seat (none on yourself)
				if s.focus != "" {
					rm.cycleBackBehind(s)
				}
			case 'r', 'R': // broke-relief re-buy when you can't cover the min bet
				if s.bal < betTiers[0] {
					rm.buyback(s) // on success s.bal is topped up; the bet controls light back up next render
				}
			}
		}
	case phResults:
		// Confirm (Enter/Space) readies up for the next hand; once every seated
		// player is ready the table deals straight away instead of waiting out
		// the results flash.
		if kit.Resolve(in, kit.CtxNav) == kit.ActConfirm {
			s.ready = true
			if rm.allSeatedReady() {
				rm.enterBetting(r)
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
		return "EVEN"
	}
}
