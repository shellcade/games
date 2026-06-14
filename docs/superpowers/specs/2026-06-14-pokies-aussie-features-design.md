# Pokies — authentic Aussie machine features (PR1)

**Date:** 2026-06-14
**Author:** bcook (with Claude)
**Game:** `games/bcook/pokies` (kit v2.9.0)
**Status:** Approved design, ready for implementation plan

## Goal

Make the existing pokies machine *feel* like a real Australian poker machine by
adding the three features that define the experience, without changing the
multiplayer floor (five 3-reel cabinets side-by-side on the 80×24 canvas):

1. **Wild substitutions** — a WILD face completes the best payline win.
2. **Scatter free spins** — 3+ SCATTER faces anywhere trigger auto-played free
   spins that can retrigger.
3. **Gamble ladder** — after a base-game win, gamble it Red/Black (×2) or by
   Suit (×4), repeatedly, up to a cap, or take the win.

Explicitly **out of scope** (deferred to PR2): the walkable resident floor,
avatars, sit-down, multiple physical themed machines, 5 reels / ways-to-win.

## Background — what exists today

- Each player owns a 3-reel, single-payline cabinet; up to 5 render
  side-by-side (15 cols each) on the fixed 80×24 canvas.
- A single weighted **virtual strip** of five symbols (`7 $ * B C`); each reel
  draws independently and uniformly from it (`Rand().Intn(len(strip))`).
- Only **three-of-a-kind on the center payline** pays. RTP is computed exactly
  by enumerating all `strip³` outcomes in `variant.stats()`.
- Odds are an admin-tunable "PAR sheet" config (`weights` + `paytable`),
  validated to an RTP band `[10%, 150%]` in `compileVariant`.
- Durable wallet (balance summed, peak max-merged), peak leaderboard, room-wide
  big-win ticker, per-player character tiles. Wake-driven animation; a spin
  pins the variant it started under.

The design preserves all of this. The three features layer onto the same
single-strip, exact-enumeration model.

## Design

### 1. Symbol set & faces

Add two symbols to the existing five:

| Symbol | Byte | Face (proposed) | Role |
|--------|------|-----------------|------|
| WILD    | `W` | 👑 U+1F451 | Substitutes on the payline |
| SCATTER | `S` | 🎁 U+1F381 | Counts anywhere; triggers free spins |

Both are single-codepoint, unambiguously East-Asian-**Wide** emoji — selected
with the same care as the fullwidth-7 incident (no contested width across
runewidth / uniseg / x/ansi / real terminals). Add to `faceArt`, `stripOrder`,
`symbolByName`, and the `asciiFallback` map (👑→`W`, 🎁→`S`) for non-UTF-8
sessions.

### 2. Evaluation model

Scoring moves from "center 3 faces" to reading the **full 3×3 window** (already
derivable from each reel's `stopIdx` via `windowAt`).

**Wild** (payline / center row only):
- Collect the three center faces. Let the non-wild faces among them be the
  "anchor" set.
- If all non-wild faces are equal to some symbol `s` (and ≥1 is non-wild),
  the line pays `triples[s]` (wilds count toward the three).
- If all three are WILD, the line pays the **top** configured multiplier.
- WILD never substitutes for SCATTER.

**Scatter** (anywhere): count SCATTER faces across all 9 visible cells. A count
≥ the lowest configured scatter-trigger threshold awards free spins. Scatter is
**trigger-only** in v1 (no separate scatter line-pay).

### 3. Strip layout — approach B (approved)

`buildStripFrom` distributes WILD and SCATTER stops **evenly** across the strip
rather than grouping them (as the regular symbols remain grouped). Even spacing
gives scatters a natural spread (a reel window of 3 shows a realistic 0/1/2
scatter count instead of clumping 0-or-3), while keeping a single strip so
`stats()` stays an exact `strip³` enumeration.

Determinism is preserved: the distribution is a pure function of the weights
(no RNG), so a seeded room still reproduces draws. The exact interleave
algorithm (e.g. evenly-spaced insertion by ideal position) will be pinned by a
golden test.

### 4. RTP math (extends `stats()` and the validation gate)

The `strip³` enumeration already yields base **line RTP** and hit frequency.
Extend each enumerated outcome to also compute the 3×3 window and:

- `t` = P(scatter count in the window ≥ trigger threshold), the share of
  outcomes that trigger free spins.

Fold free spins into total RTP with the standard branching-process closed form
(`F` = spins awarded per trigger; retrigger probability during a free spin
equals `t`, same reels):

```
m         = F / (1 − t·F)            # expected free spins per trigger
RTP_total = RTP_line · (1 + t·m)     # free spins pay line RTP at no cost
```

New gates in `compileVariant`:
- Reject **t·F ≥ 1** (non-converging retrigger / money printer).
- Apply the existing `[10%, 150%]` band to **RTP_total** (not just line RTP).

The admin form / paytable summary surfaces: line RTP, trigger rate `t`, average
free spins per trigger `m`, and total RTP.

> If multiple scatter thresholds with different `F` are configured (e.g. 3→8,
> 4→15), `t` and the awarded `F` become threshold-dependent. v1 keeps this
> tractable by computing `t` and expected `F` from the enumerated count
> distribution; the closed form uses the **expected** award. The convergence
> guard uses the worst-case (largest) `F`.

### 5. Free spins — behaviour & layout

- 3/4/… scatters → award `F` spins from a config table (`{count, spins}` rows,
  highest matching threshold wins).
- **Auto-played** by the existing wake clock: one spin per settle cycle, paying
  at the **triggering bet**, costing nothing. Reuses the existing staggered
  reel-landing animation.
- **Retrigger**: 3+ scatters during a free spin adds `F` more (capped to a sane
  ceiling to bound a session).
- Cabinet shows `FREE N` in place of `BET`; the border recolors gold so the
  floor can see who is in a feature. The room-wide ticker announces the trigger
  (`X hit FREE SPINS!`).
- Gamble is **disabled** during free spins; the feature total credits directly.

### 6. Gamble ladder — flow & layout

- After a **base-game** line win, the win is held *at risk* and the machine
  enters `gamble` state (free-spin wins are not gambled in v1).
- **Per-viewer rendering** (frames are already per-viewer): the owner sees the
  interactive selector; other viewers see a compact read-only `🎲 +N` on that
  cabinet.
- Selector inside the cabinet footprint:
  - Row 1: `[TAKE] [RED] [BLACK]` (×2 guesses)
  - Row 2: `[♠ ♥ ♦ ♣]` (×4 guesses)
  - `←/→/↑/↓` move the highlight, `SPACE`/Confirm locks the highlighted choice.
- On lock: deal a card via `r.Rand()`. Correct → win grows (×2 or ×4) and the
  machine re-enters the ladder; wrong → forfeit the at-risk win (flash
  `GAMBLED AWAY`). `TAKE` banks the current win and exits.
- **Cap**: auto-take after a rung cap (5) or a max-credit ceiling (config),
  whichever first.
- The gamble is a fair deal (RTP-neutral), so it does **not** touch the RTP
  gate.

#### Input routing

`OnInput` currently resolves under `CtxNav` (Up/Down = bet, Confirm = spin).
Add a per-machine mode: when `gamble` is active, route input to the gamble
handler (highlight move + lock + take) instead of bet/spin. Free-spin auto-play
ignores input except leave. Document the mode in the cabinet so the controls
line reflects the active mode.

### 7. Config schema changes (`config.go`)

Extend `oddsVariantSchema` and `defaultVariantJSON`:
- `weights` gains `W` and `S` (non-negative integers).
- New `scatter` array: rows of `{count ≥ 3, spins ≥ 1}` (the trigger table).
- New `gamble` object: `{maxRungs, maxWin}` (cap config), with defaults.
- `paytable` unchanged in shape; WILD's top-payout is derived (no explicit
  `W` paytable row needed — three wilds pay the top configured multiplier).

