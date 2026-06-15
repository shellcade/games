# Spaceterm — design spec

A frantic, real-time, co-operative bridge-crew game for the terminal. Each
crewmate stares at a panel of absurdly named controls; orders stream in that
usually refer to controls on *someone else's* panel; the crew yells (or hails)
across the void to keep the hull together long enough to clear another sector.

An homage to the party game *Spaceteam* (Sleeping Beast Games), rebuilt from
scratch for an 80×24 terminal: original name, original control vocabulary, no
assets or text reused. "Spaceteam" is their trademark and is not used anywhere
in-game.

- **Slug:** `spaceterm` · **Namespace:** `games/bcook/spaceterm/`
- **Players:** 1–6 (sweet spot 3–4) · **Lifecycle:** ephemeral · **Language:** Go (TinyGo / wasip1)
- **Kit:** shellcade-kit v2.10.0+ (per-player frames, touch-deck control chips, lobby pattern)
- Screens referenced as **AB-1 … AB-8** are in [ARTBOARDS.md](ARTBOARDS.md).

---

## 1. Design pillars

1. **Panic responsibly.** Pressure comes from short timers and split attention,
   never from twitch dexterity. Every individual action is trivial — one
   keypress — so all difficulty lives in *routing information between humans*.
2. **The comms channel is the game.** The UI never tells you whose panel a
   control is on. Finding out is the gameplay. Voice (co-located or call) is
   the intended mode; the built-in HAIL system is the remote-play fallback and
   is deliberately lower-bandwidth than shouting.
3. **Readable at a glance.** One order, one timer, one panel. No scrolling, no
   modes to memorize. A new player can board mid-party and contribute in the
   first sector.
4. **Co-op all the way down.** Shared hull, shared warp progress, shared
   failure. No per-player elimination — the ship lives or dies as one.

## 2. Glossary

| Term | Meaning |
|---|---|
| **Control** | One widget on a panel (switch / dial / slider / button) with a generated technobabble name, unique across the whole ship. |
| **Panel** | The 6–8 controls owned by one crewmate. Only the owner can actuate them. |
| **Order** | A demand shown to one crewmate ("SET THE GYROSCOPIC PLURALIZER TO 4") with a countdown. Targets any control on the ship. |
| **Hail** | Broadcasting your current order to every crewmate's comms ticker. |
| **Warp charge** | One completed order. Fill the warp bar to clear the sector. |
| **Hull** | Shared ship HP (max 10). Zero = run over. |
| **Anomaly** | A timed sector hazard that disrupts panels or demands a group response. |
| **Sector** | One difficulty step. Endless progression; score = sectors cleared. |

## 3. Game flow

```
LOBBY ──launch──▶ SECTOR n ──warp bar full──▶ WARP JUMP ──▶ SECTOR n+1 …
                     │
                     └─hull = 0──▶ DEBRIEF ──[SPACE]──▶ LOBBY (same room)
```

### 3.1 Lobby (AB-1)

Follows the salvo gather pattern. Joiners muster on a crew roster (character
tiles via `CtxFeatCharacter`). Any crewmate can:

- **◂ ▸** — cycle difficulty: `CADET` / `CAPTAIN` / `ADMIRAL` (shared setting,
  last writer wins, change is broadcast).
- **SPACE** — launch immediately.

A 20 s fallback timer auto-launches once ≥1 player is aboard (matches salvo's
`lobbyWait`). Solo launch goes straight to Solo shift rules (§11).

### 3.2 Sector loop (AB-2, AB-3)

On sector start every crewmate receives a freshly generated panel and one
order. Orders are independent and concurrent — exactly one active order per
crewmate at all times:

- **Completed** (the named control reaches the demanded state, regardless of
  who caused it): +1 warp charge, the order's owner immediately draws a new
  order. Brief green flash on the order box.
- **Expired** (timer hits zero): −1 hull, red flash + screen-edge damage tick
  on all crew frames, owner draws a new order.

When the warp bar fills, all pending orders are cancelled (no penalty) and the
warp jump interstitial plays (§3.3). When hull reaches 0, the run ends (§3.4).

### 3.3 Warp jump (AB-7)

~4 s non-interactive interstitial: starfield streaks, sector summary (orders
completed, hull patched **+2**, next sector name). New panels are generated and
dealt; crew who joined mid-sector board here (§15).

### 3.4 Debrief (AB-8)

