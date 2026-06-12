# Spaceterm — artboards

Eight screens covering the full loop, drawn at the kit's fixed **80×24**
frame. Every line below is width-asserted ≤ 80 columns (trailing blank space
trimmed; the right edge of the canvas is implied). Companion to
[SPEC.md](SPEC.md) — boards are referenced there as AB-1 … AB-8, and the
layouts mirror `render.go` (run `go run . -smoke smoke.yaml` for live shots).

Color/style key (monochrome here; styles via `kit.Style`):

| Element | Style |
|---|---|
| Hull bar `▮` | red; whole bar flashes on damage |
| Warp bar `▰` | cyan |
| Order text | bold white |
| Order timer bar | green → amber (<50 %) → red (<25 %) |
| Hotkey badges `[W]` | yellow |
| Widget state glyphs | white/green (state always glyph+number, never color-only) |
| Comms ticker | dim cyan |
| Anomaly banners | amber (the INBOUND warning blinks) |
| Completion flashes | order box + widget borders go green for ~0.5 s |


## AB-1 · Lobby — crew muster

```text

      ╔══════════════════════════════════════════════╗
      ║  S P A C E T E R M                           ║
      ║  panic responsibly — a co-op bridge crew     ║
      ╚══════════════════════════════════════════════╝

      CREW MUSTER ── 3 ABOARD

        ◉ brandon   ENGINEER    READY   best 6
        ◍ alan      ENGINEER    READY   best 4
        ◎ matt      ENGINEER    READY




      DIFFICULTY    < CAPTAIN >    (left/right to change — shared)

      MISSION       ENDLESS — clear sectors until the hull gives out

      ▸ [SPACE] LAUNCH     auto-launch in 12s



 [< >] difficulty     [SPACE] launch     crew 1-6 — share the room code
```

- The salvo gather pattern: roster builds as crew beam in; character tiles
  (`◉ ◍ ◎`) come from `CtxFeatCharacter` and use each player's arcade glyph
  and color. `best N` is each member's career-best sectors, from KV.
- `< CAPTAIN >` is a shared setting — any crewmate can flip it with the
  arrow keys.
- `auto-launch in 12s` is the 20 s lobby fallback timer; SPACE launches
  early. A solo launch goes straight to a Solo shift.
- Style: title box bold white; READY in green; the launch line in green.


## AB-2 · Bridge — the core loop

```text
 SECTOR 3 · THE CRAB NEBULA     HULL ▮▮▮▮▮▮▮▯▯▯     WARP ▰▰▰▰▰▰▱▱▱▱▱▱▱▱▱▱▱ 6/16

┌─ YOUR ORDER ─────────────────────────────────────────────────────────────────┐
│ SET THE GYROSCOPIC PLURALIZER TO 4                                           │
│ ▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯                                     │
└──────────────────────────────────────────────────────────────────────────────┘

 ── YOUR PANEL ── brandon ─────────────────────────────────────────────────────
┌[W]───────────────┐┌[E]───────────────┐┌[R]───────────────┐┌[T]───────────────┐
│ GYROSCOPIC       ││ POLARIZED        ││ TACHYON          ││ PHOTON           │
│ PLURALIZER       ││ SLIPNOZZLE       ││ BELLOWS          ││ WHISK            │
│ DIAL     ( 2 )   ││ SWITCH           ││ SLIDER  LVL 1/4  ││ BUTTON           │
│ ○─○─●─○─○        ││ ╶───○   OFF      ││ ▰▱▱▱             ││   ( PRESS )      │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘
┌[S]───────────────┐┌[D]───────────────┐┌[F]───────────────┐┌[G]───────────────┐
│ DORSAL           ││ SUBSPACE         ││ IONIC            ││ RECURSIVE        │
│ GRAVIMETER       ││ CROUTON          ││ DEFROSTER        ││ MANIFOLD         │
│ DIAL     ( 0 )   ││ SWITCH           ││ SLIDER  LVL 3/4  ││ BUTTON           │
│ ●─○─○─○─○        ││ ●───╴   ON       ││ ▰▰▰▱             ││   ( PRESS )      │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘

 COMMS  ▸ alan     SET THE FERROUS HOLOSPINDLE TO 2                   · 3s
        ▸ matt     ENGAGE THE BEVELED NANOBUZZER                      · just now
 [w e r t s d f g] actuate    [h] hail your order    [esc] leave
```

