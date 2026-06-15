# Leaderboard Coverage + DC/Continuous-Save Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every Shellcade game records to the leaderboard, scores survive a mid-game disconnect, and continuous games flush periodically — built on a shared `kit.ScoreKeeper` helper plus a conformance guardrail.

**Architecture:** Add a small, timer-free `ScoreKeeper` to the `kit` SDK that standardises live posting, disconnect flush, and periodic flush over the existing `Room.Post` + per-account KV surface (no wire/ABI change). Adopt it in the 8 gap games + 2 hardening games; declare a `LeaderboardSpec` for the two unranked games (chess Go, tic-tac-toe Rust). Add static + behavioral conformance checks in the platform so future games can't reship the bug.

**Tech Stack:** Go (kit SDK + most games, TinyGo→wasm), Rust (tic-tac-toe-rs), PostgreSQL-backed leaderboard reader (platform). Repos: `/Users/bcook/dev/shellcade/{kit,games,shellcade}`.

**Spec:** `docs/superpowers/specs/2026-06-15-leaderboard-coverage-dc-save-design.md`

---

## File structure

**kit** (worktree, new branch `scorekeeper` off `kit` main):
- Create: `kit/internal/game/scorekeeper.go` — the helper (one responsibility: track + post metrics).
- Create: `kit/internal/game/scorekeeper_test.go` — unit tests using the `kittest` double.
- Modify: `kit/kit.go` — re-export `ScoreKeeper`, `NewScoreKeeper`, `Cadence`, `OnImprove`, `OnChange`.
- Modify: `kit/CHANGELOG.md` — minor version entry.

**games** (worktree `leaderboard-coverage` off `origin/main`; meltdown on `bcook/meltdown`):
- Modify each gap game's `room.go`/`main.go` + add a `*_leaderboard_test.go` per game.

**shellcade** (worktree, new branch `leaderboard-conformance` off `shellcade` main):
- Create: `shellcade/internal/sdk/leaderboard_conformance_test.go` — static "spec declared" check.
- Modify: `shellcade/internal/gameabi/conformance/conformance.go` (+ test) — behavioral "posts on leave" verdict.

---

## Phase 0 — Workspace setup

### Task 0: Local go.work so games resolve the local kit

**Files:**
- Create: `/Users/bcook/dev/shellcade/go.work` (uncommitted; lives above all repos)

- [ ] **Step 1: Create the kit worktree**

```bash
cd /Users/bcook/dev/shellcade/kit
git worktree add -b scorekeeper .worktrees/scorekeeper main
git -C .worktrees/scorekeeper rev-parse --short HEAD
```

- [ ] **Step 2: Create a go.work tying the kit worktree to the game modules**

```bash
cd /Users/bcook/dev/shellcade
go work init
go work use ./kit/.worktrees/scorekeeper
# add each game module as it is touched, e.g.:
go work use ./games/.worktrees/leaderboard/games/matt/voidrunners
```

Expected: `go.work` lists the kit worktree + game modules. This makes `go test`/`go build` resolve `github.com/shellcade/kit/v2` from the local worktree without editing any committed `go.mod`. (The committed version bump is the release step, Task 17.)

- [ ] **Step 3: Confirm it resolves**

Run: `cd /Users/bcook/dev/shellcade && go list -m github.com/shellcade/kit/v2`
Expected: points at the local `kit/.worktrees/scorekeeper` path.

> Note: `go.work` is intentionally NOT committed to any repo. Add `go.work*` to your local ignore if needed.

---

## Phase A — kit ScoreKeeper helper

### Task 1: ScoreKeeper core + Record cadence

**Files:**
- Create: `kit/.worktrees/scorekeeper/internal/game/scorekeeper.go`
- Test: `kit/.worktrees/scorekeeper/internal/game/scorekeeper_test.go`

- [ ] **Step 1: Read the kit test double to learn the fake Room API**

Run: `sed -n '1,120p' kit/.worktrees/scorekeeper/kittest/*.go` and inspect how existing tests construct a fake `Room`, capture `Post` calls, and build a `Player`. Mirror that exact construction in the test below (the names in the test stub here are placeholders to be matched to `kittest`).

- [ ] **Step 2: Write the failing test (Record with OnImprove only posts on a new high)**

