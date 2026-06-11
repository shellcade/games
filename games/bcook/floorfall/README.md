# Floorfall 🕳️

A last-one-standing crumbling-floors party game for the terminal, built on the
[shellcade/kit](https://github.com/shellcade/kit) developer kit and playable
over SSH at [shellcade.com](https://shellcade.com).

Three stacked floors, one rule: **step off a tile and it falls away.** You only
see the floor you're standing on. Tiles you leave behind crack (`█ → ▓ → ░`) and
then vanish a beat later; walk onto a hole and you drop to the same spot on the
floor below. Fall through the bottom floor and you're out. Last one standing wins
the round — then a fresh arena. No combat: it's all footwork.

```
+--------------------------------------------------------------------------------+
| ● λ alice*  ● Ω bob  ○ # cleo                  FLOOR 2/3  ALIVE 2  9s  BEST 24s|
|███████████████░████████████████████████████████████·███████████████████████████|
|███████████████████████████████████·███████████@████████████████████████████████|
|███████████████████·████████·███████████████████████████████·███████████████████|
|█·████████████████████████████████████████████████··█████████████████▓·█████████|
|█████████·███████████████████████████·█·██████████████████████████████████·█████|
|████████████████████████████████·████████████·██████████▓█████████████████████ █|
|██████████████████████████████████████████████·██████·██████████████████████████|
|██████████░████·██████████████·██████████████████████████████████████████████████|
|██████████·█████████████████████████████████████████████████████████████████████|
|██████·███████████████████████████·███████████████·█████████████████████████████|
|                       ▼ ▼ ▼  YOU FELL A FLOOR  ▼ ▼ ▼                            |
|███████████████████████████████████████·████·██·█████·██████▓█████·███████████████|
|██████████████████████████████████████████████▓█████████████████████████████████|
|████████████████████████████████████████████████████████████████████████████·███|
|█████████·███████████████████▓███████████████████████████████████·█████·████████|
|███████████████████████████████·██·██████████████████·██████████████████████████|
|███████████████████████·██████████████████████████████████████████·███·█████████|
|·█████·████████████████████████████··███████████████████████████████████████████|
|███████████████████·████████████████████████████████████████████·███████████████|
|█░█████████████████·████████████████████████████████████████████████████████████|
|███████████████████▓█████████████████████████████████·███████·██████████████████|
| ←/→/↑/↓ (hjkl) move — step off and the floor crumbles    Q quit                |
+--------------------------------------------------------------------------------+
```

Your own avatar is drawn reverse-video so you can find yourself instantly; every
contestant has a distinct color shown on the scoreboard (and their shellcade
character becomes their avatar glyph). Holes show as the dark `·` of the floor
below, and a worn tile sitting right above a hole on the next floor down glows as
a warning so you can read the drop before you take it.

## Controls

| Key                        | Action                       |
|----------------------------|------------------------------|
| `↑ ↓ ← →` (or `k j h l`)   | Step one tile that direction |
| `Q` / `Esc`                | Leave the arena              |

There's no combat and no jump — moving is the whole game. Every tile you step off
starts crumbling a beat later, so you're always leaving a trail of holes behind
you. Linger too long and the floor under your own feet gives way.

## Gameplay

- **Three floors.** You see only the floor you stand on; the HUD shows your depth
  (`FLOOR 2/3`), how many contestants are still alive, your current survival
  time, and your all-time best.
- **Crumbling.** A tile you step off ages `█ → ▓ → ░ → gone` over about a second.
  Step onto a hole and you drop to the same cell one floor down — aligned holes
  drop you several floors at once. Fall through the **bottom** floor and you're
  eliminated.
- **Solo (Outrun the crumble).** First-class survival mode: an ambient *crumble
  wave* decays random tiles on its own and **accelerates** as the round goes on,
  so a solo run is survive-as-long-as-you-can. Standing still is never safe — the
  wave will eventually claim the tile under your feet. Your best survival time
  (in seconds) rides the leaderboard.
- **Multiplayer (1–6).** The crumble wave still runs but far gentler — your
  rivals' footsteps are the main destroyer. When all but one have dropped out,
  the survivor wins the round. A short intermission shows the banner, then a
  fresh, fully-solid arena spins up.
- **Juice.** Distinct per-player colors, your own glyph reverse-video, tiles that
  visibly age under your feet, a flash when you fall a floor, and win / survival
  banners between rounds.

## Develop

This is a standard Go program in the inner loop — no wasm, no network, no setup.

```sh
go run .                 # play it in your terminal
go run . -seats 3        # hot-seat 3 players (Ctrl-T switches the active seat)
go run . -heartbeat 33ms # snappier frame rate
go test ./...            # logic tests (decay, falls, win/elimination, crumble)
```

`go test -run TestSnapshot -v` prints a plain-text snapshot of a composed frame,
handy for eyeballing layout changes.

## Build the arcade artifact

The arcade runs a sandboxed WebAssembly build. With
[TinyGo](https://tinygo.org) installed:

```sh
tinygo build -opt=1 -no-debug -gc=leaking \
    -o floorfall.wasm -target wasip1 -buildmode=c-shared .
```

Then verify, play, and smoke it on the production engine via the kit CLI:

```sh
shellcade-kit check floorfall.wasm
shellcade-kit play  floorfall.wasm --seats 2
shellcade-kit smoke .              # runs smoke.yaml, writes the preview shots
```

## How it works

The game implements the kit `Game` + `Handler` contract — six callbacks the
arcade invokes one at a time:

- `OnStart` builds the first arena (three fully-solid floors).
- `OnJoin` / `OnLeave` add and remove contestants (state is keyed by `AccountID`
  so it survives room hibernation) and load/persist the durable best time.
- `OnInput` steps a contestant one tile, schedules the tile they left to crumble,
  and drops them through any hole they land on.
- `OnWake` is the ~20 Hz heartbeat: it ages scheduled tiles, runs the ambient
  crumble wave, drops anyone a hole just opened under, resolves round ends, and
  renders a per-player frame.

| File          | Responsibility                                            |
|---------------|-----------------------------------------------------------|
| `main.go`     | Game metadata + room factory + native entrypoint          |
| `exports.go`  | The eight ABI export trampolines (wasm build only)        |
| `types.go`    | Constants, the player + decay-event structs               |
| `room.go`     | Handler callbacks, crumble simulation, rounds, persistence |
| `render.go`   | Composing the 80×24 frame                                 |
