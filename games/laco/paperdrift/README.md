# Paperdrift

A real-time paper-glider run: trim your nose with ↑/↓, dive to build speed,
climb to bank height, ride thermals, thread the gaps in the walls — and
outrun the storm closing in from behind. Up to six gliders launch together
over the same sky; the furthest flight without crashing wins the round.

## Flying it

- **↑/↓ trim the nose.** A tap is 5°; hold the key for a continuous rotation.
- **Momentum is life.** A glider has no engine: diving trades height for
  speed, climbing trades speed for height. Below stall speed the nose drops
  no matter your trim — the red **STALL** warning means you're mushing.
- **Thermals** (columns of rising ↑) lift you without costing speed.
- **Gates** are walls with a gap; the gaps tighten the further you fly.
- **The storm** advances at just under cruise speed. Hover and it eats you.

Crashed pilots spectate the leader until the last glider is down, then the
round posts to the Distance leaderboard and the next launch counts down.

## Developing

```sh
go run .              # play it natively
go run . -seats 3     # three seats; Ctrl-T switches who you control
go test ./...         # flight model, terrain, and round-flow tests
go run . -smoke smoke.yaml -smoke-out shots/   # the CI preview screens
```
