# Spaceterm

Frantic co-op bridge duty for 1–6 crew. Every crewmate gets a panel of
absurdly named controls — `GYROSCOPIC PLURALIZER`, `SUBSPACE CROUTON` — and a
stream of timed orders that usually name a control on *someone else's* panel.
Find it, shout it, fix it. Fumbled orders chip the shared hull; completed ones
charge the warp drive. Clear sectors until the ship comes apart.

An original homage to the party game *Spaceteam* (Sleeping Beast Games),
rebuilt from scratch for the 80×24 terminal: original name, original
technobabble, no assets or text reused.

## How to play

- **`w e r t` / `s d f g`** — actuate the matching control on your panel.
  One press: switches toggle, dials and sliders cycle, buttons pluck.
- **`h`** — HAIL: broadcast your current order to every crewmate's comms
  ticker (2 s cooldown). The UI never tells you whose panel a control is on —
  finding out is the game. Voice chat is even better; hails keep silent and
  remote crews viable.
- **Arrows + SPACE** — fallback selection ring, mostly for touch decks.
- An order completes the instant its control reaches the demanded state, no
  matter who caused it — even by accident.

The shared hull takes 1 damage per fumbled order; each warp jump patches 2
back. Watch for anomalies from sector 2: meteor storms (mash your assigned
key!), solar flares (labels scramble), wormholes (the panel draws mirrored),
and coolant leaks (wipe fogged controls). The crew banks one shared score —
sectors cleared — on the leaderboard.

Solo shifts work too: every order is yours, and only the solo-friendly
anomalies fire.

## Design docs

The full spec and screen artboards live next to the source: [SPEC.md](SPEC.md)
and [ARTBOARDS.md](ARTBOARDS.md).

## Dev loop

```sh
go run .              # solo shift in your terminal
go run . -seats 3     # hot-seat a crew (Ctrl-T switches seats)
go test ./...         # logic + allocation tests
go run . -smoke smoke.yaml -smoke-out shots/
```
