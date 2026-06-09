# Voidrunners 🚀

A free-for-all space shooter for the terminal, built on the
[shellcade/kit](https://github.com/shellcade/kit) developer kit and playable
over SSH at [shellcade.com](https://shellcade.com).

Fly an asteroids-style fighter around an 80×24 arena, weave through drifting
craters, and blast every other pilot you can line up. Get destroyed, explode,
respawn, and keep hunting — it's an endless arena and your all-time best kill
count rides the leaderboard. No rounds, no waiting: jump in and start flying.

```
+--------------------------------------------------------------------------------+
| ● alice 7*  ● bob 3  ● cleo 0                                  K 7  D 0  BEST 0|
|                    ·                     ·        ·                  *          |
|                    ◆→•                 *                                 ·      |
|                                              o###o                             |
|                                             o#***#o          ←◆  ·             |
|                          ·                   o###o                             |
|##o                                       o###o                           ·   o#|
|###o                                 ·          ◆↓                *        ·o## |
| ←/→ turn  ↑ thrust  ↓ brake  SPACE fire  Q quit                                |
+--------------------------------------------------------------------------------+
```

Each ship is a two-cell craft — a `◆` hull with a directional nose arrow
(`→ ↘ ↓ ↙ ← ↖ ↑ ↗`) showing its heading. Your own ship is reverse-video so you
can find yourself instantly.

## Controls

| Key            | Action                                       |
|----------------|----------------------------------------------|
| `↑ ↓ ← →` (or `k j h l`) | Steer that way — the nose snaps over and you thrust |
| `Space`        | Fire                                         |
| `Q` / `Esc`    | Leave the arena                              |

Flight is **directional with momentum**: press an arrow and your ship points
that way instantly and starts moving — but you keep drifting, so you don't stop
on a dime. Press two perpendicular arrows in quick succession (e.g. `↑` then
`←`) to head the diagonal between them. There's no brake key: to slow down, tap
the direction opposite your drift. You shoot in the direction your nose points.

## Gameplay

- **Craters** drift around the arena. Shoot a large one and it breaks into two
  smaller fragments; the smallest shatter for good. Each fragment destroyed is
  worth **1 kill**. *Ramming* a crater destroys you — dodge or shoot. Solo, the
  arena is full of them for target practice; once a second pilot joins it drops
  to a single crater so the focus is the dogfight.
- **Rival pilots** are fair game. A direct hit destroys them and scores you
  **5 kills**.
- **Death & respawn**: when you're hit you explode, then respawn a couple of
  seconds later with a brief blinking invulnerability window so you can't be
  spawn-camped.
- **Scoring**: your session kill count shows top-right (`K`); your all-time
  best is durable (`BEST`) and feeds the arcade leaderboard.

Your own ship is drawn reverse-video so you can always pick yourself out of the
fray; every pilot also has a distinct color shown on the scoreboard.

## Develop

This is a standard Go program in the inner loop — no wasm, no network, no setup.

```sh
go run .                 # play it in your terminal
go run . -seats 3        # hot-seat 3 players (Ctrl-T switches the active seat)
go run . -heartbeat 33ms # snappier frame rate
go test ./...            # logic tests (collisions, scoring, respawn, render)
```

`go test -run TestSnapshot -v` prints a plain-text snapshot of a composed frame,
handy for eyeballing layout changes.

## Build the arcade artifact

The arcade runs a sandboxed WebAssembly build. With
[TinyGo](https://tinygo.org) installed:

```sh
tinygo build -opt=1 -no-debug -gc=leaking \
    -o voidrunners.wasm -target wasip1 -buildmode=c-shared .
```

Then verify, play, and smoke it on the production engine via the kit CLI:

```sh
shellcade-kit check voidrunners.wasm
shellcade-kit play  voidrunners.wasm --seats 2
shellcade-kit smoke .              # runs smoke.yaml, writes the preview shots
```

## How it works

The game implements the kit `Game` + `Handler` contract — six callbacks the
arcade invokes one at a time:

- `OnStart` seeds the starfield and the first wave of craters.
- `OnJoin` / `OnLeave` add and remove pilots (state is keyed by `AccountID` so
  it survives room hibernation) and load/persist the durable best score.
- `OnInput` applies discrete control impulses (terminals have no key-up events,
  so each keypress nudges the ship and momentum does the rest).
- `OnWake` is the ~20 Hz heartbeat: it advances all motion against elapsed time
  (`r.Now()`), resolves collisions, tops up craters, respawns the dead, and
  renders a per-player frame.

| File          | Responsibility                                            |
|---------------|-----------------------------------------------------------|
| `main.go`     | Game metadata + room factory + native entrypoint          |
| `exports.go`  | The eight ABI export trampolines (wasm build only)        |
| `types.go`    | Constants, entity structs, toroidal math                  |
| `room.go`     | Handler callbacks, physics, combat, spawning, persistence |
| `render.go`   | Composing the 80×24 frame                                 |