Defaults tuned so `RTP_total` lands near today's ~75% profile. `compileVariant`
remains the final word: shape in the schema, all semantic gates in code.

### 8. Components & file layout

Work stays within the existing package, extending current files rather than
adding broad new structure (the files are small and focused):

- `variant.go` — new symbols, `buildStripFrom` interleave (approach B), wild +
  scatter evaluation, extended `stats()` (line RTP, `t`, `m`, total RTP),
  new validation gates, paytable/summary display of feature stats.
- `room.go` — free-spin state machine (award, auto-play, retrigger, credit),
  gamble state machine (at-risk win, ladder, cap, card deal), input routing by
  mode, ticker announcement for feature triggers.
- `layout.go` — gold free-spin border + `FREE N` readout, gamble selector
  (owner) / compact indicator (others), scatter/wild faces, controls line per
  mode.
- `config.go` — extended schema + default doc + feature-stat surfacing.
- New faces in `faceArt` / `asciiFallback`.

If `room.go` or `layout.go` grows unwieldy, split the gamble and free-spin
state machines into their own files (`gamble.go`, `freespins.go`) — decide
during implementation based on file size.

### 9. Testing (TDD throughout)

Matches the existing heavy `pokies_test.go` coverage:
- **Strip interleave** — golden test pinning the deterministic WILD/SCATTER
  distribution for representative weights.
- **Wild evaluation** — truth table: `7 W 7`, `W W B`, `W W W`, `W $ 7` (no
  win), etc.
- **Scatter counting** — counts across the full 3×3 window; trigger thresholds.
- **RTP math** — exact `RTP_line`, `t`, `m`, `RTP_total` for the default and a
  couple of hand-checkable variants; band + `t·F ≥ 1` rejection.
- **Free spins** — award, auto-play credits at triggering bet, retrigger,
  retrigger ceiling, no balance deduction.
- **Gamble** — fairness over many seeded deals (×2 ≈ 50%, ×4 ≈ 25%), ladder
  growth, rung cap, max-win ceiling, take banks the win, loss forfeits.
- **Alloc** — no per-render allocations on the new draw paths (matches
  `alloc_test.go` discipline under `-gc=leaking`).
- **Config** — default JSON parses/compiles identical to the compiled default;
  schema accepts/rejects the right documents.

### 10. Risks & mitigations

- **Emoji width** — the #1 historical hazard. Verify 👑/🎁 render width-2
  everywhere before committing; swap if contested. Keep ASCII fallbacks.
- **15-col gamble UI** — tight. Validate the selector fits the cabinet; the
  compact non-owner indicator avoids overflowing other viewers' frames.
- **RTP closed form with mixed thresholds** — keep v1 to a single or
  expected-`F` model (documented above); add a test that the enumerated mean
  free-spin payout matches the closed form on a small variant.
- **Scope** — three features in one PR. Mitigate with incremental, independently
  tested commits (symbols+strip → wild → scatter/free-spins → gamble →
  config/admin surface), each green before the next.
