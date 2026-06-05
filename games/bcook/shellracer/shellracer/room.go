package shellracer

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	kit "github.com/shellcade/kit"
)

const (
	countdownDur = 10 * time.Second
	resultsDur   = 15 * time.Second
	stragglerDur = 60 * time.Second
	graceWindow  = 30 * time.Second
	afkTimeout   = 25 * time.Second
	maxRaceDur   = 4 * time.Minute
	echoCap      = 240

	configRefresh = 30 * time.Second
)

// phases — internal state only. The native game published these to the host via
// SetPhase to drive joinability and a phase banner; the lean ABI has no phase
// surface (joinability is host-derived), so these stay private and SetPhase is
// dropped. They still drive the game's own state machine and rendering.
const (
	phLobby     = "lobby"
	phCountdown = "countdown"
	phRacing    = "racing"
	phResults   = "results"
)

// defaultFlagThreshold is the anti-cheat hook: a net WPM above this is
// "physically impossible" and the result is flagged. The native game read the
// real value from private env (SHELLCADE_TR_FLAG_WPM); the wasm sandbox has no
// env, so it is read from per-game config (key "flag-wpm") with this open-source
// default — high enough to never flag a legitimate human.
const defaultFlagThreshold = 100000

type cell struct {
	r   rune
	err bool
}

// pstate is one racer's live typing state. Keyed by account id (hibernation
// safe), never by connection.
type pstate struct {
	cursor      int
	errorsTotal int
	outstanding int
	typed       []cell
	lastKey     time.Time
	status      kit.Status // statusPlaying == still typing
	statusSet   bool       // distinguishes "playing" from a real terminal status
	wpmSnap     int
	joinOrder   int
}

func (ps *pstate) playing() bool { return !ps.statusSet }

type room struct {
	kit.Base
	cfg     kit.RoomConfig
	svc     kit.Services
	passage []rune
	ptext   string
	pdiff   string

	phase   string
	st      map[string]*pstate // account id -> state (hibernation-safe key)
	order   []string           // join order of account ids
	names   map[string]kit.Player
	joinSeq int

	flagThreshold int
	nextCfg       time.Time

	raceStart   time.Time
	firstFinish time.Time
	lastNow     time.Time

	// Wake-driven deadlines (the native engine's After timers held in guest
	// memory; the zero value means "not armed").
	graceDeadline     time.Time // quick solo fallback
	countdownDeadline time.Time // countdown -> racing
	raceCapDeadline   time.Time // racing cap -> results
	stragglerDeadline time.Time // first finish -> results
	resultsDeadline   time.Time // results hold -> end

	result   kit.Result
	resultOK bool

	frame *kit.Frame // reused render scratch (allocation-light steady state)
}

func newRoom(cfg kit.RoomConfig, svc kit.Services) *room {
	return &room{
		cfg:           cfg,
		svc:           svc,
		st:            map[string]*pstate{},
		names:         map[string]kit.Player{},
		phase:         phLobby,
		flagThreshold: defaultFlagThreshold,
		frame:         kit.NewFrame(),
	}
}

func (rm *room) OnStart(r kit.Room) {
	p := pickPassage(r.Rand())
	rm.ptext = p.Text
	rm.pdiff = p.Difficulty
	rm.passage = []rune(p.Text)
	rm.loadConfig(r)
	rm.nextCfg = r.Now().Add(configRefresh)
	r.SetInputContext(kit.CtxNav) // pre-race lobby/countdown: q/Esc backs out
}

// loadConfig reads the anti-cheat flag threshold from per-game config. A
// missing or unparsable value keeps the compiled default, mirroring the native
// env fallback.
func (rm *room) loadConfig(r kit.Room) {
	rm.flagThreshold = defaultFlagThreshold
	blob, ok, err := r.Services().Config.Get(context.Background(), "flag-wpm")
	if err != nil || !ok {
		return
	}
	if n, err := strconv.Atoi(strings.TrimSpace(string(blob))); err == nil {
		rm.flagThreshold = n
	}
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.names[p.AccountID] = p
	if _, ok := rm.st[p.AccountID]; !ok {
		rm.st[p.AccountID] = &pstate{joinOrder: rm.joinSeq, lastKey: r.Now()}
		rm.order = append(rm.order, p.AccountID)
		rm.joinSeq++
	}

	if rm.phase == phRacing || rm.phase == phResults {
		rm.render(r)
		return // late arrival just watches; matchmaker shouldn't route here
	}
	switch rm.cfg.Mode {
	case kit.ModeSolo:
		rm.enterRacing(r)
	default: // quick / private
		switch {
		case rm.cfg.Capacity > 0 && r.Count() >= rm.cfg.Capacity:
			rm.enterRacing(r)
		case r.Count() >= 2 && rm.phase == phLobby:
			rm.startCountdown(r)
		case r.Count() == 1 && rm.cfg.Mode == kit.ModeQuick:
			rm.graceDeadline = r.Now().Add(graceWindow)
		}
	}
	rm.render(r)
}

