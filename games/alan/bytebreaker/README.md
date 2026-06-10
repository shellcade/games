# Bytebreaker

A neon brick-breaker for the terminal, built on the
[shellcade/kit](https://github.com/shellcade/kit) developer kit and playable over
SSH at [shellcade.com](https://shellcade.com).

Bounce a bit off your paddle to smash a wall of glowing bytes, catch the powerups
they drop, and chase a high score across faster and faster levels. Each player
runs their own cabinet; with friends at the table it's a race for the top of the
board.

```
  BYTEBREAKER   SCORE 240  LVL 2  HI 1180                          BALLS OOO
 ╔════════════════════════════════════════════════════════════════════════╗
 ║  █████ █████ █████ █████ █████ █████ █████ █████ █████ █████ █████ █████ ║
 ║  █████ █████ █████ █████       █████ █████ █████ █████ █████ █████ █████ ║
 ║  █████ █████ █████ █████ █████ █████       █████ █████ █████ █████ █████ ║
 ║                            *  .                                          ║
 ║                          '   O                                          ║
 ║                                          ███████████                    ║
 ║                                            (the danger floor below)     ║
```

## Controls

| Key | Action |
|---|---|
| ← / → | slide the paddle |
| Space / Enter | launch the bit — and start a fresh run after game over |

## Gameplay

- **Smash the wall.** Every byte you break scores, and the higher rows are worth
  more. Clear the wall to advance: each level adds rows, armours the top bytes
  (they take two hits), and speeds the bit up.
- **Powerups** drop from shattered bytes — catch them with the paddle:
  **W** wide paddle · **M** multiball · **S** slow bit · **+** extra life.
- **Three lives.** Miss the bit and it's gone; lose them all and it's game over —
  Space starts again. Your best run is saved and posted to the leaderboard.
- **Multiplayer** is a friendly score race: everyone plays their own board at the
  same time and sees rivals' live scores along the bottom.

## Develop

This is a standard Go program; run it natively for a fast dev loop.

```sh
go run .              # play it in your terminal
go run . -seats 2     # hot-seat two players (Ctrl-T switches the active seat)
go test ./...         # logic + allocation tests
```

## Build the arcade artifact

The arcade runs a sandboxed WebAssembly build:

```sh
tinygo build -opt=2 -no-debug -gc=leaking \
    -o bytebreaker.wasm -target wasip1 -buildmode=c-shared .
```

Then verify and play it locally with `shellcade-kit`:

```sh
shellcade-kit check bytebreaker.wasm
shellcade-kit play  bytebreaker.wasm
```

## How it works

The game implements the kit `Game` + `Handler` contract. There is no fixed tick:
the host calls `OnWake` on a heartbeat and the game advances all motion against
the real elapsed time, so a hibernation pause never teleports the bit.

| File | Responsibility |
|------|----------------|
| `main.go` / `game.go` | entrypoint + static metadata and the room factory |
| `exports.go` | the eight wasm ABI exports, trampolined to the kit |
| `room.go` | lifecycle, the per-player cabinet, the heartbeat, durable high score |
| `board.go` | one board: bit physics, collisions, bricks, powerups, levels |
| `render.go` | the per-viewer frame — bricks, paddle, bit, HUD, overlays |
