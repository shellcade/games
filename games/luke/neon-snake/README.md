# Neon Snake

A neon two-snake duel for the terminal, built on the
[shellcade/kit](https://github.com/shellcade/kit) developer kit and playable over
SSH at [shellcade.com](https://shellcade.com).

Steer a glowing snake around an 80×24 arena, eat the pulsing star to grow and
score, and outlast a second snake — a friend on the next seat, an AI bot, or
your own other hand in solo co-op. The walls wrap, the board speeds up as you
feed, and five modes change what's trying to kill you. Your best run is saved
and posted to the leaderboard.

```
╔═════════════════════════════════════════════════════════════════════════════╗
║ ▲▼ NEON DUEL ▲▼  P1:0090  P2:0040   HI:0120   PB:0120        THEME: Cyberpunk  ║
╠═══════════════════════════════════════════════════════════════════════════════
║ · · · · · · · · · · · · · · · · · · · · · · · · · · · · · · · · · · · · · · · · ║
║ · · · · · █ █ █ █ ◀ · · · · · · ★ · · · · · · · · · · · ▶ █ █ █ · · · · · · · · ║
║ · · · · · · · · · · · · · · ▲ · · · · · · · · ❖ · · · · · · · · · · · · · · · · ║
╠══ MODE: CLASSIC │ P1: alice [SHIELD 4s] VS P2: bob ════════════════════════════
║ CONTROLS: [P1:WASD P2:Arrows] Move [T]Theme [M]Mode [S]Settings [Esc]Quit      ║
╚═══════════════════════════════════════════════════════════════════════════════╝
```

## Controls

| Key | Action |
|---|---|
| `W A S D` | Steer snake 1 |
| `↑ ↓ ← →` | Steer snake 2 (in solo, this is your second snake) |
| `Space` | Pause / resume — and restart after game over |
| `T` | Cycle the colour theme |
| `M` | Cycle the game mode (in the lobby, while paused, or after game over) |
| `B` | Toggle the AI bot for snake 2 (solo only) |
| `S` | Open settings (while paused or after game over) |
| `Esc` | Leave the game |

## Playing

- **Eat to grow.** Each star is +10 and adds a body segment; the snake speeds up
  as the combined score climbs. Hit a wall? You don't — the edges wrap.
- **Don't crash.** Running into a snake (yours or the other), an obstacle, or a
  hazard ends the round. Last snake standing wins; a mutual crash is a draw.
- **One seat or two.** With a single player you drive both snakes (co-op), or
  press `B` to hand snake 2 to an AI bot. With two seats it's head-to-head:
  P1 on `WASD`, P2 on the arrows.
- **Power-ups** spawn as you feed: **🛡 Shield** makes a snake briefly
  crash-proof; **❄ Freeze** slows the other snake to half speed. Grab one by
  driving over it.

## Modes

Cycle with `M`:

- **Classic** — open arena, just the two snakes and the food.
- **Hazards** — neon sentinels patrol fixed lanes; touching one is fatal.
- **Maze** — fixed walls carve the arena into corridors.
- **Portals** — two linked gateways teleport a snake across the board.
- **Bomb** — a timed bomb ticks down and detonates in a blast radius; don't be
  near it when it goes off (or trigger it early by touching it).

## Settings

Open with `S` while paused or after game over: snake skins, grid dots on/off,
starting speed, and the intensity of the screen-shake and screen-flash effects
(off / gentle / strong).

## Develop

This is a standard Go program; run it natively for a fast dev loop.

```sh
go run .              # play it in your terminal
go run . -seats 2     # hot-seat two players (Ctrl-T switches the active seat)
go run . -seed 42     # reproducible run
go test ./...         # logic tests
```

## Build the arcade artifact

The arcade runs a sandboxed WebAssembly build:

```sh
tinygo build -opt=1 -no-debug -gc=leaking \
    -o neon-snake.wasm -target wasip1 -buildmode=c-shared .
```

Then verify it with the developer kit: `shellcade-kit check neon-snake.wasm`, and
play the real artifact with `shellcade-kit play neon-snake.wasm`.