- The one screen players live on. Top strip: sector, shared hull (red ▮),
  shared warp progress (cyan ▰, scaled to a fixed 17-cell bar). Order box:
  bold white text, 40-cell timer bar draining green → amber (<50 %) → red
  (<25 %).
- This order targets the player's own panel — but the UI never says so;
  scanning for `GYROSCOPIC PLURALIZER` and pressing `w` IS the game.
- Each widget shows its hotkey badge `[W]`, two label lines, its type, and a
  glyph-rendered state (never color-only). Pressing a hotkey actuates
  instantly — no cursor. (The block is `W E R T / S D F G`: the canonical
  vocabulary reserves `q` as Back.)
- COMMS ticker: the latest hails, text column aligned, ages on the right.
  Frames are per-player (`r.Send`); only the panel and order differ.


## AB-3 · Bridge — hailing a misrouted order

```text
 SECTOR 3 · THE CRAB NEBULA     HULL ▮▮▮▮▮▮▮▯▯▯     WARP ▰▰▰▰▰▰▱▱▱▱▱▱▱▱▱▱▱ 6/16

┌─ YOUR ORDER ─────────────────────────────────────────────────────────────────┐
│ ENGAGE THE OSMOTIC PHASELOOP                                                 │
│ ▮▮▮▮▮▮▮▮▮▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯  3s                                 │
└──────────────────────────────────────────────────────────────────────────────┘

 ── YOUR PANEL ── brandon ─────────────────────────────────────────────────────
┌[W]───────────────┐┌[E]───────────────┐┌[R]───────────────┐┌[T]───────────────┐
│ GYROSCOPIC       ││ POLARIZED        ││ TACHYON          ││ PHOTON           │
│ PLURALIZER       ││ SLIPNOZZLE       ││ BELLOWS          ││ WHISK            │
│ DIAL     ( 2 )   ││ SWITCH           ││ SLIDER  LVL 1/4  ││ BUTTON           │
│ ○─○─●─○─○        ││ ╶───○   OFF      ││ ▰▱▱▱             ││   ( PRESS )      │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘
┌[S]───────────────┐┌[D]───────────────┐┌[F]───────────────┐┌[G]───────────────┐
│ DORSAL           ││ SUBSPACE         ││ IONIC            ││ RECURSIVE        │
│ GRAVIMETER       ││ CROUTON          ││ DEFROSTER        ││ MANIFOLD         │
│ DIAL     ( 0 )   ││ SWITCH           ││ SLIDER  LVL 3/4  ││ BUTTON           │
│ ●─○─○─○─○        ││ ●───╴   ON       ││ ▰▰▰▱             ││   ( PRESS )      │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘

 COMMS  ▸ you      ENGAGE THE OSMOTIC PHASELOOP                       · just now
        ▸ alan     SET THE FERROUS HOLOSPINDLE TO 2                   · 6s
 [w e r t s d f g] actuate    [h] hail your order    [esc] leave
```

- `OSMOTIC PHASELOOP` is on someone else's panel. The player pressed `h`:
  their order now tops every crewmate's ticker (`you ▸ …` on their own frame
  as confirmation). The `[h]` hint dims while the 2 s cooldown runs.
- Timer in the red zone: bar red, numeric countdown appears only in the
  final 5 s (turn-clock restraint, spec §16).


## AB-4 · Anomaly — meteor storm