```go
package game

import "testing"

func TestScoreKeeperRecordOnImprovePostsOnlyOnNewHigh(t *testing.T) {
	r := newFakeRoom(t)            // from kittest — match its real constructor
	p := fakePlayer("acct-1")     // from kittest
	sk := NewScoreKeeper(OnImprove)

	sk.Record(r, p, 10)           // first ever -> posts 10
	sk.Record(r, p, 5)            // lower -> no post
	sk.Record(r, p, 12)           // new high -> posts 12

	posts := r.Posts()            // captured Result slice
	if len(posts) != 2 {
		t.Fatalf("want 2 posts, got %d", len(posts))
	}
	if posts[0].Rankings[0].Metric != 10 || posts[1].Rankings[0].Metric != 12 {
		t.Fatalf("unexpected metrics: %+v", posts)
	}
	if posts[1].Rankings[0].Status != StatusFinished {
		t.Fatalf("live posts should be StatusFinished")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd kit/.worktrees/scorekeeper && go test ./internal/game/ -run TestScoreKeeperRecord -v`
Expected: FAIL (undefined `NewScoreKeeper`/`ScoreKeeper`).

- [ ] **Step 4: Write minimal implementation**

```go
package game

import (
	"context"
	"sort"
	"strconv"
	"sync"
)

// Cadence controls when Record auto-posts a player's metric.
type Cadence int

const (
	// OnImprove posts only when the new metric beats the last posted value —
	// for monotonic high-water boards (peak credits, best survival, kills).
	OnImprove Cadence = iota
	// OnChange posts whenever the metric changes.
	OnChange
)

// ScoreKeeper tracks each player's current leaderboard metric and standardises
// posting it live (Record), on disconnect (FlushLeave), and — for continuous
// games — periodically (FlushAll). It holds NO goroutines or timers: periodic
// flushing is driven by the game's own OnWake heartbeat so behaviour stays
// deterministic under hibernation/replay.
type ScoreKeeper struct {
	mu      sync.Mutex
	cadence Cadence
	cur     map[string]int64
	posted  map[string]int64
	players map[string]Player
}

// NewScoreKeeper returns a ScoreKeeper with the given auto-post cadence.
func NewScoreKeeper(c Cadence) *ScoreKeeper {
	return &ScoreKeeper{
		cadence: c,
		cur:     map[string]int64{},
		posted:  map[string]int64{},
		players: map[string]Player{},
	}
}

// Record updates the player's current metric and posts it per the cadence.
func (sk *ScoreKeeper) Record(r Room, p Player, metric int64) {
	sk.mu.Lock()
	sk.cur[p.AccountID] = metric
	sk.players[p.AccountID] = p
	last, seen := sk.posted[p.AccountID]
	should := !seen
	switch sk.cadence {
	case OnImprove:
		should = should || metric > last
	case OnChange:
		should = should || metric != last
	}
	if should {
		sk.posted[p.AccountID] = metric
	}
	sk.mu.Unlock()
	if should {
		r.Post(Result{Rankings: []PlayerResult{{Player: p, Metric: metric, Status: StatusFinished}}})
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd kit/.worktrees/scorekeeper && go test ./internal/game/ -run TestScoreKeeperRecord -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd kit/.worktrees/scorekeeper
git add internal/game/scorekeeper.go internal/game/scorekeeper_test.go
git commit -m "feat(scorekeeper): core type + Record cadence"
```

### Task 2: FlushLeave + FlushAll