func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	if ps := rm.st[p.AccountID]; ps != nil && ps.playing() {
		ps.status = kit.StatusDNF
		ps.statusSet = true
		ps.wpmSnap = rm.netWPM(ps, r.Now())
	}
	delete(rm.names, p.AccountID)
	if rm.phase == phRacing && rm.allDone(r) {
		rm.enterResults(r)
	}
	rm.render(r)
}

func (rm *room) startCountdown(r kit.Room) {
	rm.phase = phCountdown
	rm.graceDeadline = time.Time{}
	rm.countdownDeadline = r.Now().Add(countdownDur)
}

func (rm *room) enterRacing(r kit.Room) {
	if rm.phase == phRacing {
		return
	}
	rm.phase = phRacing
	rm.graceDeadline = time.Time{}
	rm.countdownDeadline = time.Time{}
	rm.raceStart = r.Now()
	rm.raceCapDeadline = r.Now().Add(maxRaceDur)
	r.SetInputContext(kit.CtxText) // typing the passage: letters incl. q/j/k are literal
}

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	if rm.phase == phResults {
		if in.Kind == kit.InputKey && in.Key == kit.KeyEnter {
			rm.finish(r)
		}
		return
	}
	if rm.phase != phRacing {
		return // pre-race keystrokes dropped
	}
	ps := rm.st[p.AccountID]
	if ps == nil || !ps.playing() {
		return
	}
	ps.lastKey = r.Now()

	switch {
	case in.Kind == kit.InputKey && in.Key == kit.KeyBackspace:
		if ps.outstanding > 0 {
			ps.outstanding--
		} else if ps.cursor > 0 {
			ps.cursor--
		}
		if n := len(ps.typed); n > 0 {
			ps.typed = ps.typed[:n-1]
		}
	case in.Kind == kit.InputRune && in.Rune >= 0x20:
		isErr := ps.outstanding > 0 || ps.cursor >= len(rm.passage) || in.Rune != rm.passage[ps.cursor]
		rm.pushEcho(ps, in.Rune, isErr)
		if !isErr {
			ps.cursor++
			if ps.cursor >= len(rm.passage) {
				rm.markFinished(r, ps)
			}
		} else {
			ps.errorsTotal++
			ps.outstanding++
		}
	default:
		// non-printable, non-backspace keys are ignored
	}

	if rm.allDone(r) {
		rm.enterResults(r)
	}
	rm.render(r)
}

func (rm *room) pushEcho(ps *pstate, ru rune, isErr bool) {
	ps.typed = append(ps.typed, cell{r: ru, err: isErr})
	if len(ps.typed) > echoCap {
		ps.typed = ps.typed[len(ps.typed)-echoCap:]
	}
}

func (rm *room) markFinished(r kit.Room, ps *pstate) {
	ps.status = kit.StatusFinished
	ps.statusSet = true
	ps.wpmSnap = rm.netWPM(ps, r.Now())
	if rm.firstFinish.IsZero() {
		rm.firstFinish = r.Now()
		rm.stragglerDeadline = r.Now().Add(stragglerDur)
	}
}

// OnWake is the host heartbeat: it advances every time-driven element by
// comparing guest-held deadlines against r.Now(), then renders once. This
// replaces the native engine's After timers and OnTick simulation callback.
func (rm *room) OnWake(r kit.Room) {
	now := r.Now()
	changed := false

	// Slow-cadence config refresh (anti-cheat threshold), mirroring native.
	if !rm.nextCfg.IsZero() && now.After(rm.nextCfg) {
		rm.loadConfig(r)
		rm.nextCfg = now.Add(configRefresh)
	}

	switch rm.phase {
	case phLobby:
		// Quick solo fallback: a lone quick player races after the grace window.
		if !rm.graceDeadline.IsZero() && now.After(rm.graceDeadline) &&
			rm.cfg.Mode == kit.ModeQuick && r.Count() == 1 {
			rm.enterRacing(r)
			changed = true
		}
	case phCountdown:
		if !rm.countdownDeadline.IsZero() && now.After(rm.countdownDeadline) {
			rm.enterRacing(r)
			changed = true
		}
	case phRacing:
		// AFK timeout (was the native OnTick sweep): a racer idle past the
		// timeout is dropped as DNF.
		for _, ps := range rm.st {
			if ps.playing() && now.Sub(ps.lastKey) > afkTimeout {
				ps.status = kit.StatusDNF
				ps.statusSet = true
				ps.wpmSnap = rm.netWPM(ps, now)
				changed = true
			}
		}
		switch {
		case rm.allDone(r):
			rm.enterResults(r)
			changed = true
		case !rm.stragglerDeadline.IsZero() && now.After(rm.stragglerDeadline):
			rm.enterResults(r)
			changed = true
		case !rm.raceCapDeadline.IsZero() && now.After(rm.raceCapDeadline):
			rm.enterResults(r)
			changed = true
		}
	case phResults:
		if !rm.resultsDeadline.IsZero() && now.After(rm.resultsDeadline) {
			rm.finish(r)
			return
		}
	}

	// During racing the live WPM/countdown clocks change every heartbeat, so
	// always repaint; otherwise repaint only on a state change.
	if rm.phase == phRacing || rm.phase == phCountdown || changed {
		rm.render(r)
	}
}

