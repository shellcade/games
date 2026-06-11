# Spaceterm — artboards

Eight screens covering the full loop, drawn at the kit's fixed **80×24**
frame. Every line below is width-asserted ≤ 80 columns (trailing blank space
trimmed; the right edge of the canvas is implied). Companion to
[SPEC.md](SPEC.md) — boards are referenced there as AB-1 … AB-8.

Color/style key (monochrome here; styles via `kit.Style`):

| Element | Style |
|---|---|
| Hull bar `▮` | red, bold when ≤3 |
| Warp bar `▰` | cyan |
| Order text | bold white; struck dim red for 1 s when fumbled |
| Order timer bar | green → amber (<50 %) → red (<25 %) |
| Hotkey badges `[Q]` | yellow |
| Widget state glyphs | white (state always glyph+number, never color-only) |
| Comms ticker | dim cyan, dimming further with age |
| Anomaly banners | blinking amber |


## AB-1 · Lobby — crew muster

```text

      ╔══════════════════════════════════════════════╗
      ║   S P A C E T E R M                          ║
      ║   panic responsibly — a co-op bridge crew    ║
      ╚══════════════════════════════════════════════╝

      CREW MUSTER ── 3 ABOARD

        ◉ brandon     ENGINEER      READY
        ◍ alan        ENGINEER      READY
        ◎ matt        ENGINEER      beaming in…


      DIFFICULTY    ◂ CAPTAIN ▸          (◂ ▸ to change — shared setting)

      MISSION       ENDLESS — clear sectors until the hull gives out


      ▶ [SPACE] LAUNCH                   auto-launch in 0:12




 [◂ ▸] difficulty     [SPACE] launch     crew 1–6 — share the room code
```

- The salvo gather pattern: roster builds as crew beam in; character tiles
  (`◉ ◍ ◎`) come from `CtxFeatCharacter` and use each player's arcade glyph
  and color.
- `◂ CAPTAIN ▸` is a shared setting — any crewmate can flip it; a change
  pulses amber on everyone's frame.
- `auto-launch in 0:12` is the 20 s lobby fallback timer; SPACE launches
  early. Solo players skip straight to a Solo shift.
- Style: title box bold white on default; READY in green; `beaming in…` dim.


## AB-2 · Bridge — the core loop

```text
 SECTOR 3 · THE CRAB NEBULA     HULL ▮▮▮▮▮▮▮▯▯▯       WARP ▰▰▰▰▰▰▱▱▱▱▱▱▱▱▱▱ 6/16

┌─ YOUR ORDER ─────────────────────────────────────────────────────────────────┐
│ SET THE GYROSCOPIC PLURALIZER TO 4                                           │
│ ▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯                                     │
└──────────────────────────────────────────────────────────────────────────────┘

 ── YOUR PANEL ── brandon @ ENGINEERING ───────────────────────────────────────
┌[Q]───────────────┐┌[W]───────────────┐┌[E]───────────────┐┌[R]───────────────┐
│ GYROSCOPIC       ││ POLARIZED        ││ TACHYON          ││ PHOTON           │
│ PLURALIZER       ││ SLIPNOZZLE       ││ BELLOWS          ││ WHISK            │
│ DIAL      ⟨ 2 ⟩  ││ SWITCH           ││ SLIDER  LVL 1/4  ││ BUTTON           │
│ ○─○─●─○─○        ││ ╶───○   OFF      ││ ▰▱▱▱             ││   ( PRESS )      │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘
┌[A]───────────────┐┌[S]───────────────┐┌[D]───────────────┐┌[F]───────────────┐
│ DORSAL           ││ SUBSPACE         ││ IONIC            ││ RECURSIVE        │
│ GRAVIMETER       ││ CROUTON          ││ DEFROSTER        ││ MANIFOLD         │
│ DIAL      ⟨ 0 ⟩  ││ SWITCH           ││ SLIDER  LVL 3/4  ││ BUTTON           │
│ ●─○─○─○─○        ││ ●───╴   ON       ││ ▰▰▰▱             ││   ( PRESS )      │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘

 COMMS  alan ▸ SET THE FERROUS HOLOSPINDLE TO 2                · 3s
        matt ▸ ENGAGE THE BEVELED NANOBUZZER                   · just now
 [q w e r a s d f] actuate control     [h] hail your order     [◂▸ ␣] fallback
```