**Files:**
- Modify: `kit/.worktrees/scorekeeper/internal/game/scorekeeper.go`
- Test: `kit/.worktrees/scorekeeper/internal/game/scorekeeper_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestScoreKeeperFlushLeavePostsDNF(t *testing.T) {
	r := newFakeRoom(t)
	p := fakePlayer("acct-1")
	sk := NewScoreKeeper(OnImprove)
	sk.Record(r, p, 7)            // posts 7 (Finished)
	sk.FlushLeave(r, p, StatusDNF)

	posts := r.Posts()
	last := posts[len(posts)-1].Rankings[0]
	if last.Metric != 7 || last.Status != StatusDNF {
		t.Fatalf("want metric=7 DNF, got %+v", last)
	}
	// Leaving again is a no-op (player untracked).
	before := len(r.Posts())
	sk.FlushLeave(r, p, StatusDNF)
	if len(r.Posts()) != before {
		t.Fatalf("flush after leave should be a no-op")
	}
}

func TestScoreKeeperFlushAllPostsAllSortedDeterministic(t *testing.T) {
	r := newFakeRoom(t)
	a, b := fakePlayer("acct-b"), fakePlayer("acct-a")
	sk := NewScoreKeeper(OnChange)
	sk.Record(r, a, 1)
	sk.Record(r, b, 2)
	r.ResetPosts()               // ignore live posts; assert FlushAll only
	sk.FlushAll(r, StatusDNF)

	posts := r.Posts()
	if len(posts) != 2 {
		t.Fatalf("want 2 posts, got %d", len(posts))
	}
	// Deterministic order: sorted by AccountID -> acct-a then acct-b.
	if posts[0].Rankings[0].Player.AccountID != "acct-a" ||
		posts[1].Rankings[0].Player.AccountID != "acct-b" {
		t.Fatalf("FlushAll must post in AccountID order, got %+v", posts)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd kit/.worktrees/scorekeeper && go test ./internal/game/ -run 'TestScoreKeeperFlush' -v`
Expected: FAIL (undefined `FlushLeave`/`FlushAll`).

- [ ] **Step 3: Implement**

```go
// FlushLeave posts the player's current tracked metric with the given status
// (normally StatusDNF) and stops tracking them. Call from OnLeave.
//
// IMPORTANT: the platform ranks DNF rows the same as finished ones. For a
// lower-is-better board, pass a fair full-run metric (e.g. par-extrapolated),
// not a raw partial, or a half-played run will top the board.
func (sk *ScoreKeeper) FlushLeave(r Room, p Player, status Status) {
	sk.mu.Lock()
	metric, ok := sk.cur[p.AccountID]
	delete(sk.cur, p.AccountID)
	delete(sk.posted, p.AccountID)
	delete(sk.players, p.AccountID)
	sk.mu.Unlock()
	if !ok {
		return
	}
	r.Post(Result{Rankings: []PlayerResult{{Player: p, Metric: metric, Status: status}}})
}

// FlushAll posts every tracked player's current metric with the given status,
// in deterministic AccountID order. Continuous games call this from OnWake on
// an interval so an abandoned world still records progress.
func (sk *ScoreKeeper) FlushAll(r Room, status Status) {
	sk.mu.Lock()
	ids := make([]string, 0, len(sk.cur))
	for id := range sk.cur {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	type row struct {
		p Player
		m int64
	}
	rows := make([]row, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, row{sk.players[id], sk.cur[id]})
		sk.posted[id] = sk.cur[id]
	}
	sk.mu.Unlock()
	for _, rw := range rows {
		r.Post(Result{Rankings: []PlayerResult{{Player: rw.p, Metric: rw.m, Status: status}}})
	}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd kit/.worktrees/scorekeeper && go test ./internal/game/ -run 'TestScoreKeeperFlush' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/game/scorekeeper.go internal/game/scorekeeper_test.go
git commit -m "feat(scorekeeper): FlushLeave + deterministic FlushAll"
```

### Task 3: KV persist sugar

**Files:**
- Modify: `kit/.worktrees/scorekeeper/internal/game/scorekeeper.go`
- Test: `kit/.worktrees/scorekeeper/internal/game/scorekeeper_test.go`

- [ ] **Step 1: Write failing test (PersistBest writes MergeMax int)**

```go
func TestScoreKeeperPersistBestWritesMergeMax(t *testing.T) {
	r := newFakeRoom(t)
	p := fakePlayer("acct-1")
	sk := NewScoreKeeper(OnImprove)
	sk.PersistBest(r, p, "best", 42)

	got, ok := r.KVOf("acct-1", "best")   // kittest accessor for stored KV + rule
	if !ok || string(got.Value) != "42" || got.Rule != MergeMax {
		t.Fatalf("want best=42 MergeMax, got %+v ok=%v", got, ok)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd kit/.worktrees/scorekeeper && go test ./internal/game/ -run TestScoreKeeperPersist -v`
Expected: FAIL (undefined `PersistBest`).

- [ ] **Step 3: Implement**