Shows sectors cleared (the score), a crew log (orders completed, hails sent,
orders fumbled per crewmate — flavor only, never ranked as winners/losers),
and the room's personal best. **SPACE** returns everyone to the lobby of the
same room for another shift.

## 4. The panel

### 4.1 Layout & hotkeys

A panel is a 2×3 (sectors 1–2) or 2×4 (sector 3+) grid of widgets, hotkeyed to
the physical key block:

```
[W] [E] [R] [T]
[S] [D] [F] [G]
```

(The block sits one column right of the corner: the platform's canonical
vocabulary reserves `q` as Back in every non-text input context, so a `Q`
hotkey would eject players from the room. Sector play runs in `CtxCommand`,
where Esc/`q` leave and every other rune is the game's.)

Pressing a hotkey actuates that control directly — **no cursor, no
navigation**. This is the core terminal adaptation: the frantic feel comes
from scanning labels and stabbing one key, not from menu traversal. Arrow-key
navigation + SPACE is supported as a fallback for the same actions (a thin
selection ring), primarily so touch users without a keyboard row can play via
chips (§12).

### 4.2 Control types

| Type | States | Actuation (hotkey press) | Render |
|---|---|---|---|
| **SWITCH** | OFF / ON | toggle | `●───╴ ON` / `╶───○ OFF` |
| **DIAL** | 0…4 (5 positions) | cycle +1, wraps to 0 | `( 2 )` + `○─○─●─○─○` |
| **SLIDER** | 1…4 levels | cycle +1, wraps to 1 | `▰▰▱▱  LVL 2/4` |
| **BUTTON** | momentary | press (briefly lit) | `( PRESS )` |

Wrong actuations cost nothing directly — but they *change ship state*, so
mashing a dial past its demanded value means cycling all the way around again.
Self-inflicted chaos is authentic and intended.

### 4.3 Name generation

Names are `ADJECTIVE + NOUN` drawn from two original wordlists (~64 each,
e.g. *Gyroscopic Pluralizer*, *Ferrous Holospindle*, *Subspace Crouton*),
sampled **without replacement across the entire ship** so every order is
unambiguous. Sector 5+ adds suffix variants (`MK-II`, `(AUX)`) to stretch the
pool and force more careful reading. All sampling uses the room-seeded RNG
(§17). Wordlists must pass the repo content policy: silly, never crude.

## 5. Orders

### 5.1 Generation & routing

When a crewmate needs a new order, the generator picks:

1. **Target control:** uniform over all ship controls **except** (a) controls
   already targeted by another pending order, and (b) with 2+ crew, the
   owner's own panel is down-weighted so ~70–75 % of orders target someone
   else's panel (solo: always own panel).
2. **Demanded state:** uniform over the control's states *excluding its
   current state* (orders are never pre-satisfied). Buttons demand a press.
3. **Phrasing:** template by type — `SET THE <name> TO <n>` (dial/slider),
   `ENGAGE THE <name>` / `DISENGAGE THE <name>` (switch), `PLUCK THE <name>`
   (button). A small synonym pool (`CRANK`, `EASE`) is mixed in at sector 4+
   for flavor.

### 5.2 Timing

Per-order countdown, rendered as a 40-cell bar that drains green → amber
(<50 %) → red (<25 %):

```
T(sector, difficulty) = max(5.0 s, base − 0.5 s × (sector − 1))
base: CADET 13 s · CAPTAIN 11 s · ADMIRAL 9 s
```

### 5.3 Resolution

An order completes the instant its control reaches the demanded state — even
accidentally, even by panel-owner experimentation. There is no "claim" step.
Buttons complete on the next press after the order was issued.

## 6. Comms & the HAIL system

- **H = HAIL.** Broadcasts your current order verbatim to the 2-line comms
  ticker at the bottom of *every* crew frame (including yours, as
  confirmation), tagged with your name and age (`alan ▸ SET THE FERROUS
  HOLOSPINDLE TO 2 · 3s`).
- **Cooldown 2 s** per player (shown on the hint line). Ticker holds the 3
  most recent hails; entries auto-clear when their order resolves.
- A hail does **not** reveal whose panel the control is on — receivers still
  have to scan their own panel. Bandwidth and ambiguity are the point.
- Voice chat (couch / call) remains the intended premium experience; the
  README will say so. HAIL keeps fully-remote and silent play viable.

## 7. Sectors & difficulty curve

| Sector | Order timer (CAPTAIN) | Controls/panel | Warp charges needed | Anomalies | New wrinkle |
|---|---|---|---|---|---|
| 1 | 11.0 s | 6 | `4 + 3×crew` | — | tutorial-calm |
| 2 | 10.5 s | 6 | `5 + 3×crew` | 1 | anomalies begin |
| 3 | 10.0 s | 8 | `5 + 4×crew` | 1 | 8-control panels |
| 4 | 9.5 s | 8 | `6 + 4×crew` | 1 | synonym phrasing |
| 5 | 9.0 s | 8 | `6 + 4×crew` | 2 | name suffixes (MK-II) |
| 6+ | −0.5 s/sector (floor 5 s) | 8 | `6 + 4×crew` | 2 | — |

Anomalies fire at a seeded random point between 30 % and 70 % of the sector's
warp progress. All constants above are provisional; tune in playtesting
(tracked in §19).

## 8. Anomalies

Each anomaly is telegraphed by a 3 s flashing banner (`⚠ INBOUND: …`) before
its effect lands. One anomaly type per event, seeded-random, no immediate
repeats.

| Anomaly | Effect | Counterplay | Failure cost |
|---|---|---|---|
| **METEOR STORM** (AB-4) | Orders suspended; every crewmate must mash a personally assigned hotkey ×12 within 4 s. | Mash. | −1 hull per crewmate who misses (cap −2 per storm). |
| **SOLAR FLARE** (AB-5) | All control *labels* render as static (`▒▒▒`) for 6 s; orders keep flowing. | Spatial memory; hail more. | None beyond expiries it causes. |
| **WORMHOLE TRANSIT** (AB-6) | Panel layout is mirrored left↔right for 6 s. Hotkeys stay bound to their controls — only the drawing moves. | Read the key badges, not muscle memory. | None beyond expiries. |
| **COOLANT LEAK** | Two controls per panel fog over (`░░`); a fogged control cannot be actuated until its owner wipes it (3 presses of its hotkey). | Wipe before it's needed. | None beyond expiries. |

Solo shift draws only METEOR STORM and SOLAR FLARE (mirroring and fog are
boring without comms pressure).

## 9. Hull, warp, scoring

- **Hull:** shared, max 10, starts 10. −1 per expired order, anomaly penalties
  per §8, **+2 on each warp jump** (clamped). Rendered in the top status strip
  on every frame.
- **Warp:** charges per §7, rendered as a labeled bar (`WARP ▰▰▰▱… 6/16`).
- **Score:** sectors *cleared* (a run that dies in sector 5 scores 4).
- **Leaderboard:** `MetricLabel: "Sectors"`, `HigherBetter`, `BestResult`,
  `Integer`. The score posts identically for every crew member via `r.End()` —
  co-op means everyone banks the same number.

## 10. Multiplayer model

Every crewmate gets a **private frame** (`r.Send(player, frame)`) showing
their own panel and order; shared elements (status strip, comms, warp/hull)
are identical across frames. One `OnWake` heartbeat (~100 ms) drives all
timers against `r.Now()`.

## 11. Solo shift

Same game, one chair: all orders target your own panel, HAIL is disabled
(hidden from hints/chips), warp charges use the table with `crew = 1` plus
+4 (so sector 1 = 11 charges), and only the solo-safe anomalies fire. The
score posts to the same leaderboard — solo grinding a high sector should feel
proud, not cheesy, because the timers are identical.

## 12. Input map & touch-deck chips

| Input | Context | Action |
|---|---|---|
| `w e r t s d f g` | sector | actuate that control |
| `h` | sector (2+ crew) | hail current order |
| arrows + SPACE | lobby / sector | difficulty (lobby); fallback select+actuate (sector) |
| SPACE | lobby / debrief | launch / new shift |
| Esc (or `q` outside sector play) | any | leave the room |

The lobby and debrief run in `CtxNav`; sector play runs in `CtxCommand` so
the panel runes stay the game's. Declared controls
(`Controls: []kit.ControlDecl`) give touch users chips: `W E R T S D F G`
(labels mirror the on-widget key badges) plus `kit.RuneControl('h', "HAIL")`.
During METEOR STORM the assigned mash key's chip is the only one that
matters; the overlay names it explicitly (AB-4).

## 13. Screen layouts

All on the fixed 80×24 kit frame. See ARTBOARDS.md for exact mockups:

- **AB-1** Lobby · **AB-2** Bridge (the core loop screen) · **AB-3** Bridge
  with hail traffic · **AB-4** Meteor storm · **AB-5** Solar flare ·
  **AB-6** Wormhole transit · **AB-7** Warp jump · **AB-8** Debrief

Shared chrome on every sector frame: status strip (row 0), order box
(rows 2–5), panel grid (rows 7–19), comms ticker (rows 21–22), hint line
(row 23).

## 14. kit Meta (as implemented in game.go)

```go
func (Game) Meta() kit.GameMeta {
    return kit.GameMeta{
        Slug:             "spaceterm",
        Name:             "Spaceterm",
        ShortDescription: "Frantic co-op bridge duty: orders fly, timers drain, and the dial you need is on someone else's panel. Shout, hail, survive.",
        MinPlayers:       1,
        MaxPlayers:       6,
        Tags:             []string{"co-op", "party", "real-time", "frantic", "crew"},
        CtxFeatures:      kit.CtxFeatCharacter,
        Lifecycle:        kit.LifecycleEphemeral,
        QuickModeLabel:   "Crew up",
        SoloModeLabel:    "Solo shift",
        PrivateInviteLine: "Crewmates beam aboard when they enter the code.",
        Controls: []kit.ControlDecl{
            kit.RuneControl('w', "W"), kit.RuneControl('e', "E"),
            kit.RuneControl('r', "R"), kit.RuneControl('t', "T"),
            kit.RuneControl('s', "S"), kit.RuneControl('d', "D"),
            kit.RuneControl('f', "F"), kit.RuneControl('g', "G"),
            kit.RuneControl('h', "HAIL"),
        },
        Leaderboard: &kit.LeaderboardSpec{
            MetricLabel: "Sectors",
            Direction:   kit.HigherBetter,
            Aggregation: kit.BestResult,
            Format:      kit.Integer,
        },
    }
}
```

## 15. Join / leave during a run

- **Join mid-sector:** spectator until the next warp jump — they see the
  status strip, comms, and a `BEAMING ABOARD AT NEXT WARP…` banner over a
  dimmed copy of the captain's (first-seated player's) panel. At warp they get
  a panel and join the order rotation; warp-charge requirements recalculate.
