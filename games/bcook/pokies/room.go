package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// Game is the pokies registry entry: static metadata plus the per-room factory.
type Game struct{}

// Meta returns the static game metadata (mirrors the native pokies meta). The
// Slug is the BARE name; the platform composes the namespaced "bcook/pokies"
// from the catalog path, so game.toml and Meta never carry a slash.
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "pokies",
		Name:             "Pokies",
		ShortDescription: "Pull the lever on your own slot machine and chase your high score.",
		MinPlayers:       1,
		MaxPlayers:       32,
		Tags:             []string{"slots", "casual", "social"},

		// A resident social lounge: the room persists when players leave,
		// offering Resume-menu entry for returning players (kit v2.7.0+).
		Lifecycle: kit.LifecycleResident,

		// Per-member arcade characters (kit v2.9.0) + roster epoch tracking
		// for multiplayer awareness (kit v2.11.0+).
		CtxFeatures: kit.CtxFeatCharacter | kit.CtxFeatRosterEpoch,

		QuickModeLabel:    "Quick spin",
		SoloModeLabel:     "Solo spin",
		PrivateInviteLine: "Friends join your floor when they enter the code.",

		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Credits",
			Direction:   kit.HigherBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Integer,
		},

		// The admin-settable config surface (config.go): the odds-variant
		// PAR sheet, declared so the arcade's admin tools render a rich form.
		Config: configSpecs(),
	}
}

// NewRoom returns the per-room behavior.
func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return newRoom(cfg, svc)
}

const (
	startBalance = 1000 // credits a fresh machine starts with
	rebuyAmount  = 1000 // balance restored on a bust
	tickerMult   = 12   // a win at this multiplier or above announces room-wide

	cycleRate    = 80 * time.Millisecond  // reel-cycling animation step
	reelStopBase = 150 * time.Millisecond // when the first reel settles
	reelStopStep = 250 * time.Millisecond // stagger between successive reels
	flashDur     = 1500 * time.Millisecond
	tickerDur    = 5 * time.Second
	freeSpinGap  = 700 * time.Millisecond // pause between auto-played free spins

	configRefresh = 30 * time.Second // how often the room re-reads its odds variant
)

// configKey is the per-game config key under which the pokies odds variant lives
// (the same key the native arcade admin area writes).
const configKey = "odds-variant"

// betTiers are the selectable stakes, lowest first.
var betTiers = []int{10, 50, 100, 500}

// spinState is the live animation of one pull. The outcome is rolled up front (a
// landing index per reel); the wake idiom replaces the native engine timers:
// reel i lands when now passes startedAt + reelStopBase + i*reelStopStep, and
// the scroll cycle is DERIVED from elapsed time (hibernation-stable,
// heartbeat-rate independent). It pins the variant it started under so a config
// change mid-spin never re-evaluates the outcome.
type spinState struct {
	startedAt time.Time
	stopIdx   [3]int    // landing position on the strip per reel
	final     [3]symbol // center (payline) face per reel = strip[stopIdx]
	landed    int       // number of reels settled (0..3)
	variant   *variant  // the odds variant this spin started under (settles under it)
}

// cycle is the current scroll frame for an in-flight spin, derived from elapsed
// time (idiom 3: a derived animation clock, never a per-wake accumulator).
func (s *spinState) cycle(now time.Time) int {
	return int(now.Sub(s.startedAt) / cycleRate)
}

// machine is one player's slot machine. The visible reel area is a 3x3 window;
// the center row is the payline. reels holds the settled center faces.
type machine struct {
	balance    int
	highScore  int
	bet        int
	reels      [3]symbol // last settled center (payline) faces
	lastIdx    [3]int    // last settled landing index per reel (for the idle window)
	lastStrip  []symbol  // strip the lastIdx values index into (the variant of the last spin)
	spun       bool      // false until the first spin settles (shows blanks)
	spin       *spinState
	flash      string    // transient status line: "WIN! +N" / "RE-BUY"
	flashUntil time.Time // when the flash clears (deadline held in guest memory)
	postedPeak int       // last peak posted to the leaderboard (post only on increase)
	lastVar    *variant  // variant the last spin settled under (for the gamble caps)

	// Free spins (the scatter feature). When freeSpins > 0 the reels auto-play
	// at no cost, paying at freeBet under freeVar; freeWin accumulates the total.
	freeSpins int
	freeBet   int
	freeWin   int
	freeVar   *variant
	nextFree  time.Time // earliest time the next auto free spin may start

	// Gamble (double-up). Non-nil while a base-game win is held at risk.
	gamble *gambleState
}