```go
// PersistBest writes a monotonic high-water value to the player's per-account KV
// (MergeMax) for session resume. The leaderboard board itself is fed by
// Record/FlushLeave/FlushAll; this only preserves state across reconnects.
func (sk *ScoreKeeper) PersistBest(r Room, p Player, key string, value int64) {
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return
	}
	_ = acct.Store().Set(context.Background(), key, []byte(strconv.FormatInt(value, 10)), MergeMax)
}

// PersistWallet writes a carryable balance (MergeSum) and a high-water peak
// (MergeMax). Replaces the duplicated persistWallet helpers in casino games.
func (sk *ScoreKeeper) PersistWallet(r Room, p Player, balanceKey string, balance int64, peakKey string, peak int64) {
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return
	}
	st := acct.Store()
	_ = st.Set(context.Background(), balanceKey, []byte(strconv.FormatInt(balance, 10)), MergeSum)
	_ = st.Set(context.Background(), peakKey, []byte(strconv.FormatInt(peak, 10)), MergeMax)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd kit/.worktrees/scorekeeper && go test ./internal/game/ -run TestScoreKeeperPersist -v`
Expected: PASS. Then run the whole package: `go test ./internal/game/...` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/game/scorekeeper.go internal/game/scorekeeper_test.go
git commit -m "feat(scorekeeper): PersistBest/PersistWallet KV sugar"
```

### Task 4: Re-export in kit.go + changelog

**Files:**
- Modify: `kit/.worktrees/scorekeeper/kit.go`
- Modify: `kit/.worktrees/scorekeeper/CHANGELOG.md`

- [ ] **Step 1: Add re-exports**

In `kit.go`, alongside the existing `type (...)` aliases (the block that aliases `Result`, `PlayerResult`, `MergeRule`, etc.):

```go
// ScoreKeeper standardises live/disconnect/periodic leaderboard posting.
type (
	ScoreKeeper = game.ScoreKeeper
	Cadence     = game.Cadence
)

const (
	OnImprove = game.OnImprove
	OnChange  = game.OnChange
)

// NewScoreKeeper constructs a ScoreKeeper with the given auto-post cadence.
func NewScoreKeeper(c Cadence) *ScoreKeeper { return game.NewScoreKeeper(c) }
```

- [ ] **Step 2: Add changelog entry**

Prepend a new minor version section to `CHANGELOG.md` (bump minor from current 2.10.0 → 2.11.0):

```markdown
## v2.11.0

- Add `ScoreKeeper` helper: standardises live (`Record`), disconnect
  (`FlushLeave`), and periodic (`FlushAll`) leaderboard posting plus
  `PersistBest`/`PersistWallet` KV sugar. No wire/ABI change.
