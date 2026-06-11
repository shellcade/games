# Putt ⛳

ASCII mini-golf for the terminal, built on the
[shellcade/kit](https://github.com/shellcade/kit) developer kit and playable
over SSH at [shellcade.com](https://shellcade.com).

Nine handcrafted holes of rising mischief on an 80×24 green. Aim, dial your
power, and putt — the ball rolls with real friction, bounces off the rails,
bogs down in the sand, and a splash in the water costs you a stroke. Everyone
plays the **same hole at the same time** — no turn waiting. Your ball is bright;
rivals are dim ghosts you roll right through. Lowest total over the round wins.

```
+--------------------------------------------------------------------------------+
| HOLE 8/9  Island Green  PAR 5                               STROKES 2   TOTAL 14|
|################################################################################|
|#..............................................................................#|
|#...........~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~..................#|
|#...........~~~~~......................................~~~~~...................#|
|#........●·····→~......................................~~~~~...................#|
|#...........~~~~~..........::::::::::::::::....P.......~~~~~...................#|
|#...........~~~~~..........::::::::::::::::....H...○...~~~~~...................#|
|#...........~~~~~..........::::::::::::::::............~~~~~...................#|
|#...........~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~.................#|
|#..............................................................................#|
| ←/→ aim  ↑/↓ power  SPACE putt  Q quit   PWR ▮▮▮▮▮▮▮▯▯▯                        |
+--------------------------------------------------------------------------------+
```

Your ball is `●` (bright, reverse-video); the aim line `·····→` shows where the
putt will go and lengthens as you dial up power. Rivals show as dim `○` ghosts —
you pass straight through them. `P` is the flag, `H` is the cup; `:` is sand and
`~` is water.

## Controls

| Key            | Action                                                       |
|----------------|--------------------------------------------------------------|
| `←` / `→` (or `h` / `l`) | Swing the aim indicator around your ball           |
| `↑` / `↓` (or `k` / `j`) | Step the power dial up/down a notch                |
| `Space`        | Putt — fires immediately at the dialed power                 |
| `Q` / `Esc`    | Leave the course                                             |

Power is a **notched dial**, not a hold-and-release charge: dial it once and it
**stays put between shots** — like a scroll wheel. (In fact, many terminals
translate mouse scroll into arrow up/down, so you can literally scroll the
dial.) The always-visible `PWR ▮▮▮▮▮▯▯▯▯▯` meter shows your setting, and the
notch→speed curve is **quadratic**: the low notches nudge the ball a few cells
for a delicate finish, while the top of the dial drives it most of the way
across the green.

## Gameplay

- **Nine holes, increasing mischief.** Straightaways, doglegs, bunkers, water
  carries, chicanes, a sand-trap alley, a spinning **windmill** to time, an
  island green, and a finale that throws sand, walls, water, and a windmill at
  you at once. Each hole has a **par**; beat it for a birdie or better.
- **Everyone plays at once.** No turns — all golfers putt the same hole
  simultaneously. There is no collision between players: rival balls are dim
  ghosts you roll straight through.
- **Physics with teeth.** Friction bleeds the roll off until the ball settles;
  it **bounces** off the course rails and the windmill arm; **sand** (`:`) bogs
  it down hard; **water** (`~`) is a hazard — a splash costs a stroke and resets
  you to where you putted from.
- **Finishing a hole.** Sink the ball (a slow enough ball drops; a screamer
  lips out) or run out of patience: a hole concedes at **par + 4** strokes. When
  every golfer is done, a **scorecard** shows each player's result (birdie, par,
  bogey…) and running total, then the next hole tees up. After hole 9, a final
  scorecard crowns the winner.
- **Juice.** Fast balls trail; a sunk ball flashes gold with its result label;
  each golfer has a distinct colour on the course and the scorecard.

## Solo mode — "Round of 9"

Solo is first-class: the exact same nine holes, just no ghosts — a pure
stroke-count attack. Your **total strokes for the full round** ride the
arcade leaderboard (lower is better, best round kept).

## Develop

This is a standard Go program in the inner loop — no wasm, no network, no setup.

```sh
go run .                 # play it in your terminal
go run . -seats 3        # hot-seat 3 players (Ctrl-T switches the active seat)
go run . -heartbeat 33ms # snappier frame rate
go test ./...            # logic tests (physics, hazards, hole-out, scoring)
```

`go test -run TestSnapshot -v` and `go test -run TestSnapshotHazards -v` print
plain-text snapshots of composed frames, handy for eyeballing layout changes.

## Build the arcade artifact

The arcade runs a sandboxed WebAssembly build. With
[TinyGo](https://tinygo.org) installed:

```sh
tinygo build -opt=1 -no-debug -gc=leaking \
    -o putt.wasm -target wasip1 -buildmode=c-shared .
```

Then verify, play, and smoke it on the production engine via the kit CLI:

```sh
shellcade-kit check putt.wasm
shellcade-kit play  putt.wasm --seats 2
shellcade-kit smoke .              # runs smoke.yaml, writes the preview shots
```

## How it works

The game implements the kit `Game` + `Handler` contract — the callbacks the
arcade invokes one at a time:

- `OnStart` resets the round to hole 1.
- `OnJoin` / `OnLeave` add and remove golfers (state is keyed by `AccountID` so
  it survives room hibernation); a late joiner is backfilled at par for holes
  already completed so totals stay comparable.
- `OnInput` swings the aim, steps the per-golfer power dial, and fires the putt
  on Space — all discrete events, so SSH latency jitter can't move a shot.
- `OnWake` is the heartbeat: it spins the windmill, integrates the rolling
  balls against elapsed time (`r.Now()`) with sub-stepped wall bounces,
  resolves hazards and hole-outs, drives the scorecard timers, and renders a
  per-player frame. The round settles with `r.End`, submitting each golfer's
  total to the leaderboard.

| File          | Responsibility                                                  |
|---------------|-----------------------------------------------------------------|
| `main.go`     | Game metadata + room factory + native entrypoint                |
| `exports.go`  | The eight ABI export trampolines (wasm build only)              |
| `types.go`    | Constants, the golfer struct, aspect-aware math                 |
| `holes.go`    | Tile model + the nine handcrafted courses (ASCII art)           |
| `room.go`     | Handler callbacks, physics, hole flow, scoring, leaderboard     |
| `render.go`   | Composing the 80×24 frame (course, balls, HUD, scorecard)       |