- The one screen players live on. Top strip: sector, shared hull (red ▮),
  shared warp progress (cyan ▰). Order box: bold white text, 40-cell timer
  bar draining green → amber (<50 %) → red (<25 %).
- This order targets the player's own panel — but the UI never says so;
  scanning for `GYROSCOPIC PLURALIZER` and pressing `q` IS the game.
- Each widget shows its hotkey badge `[Q]`, two label lines, its type, and a
  glyph-rendered state (never color-only). Pressing a hotkey actuates
  instantly — no cursor.
- COMMS ticker: last hails from crewmates, dimming with age. Frames are
  per-player (`r.Send`); only the panel and order differ between crew.


## AB-3 · Bridge — hailing a misrouted order

```text
 SECTOR 3 · THE CRAB NEBULA     HULL ▮▮▮▮▮▮▮▯▯▯       WARP ▰▰▰▰▰▰▱▱▱▱▱▱▱▱▱▱ 6/16

┌─ YOUR ORDER ─────────────────────────────────────────────────────────────────┐
│ ENGAGE THE OSMOTIC PHASE LOOP                                                │
│ ▮▮▮▮▮▮▮▮▮▮▮▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯▯  3.1s                               │
└──────────────────────────────────────────────────────────────────────────────┘

 ── YOUR PANEL ── brandon @ ENGINEERING ───────────────────────────────────────
┌[Q]───────────────┐┌[W]───────────────┐┌[E]───────────────┐┌[R]───────────────┐
│ GYROSCOPIC       ││ POLARIZED        ││ TACHYON          ││ PHOTON           │
│ PLURALIZER       ││ SLIPNOZZLE       ││ BELLOWS          ││ WHISK            │
│ DIAL      ⟨ 2 ⟩  ││ SWITCH           ││ SLIDER  LVL 1/4  ││ BUTTON           │
│ ○─○─●─○─○        ││ ╶───○   OFF      ││ ▰▱▱▱             ││   ( PRESS )      │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘
┌[A]───────────────┐┌[S]───────────────┐┌[D]───────────────┐┌[F]───────────────┐
│ DORSAL           ││ SUBSPACE         ││ IONIC            ││ RECURSIVE        │
│ GRAVIMETER       ││ CROUTON          ││ DEFROSTER        ││ MANIFOLD         │
│ DIAL      ⟨ 0 ⟩  ││ SWITCH           ││ SLIDER  LVL 3/4  ││ BUTTON           │
│ ●─○─○─○─○        ││ ●───╴   ON       ││ ▰▰▰▱             ││   ( PRESS )      │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘

 COMMS  you  ▸ ENGAGE THE OSMOTIC PHASE LOOP                   · just now
        alan ▸ SET THE FERROUS HOLOSPINDLE TO 2                · 6s
 [q w e r a s d f] actuate control     [h] hail (cooling 1.2s)     [◂▸ ␣]
```

- `OSMOTIC PHASE LOOP` is on someone else's panel. The player pressed `h`:
  their order now sits at the top of every crewmate's ticker (`you ▸ …` on
  their own frame as confirmation).
- The hint line shows the 2 s hail cooldown counting down in place.
- Timer is in the red zone (<25 %): bar red, numeric countdown appears only
  in the final 5 s (turn-clock restraint, per spec §16).


## AB-4 · Anomaly — meteor storm

