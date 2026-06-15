# Changelog

## A muster lobby, CPU setup, and battlefield readouts

- Joining drops you into a lobby where everyone gathers and starts the battle
  together — no more landing as a spectator because someone else's join
  auto-started a match, and the match is sized for everyone present.
- Pick the number of CPU opponents (up/down) and their difficulty (left/right)
  in the lobby; easy CPUs lob wide, hard ones are nearly dead-on.
- A small health bar floats over any wounded tank so you can spot the cripples,
  and a turn clock surfaces in the last seconds of a turn.

## Initial release

Salvo — turn-based tank artillery for the terminal.

- Rolling, fully destructible terrain: every shell scoops a crater, and tanks
  tumble into fresh holes (taking fall damage) or off the map entirely.
- Ballistic shells under gravity and a per-turn crosswind, animated against real
  elapsed time on the kit's heartbeat with a trailing arc and a chunky,
  white-hot explosion.
- Three weapons — SHELL, the big HEAVY (limited rounds), and the precise TRACER —
  plus a dotted trajectory stub to read your launch.
- Solo play against CPU tanks (a simulate-and-pick-best aimer), or 2–6 players
  taking turns on one field, each tank wearing its player's character.
- Career wins persisted per account and posted to a shared leaderboard.
