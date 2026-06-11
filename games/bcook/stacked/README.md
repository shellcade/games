# Stacked 🧱

A competitive falling-blocks battler for the terminal, built on the
[shellcade/kit](https://github.com/shellcade/kit) developer kit and playable
over SSH at [shellcade.com](https://shellcade.com).

Steer falling pieces into your 10-wide well, clear rows for points, and weld
your rivals shut. The hook: clearing **two or more rows at once** dumps garbage
on whoever is stacked the tallest — so the leader always has a target on their
back. Top out and you're eliminated; the **last well standing wins the match**.

Your own well renders big; every rival's well renders as a **live miniature** in
the side panel, so you can watch them tumble in real time.

```
+--------------------------------------------------------------------------------+
| ●   alice 2400*  ●   bob 0  ●   cleo 0                                         |
|   ◆ YOUR WELL            NEXT          RIVALS                                  |
|  ┌────────────────────┐  ┌────────┐     ◆ bob                                  |
|  │                    │  │####    │    ┌──────────┐                            |
|  │                    │  │  ####  │    │          │                            |
|  │          ##        │  │        │    │          │                            |
|  │      ######        │  │        │    │          │                            |
|  │                    │  Zag──────┘    │          │                            |
|  │                    │                │████████  │                            |
|  │                    │  SCORE 2400    │████████  │                            |
|  │                    │  LINES 12      └──────────┘                            |
|  │                    │  LEVEL 1                                               |
|  │                    │  BEST  0        ◆ cleo                                 |
|  │                    │                ┌──────────┐                            |
|  │                    │                │          │                            |
|  │                    │                │          │                            |
|  │                    │                │██        │                            |
|  │          ··        │                │██        │                            |
|  │##  ##······##  ##  │                │██        │                            |
|  │▒▒▒▒▒▒▒▒  ▒▒▒▒▒▒▒▒▒▒│                └──────────┘                            |
|  └────────────────────┘                                                        |
|                                                                                |
| ←/→ move  ↓ soft  ↑/x rotate  z ccw  SPACE drop  Q quit                        |
+--------------------------------------------------------------------------------+
```

The `··` row near the floor is the **ghost**: where the current piece will land
on a hard drop. The `▒` cells are **garbage** sent by a rival — solid except for
one gap you have to dig out.

## Controls

| Key                 | Action                                  |
|---------------------|-----------------------------------------|
| `←` / `→`           | Move the piece left / right             |
| `↓`                 | Soft drop (fall faster, +1 per row)     |
| `↑` / `x`           | Rotate clockwise                        |
| `z`                 | Rotate counter-clockwise                |
| `Space`             | Hard drop — slam to the floor and lock  |
| `Q` / `Esc`         | Leave the match                         |

Rotation uses simple **wall kicks**: rotating against a wall or the stack nudges
the piece a cell or two to make it fit before giving up.

## The pieces

Stacked ships its own block set — a custom mix of four-, three-, and five-cell
shapes with their own names (no guideline trade dress):

| Name  | Cells | Shape                          |
|-------|-------|--------------------------------|
| Bar   | 4     | four in a line                 |
| Box   | 4     | 2×2 square                     |
| Tee   | 4     | three across with a stem       |
| Ell   | 4     | an L hook                      |
| Jay   | 4     | a mirrored L                   |
| Zig   | 4     | an S step                      |
| Zag   | 4     | a Z step                       |
| Wedge | 3     | a corner tromino               |
| Star  | 5     | a plus pentomino               |
| Cup   | 5     | a U pentomino                  |

Pieces are dealt from a **shuffle bag** — one of every shape before any repeats,
drawn deterministically from the room seed.

## Gameplay

- **Clear rows for points.** A single clear pays `100 × (level+1)`; bigger
  simultaneous clears pay disproportionately more (a double is worth far more
  than two singles). Soft- and hard-drops bank small bonus points for active
  play.
- **The attack (PvP).** Clearing **2+ rows at once** sends garbage rows — with a
  single open gap — to the rival with the **currently tallest stack**. A brief
  blinking `⚠ INCOMING` warning lands before the junk does, so the target can
  brace. The bigger your clear, the more rows you send.
- **Elimination.** When your stack reaches the ceiling you **top out** and are
  eliminated; you spectate the rest of the match from your seat. The last well
  standing wins.
- **Levels speed up gravity.** Every few cleared rows raises your level and
  shortens the drop interval.

## Solo: Score Attack

With one player it's pure **score attack** (`Score attack` mode): no rivals, no
elimination by a person — instead a garbage row creeps in on a timer that keeps
**accelerating** as the run goes on, while gravity speeds up by level. Survive,
clear, and stack score until you top out. Your best run rides the leaderboard
(`Score`, higher is better).

## Develop

This is a standard Go program in the inner loop — no wasm, no network, no setup.

```sh
go run .                 # play it in your terminal
go run . -seats 3        # hot-seat 3 players (Ctrl-T switches the active seat)
go run . -heartbeat 33ms # snappier frame rate
go test ./...            # logic tests (rotation, clears, attacks, elimination)
```

`go test -run TestSnapshot -v` prints a plain-text snapshot of a composed frame,
handy for eyeballing layout changes.

## Build the arcade artifact

The arcade runs a sandboxed WebAssembly build. With
[TinyGo](https://tinygo.org) installed:

```sh
tinygo build -opt=1 -no-debug -gc=leaking \
    -o stacked.wasm -target wasip1 -buildmode=c-shared .
```

Then verify, play, and smoke it on the production engine via the kit CLI:

```sh
shellcade-kit check stacked.wasm
shellcade-kit play  stacked.wasm --seats 2
shellcade-kit smoke .              # runs smoke.yaml, writes the preview shots
```

## How it works

The game implements the kit `Game` + `Handler` contract — six callbacks the
arcade invokes one at a time:

- `OnStart` sets the input context.
- `OnJoin` / `OnLeave` add and remove wells (state is keyed by `AccountID` so it
  survives room hibernation) and load/persist the durable best score.
- `OnInput` applies one move/rotate/drop (terminals have no key-up, so soft-drop
  is a per-press nudge and hard-drop slams to the floor).
- `OnWake` is the heartbeat: it advances gravity, resolves locks and line-clear
  animations, delivers queued and (solo) timed garbage, decides the winner, and
  renders a per-player frame.

| File         | Responsibility                                                |
|--------------|---------------------------------------------------------------|
| `main.go`    | Game metadata + room factory + native entrypoint              |
| `exports.go` | The eight ABI export trampolines (wasm build only)            |
| `types.go`   | Canvas/well geometry, tuning constants, well + piece structs  |
| `pieces.go`  | The original block set, rotation states, the shuffle bag      |
| `room.go`    | Handler callbacks, simulation, clears, attacks, persistence   |
| `render.go`  | Composing the 80×24 frame (big well, miniatures, HUD)         |