- **Leave / disconnect mid-sector:** their pending order is discarded
  (no hull penalty); any other crewmate's order targeting *their* controls is
  silently rerolled; their panel's controls leave the name pool at next warp.
- **Room empties:** ephemeral lifecycle, room closes (salvo behavior).

## 16. Failure & feedback polish

- Hull loss: 300 ms red edge-flash on all frames + the expired order text
  shown struck-through for 1 s before the replacement order slides in.
- Order completed: green flash on the order box; the *control's owner* also
  gets a 500 ms highlight ring on the widget, so accidental completions are
  legible.
- Turn-clock restraint (salvo lesson): countdown numerals only render in the
  final 5 s; before that the draining bar alone carries urgency.

## 17. Determinism & smoke plan

All randomness (names, panel layouts, order routing, anomaly schedule) flows
from one RNG seeded by the room seed — required for `smoke.yaml` and
hibernation-free replay of bug reports.

The shipped `smoke.yaml` drives three seats with seed 7: muster lobby →
launch → seat 0 hails an order that lives on seat 2's panel → seat 2 cycles
the named dial home (the cross-panel loop, on camera) → the crew goes AFK so
orders fumble and the hull drains → debrief. The scripted keys were read off
the seeded run, so the story replays byte-identically.

CI gates per SCHEMA.md: TinyGo wasip1 build, `shellcade-kit check`, meta slug
== dir, MIT LICENSE file, smoke shots posted as PR previews.

## 18. Out of scope (v1)

- Text chat beyond HAIL (voice or hails only).
- Campaign / named missions; persistent ship upgrades.
- Spectator-count display, replays, mid-run difficulty changes.
- Color-blind palettes beyond "never encode state in color alone" (state is
  always also a glyph/number — keep it that way).

## 19. Open questions

1. **Tuning:** warp-charge formula and timer floor need real playtests at 2
   and 6 crew; §7 numbers are first guesses.
2. **HAIL cooldown** (2 s) vs. order timer (≥5 s): is one hail per order
   enough at ADMIRAL? May need cooldown scaling by difficulty.
3. **Meteor mash count** (×12 in 4 s) on touch chips — verify chip tap rate
   makes this fair on mobile before locking the numbers.
4. Should the debrief crew log show "orders fumbled"? It's funny but mildly
   blame-y; decide after a party test.
