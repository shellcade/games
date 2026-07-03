package main

import (
	"context"
	"errors"
	"fmt"
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

		// Casino-kind: the machine wagers account-wide platform Credits
		// (kit v2.16.0). The host owns every balance; the game holds no
		// wallet of its own. MaxPayoutMultiplier caps a settled gross to
		// stake x this value (see the stake-relative gamble/free-spin caps).
		Kind:                kit.GameKindCasino,
		MaxPayoutMultiplier: maxPayoutMult,

		// A resumable session (NOT resident): a casino room hibernates on
		// abandon and resumes for a returning player. Resident is unsupported
		// for casino-kind — a resident room gets no per-seat wallet, so every
		// Wager/Settle/Balance would be denied. The floor/pawn lounge still
		// runs; players enter a session rather than an always-on lounge.
		Lifecycle: kit.LifecycleResumable,

		// Per-member arcade characters (kit v2.9.0) + roster epoch tracking
		// for multiplayer awareness (kit v2.11.0+), and the Credits declaration
		// bit (kit v2.16.0) so the guest may call the credits host functions.
		CtxFeatures: kit.CtxFeatCharacter | kit.CtxFeatRosterEpoch | kit.CtxFeatCredits,

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
	// maxPayoutMult is the declared casino MaxPayoutMultiplier: a settled gross
	// is capped to stake x this value. Both the gamble ladder's at-risk win and
	// the free-spin accumulation are held stake-relative to it, and every gross
	// is clamped to stake*maxPayoutMult before Settle so the UI never shows a
	// number bigger than the host will pay.
	maxPayoutMult = 10000

	tickerMult = 12 // a win at this multiplier or above announces room-wide

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
	stopIdx   [numReels]int    // landing position on the strip per reel
	final     [numReels]symbol // center face per reel = strip[stopIdx]
	landed    int              // number of reels settled (0..numReels)
	variant   *variant         // the odds variant this spin started under (settles under it)
}

// cycle is the current scroll frame for an in-flight spin, derived from elapsed
// time (idiom 3: a derived animation clock, never a per-wake accumulator).
func (s *spinState) cycle(now time.Time) int {
	return int(now.Sub(s.startedAt) / cycleRate)
}