// ticker is the room-wide big-win banner. text starts with the winner's
// name; ch is their character tile, rendered immediately before it.
type ticker struct {
	text  string
	ch    kit.Character
	until time.Time
}

type room struct {
	kit.Base
	cfg kit.RoomConfig
	svc kit.Services

	machines map[string]*machine   // keyed by account id (hibernation-safe)
	order    []string              // join order of account ids, for left-to-right layout
	names    map[string]kit.Player // account id -> player (for handles + leaderboard Post)
	ticker   ticker                // room-wide big-win banner
	variant  *variant              // the active odds variant, refreshed on a deadline
	nextCfg  time.Time             // next config-refresh deadline
	lastNow  time.Time             // room clock captured at the last render
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:      cfg,
		svc:      svc,
		machines: map[string]*machine{},
		names:    map[string]kit.Player{},
		variant:  defaultVariant(),
	}
}

func (rm *room) OnStart(r kit.Room) {
	// Bet-adjust + spin is a navigation screen throughout.
	r.SetInputContext(kit.CtxNav)
	// Load the odds variant from per-game config now, then refresh it on a rolling
	// deadline so an admin's save takes effect on subsequent spins within
	// configRefresh — a spin pins the variant it started under, so a refresh never
	// re-evaluates an in-flight spin.
	rm.loadVariant(r)
	rm.nextCfg = r.Now().Add(configRefresh)
}

// loadVariant reads the stored odds variant from per-game config and caches it. A
// missing key, a read error, or an unparsable/invalid document keeps the last
// good variant (the compiled default until one parses), so a config blip or a bad
// save can never leave a dead machine — mirroring the native game.
func (rm *room) loadVariant(r kit.Room) {
	cfg := r.Services().Config
	if cfg == nil {
		return // no config surface: keep the current variant
	}
	blob, ok, err := cfg.Get(context.Background(), configKey)
	if err != nil {
		r.Log("pokies: odds config read failed; keeping current variant")
		return
	}
	if !ok {
		rm.variant = defaultVariant() // no stored variant: compiled default
		return
	}
	if v, err := parseVariant(blob); err == nil {
		rm.variant = v
	} else {
		r.Log("pokies: stored odds variant is invalid; using default")
		rm.variant = defaultVariant()
	}
}

// --- durable wallet ----------------------------------------------------------
//
// The casino pattern over kv: balance (merge rule sum, the carryable bankroll)
// and peak (merge rule max, the high-water mark and leaderboard metric) — the
// same keys and merge rules the native casino package used.

const (
	keyBalance = "balance"
	keyPeak    = "peak"
)

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
func (rm *room) seedWallet(r kit.Room, p kit.Player) (int, int) {
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return startBalance, startBalance
	}
	store := acct.Store()
	bal, ok := kvInt(store, keyBalance)
	if !ok || bal <= 0 {
		bal = startBalance
	}
	peak, ok := kvInt(store, keyPeak)
	if !ok || peak < bal {
		peak = bal
	}
	return bal, peak
}

// persistWallet writes the current balance (summed) and raises peak (max). peak
// uses a monotonic max-on-write, so out-of-order or concurrent same-account
// writes can never regress the leaderboard metric.
func (rm *room) persistWallet(r kit.Room, p kit.Player, bal, peak int) {
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return
	}
	store := acct.Store()
	_ = store.Set(context.Background(), keyBalance, []byte(strconv.Itoa(bal)), kit.MergeSum)
	_ = store.Set(context.Background(), keyPeak, []byte(strconv.Itoa(peak)), kit.MergeMax)
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.names[p.AccountID] = p
	if _, ok := rm.machines[p.AccountID]; ok {
		rm.render(r)
		return
	}
	// Seed the machine balance from the player's durable wallet (default first time).
	bal, peak := rm.seedWallet(r, p)
	rm.machines[p.AccountID] = &machine{balance: bal, highScore: peak, bet: betTiers[0], postedPeak: peak}
	rm.order = append(rm.order, p.AccountID)
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	m := rm.machines[p.AccountID]
	if m == nil {
		return
	}
	rm.persistWallet(r, p, m.balance, m.highScore)
	delete(rm.machines, p.AccountID)
	delete(rm.names, p.AccountID)
	for i, id := range rm.order {
		if id == p.AccountID {
			rm.order = append(rm.order[:i], rm.order[i+1:]...)
			break
		}
	}
	rm.render(r)
}

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	m := rm.machines[p.AccountID]
	if m == nil {
		return
	}
	act := kit.Resolve(in, kit.CtxNav)
	switch {
	case m.gamble != nil:
		rm.gambleInput(r, p.AccountID, act) // double-up ladder owns input
	case m.freeSpins > 0:
		// free spins auto-play; ignore bet/spin during the feature
	default:
		switch act {
		case kit.ActUp:
			rm.adjustBet(m, +1)
		case kit.ActDown:
			rm.adjustBet(m, -1)
		case kit.ActConfirm:
			rm.startSpin(r, p)
		}
	}
	rm.render(r)
}

