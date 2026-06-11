# Salvo

Turn-based tank artillery for the terminal, built on the
[shellcade/kit](https://github.com/shellcade/kit) developer kit and playable over
SSH at [shellcade.com](https://shellcade.com).

Lob shells over rolling, destructible hills, read the wind, and blast your rivals
off the map. Aim the angle, dial the power, pick a weapon, and fire — the shell
arcs across the field, craters the ground, and chips health off anyone in the
blast. Last tank standing wins. Play solo against the CPU or share the field with
friends.

```
  SALVO                         WIND >>>                               WINS 3
                .  .
            .          .
         .                .
        /                    .
       /                        :*
      λ            #               \
 ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~M~~~~~~~~~~~~
 ████████████████░░░       ░░░████████████████████
 ██████████████████       ██████████████████████████
  > you   HP 100  ANG 58  PWR 62  SHELL      left/right aim  up/down power  fire
```

## Controls

| Key | Action |
|---|---|
| ← / → | rotate the barrel (angle) |
| ↑ / ↓ | power |
| `W` | cycle weapon |
| Space / Enter | fire — and start a rematch after a battle |

A faint dotted **trajectory stub** shows your launch direction; the rest of the
arc you learn by ranging your shots. In the **lobby**, ↑/↓ set how many CPU
opponents to field and ←/→ set their difficulty; SPACE starts the battle for
everyone gathered.

## Gameplay

- **Destructible terrain.** Every blast scoops a crater out of the hills; a tank
  whose ground is blown out tumbles into the hole (and takes a knock), or off the
  map entirely if the column is punched clean through.
- **Read the wind.** It's re-rolled every turn and shoves the shell sideways in
  flight — the gauge up top shows which way and how hard.
- **Three weapons:** `SHELL` (steady, unlimited), `HEAVY` (a huge blast, three
  rounds a battle), and `TRACER` (precise but feeble, unlimited).
- **Solo or social.** Gather in the lobby, choose how many CPU opponents you want
  (and how sharp they shoot), then deploy together — solo against the CPU, or 2–6
  of you taking turns on one field. A small bar floats over any wounded tank so
  you can gang up on the cripples; each match win bumps your career total on the
  shared leaderboard.

## Develop

This is a standard Go program; run it natively for a fast dev loop.

```sh
go run .              # play solo against the CPU
go run . -seats 2     # hot-seat two players (Ctrl-T switches the active seat)
go test ./...         # logic + allocation tests
```

## Build the arcade artifact

The arcade runs a sandboxed WebAssembly build:

```sh
tinygo build -opt=2 -no-debug -gc=leaking \
    -o salvo.wasm -target wasip1 -buildmode=c-shared .
```

Then verify and play it locally with `shellcade-kit`:

```sh
shellcade-kit check salvo.wasm
shellcade-kit play  salvo.wasm
```

## How it works

The game is turn-based, but the shell's flight, the explosion, and tanks tumbling
into craters are all animated against real elapsed time on the kit's heartbeat —
so a hibernation pause never teleports a shell mid-air.

| File | Responsibility |
|------|----------------|
| `main.go` / `game.go` | entrypoint + static metadata and the room factory |
| `exports.go` | the eight wasm ABI exports, trampolined to the kit |
| `room.go` | match lifecycle, turns, the heartbeat, durable wins |
| `world.go` | tanks, weapons, and the ballistic shell |
| `terrain.go` | the destructible heightmap (generation, craters, collision) |
| `ai.go` | the CPU's simulate-and-pick-best firing solution |
| `render.go` | the per-viewer scene — sky, terrain, tanks, shell, blast, HUD |