```text
 SECTOR 3 · THE CRAB NEBULA     HULL ▮▮▮▮▮▮▮▯▯▯       WARP ▰▰▰▰▰▰▱▱▱▱▱▱▱▱▱▱ 6/16
    *                                                                 *
┌─ ⚠ METEOR STORM ⚠ ───────────────────────────────────────────────────────────┐
│ ORDERS SUSPENDED — BRACE FOR IMPACT                                          │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
          *                                                       ·
 ── YOUR PANEL ── brandon @ ENGINEERING ───────────────────────────────────────
┌[Q]───────────────┐┌[W]───────────────┐┌[E]───────────────┐┌[R]───────────────┐
│ GYROSCOPIC       ││ ╔════════ ⚠ METEOR STORM ⚠ ════════╗ ││ PHOTON           │
│ PLURALIZER       ││ ║                                  ║ ││ WHISK            │
│ DIAL      ⟨ 2 ⟩  ││ ║      MASH  [ E ]   ×12           ║ ││ BUTTON           │
│ ○─○─●─○─○        ││ ║      ▮▮▮▮▮▯▯▯▯▯▯▯   5/12         ║ ││   ( PRESS )      │
└──────────────────┘└─║      2.9s                        ║─┘└──────────────────┘
┌[A]───────────────┐┌[║                                  ║─┐┌[F]───────────────┐
│ DORSAL           ││ ╚══════════════════════════════════╝ ││ RECURSIVE        │
│ GRAVIMETER       ││ CROUTON          ││ DEFROSTER        ││ MANIFOLD         │
│ DIAL      ⟨ 0 ⟩  ││ SWITCH           ││ SLIDER  LVL 3/4  ││ BUTTON           │
│ ●─○─○─○─○        ││ ●───╴   ON       ││ ▰▰▰▱             ││   ( PRESS )      │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘
     *                                                                  ·
 COMMS  alan ▸ SET THE FERROUS HOLOSPINDLE TO 2                · 9s

 every crewmate: mash your assigned key — misses cost hull
```

- Orders suspend; the order box becomes the storm banner. A centered modal
  names each crewmate's personally assigned mash key (here `E` — it varies
  per player) with a 12-press progress bar and the 4 s window.
- Panel renders dimmed beneath the modal (annotation: 50 % luminance); `*`
  debris drifts in the margins on each heartbeat.
- A crewmate who misses costs −1 hull (capped −2 per storm). On touch decks
  the assigned key's chip is the obvious mash target.


## AB-5 · Anomaly — solar flare

```text
 SECTOR 3 · THE CRAB NEBULA     HULL ▮▮▮▮▮▮▮▯▯▯       WARP ▰▰▰▰▰▰▱▱▱▱▱▱▱▱▱▱ 6/16

┌─ YOUR ORDER ─────────────────────────────────────────────────────────────────┐
│ SET THE IONIC DEFROSTER TO 2                                                 │
│ ▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▯▯▯▯▯▯▯▯▯                                     │
└──────────────────────────────────────────────────────────────────────────────┘
 ☼ SOLAR FLARE — control labels scrambled · 4.8s
 ── YOUR PANEL ── brandon @ ENGINEERING ───────────────────────────────────────
┌[Q]───────────────┐┌[W]───────────────┐┌[E]───────────────┐┌[R]───────────────┐
│ ▒▒▒▒▒▒▒▒▒▒▒      ││ ▒▒▒▒▒▒▒▒▒▒▒      ││ ▒▒▒▒▒▒▒▒▒▒▒      ││ ▒▒▒▒▒▒▒▒▒▒▒      │
│ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒▒        │
│ ▒▒▒▒▒▒  ▒▒▒ ▒▒▒  ││ ▒▒▒▒▒▒  ▒▒▒ ▒▒▒  ││ ▒▒▒▒▒▒  ▒▒▒ ▒▒▒  ││ ▒▒▒▒▒▒  ▒▒▒ ▒▒▒  │
│ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒▒        │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘
┌[A]───────────────┐┌[S]───────────────┐┌[D]───────────────┐┌[F]───────────────┐
│ ▒▒▒▒▒▒▒▒▒▒▒      ││ ▒▒▒▒▒▒▒▒▒▒▒      ││ ▒▒▒▒▒▒▒▒▒▒▒      ││ ▒▒▒▒▒▒▒▒▒▒▒      │
│ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒▒        │
│ ▒▒▒▒▒▒  ▒▒▒ ▒▒▒  ││ ▒▒▒▒▒▒  ▒▒▒ ▒▒▒  ││ ▒▒▒▒▒▒  ▒▒▒ ▒▒▒  ││ ▒▒▒▒▒▒  ▒▒▒ ▒▒▒  │
│ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒▒        ││ ▒▒▒▒▒▒▒▒▒        │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘

 COMMS  matt ▸ SET THE IONIC DEFROSTER TO 2                    · 1s
        alan ▸ ENGAGE THE SUBSPACE CROUTON                     · 4s
 [q w e r a s d f] actuate control     [h] hail your order     [◂▸ ␣] fallback
```

- Labels render as static (`▒`) for 6 s — states, types, and hotkey badges
  stay readable, and orders keep flowing. The order names a control you can
  no longer read: spatial memory or a crewmate's memory saves you.
- Banner row flashes amber; static cells shimmer (re-randomized each
  heartbeat) so it reads as interference, not a crash.