// OnWake advances every time-driven element against CallContext time, then
// renders once: the periodic config refresh, flash expiry, and reel landings.
func (rm *room) OnWake(r kit.Room) {
	now := r.Now()
	// Periodic config refresh on a rolling deadline (idiom 3).
	if !rm.nextCfg.IsZero() && now.After(rm.nextCfg) {
		rm.loadVariant(r)
		rm.nextCfg = now.Add(configRefresh)
	}
	// Iterate machines in a stable (join) order so any host call ordering is
	// deterministic and hibernation-stable — never range the map directly.
	for _, id := range rm.order {
		m := rm.machines[id]
		if m == nil {
			continue
		}
		// One-shot flash expiry (idiom 1).
		if m.flash != "" && now.After(m.flashUntil) {
			m.flash = ""
		}
		if m.spin == nil {
			// Auto-play free spins: when none is in flight and the inter-spin gap
			// has elapsed, roll the next free spin (settled by the loop below on
			// later wakes).
			if m.freeSpins > 0 && now.After(m.nextFree) {
				rm.autoFreeSpin(r, id)
			}
			if m.spin == nil {
				continue
			}
		}
		// Staggered reel landings: land every reel whose derived deadline has
		// passed, in order (idiom 2).
		for i := m.spin.landed; i < 3; i++ {
			due := m.spin.startedAt.Add(reelStopBase + time.Duration(i)*reelStopStep)
			if !now.After(due) {
				break // not due yet, and later reels are even later — stop
			}
			rm.landReel(r, id, i)
			if m.spin == nil {
				break // the final reel settled and cleared the spin
			}
		}
	}
	rm.render(r)
}

func (rm *room) OnClose(r kit.Room) {
	for _, id := range rm.order {
		m := rm.machines[id]
		if m == nil {
			continue
		}
		if p, ok := rm.names[id]; ok {
			rm.persistWallet(r, p, m.balance, m.highScore)
		}
	}
}

// --- betting -----------------------------------------------------------------

func tierIndex(bet int) int {
	for i, t := range betTiers {
		if t == bet {
			return i
		}
	}
	return 0
}

func (rm *room) adjustBet(m *machine, dir int) {
	i := tierIndex(m.bet) + dir
	if i < 0 {
		i = 0
	}
	if i >= len(betTiers) {
		i = len(betTiers) - 1
	}
	m.bet = betTiers[i]
	rm.clampBet(m)
}

// clampBet drops the bet to the highest tier the balance can cover.
func (rm *room) clampBet(m *machine) {
	for m.bet > m.balance && tierIndex(m.bet) > 0 {
		m.bet = betTiers[tierIndex(m.bet)-1]
	}
}

// --- spinning ----------------------------------------------------------------

func (rm *room) startSpin(r kit.Room, p kit.Player) {
	m := rm.machines[p.AccountID]
	if m == nil || m.spin != nil || m.freeSpins > 0 || m.gamble != nil {
		return // auto-play owns the reels during a feature / gamble holds the win
	}
	rm.clampBet(m)
	if m.bet > m.balance {
		return // can't afford the lowest tier
	}
	m.balance -= m.bet
	m.flash = ""

	// Pin the variant this spin starts under: a later config refresh never
	// re-evaluates an in-flight spin. The strip is its variant's strip, so a
	// seeded room reproduces outcomes for a given variant.
	v := rm.variant
	if v == nil {
		v = defaultVariant()
	}
	s := &spinState{startedAt: r.Now(), variant: v}
	for i := range s.final {
		s.stopIdx[i] = r.Rand().Intn(len(v.strip))
		s.final[i] = v.strip[s.stopIdx[i]]
	}
	m.spin = s
}

