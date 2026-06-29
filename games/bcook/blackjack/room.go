package main

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	kit "github.com/shellcade/kit/v2"
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

// pairsTiers are the Perfect Pairs side-bet stakes, lowest first; index 0 is
// "off" (no side bet). Adjusted on the Left/Right axis during betting.
var pairsTiers = []int{0, 10, 25, 50, 100}

// phand is one hand a seat plays (a seat holds more than one after a split).
type phand struct {
	cards       hand
	bet         int
	resolved    bool // stood / busted / blackjack / doubled / surrendered
	doubled     bool
	surrendered bool
	fromSplit   bool // a split hand: a two-card 21 is a plain 21, not a blackjack
}

// backBet is one seat's wager ON ANOTHER seat: a "behind" bet that rides the
// backed player's first hand vs the dealer, and/or a Perfect Pairs bet on the
// backed player's first two cards. Stakes are chosen during betting; the result
// fields are filled at settlement (their-pairs at the deal, behind at settle).
type backBet struct {
	behind    int    // behind-bet stake (0 = none)
	pairs     int    // their-Perfect-Pairs stake (0 = none)
	pairsKind string // resolved at deal: "" | "mixed" | "colored" | "perfect"
	pairsWin  int    // their-pairs chips credited at deal (0 = lost/none)
	behindWin int    // behind chips credited at settle (0 = lost)
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
	pairsBet         int                 // Perfect Pairs side stake (0 = off), carried between rounds like bet
	pairsKind        string              // this round's pairs result: "" | "mixed" | "colored" | "perfect"
	pairsWin         int                 // chips credited on the pairs side bet this round (0 = lost/none)
	focus            string              // betting UI: "" edits own bet, else the account id whose backs are being edited
	backs            map[string]*backBet // wagers on other seats, keyed by target account id (iterate via rm.order)
	insurance        int
	insuranceDecided bool
	hands            []*phand
	joinOrder        int
	result           string // settlement summary for the results phase
	ready            bool   // readied up during results to skip the wait
}

// pending names the deferred one-shot the room is waiting on, replacing the
// native engine timers: each is a deadline held in guest memory and landed in
// OnWake when r.Now() passes it (the wake idiom — no host timer survives a thaw).
type pending uint8