```

- [ ] **Step 3: Build + vet the whole module**

Run: `cd kit/.worktrees/scorekeeper && go build ./... && go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add kit.go CHANGELOG.md
git commit -m "feat(scorekeeper): re-export public API; changelog v2.11.0"
```

> Release note (not a code step): the actual `git tag kit/v2.11.0` + push happens before games/platform pin (Task 17), per the kit-tag-before-pin convention.

---

## Phase B — per-game adoption (games repo)

> All Phase B tasks are in worktree `games/.worktrees/leaderboard` (branch
> `leaderboard-coverage`) EXCEPT Task 14 (meltdown) which is on `bcook/meltdown`.
> For each game: `go work use ./games/.worktrees/leaderboard/games/<author>/<game>`
> first so the local kit resolves. Each task: write a leaderboard test, make it
> pass, build the module, commit. The executor reads the game's current source to
> produce the exact diff — the steps below specify the precise behavior + test.

### Task 5: voidrunners — Post kills live + on leave

**Files:**
- Modify: `games/matt/voidrunners/room.go` (+ `main.go` if keeper is held on room struct)
- Test: `games/matt/voidrunners/voidrunners_leaderboard_test.go`

Audit: declares spec (Kills), tracks `best` in KV, but **never `Post`s**. `OnLeave` exists and persists best to KV.

- [ ] **Step 1: Write failing test** — drive a ship to a kill, fire `OnLeave`, assert a leaderboard `Post` with the kill count + `StatusDNF` was produced (capture via the game's test harness / fake room). Also assert a kill bumps a live `Post`.
- [ ] **Step 2: Run → FAIL** (`go test ./... -run Leaderboard -v`).
- [ ] **Step 3: Implement** — add `sk *kit.ScoreKeeper` (`NewScoreKeeper(kit.OnImprove)`) to the room; on each kill call `rm.sk.Record(r, p, best)`; in `OnLeave` call `rm.sk.FlushLeave(r, p, kit.StatusDNF)` (keep the existing KV persist or switch to `rm.sk.PersistBest`).
- [ ] **Step 4: Run → PASS**; then `go build ./...`.
- [ ] **Step 5: Commit** — `feat(voidrunners): post kills to leaderboard live + on disconnect`.

### Task 6: neon-snake — add OnLeave flush

**Files:**
- Modify: `games/luke/neon-snake/main.go`
- Test: `games/luke/neon-snake/neon_snake_leaderboard_test.go`

Audit: posts only on crash (`gameOver`); **no `OnLeave`** → mid-game DC loses score.

- [ ] **Step 1: Write failing test** — start a game, advance to a non-zero score, fire `OnLeave` for a player, assert a `Post` with current score + `StatusDNF`.
- [ ] **Step 2: Run → FAIL**.
- [ ] **Step 3: Implement** — add a `ScoreKeeper(kit.OnImprove)`; call `Record` where the score updates (or just before the existing crash `Post`); add an `OnLeave(r, p)` that calls `FlushLeave(r, p, kit.StatusDNF)` and persists PB (`PersistBest`).
- [ ] **Step 4: Run → PASS**; `go build ./...`.
- [ ] **Step 5: Commit** — `feat(neon-snake): flush score on disconnect`.

### Task 7: spaceterm — flush run score on OnLeave

**Files:**
- Modify: `games/bcook/spaceterm/room.go`
- Test: `games/bcook/spaceterm/spaceterm_leaderboard_test.go`

Audit: co-op; persists best to KV + `Post`s only in `endRun()` (core death). If all crew DC before death, run lost. `OnLeave` removes crew but doesn't flush.

- [ ] **Step 1: Write failing test** — board a crew member, advance `rm.score`, fire `OnLeave`, assert a `Post` with the shared run score + `StatusDNF` (and KV best updated).
- [ ] **Step 2: Run → FAIL**.
- [ ] **Step 3: Implement** — in `OnLeave`, before removing the crew member, `FlushLeave(r, p, kit.StatusDNF)` with `rm.score` (and `PersistBest` "best"). Keep `endRun()` posting for finishers. Use a keeper seeded via `Record(r, c.player, rm.score)` when score changes, OR flush directly with the current run score (co-op shared metric).
- [ ] **Step 4: Run → PASS**; `go build ./...`.
- [ ] **Step 5: Commit** — `feat(spaceterm): flush run score on crew disconnect`.

### Task 8: boneyard — flush banked depth on OnLeave

**Files:**
- Modify: `games/bcook/boneyard/game.go` (`OnLeave`) and/or `bones.go`
- Test: `games/bcook/boneyard/boneyard_leaderboard_test.go`

Audit: resident roguelike; `Post`s on death + collapse grace-bank. Mid-run DC before death/collapse → current depth not banked. `OnLeave` only sets `d.online=false`.

- [ ] **Step 1: Write failing test** — seat a delver, descend (raise `banked`/current depth), fire `OnLeave`, assert a `Post` with the delver's banked depth + `StatusDNF`.
- [ ] **Step 2: Run → FAIL**.
- [ ] **Step 3: Implement** — in `OnLeave`, when a delver has progress, `Post` their banked depth with `StatusDNF` (reuse the existing death-post path with DNF status) in addition to marking `online=false`. Keep the run in-world for rejoin.
- [ ] **Step 4: Run → PASS**; `go build ./...`.
- [ ] **Step 5: Commit** — `feat(boneyard): bank current depth on disconnect`.

### Task 9: putt — par-extrapolated DNF on OnLeave

**Files:**
- Modify: `games/bcook/putt/room.go` (`OnLeave`)
- Test: `games/bcook/putt/putt_leaderboard_test.go`

Audit: `LowerBetter` (Strokes); only `End()` after hole 9; **`OnLeave` deletes golfer, no save**. Lower-better + unfiltered DNF means a raw partial would top the board → must par-extrapolate.

- [ ] **Step 1: Write failing test** — golfer plays N of 9 holes with K strokes, disconnects; assert `Post` metric == `K + par*(9-N)` and `StatusDNF`. Include a guard test: a golfer who has played 0 holes posts the full par-round estimate (not 0).
- [ ] **Step 2: Run → FAIL**.
- [ ] **Step 3: Implement** — in `OnLeave`, before deleting the golfer, compute `est = strokesSoFar + parTotalOfUnplayedHoles` and `r.Post(kit.Result{Rankings: []kit.PlayerResult{{Player: p, Metric: est, Status: kit.StatusDNF}}})`. Derive per-hole par from the existing course definition.
- [ ] **Step 4: Run → PASS**; `go build ./...`.
- [ ] **Step 5: Commit** — `feat(putt): record par-extrapolated DNF on disconnect`.

### Task 10: chess — declare spec + post wins

**Files:**
- Modify: `games/alan/chess/game.go` (`Meta()`), `games/alan/chess/room.go` (settle/forfeit paths)
- Test: `games/alan/chess/chess_leaderboard_test.go`

Audit: **no `LeaderboardSpec`**; `End()` only, winner's win never reaches a board.

- [ ] **Step 1: Write failing test** — assert `Meta().Leaderboard` is non-nil with `MetricLabel:"Wins"`, `Direction:HigherBetter`, `Aggregation:CumulativeSum`, `Format:Integer`. Then play a game to checkmate (and separately a forfeit-by-leave) and assert the winner gets a result with `Metric:1` and the loser `Metric:0` / `StatusDNF`.
- [ ] **Step 2: Run → FAIL**.
- [ ] **Step 3: Implement** — add the `Leaderboard` spec to `Meta()`; in `finishGame`/settle and the `OnLeave` forfeit path, set winner `Metric:1` and loser `Metric:0`. (`End(Result)` already carries rankings; just ensure metrics encode wins.)
- [ ] **Step 4: Run → PASS**; `go build ./...`.
- [ ] **Step 5: Commit** — `feat(chess): declare wins leaderboard + post win counts`.

### Task 11: roulette — periodic FlushAll (hardening)

**Files:**
- Modify: `games/alan/roulette/room.go` (`OnWake`)
- Test: `games/alan/roulette/roulette_leaderboard_test.go`

Audit: continuous; posts peak-on-increase + persists wallet on leave; no periodic flush for a mid-spin abandoned player.

- [ ] **Step 1: Write failing test** — seat a player, raise peak, simulate the room ticking with the player abandoned mid-spin (no peak change), assert a periodic `Post` of the current peak occurs on the wake interval.
- [ ] **Step 2: Run → FAIL**.
- [ ] **Step 3: Implement** — add a `ScoreKeeper(kit.OnImprove)` fed by the existing `postPeak`; on a throttled `OnWake` interval (e.g. every N seconds of game time) call `sk.FlushAll(r, kit.StatusFinished)`. Keep wallet persistence as-is.
- [ ] **Step 4: Run → PASS**; `go build ./...`.
- [ ] **Step 5: Commit** — `feat(roulette): periodic peak flush for abandoned tables`.

### Task 12: shellracer — End on last-leave (hardening)

**Files:**
- Modify: `games/bcook/shellracer/shellracer/room.go` (`OnLeave`)
- Test: `games/bcook/shellracer/shellracer/shellracer_leaderboard_test.go`

Audit: round-based; `OnLeave` snaps DNF WPM and calls `enterResults` only if `allDone`. If everyone leaves mid-race, no `End`, nothing posts.

- [ ] **Step 1: Write failing test** — two racers racing; both leave; assert the room reaches `End` with both as `StatusDNF` (their snapped WPM), i.e. results are flushed when the last racer leaves.
- [ ] **Step 2: Run → FAIL**.
- [ ] **Step 3: Implement** — in `OnLeave`, after marking DNF, if `phRacing` and no racers remain (roster empty of players), call `enterResults(r)` (which leads to `End`). Guard against double-end.
- [ ] **Step 4: Run → PASS**; `go build ./...`.
- [ ] **Step 5: Commit** — `feat(shellracer): settle race when the last racer disconnects`.

### Task 13: tic-tac-toe-rs — declare spec + post wins (Rust)

**Files:**
- Modify: `games/bcook/tic-tac-toe-rs/src/lib.rs` (`Meta`), `games/bcook/tic-tac-toe-rs/src/game.rs` (settle paths)
- Test: `games/bcook/tic-tac-toe-rs/src/game.rs` (`#[cfg(test)]`) or `tests/`