func (rm *room) landReel(r kit.Room, id string, i int) {
	m := rm.machines[id]
	if m == nil || m.spin == nil {
		return
	}
	m.spin.landed = i + 1
	m.reels[i] = m.spin.final[i]
	m.lastIdx[i] = m.spin.stopIdx[i]
	if v := m.spin.variant; v != nil {
		m.lastStrip = v.strip
	}
	if m.spin.landed >= 3 {
		rm.settleSpin(r, id)
	}
}

func (rm *room) settleSpin(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil || m.spin == nil {
		return
	}
	m.reels = m.spin.final
	m.lastIdx = m.spin.stopIdx
	// Settle under the variant the spin started with (never a refreshed one).
	v := m.spin.variant
	if v == nil {
		v = defaultVariant()
	}
	m.lastStrip = v.strip
	m.lastVar = v
	wasFree := m.freeSpins > 0
	bet := m.bet
	if wasFree {
		bet = m.freeBet
	}
	m.spin = nil
	m.spun = true

	win := bet * v.payout(m.reels)

	if wasFree {
		// Free spin: credit at the locked bet (no charge), retrigger, then advance
		// the feature. Gamble is never offered inside a feature.
		m.freeSpins--
		m.freeWin += win
		rm.creditWin(r, id, win, false)
		rm.triggerFreeSpins(m, v, bet)
		if win >= bet*tickerMult {
			rm.announce(r, id, win)
		}
		if m.freeSpins == 0 {
			rm.endFreeSpins(r, id)
		}
		rm.scheduleNextFree(r, m)
		return
	}

	// Base game. A spin can both pay a line and trigger free spins; on a trigger
	// credit any line win directly (no gamble) and start the feature.
	if award := rm.triggerFreeSpins(m, v, bet); award > 0 {
		rm.creditWin(r, id, win, false)
		rm.announce(r, id, 0) // "X hit FREE SPINS!"
		rm.scheduleNextFree(r, m)
		return
	}

	if win > 0 {
		rm.enterGamble(r, m, win) // hold the win on the double-up ladder
		m.flash = ""
		return
	}

	rm.creditWin(r, id, 0, true) // no win: rebuy check + clear flash
}

// creditWin adds win to the balance, raises the peak, posts a new personal best
// to the leaderboard, sets the WIN flash, and (when allowZeroRebuy) re-buys a
// busted machine. It is the single credit path for taken base wins, free-spin
// wins, and the no-win settle.
func (rm *room) creditWin(r kit.Room, id string, win int, allowZeroRebuy bool) {
	m := rm.machines[id]
	if m == nil {
		return
	}
	m.balance += win
	if m.balance > m.highScore {
		m.highScore = m.balance
	}
	switch {
	case allowZeroRebuy && m.balance <= 0:
		m.balance = rebuyAmount
		m.flash = "RE-BUY"
	case win > 0:
		m.flash = fmt.Sprintf("WIN! +%d", win)
	}
	m.flashUntil = r.Now().Add(flashDur)
	rm.clampBet(m)
	if p, ok := rm.names[id]; ok {
		// Persist the durable wallet (peak excludes the rebuy).
		rm.persistWallet(r, p, m.balance, m.highScore)
		// Leaderboard: Post feeds the board declared in GameMeta.Leaderboard.
		// Post on a new personal peak — the board keeps each account's best.
		if m.highScore > m.postedPeak {
			m.postedPeak = m.highScore
			r.Post(kit.Result{Rankings: []kit.PlayerResult{{
				Player: p, Metric: m.highScore, Status: kit.StatusFinished,
			}}})
		}
	}
}

// announce raises the room-wide ticker: a free-spin trigger banner when win == 0,
// otherwise the big-win banner naming the player.
func (rm *room) announce(r kit.Room, id string, win int) {
	p, ok := rm.names[id]
	if !ok {
		return
	}
	text := fmt.Sprintf("%s hit a big win  +%d", p.DisplayName(), win)
	if win == 0 {
		text = fmt.Sprintf("%s hit FREE SPINS!", p.DisplayName())
	}
	rm.ticker = ticker{text: text, ch: p.Character, until: r.Now().Add(tickerDur)}
}

// --- ticker ------------------------------------------------------------------

func (rm *room) tickerActive(now time.Time) bool {
	return rm.ticker.text != "" && now.Before(rm.ticker.until)
}