```text
 SECTOR 3 · THE CRAB NEBULA     HULL ▮▮▮▮▮▮▮▯▯▯     WARP ▰▰▰▰▰▰▱▱▱▱▱▱▱▱▱▱▱ 6/16
    *                                                                 *
┌─ METEOR STORM ───────────────────────────────────────────────────────────────┐
│ ORDERS SUSPENDED — BRACE FOR IMPACT                                          │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
          *                                                       ·
 ── YOUR PANEL ── brandon ─────────────────────────────────────────────────────
┌[W]───────────────┐┌[E]───────────────┐┌[R]───────────────┐┌[T]───────────────┐
│ GYROSCOPIC       ││ ╔══════════ METEOR STORM ══════════╗ ││ PHOTON           │
│ PLURALIZER       ││ ║                                  ║ ││ WHISK            │
│ DIAL     ( 2 )   ││ ║      MASH  [E]  x12              ║ ││ BUTTON           │
│ ○─○─●─○─○        ││ ║      ▮▮▮▮▮▯▯▯▯▯▯▯   5/12         ║ ││   ( PRESS )      │
└──────────────────┘└─║      2s                          ║─┘└──────────────────┘
┌[S]───────────────┐┌[║                                  ║─┐┌[G]───────────────┐
│ DORSAL           ││ ╚══════════════════════════════════╝ ││ RECURSIVE        │
│ GRAVIMETER       ││ CROUTON          ││ DEFROSTER        ││ MANIFOLD         │
│ DIAL     ( 0 )   ││ SWITCH           ││ SLIDER  LVL 3/4  ││ BUTTON           │
│ ●─○─○─○─○        ││ ●───╴   ON       ││ ▰▰▰▱             ││   ( PRESS )      │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘
     *                                                                  ·
 COMMS  ▸ alan     SET THE FERROUS HOLOSPINDLE TO 2                   · 9s

 [w e r t s d f g] actuate    [h] hail your order    [esc] leave
```

- Orders suspend (their clocks bank and resume after); the order box becomes
  the storm banner. A centered modal names each crewmate's personally
  assigned mash key (here `E` — it varies per player) with a 12-press
  progress bar and the 4 s window.
- The modal overlays the panel; `*` debris drifts in the margins.
- Each crewmate who misses costs 1 hull, capped at 2 per storm. On touch
  decks the assigned key's chip is the obvious mash target.


## AB-5 · Anomaly — solar flare

```text
 SECTOR 3 · THE CRAB NEBULA     HULL ▮▮▮▮▮▮▮▯▯▯     WARP ▰▰▰▰▰▰▱▱▱▱▱▱▱▱▱▱▱ 6/16

┌─ YOUR ORDER ─────────────────────────────────────────────────────────────────┐
│ SET THE IONIC DEFROSTER TO 2                                                 │
│ ▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▯▯▯▯▯▯▯▯▯                                     │
└──────────────────────────────────────────────────────────────────────────────┘
 SOLAR FLARE — control labels scrambled
 ── YOUR PANEL ── brandon ─────────────────────────────────────────────────────
┌[W]───────────────┐┌[E]───────────────┐┌[R]───────────────┐┌[T]───────────────┐
│ ▒▒▒▒▒▒▒▒▒▒       ││ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒          ││ ▒▒▒▒▒▒           │
│ ▒▒▒▒▒▒▒▒▒▒       ││ ▒▒▒▒▒▒▒▒▒▒       ││ ▒▒▒▒▒▒▒          ││ ▒▒▒▒▒            │
│ DIAL     ( 2 )   ││ SWITCH           ││ SLIDER  LVL 1/4  ││ BUTTON           │
│ ○─○─●─○─○        ││ ╶───○   OFF      ││ ▰▱▱▱             ││   ( PRESS )      │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘
┌[S]───────────────┐┌[D]───────────────┐┌[F]───────────────┐┌[G]───────────────┐
│ ▒▒▒▒▒▒           ││ ▒▒▒▒▒▒▒▒         ││ ▒▒▒▒▒            ││ ▒▒▒▒▒▒▒▒▒        │
│ ▒▒▒▒▒▒▒▒▒▒       ││ ▒▒▒▒▒▒▒          ││ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒         │
│ DIAL     ( 0 )   ││ SWITCH           ││ SLIDER  LVL 3/4  ││ BUTTON           │
│ ●─○─○─○─○        ││ ●───╴   ON       ││ ▰▰▰▱             ││   ( PRESS )      │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘

 COMMS  ▸ matt     SET THE IONIC DEFROSTER TO 2                       · 1s
        ▸ alan     ENGAGE THE SUBSPACE CROUTON                        · 4s
 [w e r t s d f g] actuate    [h] hail your order    [esc] leave
```

- Label lines render as static (`▒`) for 6 s — types, states, and hotkey
  badges stay readable, and orders keep flowing. The order names a control
  you can no longer read: spatial memory or a crewmate's memory saves you.
- Banner row in amber; no direct hull cost beyond the expiries it causes.


## AB-6 · Anomaly — wormhole transit

