# Changelog

## Directional flight and a calmer multiplayer arena

Reworked the controls. Arrows are now directional: press one and your ship
points that way instantly and starts moving — you still keep your drift, so
momentum carries. Press two perpendicular arrows in quick succession (e.g. up
then left) to head the diagonal between them. The old rotate-and-thrust scheme
and the dedicated brake are gone; to slow down, tap the direction opposite your
drift.

Also: once a second pilot joins, the arena drops from a field of craters to a
single one, so multiplayer is a dogfight rather than an obstacle course (solo
play keeps the full field for target practice).

## Fix multiplayer out-of-memory crash

Fixed a crash that quarantined v1: the renderer allocated a fresh frame buffer
for every viewer on every heartbeat. Under the arcade's `-gc=leaking` runtime
nothing is ever reclaimed, so that memory grew without bound until the room ran
out — and because it scaled with the number of pilots, it only struck in
multiplayer. The renderer now reuses a single long-lived frame buffer (the
platform copies each frame as it's sent), so per-tick rendering is allocation-
free regardless of player count. Added a steady-state wake allocation gate so a
regression can't slip back in.

### Gameplay tuning

Slowed the pace a touch: ships accelerate and cap out slower, the fire rate is
gentler, and shots travel a shorter distance before fizzling. Easier to read a
dogfight and to dodge.