## AB-6 · Anomaly — wormhole transit

```text
 SECTOR 3 · THE CRAB NEBULA     HULL ▮▮▮▮▮▮▮▯▯▯       WARP ▰▰▰▰▰▰▱▱▱▱▱▱▱▱▱▱ 6/16

┌─ YOUR ORDER ─────────────────────────────────────────────────────────────────┐
│ PLUCK THE PHOTON WHISK                                                       │
│ ▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▯▯▯▯▯                                     │
└──────────────────────────────────────────────────────────────────────────────┘
 ◎ WORMHOLE TRANSIT — panel mirrored, keys unchanged · 5.2s
 ── YOUR PANEL ── brandon @ ENGINEERING ───────────────────────────────────────
┌[R]───────────────┐┌[E]───────────────┐┌[W]───────────────┐┌[Q]───────────────┐
│ PHOTON           ││ TACHYON          ││ POLARIZED        ││ GYROSCOPIC       │
│ WHISK            ││ BELLOWS          ││ SLIPNOZZLE       ││ PLURALIZER       │
│ BUTTON           ││ SLIDER  LVL 1/4  ││ SWITCH           ││ DIAL      ⟨ 2 ⟩  │
│   ( PRESS )      ││ ▰▱▱▱             ││ ╶───○   OFF      ││ ○─○─●─○─○        │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘
┌[F]───────────────┐┌[D]───────────────┐┌[S]───────────────┐┌[A]───────────────┐
│ RECURSIVE        ││ IONIC            ││ SUBSPACE         ││ DORSAL           │
│ MANIFOLD         ││ DEFROSTER        ││ CROUTON          ││ GRAVIMETER       │
│ BUTTON           ││ SLIDER  LVL 3/4  ││ SWITCH           ││ DIAL      ⟨ 0 ⟩  │
│   ( PRESS )      ││ ▰▰▰▱             ││ ●───╴   ON       ││ ●─○─○─○─○        │
└──────────────────┘└──────────────────┘└──────────────────┘└──────────────────┘

 COMMS  alan ▸ SET THE DORSAL GRAVIMETER TO 3                  · 2s

 [q w e r a s d f] actuate control     [h] hail your order     [◂▸ ␣] fallback
```

- The panel is drawn mirrored left↔right for 6 s — `[R]` now sits where `[Q]`
  was. Hotkeys stay bound to their controls; only the drawing moves, so the
  counterplay is "read the badges, ignore muscle memory."
- No direct hull penalty; the danger is the expiries it causes.


## AB-7 · Warp jump interstitial

```text

    ─────━━━▶                                          ────━━━━▶
                         ──━━▶                                       ───━▶
  ──━━━━▶                                  ─────━━▶

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

          ────━━▶                                            ──────━━▶
                              ───━━━▶
     ──━▶                                        ────━▶


```

- ~4 s non-interactive beat between sectors. Star streaks animate left→right
  across heartbeats; the summary box tallies the sector and previews the
  next sector's name.
- `hull patched +2` ticks the hull bar visibly. New panels deal out as the
  streaks clear; mid-sector joiners board here.


## AB-8 · Debrief — run over

```text

      ✸ ✸ ✸  H U L L   B R E A C H  ✸ ✸ ✸

      THE SHIP COMES APART IN SECTOR 5 · THE GLASS SHOALS

      SECTORS CLEARED: 4                       room best: 6

      ┌─ CREW LOG ─────────────────────────────────────────┐
      │                                                    │
      │ crew          orders   hails   fumbles             │
      │ ◉ brandon       23       9        2                │
      │ ◍ alan          19      14        1                │
      │ ◎ matt          21       6        3                │
      │                                                    │
      └────────────────────────────────────────────────────┘

      the GYROSCOPIC PLURALIZER was set to 4 a record 11 times


      ▶ [SPACE] NEW SHIFT — same crew, fresh ship



 [SPACE] back to the lobby     score posts to the Sectors leaderboard
```

- Score is sectors *cleared* (died in 5 → scored 4) and posts identically
  for the whole crew (`Sectors`, higher-better, best-result).
- Crew log is flavor, not a ranking — orders/hails/fumbles plus one
  generated "fun fact" line. (Whether `fumbles` survives playtesting is
  spec §19.4.)
- SPACE returns the whole crew to this room's lobby for another shift.
