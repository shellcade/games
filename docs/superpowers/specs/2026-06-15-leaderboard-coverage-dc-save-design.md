# Leaderboard coverage + durable disconnect/continuous save

**Date:** 2026-06-15
**Status:** Approved design, pending implementation plan
**Repos touched:** `kit` (helper + version bump), `games` (per-game adoption), `shellcade` (conformance guardrail + kit pin)

## Goal

Every game records to the leaderboard; player scores survive a mid-game
disconnect; and continuous ("never-ending") games flush periodically so an
abandoned world still records progress. Fix the recurring root cause — each game
hand-rolls its own `OnLeave` + `Post` + KV logic and gets it subtly wrong — with
a shared kit helper plus a conformance guardrail that stops future games
reshipping the bug.

## Background / current state

Leaderboards are already full platform infrastructure, **not** greenfield:

- Games call `Room.Post(Result)` (publish a result anytime) and
  `Room.End(Result)` (settle + close). The platform persists results durably
  into the `leaderboard_results` table — idempotent by round id, retried ~30s —
  aggregates per a per-game `LeaderboardSpec`, and renders them in the lobby
  (all-time / weekly / daily windows).
- A game declares its board via `Meta().Leaderboard` (`LeaderboardSpec`:
  `MetricLabel`, `Direction`, `Aggregation`, `Format`). Default if unset: best
  single result, higher-is-better, integer, label "Score".
- Disconnect handling exists: a 120s **seat grace** holds a departed seat; on
  expiry the game's `OnLeave(r, p)` fires. `End()` settlement auto-backfills DNF
  for joined players the game omitted.

### Hard constraints discovered in code (these shape the design)

1. **Boards rank `Post()`/`End()` results only.** Per-account KV
   (`MergeMax`/`MergeSum`) is for *session resume*, not the board. A game that
   only writes its "best" to KV never appears on the leaderboard unless it also
   `Post`s (or registers a custom `LeaderboardProvider`, which the catalog
   casino games do **not**).
   - `shellcade/internal/store/postgres/leaderboard.go`
2. **The Reader does NOT filter by `status`.** A `StatusDNF` row's metric is
   ranked exactly like a finished one (verified across BestResult + CumulativeSum
   and all-time/weekly/daily). Therefore a *partial* score posted as DNF still
   counts — fine for higher-is-better boards (max keeps the best), but
   **dangerous for lower-is-better** boards (a half-played round would top it).
3. **No wire/ABI change required.** The helper is pure SDK sugar over the
   existing `Post` + KV surface, so there is no wire-revision bump and no
   `kit/rust` wire sync obligation.

## Audit summary (the "audit first" deliverable)

18 games audited (16 on `origin/main` + 2 in worktrees).

### Sound — no change (8)

`bytebreaker`, `salvo`, `paperdrift`, `blackjack`, `floorfall`, `pokies`,
`scratchies`, `stacked`. Each declares a spec, Posts live (per-event / per-peak /
per-elimination), and flushes on `OnLeave`.

### Gaps to fix (8) + hardening (2)

| Game | Type | Problem | Fix |
|---|---|---|---|
| voidrunners | continuous | tracks kills in KV but **never `Post`s** — nothing reaches the board | adopt ScoreKeeper: Record on kill, FlushLeave on leave |
| chess | round | **no `LeaderboardSpec`**; winner's win never posted | declare spec (Wins, cumulative); Post +1 to winner on settle/forfeit |
| tic-tac-toe-rs | round (Rust) | **no `LeaderboardSpec`**; `End()` metric ignored | declare spec (Wins, cumulative); post win count |
| meltdown | continuous (worktree) | **never `Post`s** (KV only, on `OnClose`); no DC flush | adopt ScoreKeeper: periodic FlushAll + FlushLeave |
| neon-snake | continuous | **no `OnLeave`**; mid-game DC loses score (Posts only on crash) | add `OnLeave` → FlushLeave (DNF) |
| putt | round | **no KV / no `OnLeave` save**; mid-game DC loses everything | `OnLeave` → Post par-extrapolated total, DNF |
| spaceterm | continuous co-op | `OnLeave` doesn't flush; all-crew DC before core death loses run | FlushLeave current run score on `OnLeave` |
| boneyard | continuous | mid-run DC before death/collapse loses current depth | FlushLeave current banked depth on `OnLeave` |
| roulette | continuous | Posts peak-on-increase + wallet-on-leave; no periodic mid-spin flush | (harden) periodic FlushAll |
| shellracer | round | DNF reaches board only via `End()`; if all leave, no `End` | (harden) `End` on last-leave |

## Design

### Component 1 — `kit.ScoreKeeper` (Go)

New file `kit/internal/game/scorekeeper.go`, re-exported as `kit.ScoreKeeper` via
a type alias in `kit/kit.go` (the public facade where `Room`, `Result`,
`PlayerResult`, `MergeRule`, `LeaderboardSpec`, etc. already alias from
`internal/game`).