// machine is one player's slot machine. The visible reel area is a 3x3 window;
// the center row is the payline. reels holds the settled center faces.
type machine struct {
	// credits is the per-render cache of the player's account-wide platform
	// balance (svc.Credits.Balance); peak is its high-water mark, the leaderboard
	// metric. The host owns the real balance — these are read-through caches,
	// refreshed after each money event (Wager/Settle/Buyback), never per frame.
	credits    int64
	peak       int64
	postedPeak int64 // last peak posted to the leaderboard (post only on increase)
	// stake is the single open wagered stake for this seat (0 = none open). Set
	// at the Wager in startSpin, cleared at the one Settle that closes it; it
	// bounds the stake-relative payout caps and flags an unsettled bet on exit.
	stake      int
	bet        int
	reels      [numReels]symbol // last settled center faces
	lastIdx    [numReels]int    // last settled landing index per reel (for the idle window)
	lastStrip  []symbol         // strip the lastIdx values index into (the variant of the last spin)
	spun       bool             // false until the first spin settles (shows blanks)
	spin       *spinState
	flash      string    // transient status line: "WIN! +N" / "RE-BUY"
	flashUntil time.Time // when the flash clears (deadline held in guest memory)
	lastVar    *variant  // variant the last spin settled under (for the gamble caps)
	seatVar    *variant  // variant bound when the player sits at a floor machine (nil = room default)

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

	machines  map[string]*machine   // keyed by account id (hibernation-safe)
	order     []string              // join order of account ids, for left-to-right layout
	names     map[string]kit.Player // account id -> player (for handles + leaderboard Post)
	ticker    ticker                // room-wide big-win banner
	variant   *variant              // the active odds variant, refreshed on a deadline
	nextCfg   time.Time             // next config-refresh deadline
	lastNow   time.Time             // room clock captured at the last render
	fmap      *floorMap
	fmachines []floorMachine
	pawns     map[string]*pawn // account id -> floor presence
	occupied  map[int]string   // machine id -> account id (exclusive seat)
	themes    []*variant       // machine id -> bound variant (PR2: all default)

	// economyOff latches when the host reports the credits economy disabled
	// (Credits nil, or a call returns ErrEconomyDisabled): the cabinet renders
	// out-of-service and refuses spins rather than trapping.
	economyOff bool
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	fmap, fmachines := buildLounge()
	themes := themeVariants() // one themed 5-reel PAR sheet per machine
	for len(themes) < len(fmachines) {
		themes = append(themes, defaultVariant()) // safety: never fewer than machines
	}
	return &room{
		cfg:       cfg,
		svc:       svc,
		machines:  map[string]*machine{},
		names:     map[string]kit.Player{},
		variant:   defaultVariant(),
		fmap:      fmap,
		fmachines: fmachines,
		pawns:     map[string]*pawn{},
		occupied:  map[int]string{},
		themes:    themes,
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

// --- platform credits ---------------------------------------------------------
//
// The platform owns every balance: a Wager escrows the stake into the seat's
// single open stake, and Settle closes it with the GROSS (stake-inclusive)
// payout. The game persists no wallet of its own; it caches the balance per
// money event for the HUD and posts a peak-credits leaderboard metric.

// refreshBalance re-reads the player's account-wide balance into the machine
// cache (called after every money event, never per frame) and tracks the peak.
// A disabled economy latches economyOff and leaves the cache untouched.
func (rm *room) refreshBalance(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil {
		return
	}
	c := r.Services().Credits
	p, ok := rm.names[id]
	if c == nil || !ok {
		if c == nil {
			rm.economyOff = true
		}
		return
	}
	bal, err := c.Balance(p)
	if err != nil {
		if errors.Is(err, kit.ErrEconomyDisabled) {
			rm.economyOff = true
		}
		return
	}
	rm.economyOff = false
	m.credits = bal
	if bal > m.peak {
		m.peak = bal
	}
}

// capGross clamps a gross payout to stake x maxPayoutMult (rule 6: the UI must
// never show a number bigger than the host will pay). A zero stake leaves it
// unclamped — the host still refuses a settle with no open stake.
func capGross(gross, stake int) int {
	if stake <= 0 {
		return gross
	}
	if lim := stake * maxPayoutMult; gross > lim {
		return lim
	}
	return gross
}

// settle closes the seat's single open stake EXACTLY ONCE with the clamped
// gross, then refreshes the cache, posts a new credits peak, and rebuys a
// busted seat. Every round-ending path funnels through here.
func (rm *room) settle(r kit.Room, id string, gross int) {
	m := rm.machines[id]
	if m == nil {
		return
	}
	gross = capGross(gross, m.stake)
	c := r.Services().Credits
	p, ok := rm.names[id]
	if c == nil || !ok {
		if c == nil {
			rm.economyOff = true
		}
		m.stake = 0
		return
	}
	if err := c.Settle(p, int64(gross)); err != nil {
		if errors.Is(err, kit.ErrEconomyDisabled) {
			rm.economyOff = true
		}
		r.Log("pokies: settle failed: " + err.Error())
		m.stake = 0
		rm.refreshBalance(r, id)
		return
	}
	m.stake = 0
	rm.refreshBalance(r, id)
	rm.postPeak(r, id)
	rm.maybeRebuy(r, id)
}

// postPeak posts a new personal credits peak to the leaderboard (the board keeps
// each account's best). Posts only on improvement over the last posted value.
func (rm *room) postPeak(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil {
		return
	}
	p, ok := rm.names[id]
	if !ok || m.peak <= m.postedPeak {
		return
	}
	m.postedPeak = m.peak
	r.Post(kit.Result{Rankings: []kit.PlayerResult{{
		Player: p, Metric: int(m.peak), Status: kit.StatusFinished,
	}}})
}

// maybeRebuy triggers the platform broke-relief rebuy when the post-settle
// balance can no longer cover the lowest bet. A refusal (still solvent, or the
// daily limit reached) is rendered, not retried; the rebuy is not a peak.
func (rm *room) maybeRebuy(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil || m.credits >= int64(betTiers[0]) {
		return
	}
	c := r.Services().Credits
	p, ok := rm.names[id]
	if c == nil || !ok {
		return
	}
	bal, err := c.Buyback(p)
	if err != nil {
		return // ErrInsufficientCredits: solvent or daily limit — do not retry
	}
	m.credits = bal
	m.flash = "RE-BUY"
	m.flashUntil = r.Now().Add(flashDur)
	rm.clampBet(m)
}

// forceSettle settles any open stake for a seat leaving mid-round (voluntary
// leave, abandon, or room close): banks the known gross (the gamble at-risk take
// or the accumulated free-spin win) and otherwise books the committed bet as a
// loss with Settle(0), so escrow never leaks and a losing bet is never a free
// cancel. A no-op when no stake is open.
func (rm *room) forceSettle(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil || m.stake == 0 {
		return
	}
	gross := 0
	switch {
	case m.gamble != nil:
		gross = m.gamble.atRisk // the "take" value is the known gross
	case m.freeSpins > 0:
		gross = m.freeWin // free winnings accrued so far (a free spin has no risk)
	}
	rm.settle(r, id, gross)
	m.gamble = nil
	m.freeSpins = 0
	m.spin = nil
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.names[p.AccountID] = p
	if _, ok := rm.machines[p.AccountID]; ok {
		rm.render(r)
		return
	}
	m := &machine{bet: betTiers[0]}
	rm.machines[p.AccountID] = m
	// Seed the HUD from the player's account-wide balance; the posted-peak
	// watermark starts at the join balance so it is never itself posted.
	rm.refreshBalance(r, p.AccountID)
	m.postedPeak = m.peak
	rm.order = append(rm.order, p.AccountID)
	sx, sy := loungeSpawn()
	rm.pawns[p.AccountID] = &pawn{x: sx, y: sy, seat: -1}
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	m := rm.machines[p.AccountID]
	if m == nil {
		return
	}
	rm.forceSettle(r, p.AccountID) // settle any open stake before the seat vanishes
	delete(rm.machines, p.AccountID)
	delete(rm.names, p.AccountID)
	for i, id := range rm.order {
		if id == p.AccountID {
			rm.order = append(rm.order[:i], rm.order[i+1:]...)
			break
		}
	}
	if pw := rm.pawns[p.AccountID]; pw != nil && pw.seated {
		delete(rm.occupied, pw.seat)
	}
	delete(rm.pawns, p.AccountID)
	rm.render(r)
}

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	id := p.AccountID
	pw := rm.pawns[id]
	if pw == nil {
		return
	}
	act := kit.Resolve(in, kit.CtxNav)
	if !pw.seated {
		// Roaming: arrows move, Confirm sits.
		switch act {
		case kit.ActUp:
			rm.tryMove(id, 0, -1)
		case kit.ActDown:
			rm.tryMove(id, 0, +1)
		case kit.ActLeft:
			rm.tryMove(id, -1, 0)
		case kit.ActRight:
			rm.tryMove(id, +1, 0)
		case kit.ActConfirm:
			rm.trySit(id)
		}
		rm.render(r)
		return
	}
	// Seated: existing machine controls; Back stands up when idle.
	m := rm.machines[id]
	if m == nil {
		rm.render(r)
		return
	}
	switch {
	case act == kit.ActBack && m.spin == nil && m.gamble == nil && m.freeSpins == 0:
		rm.standUp(id)
	case m.gamble != nil:
		rm.gambleInput(r, id, act) // double-up ladder owns input
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
		if pw := rm.pawns[id]; pw == nil || !pw.seated {
			continue // only seated machines animate / auto-play
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
		for i := m.spin.landed; i < numReels; i++ {
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
	// Settle any open stakes so a room teardown never leaks escrow.
	for _, id := range rm.order {
		rm.forceSettle(r, id)
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

// clampBet drops the bet to the highest tier the cached balance can cover.
func (rm *room) clampBet(m *machine) {
	for int64(m.bet) > m.credits && tierIndex(m.bet) > 0 {
		m.bet = betTiers[tierIndex(m.bet)-1]
	}
}

// --- spinning ----------------------------------------------------------------

func (rm *room) startSpin(r kit.Room, p kit.Player) {
	m := rm.machines[p.AccountID]
	if m == nil || m.spin != nil || m.freeSpins > 0 || m.gamble != nil {
		return // auto-play owns the reels during a feature / gamble holds the win
	}
	c := r.Services().Credits
	if c == nil {
		rm.economyOff = true
		return // no economy: cabinet is out of service
	}
	rm.clampBet(m)
	// Wager the bet: escrow it into the seat's single open stake. Only start the
	// reels on success; a refusal (insufficient credits) tries the broke-relief
	// rebuy so a busted seat can spin on the next press, never leaving a stake open.
	if err := c.Wager(p, int64(m.bet)); err != nil {
		if errors.Is(err, kit.ErrInsufficientCredits) {
			rm.maybeRebuy(r, p.AccountID)
		} else if errors.Is(err, kit.ErrEconomyDisabled) {
			rm.economyOff = true
		}
		rm.refreshBalance(r, p.AccountID)
		return
	}
	m.stake = m.bet
	m.flash = ""
	rm.refreshBalance(r, p.AccountID) // reflect the escrow debit in the HUD

	// Pin the variant this spin starts under: a later config refresh never
	// re-evaluates an in-flight spin. The strip is its variant's strip, so a
	// seeded room reproduces outcomes for a given variant.
	v := m.seatVar
	if v == nil {
		v = rm.variant
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
	if m.spin.landed >= numReels {
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

	win := bet * v.waysPayout(scatterWindow(v.strip, m.lastIdx)) / wayScale

	if wasFree {
		// Free spin: NEVER wager, NEVER settle mid-feature. Accumulate the win onto
		// the ONE open stake (capped stake-relative), retrigger, and settle exactly
		// once in endFreeSpins when the feature runs out. Gamble is never offered
		// inside a feature.
		m.freeSpins--
		m.freeWin = capGross(m.freeWin+win, m.stake)
		if win > 0 {
			m.flash = fmt.Sprintf("WIN! +%d", win)
			m.flashUntil = r.Now().Add(flashDur)
		}
		rm.triggerFreeSpins(m, v, bet, false) // retrigger: keep the accumulator
		if win >= bet*tickerMult {
			rm.announce(r, id, win)
		}
		if m.freeSpins == 0 {
			rm.endFreeSpins(r, id) // the single Settle for the whole feature
		}
		rm.scheduleNextFree(r, m)
		return
	}

	// Base game. A spin can both pay a line and trigger free spins; on a trigger
	// the line win folds into the feature accumulation and the ONE open stake
	// stays open — settled once at endFreeSpins, never here.
	if award := rm.triggerFreeSpins(m, v, bet, true); award > 0 {
		m.freeWin = capGross(m.freeWin+win, m.stake)
		rm.announce(r, id, 0) // "X hit FREE SPINS!"
		rm.scheduleNextFree(r, m)
		return
	}

	if win > 0 {
		rm.enterGamble(r, m, win) // hold the win on the double-up ladder (settles at take/loss)
		m.flash = ""
		return
	}

	// No win, no feature: settle the open stake as a loss (Settle 0).
	m.flash = ""
	rm.settle(r, id, 0)
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
