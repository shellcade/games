# Pokies Lounge — walkable resident floor (B) + 5-reel 243-ways engine (A)

**Date:** 2026-06-21
**Author:** bcook (with Claude)
**Game:** `games/bcook/pokies` (kit v2.9.0)
**Builds on:** the wild/scatter/gamble feature pass (PR #60 branch)
**Status:** Approved design, ready for implementation plans

## Goal

Turn pokies into a shared, walkable **resident lounge**: you roam a scrolling
floor as your arcade character, see other players roaming and seated, walk up to
the **themed machine you want**, sit down, and play a full-screen **5-reel,
243-ways** Aussie pokie (carrying the wild / scatter free-spins / gamble features
already built). Stand up and walk to another.

## Decomposition & ordering

Two subsystems, built and shipped in order as two stacked PRs:

- **PR2 — B: the resident scrolling floor.** Restructures the room from "five
  fixed side-by-side cabinets" into "one shared scrolling floor + per-player mode
  (roaming ↔ seated)". The seated view **reuses the existing 3-reel engine**
  (balance / bet / spin / wild / scatter / gamble), themed by the machine sat at.
- **PR3 — A: the 5-reel 243-ways engine.** Swaps the seated machine's engine and
  layout to 5×3 / 243-ways with closed-form exact RTP, plus 6 themed PAR sheets.

The floor/mode scaffolding (B) is engine-agnostic, so B-then-A minimizes rework.
Each sub-project gets its own implementation plan.

---

## B — The Pokies Lounge (resident scrolling floor)

### B.1 Lifecycle & scale

- `Lifecycle: kit.LifecycleResident` — one persistent lounge per slug (ticks with
  zero players, periodic checkpoints, boot auto-restore). Until the platform
  grants residency it behaves as resumable; `MinPlayers` stays 1 (required).
- `CtxFeatures: kit.CtxFeatCharacter | kit.CtxFeatRosterEpoch` and
  `MaxPlayers: 32` for a shared floor without per-callback O(members) cost.
- `HeartbeatMS` left default; movement and reel animation are wake/clock driven.

### B.2 Map model

- A bounded tile grid, target **~60×30**, larger than the 80×24 viewport.
- Tiles: floor, wall, entrance (spawn), decor (BAR, EXIT signage). Walls bound
  the room and form aisles.
- **6 machine objects** placed around the floor, each a 2-wide footprint bound to
  a themed variant (see A.6). A machine occupies wall-adjacent tiles you approach
  from the open side.
- The map is authored as a static layout (a `[]string` ASCII template compiled to
  tiles at startup), so it is deterministic and trivially testable.

### B.3 Player floor-state

Per player (room state, keyed by account id):
`{ x, y int; facing dir; mode roaming|seated; seat machineID }`.
On join the player spawns at the entrance in `roaming`. Wallet is the existing
durable per-account store; floor position is **not** persisted (you re-enter at
the door), occupancy **is** room state.

### B.4 Movement

- **Discrete step per arrow-key press** (`ActUp/Down/Left/Right`): one tile per
  input event, with wall/machine/occupant **collision** (blocked moves are
  no-ops). Event-driven so it is hibernation-stable — no per-wake velocity
  accumulator.
- Facing updates with the last move direction (used to pick which machine you're
  facing when you press Confirm).

### B.5 Camera & rendering

- Viewport (80×24 minus a title/status chrome row) centered on the player,
  **clamped** to map bounds so edges don't scroll past the wall.
- Render only the visible window: floor/walls/decor, machine icons with short
  theme labels, and every player in view.
- **Avatars are the player's arcade character tile** (`kit.CharacterCell`) — a
  hard requirement, never a generic glyph. The viewer's own character is
  highlighted; each visible player's **name** is labelled adjacent to their tile
  (truncated to fit). A machine's seated occupant renders **their character tile**
  at the machine.
- Alloc discipline: reuse the package frame; precompute static map cells; no
  per-render slice growth (matches `alloc_test.go`).

### B.6 Seating

- Walk **adjacent to and facing** a machine, press `Confirm` → `mode = seated`,
  `seat = machineID`, bound to that machine's variant. If the machine is already
  occupied, the move/sit is refused (it shows the occupant).
- `Back` (Esc) while seated → stand: `mode = roaming`, released seat, placed back
  on the machine's approach tile.
- **Exclusive: one seat per machine.** A machine renders its occupant's character
  tile + name so the floor can see who's playing what.

### B.7 Mode dispatch

`OnInput` and `compose` branch on the viewer's mode:
- **roaming** → movement input; floor render (camera window).
- **seated** → the existing machine input (bet/spin/gamble) and the seated
  machine render (B reuses today's cabinet, A replaces it).
Frames are already per-viewer, so a roaming player and a seated player in the
same room receive different screens from the same render pass.

### B.8 Persistence

- Wallet: unchanged (balance summed, peak max-merged), leaderboard on new peak.
- Resident checkpoints: the room's durable state is occupancy + machine identity;
  player positions reset to the entrance on (re)join, which is the natural "walk
  back in" behavior. No new KV schema is required beyond the existing wallet.

### B.9 Files

- New `floor.go` — tile map, movement + collision, camera, floor render.
- New `seat.go` — mode transitions (sit/stand), occupancy.
- `room.go` — slims to lifecycle + mode dispatch; the machine/spin/gamble/
  freespins logic is reused unchanged.
- `layout.go` — gains the floor/camera draw; the seated machine draw is reused.

### B.10 Testing

Movement + collision (walls, machines, occupants, bounds), camera clamping at
edges, **avatar = character tile** assertions for self/others/occupant, seat
exclusivity + sit/stand transitions, mode-dispatch (roaming vs seated input and
render), resident-lifecycle meta encode, alloc-free floor render.

---

## A — 5-reel, 243-ways engine (seated machine)

### A.1 Grid & win model

- 5×3 visible grid. **243 ways** (3⁵): a symbol pays when it appears on
  **adjacent reels starting from reel 1**, on **any rows**; the pay is
  `ways × multiplier`, where `ways` = the product of that symbol's visible count
  on each reel in the run. Fixed bet tiers (no line-selection UI).
- **Wild** substitutes for paying symbols within a ways-run; an all-wild leftmost
  run pays the top multiplier. Wild never substitutes for scatter.
- **Scatter** counts across all **15 cells**; thresholds award free spins, which
  retrigger — same model as today, widened from 9 to 15 cells.
- Free spins (auto-play, pinned variant) and the gamble ladder carry over
  unchanged from the 3-reel engine.

### A.2 Reel model

Each themed machine keeps a **single weighted strip** (all 5 reels draw i.i.d.
from it), extended with the existing even WILD/SCATTER distribution. Per-reel
independence + identical distribution is what makes the RTP closed form tractable.

### A.3 Exact RTP without `strip⁵`

`strip⁵` (~17M+) is infeasible to enumerate per compile. Instead:

1. Enumerate the strip **once** to get, per symbol `s`, the exact distribution of
   `countWindow(s)` — the number of `s` (counting wilds as substitutes) visible in
   one reel's 3-cell window — and likewise the scatter-count distribution per
   reel.
2. Combine analytically across 5 i.i.d. reels:
   - **Ways EV** in closed form: for each paying symbol, sum over run length
     `L=3..5` of `multiplier(L) × E[ways for a run of exactly L]`, computed from
     the per-reel count distribution (`E[count·1(count≥1)]` chained for the run,
     times the probability the run stops at `L`).
   - **Scatter trigger rate** by convolving the per-reel scatter-count
     distribution across 5 reels to get `P(total ≥ threshold)`.
3. Fold free spins into total RTP with the existing branching-process closed form
   (`m = avgAward/(1 − t·avgAward)`, `RTP_total = RTP_line·(1 + t·m)`).

`compileVariant` keeps the `[10%, 150%]` total-RTP band and the
retrigger-convergence gate, now over the closed-form numbers. A **Monte-Carlo
sampling cross-check** (in tests) validates the closed form to within tolerance.

### A.4 Seated layout

Full 80×24: a large 5×3 reel window (five width-2 face columns), the paytable,
balance/bet/feature readouts, and the existing gamble / free-spin overlays scaled
up. The roaming floor never shows reels; the seated screen never shows the floor.

### A.5 Wins, gamble, free spins

Unchanged mechanics: a base-game win enters the gamble ladder (Red/Black ×2, Suit
×4, take/cap); 3+ scatters auto-play free spins that retrigger; the room-wide
ticker announces big wins and feature triggers, naming the player with their
character tile.

### A.6 Themes

`themes.go` defines **6 named PAR sheets** (e.g. Lucky 7s, Gem Rush, Bells,
Cherry Pop, Crown, Gift Drop), each a distinct variant (weights / paytable /
scatter table / gamble caps) tuned into the RTP band. Logical symbols and face art
are **shared** across themes (themes differ by odds + name in v1); the floor binds
one theme per machine. `defaultDoc` is replaced by this set; admin config can
still override per key.

### A.7 Files

- `variant.go` — 5-reel ways evaluation + closed-form `stats()`.
- New `themes.go` — the 6 PAR sheets + the machine→theme binding.
- `layout.go` — the seated 5×3 render (replaces the 3-reel seated draw).

### A.8 Testing

243-ways truth tables (runs of length 3/4/5, wild completion, no-win), scatter
count over 15 cells, **closed-form RTP cross-checked against Monte-Carlo
sampling** on small strips (within tolerance), convergence/band rejection, seated
5×3 render, theme compile (all 6 in band), gamble/free-spin carry-over on 5 reels.

---

## Risks & mitigations

- **Closed-form ways RTP correctness** — the highest-risk item; mitigated by a
  Monte-Carlo cross-check test and hand-checked small cases.
- **Resident grant** — residency only activates when the platform grants it;
  ungranted it behaves as resumable, which is harmless for review/play.
- **Glyph width** — any new floor/decor glyphs follow the established
  unanimously-wide (or plain ASCII) discipline; avatars are width-1 character
  tiles (ABI-admitted).
- **Hibernation stability** — discrete movement + wake-derived clocks keep the
  resident room byte-stable across checkpoints.
- **Scope** — two PRs, each independently shippable and tested; B leaves a
  working walkable floor even before A lands.

## Out of scope (later)

Per-theme distinct face art, progressive jackpots, multiple seats per machine,
minimap/overview, NPC/ambient life, chat.