const (
	pendNone         pending = iota
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

	// sk standardises the durable-wallet KV writes (PersistWallet), replacing
	// the duplicated persistWallet helper. The leaderboard Post stays
	// hand-rolled in postPeak because postedPeak is seeded from the durable peak
	// at join — so a returning seat only posts on a NEW personal best, which
	// ScoreKeeper.Record (always posts the first observed value) would not
	// preserve.
	sk *kit.ScoreKeeper
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{cfg: cfg, svc: svc, seats: map[string]*seat{}, frame: kit.NewFrame(), sk: kit.NewScoreKeeper(kit.OnImprove)}
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
// Delegates to the kit's ScoreKeeper.PersistWallet, which writes the identical
// keys + merge rules, replacing the duplicated casino-wallet helper.
func (rm *room) persistWallet(r kit.Room, s *seat) {
	rm.sk.PersistWallet(r, s.p, keyBalance, s.chips, keyPeak, s.highScore)
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
	if s, ok := rm.seats[p.AccountID]; ok {
		// A re-delivered join is a fresh kit.Player (new connection): adopt
		// it so the seat renders the current handle and character tile.
		s.p = p
		rm.render(r)
		return
	}
	bal, peak := rm.seedWallet(r, p)
	rm.seats[p.AccountID] = &seat{p: p, chips: bal, highScore: peak, postedPeak: peak, bet: betTiers[1], joinOrder: rm.joinSeq}
	rm.joinSeq++
	rm.order = append(rm.order, p.AccountID)
	rm.render(r)
}

// OnLeave persists the leaver's wallet and frees the seat. Leaving with a live
// hand forfeits the stake (it was deducted at deal and settle only credits
// seats still in rm.order) — abandoning a dealt hand is a loss, as at a real
// table. Chips are conserved; nothing is credited elsewhere.
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
		s.pairsKind = ""
		s.pairsWin = 0
		s.focus = ""  // re-open editing on the seat's own bet
		s.backs = nil // backs are round-specific to particular opponents; never carried
		if s.bet > s.chips {
			s.bet = clampBet(s.chips)
		}
		rm.clampPairs(s) // a thinned stack may no longer afford the carried side bet
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

// adjustPairs steps the Perfect Pairs side stake through pairsTiers, clamped so
// the main bet plus the side bet never exceeds the seat's chips (the side bet
// shrinks to the highest affordable tier, down to off).
func (rm *room) adjustPairs(s *seat, dir int) {
	i := pairsTierIndex(s.pairsBet) + dir
	if i < 0 {
		i = 0
	}
	if i >= len(pairsTiers) {
		i = len(pairsTiers) - 1
	}
	s.pairsBet = pairsTiers[i]
	rm.clampPairs(s)
}

// clampPairs lowers the side bet to the highest tier the seat can still afford
// alongside its main bet (down to off), so a raised main bet or a thin stack can
// never leave an unaffordable side bet placed.
func (rm *room) clampPairs(s *seat) {
	for s.pairsBet > 0 && s.bet+s.pairsBet > s.chips {
		s.pairsBet = pairsTiers[pairsTierIndex(s.pairsBet)-1]
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
// bet, its own Perfect Pairs, and every behind/their-pairs stake across backs.
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

// adjustBackBehind / adjustBackPairs step the focused back's stake through
// pairsTiers, clamped so the seat's total commitment never exceeds its chips.
func (rm *room) adjustBackBehind(s *seat, dir int) {
	b := s.backOn(s.focus)
	want := stepTier(pairsTiers, b.behind, dir)
	b.behind = affordTier(pairsTiers, want, s.chips-(s.committed()-b.behind))
}

func (rm *room) adjustBackPairs(s *seat, dir int) {
	b := s.backOn(s.focus)
	want := stepTier(pairsTiers, b.pairs, dir)
	b.pairs = affordTier(pairsTiers, want, s.chips-(s.committed()-b.pairs))
}

// stepTier returns the tier `dir` steps from `cur` (clamped to the ends).
func stepTier(tiers []int, cur, dir int) int {
	i := pairsTierIndex(cur) + dir
	if i < 0 {
		i = 0
	}
	if i >= len(tiers) {
		i = len(tiers) - 1
	}
	return tiers[i]
}

// --- dealing ---------------------------------------------------------------

func (rm *room) deal(r kit.Room) {
	if rm.sh.needsReshuffle() {
		rm.sh.shuffle(r.Rand())
	}
	rm.sh.beginRound() // everything dealt before this point is recyclable discards
	rng := r.Rand()
	rm.dealer = hand{rm.sh.draw(rng), rm.sh.draw(rng)} // [up, hole]
	rm.dealerHole = true
	// Range the join-ordered slice (not the map) so dealing order is
	// deterministic — never depends on Go's map iteration order.
	for _, id := range rm.order {
		s := rm.seats[id]
		if s == nil || !s.placed {
			continue
		}
		s.chips -= s.bet
		h := &phand{cards: hand{rm.sh.draw(rng), rm.sh.draw(rng)}, bet: s.bet}
		if h.cards.isBlackjack() {
			h.resolved = true
		}
		s.hands = []*phand{h}
		rm.resolvePairs(s, h.cards)
	}

	rm.resolveBackPairs() // every hand is now dealt; settle the their-pairs side of each back

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

// resolvePairs settles a seat's Perfect Pairs side bet against its dealt cards:
// the stake is deducted (the main bet was already taken above) and any winning
// pair is credited immediately — the casino way, where the side bet stands apart
// from how the hand goes on to play out.
func (rm *room) resolvePairs(s *seat, dealt hand) {
	if s.pairsBet <= 0 || len(dealt) < 2 {
		return
	}
	if s.pairsBet > s.chips {
		s.pairsBet = s.chips // defensive: never deduct more than the seat has left
	}
	s.chips -= s.pairsBet
	kind, mult := perfectPairsOutcome(dealt[0], dealt[1])
	s.pairsKind = kind
	s.pairsWin = pairsCreditFor(mult, s.pairsBet)
	s.chips += s.pairsWin
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

// resolveBackPairs settles the their-Perfect-Pairs side of every seat's backs,
// once all hands are dealt: a back on a seat that didn't get dealt in is voided
// (never charged); otherwise the stake is deducted and any winning pair on the
// target's first two cards is credited to the backer immediately. The behind bet
// is committed here too (deducted, held) and settled later against the dealer.
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
			if b.pairs > s.chips {
				b.pairs = s.chips // defensive
			}
			if b.pairs > 0 {
				s.chips -= b.pairs
				kind, mult := perfectPairsOutcome(t.hands[0].cards[0], t.hands[0].cards[1])
				b.pairsKind = kind
				b.pairsWin = pairsCreditFor(mult, b.pairs)
				s.chips += b.pairsWin
			}
			if b.behind > s.chips {
				b.behind = s.chips // defensive
			}
			s.chips -= b.behind // committed now; the dealer comparison happens at settle
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
// slide) starting at `at`, and returns the room-clock instant the flip
// completes so dealer play can pace off it. Callers lead in with a short beat
// before `at` so the card sits face down a moment, then turns over, rather than
// snapping up the instant the dealer's turn begins.
func (rm *room) recordHoleReveal(at time.Time) time.Time {
	rm.sched = []cardAnim{{
		kind:      animDealer,
		cardIdx:   1,
		flipStart: at,
	}}
	rm.computeSchedEnd()
	return at.Add(flipDur)
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

// insuranceUndecidedCount is how many placed seats have not yet answered the
// insurance offer (the seats the table is still waiting on).
func (rm *room) insuranceUndecidedCount() int {
	n := 0
	for _, s := range rm.seats {
		if s.placed && !s.insuranceDecided {
			n++
		}
	}
	return n
}

// maybeResolveInsurance resolves the insurance window early once every placed
// seat has answered, instead of waiting out the timer (mirrors the all-bets-in
// early deal and the all-ready results skip).
func (rm *room) maybeResolveInsurance(r kit.Room) {
	placed := false
	for _, s := range rm.seats {
		if !s.placed {
			continue
		}
		placed = true
		if !s.insuranceDecided {
			return // still waiting on this seat
		}
	}
	if !placed {
		return
	}
	rm.what = pendNone // cancel the armed timer; resolving now
	rm.resolveInsurance(r)
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
	// A REJECTED action returns without beginTurn: re-arming there would reset
	// the turn deadline, letting a player stall their own clock with no-ops.
	switch a {
	case 'h':
		h.cards = append(h.cards, rm.sh.draw(r.Rand()))
		rm.recordDraw(r, p, hi, len(h.cards)-1)
		if h.cards.isBust() || h.cards.total() == 21 {
			h.resolved = true
		}
		rm.beginTurn(r)
	case 's':
		h.resolved = true
		rm.beginTurn(r)
	case 'd':
		if !first || s.chips < h.bet {
			return
		}
		s.chips -= h.bet
		h.bet *= 2
		h.doubled = true
		h.cards = append(h.cards, rm.sh.draw(r.Rand()))
		rm.recordDraw(r, p, hi, len(h.cards)-1)
		h.resolved = true
		rm.beginTurn(r)
	case 'p':
		if !rm.split(r, s, h) {
			return
		}
		rm.beginTurn(r)
	case 'r':
		if !first || len(s.hands) != 1 {
			return
		}
		h.surrendered = true
		h.resolved = true
		// Return half; the bet was deducted at deal. An odd bet's half-chip
		// rounds UP to the player (halfUp owns the policy, shared with the
		// 3:2 payout in creditFor and the net accounting in settle).
		s.chips += halfUp(h.bet)
		rm.beginTurn(r)
	}
}

// split turns a two-card equal-rank pair into two hands, each taking a new card,
// reporting whether the split happened. Split aces take one card and stand.
func (rm *room) split(r kit.Room, s *seat, h *phand) bool {
	if len(h.cards) != 2 || h.cards[0].r != h.cards[1].r || s.chips < h.bet || len(s.hands) >= maxHands {
		return false
	}
	c0, c1 := h.cards[0], h.cards[1]
	s.chips -= h.bet
	rng := r.Rand()
	nh := &phand{cards: hand{c1, rm.sh.draw(rng)}, bet: h.bet, fromSplit: true}
	h.cards = hand{c0, rm.sh.draw(rng)}
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

func (rm *room) revealAndSettle(r kit.Room) {
	rm.dealerHole = false
	// A beat with the card still face down, then turn it over, then hold a beat
	// on the revealed blackjack before settling — so the reveal animates and
	// registers instead of snapping straight to results.
	flipAt := r.Now().Add(holeRevealDelay)
	done := rm.recordHoleReveal(flipAt)
	rm.settleAt(r, done.Add(dealerDoneHold))
}

func (rm *room) enterDealer(r kit.Room) {
	rm.dealerHole = false
	// The outcome is fixed up front from the seeded shoe; the schedule only paces
	// the reveal. Hold a beat with the hole card still face down, turn it over,
	// hold a beat on the dealer's two-card total, then slide in each hit one
	// unhurried card at a time, and settle a final beat after the last card
	// lands — never the flurry the initial deal uses.
	flipAt := r.Now().Add(holeRevealDelay)
	done := rm.recordHoleReveal(flipAt).Add(holeRevealHold)
	if rm.anyLive() {
		before := len(rm.dealer)
		rm.dealer = dealerPlay(rm.dealer, rm.sh, r.Rand())
		start := done
		for i := before; i < len(rm.dealer); i++ {
			rm.recordDealerDraw(start, i)
			// The next hit waits for this card to fully land, plus a read beat.
			start = start.Add(slideDur + flipDur + dealerDrawGap)
		}
		rm.computeSchedEnd() // across the appended dealer cards
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
				net -= h.bet - halfUp(h.bet) // lost half (the same halfUp act credited)
				continue
			}
			pbj := h.cards.isBlackjack() && !h.fromSplit
			o := settleHandEx(h.cards, pbj, rm.dealer, dbj)
			credit := creditFor(o, h.bet)
			s.chips += credit
			net += credit - h.bet
		}
		// The Perfect Pairs side bet was settled at deal (stake deducted, any win
		// credited there); fold its delta into the round net so the seat's
		// WIN/LOSE summary reconciles with the chips that actually changed hands.
		net += s.pairsWin - s.pairsBet
		net += rm.settleBacks(s, dbj)
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

// settleBacks settles seat s's behind bets and folds every back's delta into the
// round, returning the net chip change to add to s's summary. Their-pairs were
// settled at the deal (only their delta folds here); each behind bet is judged
// now against the target's first hand vs the dealer — even money on a win, 3:2 on
// a natural blackjack, push returned — and is refunded if the target has left
// (their hand can't be judged) or loses if that hand surrendered.
func (rm *room) settleBacks(s *seat, dealerBJ bool) int {
	net := 0
	for _, tid := range sortedBackIDs(s) {
		b := s.backs[tid]
		net += b.pairsWin - b.pairs // their-pairs delta (settled at the deal)
		if b.behind <= 0 {
			continue
		}
		t := rm.seats[tid]
		switch {
		case t == nil || len(t.hands) == 0:
			b.behindWin = b.behind // target left: refund the behind stake (push)
		case t.hands[0].surrendered:
			b.behindWin = 0 // backed a hand that bailed out
		default:
			h0 := t.hands[0]
			o := settleHandEx(h0.cards, h0.cards.isBlackjack() && !h0.fromSplit, rm.dealer, dealerBJ)
			b.behindWin = creditFor(o, b.behind)
		}
		s.chips += b.behindWin
		net += b.behindWin - b.behind
	}
	return net
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
		// Left/Right change which seat you're betting on (self, then each other
		// seat); Up/Down set that seat's hand bet (your stake, or the behind bet
		// on a backed seat). P/B cycle the pairs side bet (yours, or theirs) for
		// the focused seat — both runes are unmapped in CtxNav, so read raw.
		switch kit.Resolve(in, kit.CtxNav) {
		case kit.ActUp:
			if s.focus == "" {
				rm.adjustBet(s, +1)
				rm.clampPairs(s) // a raised main bet may crowd out the side bet
			} else {
				rm.adjustBackBehind(s, +1)
			}
		case kit.ActDown:
			if s.focus == "" {
				rm.adjustBet(s, -1)
			} else {
				rm.adjustBackBehind(s, -1)
			}
		case kit.ActLeft:
			rm.cycleFocus(s, -1)
		case kit.ActRight:
			rm.cycleFocus(s, +1)
		case kit.ActConfirm:
			if s.chips >= betTiers[0] {
				if s.bet > s.chips {
					s.bet = clampBet(s.chips)
				}
				rm.clampPairs(s)
				s.placed = true
				rm.maybeCloseEarly(r) // deal early once every seat has bet
			}
		}
		if in.Kind == kit.InputRune {
			switch in.Rune {
			case 'p', 'P': // raise the focused seat's pairs side bet
				if s.focus == "" {
					rm.adjustPairs(s, +1)
				} else {
					rm.adjustBackPairs(s, +1)
				}
			case 'b', 'B': // lower it (down to off)
				if s.focus == "" {
					rm.adjustPairs(s, -1)
				} else {
					rm.adjustBackPairs(s, -1)
				}
			}
		}
	case phInsurance:
		if in.Kind == kit.InputRune {
			switch in.Rune {
			case 'y', 'Y':
				rm.takeInsurance(s, true)
				rm.maybeResolveInsurance(r) // all answered -> resolve, skip the timer
			case 'n', 'N':
				rm.takeInsurance(s, false)
				rm.maybeResolveInsurance(r)
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