// allDone reports whether no live member is still playing.
func (rm *room) allDone(r kit.Room) bool {
	live := 0
	for _, p := range r.Members() {
		if ps := rm.st[p.AccountID]; ps != nil && ps.playing() {
			live++
		}
	}
	return live == 0 && len(r.Members()) > 0
}

func (rm *room) netWPM(ps *pstate, now time.Time) int {
	mins := now.Sub(rm.raceStart).Seconds() / 60
	if now.Sub(rm.raceStart) < time.Second || ps.cursor == 0 || mins <= 0 {
		return 0
	}
	return int(float64(ps.cursor) / 5 / mins)
}

func (rm *room) enterResults(r kit.Room) {
	if rm.phase == phResults {
		return
	}
	rm.phase = phResults
	rm.raceCapDeadline = time.Time{}
	rm.stragglerDeadline = time.Time{}
	now := r.Now()

	// snapshot any remaining live players as dnf
	for _, p := range r.Members() {
		if ps := rm.st[p.AccountID]; ps != nil && ps.playing() {
			ps.status = kit.StatusDNF
			ps.statusSet = true
			ps.wpmSnap = rm.netWPM(ps, now)
		}
	}
	rm.result = rm.buildResult(r)
	rm.resultOK = true
	r.SetInputContext(kit.CtxNav) // results screen: q/Esc backs out, Enter advances
	rm.resultsDeadline = now.Add(resultsDur)
}

func (rm *room) finish(r kit.Room) {
	if rm.resultOK {
		r.End(rm.result)
	} else {
		r.End(kit.Result{})
	}
}

// buildResult ranks finishers (by net WPM desc) above DNF players, applies the
// anti-cheat flag hook, and is iterated in a stable order (sorted account ids,
// not map order) so the rankings — and thus every frame and the leaderboard
// post — are deterministic under hibernation.
func (rm *room) buildResult(r kit.Room) kit.Result {
	type entry struct {
		id string
		ps *pstate
	}
	var es []entry
	for _, id := range rm.sortedIDs() {
		es = append(es, entry{id, rm.st[id]})
	}
	sort.SliceStable(es, func(i, j int) bool {
		fi := es[i].ps.status == kit.StatusFinished && es[i].ps.statusSet
		fj := es[j].ps.status == kit.StatusFinished && es[j].ps.statusSet
		if fi != fj {
			return fi // finishers first
		}
		if es[i].ps.wpmSnap != es[j].ps.wpmSnap {
			return es[i].ps.wpmSnap > es[j].ps.wpmSnap
		}
		return es[i].ps.joinOrder < es[j].ps.joinOrder
	})
	res := kit.Result{}
	for i, e := range es {
		status := e.ps.status
		if !e.ps.statusSet {
			status = kit.StatusDNF
		}
		if status == kit.StatusFinished && e.ps.wpmSnap > rm.flagThreshold {
			status = kit.StatusFlagged
		}
		res.Rankings = append(res.Rankings, kit.PlayerResult{
			Player: rm.playerFor(e.id),
			Metric: e.ps.wpmSnap,
			Rank:   i + 1,
			Status: status,
		})
	}
	return res
}

// sortedIDs returns every known account id (current or departed) in a
// deterministic order: join order, then account id as a tiebreak.
func (rm *room) sortedIDs() []string {
	ids := make([]string, 0, len(rm.st))
	for id := range rm.st {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		oi, oj := rm.st[ids[i]], rm.st[ids[j]]
		if oi.joinOrder != oj.joinOrder {
			return oi.joinOrder < oj.joinOrder
		}
		return ids[i] < ids[j]
	})
	return ids
}

// playerFor reconstructs a Player for an account id. A departed player is no
// longer in the roster; fall back to a member token built from the id so the
// ranking still names them.
func (rm *room) playerFor(id string) kit.Player {
	if p, ok := rm.names[id]; ok {
		return p
	}
	return kit.Player{AccountID: id, Handle: id, Kind: kit.KindMember}
}