```text
 SECTOR 3 · THE CRAB NEBULA     HULL ▮▮▮▮▮▮▮▯▯▯     WARP ▰▰▰▰▰▰▱▱▱▱▱▱▱▱▱▱▱ 6/16

┌─ YOUR ORDER ─────────────────────────────────────────────────────────────────┐
│ PLUCK THE PHOTON WHISK                                                       │
│ ▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▯▯▯▯▯                                     │
└──────────────────────────────────────────────────────────────────────────────┘
 WORMHOLE TRANSIT — panel mirrored, keys unchanged
 ── YOUR PANEL ── brandon ─────────────────────────────────────────────────────
┌[T]───────────────┐┌[R]───────────────┐┌[E]───────────────┐┌[W]───────────────┐
│ PHOTON           ││ TACHYON          ││ POLARIZED        ││ GYROSCOPIC       │
│ WHISK            ││ BELLOWS          ││ SLIPNOZZLE       ││ PLURALIZER       │
│ BUTTON           ││ SLIDER  LVL 1/4  ││ SWITCH           ││ DIAL     ( 2 )   │
│   ( PRESS )      ││ ▰▱▱▱             ││ ╶───○   OFF      ││ ○─○─●─○─○        │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘
┌[G]───────────────┐┌[F]───────────────┐┌[D]───────────────┐┌[S]───────────────┐
│ RECURSIVE        ││ IONIC            ││ SUBSPACE         ││ DORSAL           │
│ MANIFOLD         ││ DEFROSTER        ││ CROUTON          ││ GRAVIMETER       │
│ BUTTON           ││ SLIDER  LVL 3/4  ││ SWITCH           ││ DIAL     ( 0 )   │
│   ( PRESS )      ││ ▰▰▰▱             ││ ●───╴   ON       ││ ●─○─○─○─○        │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘

 COMMS  ▸ alan     SET THE DORSAL GRAVIMETER TO 3                     · 2s

 [w e r t s d f g] actuate    [h] hail your order    [esc] leave
```

- The panel draws mirrored left↔right for 6 s — `[T]` now sits where `[W]`
  was. Hotkeys stay bound to their controls; only the drawing moves, so the
  counterplay is "read the badges, ignore muscle memory."
- No direct hull penalty; the danger is the expiries it causes.


## AB-7 · Warp jump interstitial

```text

    ─────━━━                                           ────━━━━
                         ──━━                                        ───━
  ──━━━━                                   ─────━━

                    ╔══════════════════════════════════════╗
                    ║                                      ║
                    ║        ★  W A R P   J U M P  ★       ║
                    ║                                      ║
                    ║   SECTOR 3 CLEAR                     ║
                    ║                                      ║
                    ║   orders completed ............ 16   ║
                    ║   hull patched ................ +2   ║
                    ║   new panels issued to all crew      ║
                    ║                                      ║
                    ║   NEXT: SECTOR 4 · THE GLASS SHOALS  ║
                    ║                                      ║
                    ╚══════════════════════════════════════╝

          ────━━                                             ──────━━
                              ───━━━
     ──━                                         ────━


```

- ~4 s non-interactive beat between sectors. Star streaks sweep left→right
  across heartbeats; the summary box tallies the sector and previews the
  next sector's name.
- `hull patched +2` ticks the hull bar visibly. New panels deal out as the
  streaks clear; mid-sector joiners board here.


## AB-8 · Debrief — run over

```text


                      ✸ ✸ ✸  H U L L   B R E A C H  ✸ ✸ ✸

      THE SHIP COMES APART IN SECTOR 5 · THE PALE EXPANSE

      SECTORS CLEARED: 4        room best: 6

      ┌─ CREW LOG ─────────────────────────────────────────┐
      │  crew          orders   hails   fumbles            │
      │  ◉ brandon         23       9         2            │
      │  ◍ alan            19      14         1            │
      │  ◎ matt            21       6         3            │
      │                                                    │
      │                                                    │
      └────────────────────────────────────────────────────┘


      ▸ [SPACE] NEW SHIFT — same crew, fresh ship




 [SPACE] back to the lobby     score posts to the Sectors leaderboard
```

- Score is sectors *cleared* (died in 5 → scored 4) and posts identically
  for the whole crew (`Sectors`, higher-better, best-result).
- Crew log is flavor, not a ranking — orders, hails, fumbles per crewmate.
- SPACE returns the whole crew to this room's lobby for another shift.