Audit: **no `Leaderboard`**; `end()` carries metrics already; `on_leave` settles forfeit with `Status::Dnf`.

- [ ] **Step 1: Write failing test** — assert `Meta` declares `Leaderboard { label:"Wins", direction:HigherBetter, aggregation:CumulativeSum, format:Integer }`; play to a win and assert the winner's `PlayerResult.metric == 1`, loser `0`.
- [ ] **Step 2: Run → FAIL** (`cargo test`).
- [ ] **Step 3: Implement** — add the `Leaderboard` to `Meta`; ensure `settle_win`/`settle_forfeit`/`settle_draw` set winner metric `1`, others `0`.
- [ ] **Step 4: Run → PASS**; `cargo build --release` (and the wasm target the game ships, e.g. `cargo build --target wasm32-unknown-unknown --release`).
- [ ] **Step 5: Commit** — `feat(tic-tac-toe): declare wins leaderboard + post win counts`.

### Task 14: meltdown — Post survival live + on leave (branch bcook/meltdown)

**Files:**
- Modify: `games/bcook/meltdown/room.go` (in worktree `games/.claude/worktrees/agent-a235e4c859c78fe48`, branch `bcook/meltdown`)
- Test: `games/bcook/meltdown/meltdown_leaderboard_test.go`