Responsibilities (all over existing `Room.Post` + KV):

- `Record(p Player, metric int64)` — track the player's current metric and
  `Post` it per a cadence policy (on-change, or on-improve for monotonic
  boards). Replaces ad-hoc "post when peak increased" blocks.
- `FlushLeave(r Room, p Player, status Status)` — `Post` the player's current
  tracked metric with the given status (normally `StatusDNF`). This is the
  disconnect guarantee; games call it from `OnLeave`.
- `FlushAll(r Room, status Status)` — `Post` every tracked player. Continuous
  games call this from `OnWake` on an interval to satisfy "constantly saved".
- Optional KV sugar `PersistBest` (`MergeMax`) / `PersistWallet`
  (`MergeSum` + `MergeMax`) for *resume* — folds the duplicated
  `persistWallet`/`persistBest` helpers (e.g. `scratchies/kv.go`) into one place.

Design notes:

- The keeper is **direction-agnostic**. Computing a *fair* partial metric for a
  DNF on a lower-is-better board is the caller's responsibility (see C3), and is
  documented on `FlushLeave`.
- Cadence policy is a small enum/option (`OnImprove` default for monotonic
  high-water boards; `OnChange` for live scores). Keeps existing sound games'
  behavior identical when they adopt it.
- The keeper holds no goroutines/timers; periodic flush is driven by the game's
  existing `OnWake` heartbeat so it stays deterministic for hibernation/replay.

### Component 2 — per-game fixes

Apply the table above. Sound games optionally migrate to the helper for
consistency but are not required to change behavior. Each gap game gets a unit
test asserting `OnLeave` during active play produces a `Post` (and, for
continuous games, that a periodic `FlushAll` posts without a player present).

### Component 3 — putt lower-is-better correctness

Putt's board is `Direction: LowerBetter` (strokes). On disconnect, do **not**
post the raw partial total. Instead post:

```
metric = strokesSoFar + par * unplayedHoles   // par-fill estimate
status = StatusDNF
```

a fair full-round estimate that cannot corrupt the strokes board. This is the
canonical example for the `FlushLeave` doc comment.

### Component 4 — Rust (tic-tac-toe-rs)

The Rust kit crate (`kit/rust/`) exposes `Room::post`/`Room::end`, `Leaderboard`
(== `LeaderboardSpec`), and `Status { Finished, Dnf }` (no `Flagged`, no helper
infrastructure). Scope is intentionally minimal — **no Rust ScoreKeeper**:

- Declare `Leaderboard { label: "Wins", direction: HigherBetter,
  aggregation: CumulativeSum, format: Integer }` in `Meta`.
- Ensure the settle paths post a win count (winner `1`, others `0`); the existing
  `on_leave` already settles a forfeit with `Status::Dnf`.

### Component 5 — conformance guardrail (platform)

In `shellcade`:

- **Static check:** a test iterating `sdk.Registry.All()` (filtered to non-hidden
  via `Listed()`) asserting every game declares `Meta().Leaderboard != nil`.
- **Behavioral check:** a verdict in `internal/gameabi/conformance` — drive a game
  into active play, fire `OnLeave`, and assert a `Post` (leaderboard result) is
  produced. This catches "declared a spec but never records" regressions.

## DNF semantics summary

| Board kind | Examples | On disconnect |
|---|---|---|
| HigherBetter / BestResult | survival, kills, peak, sectors, depth, score | Post partial as-is, DNF — safe (max keeps best) |
| CumulativeSum (wins) | chess, tic-tac-toe | leaver forfeits (no win); opponent posts +1 |
| LowerBetter | putt (strokes) | par-extrapolate to full-round estimate, DNF |

## Scope & branch strategy

- **kit:** add `ScoreKeeper` + re-export; minor version bump (no wire change). Tag
  the kit release before pinning it from games/platform (per project convention).
- **games:** one branch `leaderboard-coverage` off `origin/main` for the on-main
  games. **meltdown** is an unmerged worktree game, so its fix lands on its own
  branch `bcook/meltdown` (not folded into the main branch). **stacked** is
  already sound — no change.
- **shellcade:** conformance test + bump the pinned kit version.

## Testing

- **kit:** unit tests for `ScoreKeeper` — Record cadence, FlushLeave status,
  FlushAll with absent players, KV persist sugar.
- **games:** per-game test that `OnLeave` posts during active play; putt test that
  the DNF metric is par-extrapolated; a couple of live smokes (smoke harness) for
  a continuous and a round game.
- **shellcade:** static + behavioral conformance suite green over the registry.

## Out of scope

- ELO / skill rating (chess & tic-tac-toe use win count).
- New leaderboard UI / display changes.
- Any wire or ABI change.
- Migrating already-sound games beyond optional consistency cleanup.
