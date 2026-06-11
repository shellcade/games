# Meltdown ☢

A co-op reactor-room panic for the terminal, built on the
[shellcade/kit](https://github.com/shellcade/kit) developer kit and playable
over SSH at [shellcade.com](https://shellcade.com).

The whole 80×24 screen is a failing reactor ship: rooms and corridors drawn in
box characters around a glowing core. You and your crew (one engineer per
player) run the halls putting out faults that keep erupting at the stations and
**get worse the longer you ignore them**. Every unfixed fault eats the core's
shared integrity, and faults spawn faster and faster — so the run *always* ends.
Your score is how long the crew survives.

```
                                                                   CORE 100%  0s
┌─────────────────────────┬──────────────────────────┬─────────────────────────┐
│                         │                          │                         │
│   ·        ☺        ·        ·        ·        ·        ·        ≈        ·  │
│                                                                              │
│                         │                          │                         │
├────────────· ───────────┼──────────── ·────────────┼──────────── ·───────────┤
│                         │                          │                         │
│                         │    ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓    │                         │
│   ·        ·        ·        ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓·        ▲        ·        ·  │
│                         │    ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓    │                         │
│                         │                          │              ☺          │
├────────────  ───────────┼────────────  ────────────┼────────────  ───────────┤
│   ≈        ·        ·   │    Φ        ·        ·   │    ·        ·        ·  │
│                         │                          │                         │
│   ·        Φ        ·        ·        ·        ·        ·        ·        ·  │
└─────────────────────────┴──────────────────────────┴─────────────────────────┘
        LEAK [████████░░░░░░░░] MASH SPACE
 ↑↓←→/hjkl move  SPACE patch/hold  type valve keys  Q leave             FAULTS 5
```

The `▓` block at the centre is the reactor core — it **pulses, and its glow
shifts from hot yellow toward a dim cold red as integrity falls**. The glow *is*
the shared health bar. Each engineer (`☺`, your own drawn reverse-video so you
can find yourself) runs the bays fixing whatever erupts.

## Controls

| Key                       | Action                                            |
|---------------------------|---------------------------------------------------|
| `↑ ↓ ← →` (or `k j h l`)  | Walk one cell                                     |
| `Space`                   | Patch a leak (mash) / smother a fire (hold)       |
| letter keys               | Crack a jammed valve — type the shown sequence    |
| `Q` / `Esc`               | Leave the reactor                                 |

## The faults

| Glyph | Fault         | How to fix it                                                       |
|-------|---------------|--------------------------------------------------------------------|
| `≈`   | **Leak**      | Stand on it and **mash `Space`** to patch it shut.                  |
| `▲`   | **Fire**      | Stand on or next to it and **hold `Space`** — let go early and it regrows. |
| `Φ`   | **Jammed valve** | Stand on it and **type the shown 3–4 key sequence**; a wrong key re-jams it. |
| `◊`   | **Breach**    | Takes **two crew standing on it at once** — only ever spawns with a crew of 2+. |

While you're on (or beside) a fault, a labelled **progress bar** shows your fix
filling up, plus a hint of what to press. Walk away from a half-fixed fire and
it grows back; leave any fault unattended and it keeps draining the core, biting
harder the longer it festers.

## Solo & crew

**Solo is first-class.** A lone engineer plays the exact same loop with the
fault cadence scaled down to one pair of hands, and two-person breaches never
spawn. As more crew join, the spawn rate rises **sub-linearly** (it scales with
the square root of crew size), so the work per person *falls* — a bigger crew is
an easier shift. Recruiting friends is the whole incentive.

When the core finally hits zero the reactor melts down, and an end screen shows
the crew's survival time and each member's fix count. Best survival time (in
seconds) rides the arcade leaderboard.

## Develop

This is a standard Go program in the inner loop — no wasm, no network, no setup.

```sh
go run .                 # play it in your terminal
go run . -seats 3        # hot-seat 3 engineers (Ctrl-T switches the active seat)
go run . -heartbeat 33ms # snappier frame rate
go test ./...            # logic tests (each fault's fix, core damage, scaling, scoring)
```

`go test -run TestSnapshot -v` prints a plain-text snapshot of a composed frame,
handy for eyeballing layout changes.

## Build the arcade artifact

The arcade runs a sandboxed WebAssembly build. With
[TinyGo](https://tinygo.org) installed:

```sh
tinygo build -opt=1 -no-debug -gc=leaking \
    -o meltdown.wasm -target wasip1 -buildmode=c-shared .
```

Then verify, play, and smoke it via the kit CLI:

```sh
shellcade-kit check meltdown.wasm
shellcade-kit play  meltdown.wasm --seats 3
shellcade-kit smoke .              # runs smoke.yaml, writes the preview shots
```

## How it works

The game implements the kit `Game` + `Handler` contract — callbacks the arcade
invokes one at a time:

- `OnStart` builds the reactor floor plan and schedules the first fault.
- `OnJoin` / `OnLeave` add and remove engineers (state is keyed by `AccountID`
  so it survives room hibernation) and load/persist the durable best time.
- `OnInput` walks an engineer one cell, mashes a leak, or taps a valve key. Fire
  and breach **holds** are derived from terminal auto-repeat via the kit
  `keyhold` helper (terminals have no key-up event).
- `OnWake` is the ~20 Hz heartbeat: it advances the time-based fixes, drains the
  core by every active fault, erupts new faults on the accelerating schedule,
  ends the run when the core dies, and renders a per-engineer frame.

| File          | Responsibility                                              |
|---------------|-------------------------------------------------------------|
| `main.go`     | Game metadata + room factory + native entrypoint            |
| `exports.go`  | The eight ABI export trampolines (wasm build only)          |
| `types.go`    | Geometry, fault/crew structs, tuning constants              |
| `room.go`     | Handler callbacks, ship layout, movement, faults, spawning  |
| `render.go`   | Composing the 80×24 frame (reactor, core, faults, HUD)      |

## Future ideas

A *saboteur* mode — one crew member secretly worsening faults instead of fixing
them — is an obvious extension, deliberately left out of v1 to keep the co-op
loop tight.