Audit: continuous co-op; declares spec (Survival) but **never `Post`s** (KV only, in `OnClose`); no DC flush.

- [ ] **Step 1:** `go work use` the meltdown module path. Write failing test — board crew, advance survival seconds, fire `OnLeave`, assert a `Post` of survival seconds + `StatusDNF`; and that a periodic `OnWake` flush posts while crew are present.
- [ ] **Step 2: Run → FAIL**.
- [ ] **Step 3: Implement** — add `ScoreKeeper(kit.OnImprove)`; `Record(r, p, survivedSeconds)` for each crew on score change; periodic `FlushAll(r, kit.StatusFinished)` on a throttled `OnWake`; `FlushLeave(r, p, kit.StatusDNF)` in `OnLeave`; keep `recordResults` in `OnClose`.
- [ ] **Step 4: Run → PASS**; `go build ./...`.
- [ ] **Step 5: Commit on `bcook/meltdown`** — `feat(meltdown): post survival live + on disconnect`.

---

## Phase C — platform conformance (shellcade repo)

> Worktree: `shellcade/.worktrees/leaderboard-conformance` (branch off `shellcade` main).
> Create it: `cd /Users/bcook/dev/shellcade/shellcade && git worktree add -b leaderboard-conformance .worktrees/leaderboard-conformance main`.

### Task 15: Static check — every listed game declares a spec

**Files:**
- Create: `shellcade/.worktrees/leaderboard-conformance/internal/sdk/leaderboard_conformance_test.go`

- [ ] **Step 1: Write failing test**

```go
package sdk_test

import (
	"testing"
	// import the registry constructor used by other sdk tests
)

func TestAllListedGamesDeclareLeaderboard(t *testing.T) {
	reg := testRegistry(t)            // match the helper other sdk tests use
	for _, g := range reg.Listed() {  // Listed() excludes Hidden
		if g.Meta().Leaderboard == nil {
			t.Errorf("game %q must declare Meta().Leaderboard", g.Meta().Slug)
		}
	}
}
```

- [ ] **Step 2: Run → FAIL** for any game still missing a spec (confirms the check works), `go test ./internal/sdk/ -run TestAllListedGamesDeclareLeaderboard -v`.
- [ ] **Step 3: Implement** — no production code; this check passes once Phase B games declare specs. If the registry under test pins old game wasm, note the dependency: it goes green after the games are rebuilt/pinned (Task 17).
- [ ] **Step 4: Run → PASS** (after games updated).
- [ ] **Step 5: Commit** — `test(leaderboard): assert all listed games declare a spec`.

### Task 16: Behavioral verdict — OnLeave during play posts a result

**Files:**
- Modify: `shellcade/.worktrees/leaderboard-conformance/internal/gameabi/conformance/conformance.go`
- Test: `shellcade/.worktrees/leaderboard-conformance/internal/gameabi/conformance/conformance_test.go`

- [ ] **Step 1: Read** `conformance.go` to learn how it loads a game, drives Join/Wake/Input, and records verdicts.
- [ ] **Step 2: Write failing test** — load a game, Join two players, drive into active play, fire a Leave for one, assert the conformance run records a `Post` (leaderboard result) within the seat-grace/Leave path. Add the verdict to `Report.Verdicts`.
- [ ] **Step 3: Run → FAIL**.
- [ ] **Step 4: Implement** the verdict: capture `Post` calls during the Leave step; verdict passes if ≥1 result was posted for a game whose `Meta().Leaderboard != nil` and that was in active play. (Round-based 2-player games that settle via `End` count as posting.)
- [ ] **Step 5: Run → PASS**; commit — `feat(conformance): verify games post on player leave`.

---

## Phase D — release + integration

### Task 17: Tag kit, pin from games + platform, full build

**Files:**
- Modify: each touched game's `go.mod` (`games/<author>/<game>/go.mod`) — `require github.com/shellcade/kit/v2 v2.11.0`
- Modify: `shellcade` go.mod pin if it references kit directly

- [ ] **Step 1: Tag + push kit**

```bash
cd /Users/bcook/dev/shellcade/kit/.worktrees/scorekeeper
# merge/PR scorekeeper to kit main per project flow, then:
git tag kit/v2.11.0 && git push origin kit/v2.11.0
```

- [ ] **Step 2: Bump each touched game module**

For each game touched in Phase B:
```bash
cd games/.worktrees/leaderboard/games/<author>/<game>
go get github.com/shellcade/kit/v2@v2.11.0
go mod tidy
```

- [ ] **Step 3: Remove the local go.work** (so builds use the pinned version) and rebuild each game module: `go build ./...` (+ the wasm build the publish pipeline uses).
- [ ] **Step 4: Bump platform kit pin** if applicable and run `go build ./... && go test ./...` in shellcade.
- [ ] **Step 5: Commit** the go.mod bumps in games (one commit) and shellcade.

### Task 18: Full verification sweep

- [ ] **Step 1:** `cd kit/.worktrees/scorekeeper && go test ./...` → PASS.
- [ ] **Step 2:** For every touched game module: `go test ./... && go build ./...` (Rust: `cargo test && cargo build --release`) → PASS.
- [ ] **Step 3:** `cd shellcade/.worktrees/leaderboard-conformance && go test ./internal/sdk/... ./internal/gameabi/...` → PASS (both conformance checks green).
- [ ] **Step 4:** Live smoke (smoke skill): boot `serve --dev`, play one continuous game (e.g. voidrunners) and one round game (e.g. putt), disconnect mid-game, verify a score row appears on the lobby leaderboard.
- [ ] **Step 5:** Final commit / open PRs per repo (kit, games, shellcade; meltdown separately).

---

## Self-review notes

- **Spec coverage:** C1 helper → Tasks 1–4; C2 per-game → Tasks 5–14; C3 putt extrapolation → Task 9; C4 Rust → Task 13; C5 conformance → Tasks 15–16; scope/branch + release → Task 0/14/17. All spec sections mapped.
- **DNF semantics:** higher-better games pass raw partials (Tasks 5–8,14); cumulative wins pass 1/0 (Tasks 10,13); lower-better putt par-extrapolates (Task 9). Consistent with the spec table.
- **Type consistency:** `ScoreKeeper`, `NewScoreKeeper(Cadence)`, `OnImprove`/`OnChange`, `Record(r,p,metric)`, `FlushLeave(r,p,status)`, `FlushAll(r,status)`, `PersistBest`, `PersistWallet` used identically across Phase A definitions and Phase B call sites.
- **Open dependency:** the kittest fake-room accessor names (`Posts()`, `ResetPosts()`, `KVOf`) are placeholders — Task 1 Step 1 matches them to the real `kittest` API before writing tests.
