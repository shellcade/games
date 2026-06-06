# Tic-Tac-Toe (Rust)

Classic two-player noughts and crosses on the 80×24 canvas: the first two
joiners take X and O (roster order), play cells `1`–`9` on your turn, three in a
row wins, a full board draws. Leaving mid-game forfeits to your opponent, and a
turn left idle for 60s forfeits on the wake heartbeat.

This is a **Rust** catalog game — a behavioral port of the Go
[`tic-tac-toe`](../tic-tac-toe) in this same catalog, built on the
[`shellcade-kit` Rust SDK crate](https://github.com/shellcade/kit/tree/main/rust).
Its first incarnation hand-rolled the entire ABI v2 wire path (~800 lines of
frame packing, codecs, raw host imports, delta encoding, and baseline/epoch
broadcast) to prove the ABI is language-neutral; that plumbing has since
graduated into the SDK, and this game is now the proof a real game fits the
crate: a `Game`/`Handler` impl plus `shellcade_game!`, with
`#![forbid(unsafe_code)]` at the top.

The SDK owns the v2 frame-delta discipline — per-player baselines,
host-authoritative epochs, keyframes on first send / roster change, and the
in-call keyframe retry on rejection. The game just composes a `Frame` and calls
`r.identical(&frame)`.

## Build & verify

```sh
cargo build --release --target wasm32-wasip1
W=target/wasm32-wasip1/release/tic_tac_toe_rs.wasm
shellcade-kit check "$W"   # full conformance, incl. hibernation determinism
shellcade-kit meta  "$W"
```

`cargo test` runs the native game-logic suite (win detection, seating, turn
order) — no wasm runtime needed. The built `.wasm` is never committed; CI
builds what ships. The `shellcade-kit` crate dependency is pinned to the kit
release tag in `Cargo.toml` (lockstep with the SDK the arcade runs).

## Layout

| File | Role |
|---|---|
| `src/lib.rs` | `Game` impl (meta) + `shellcade_game!` registration |
| `src/game.rs` | Room logic: seating, turns, win/draw/forfeit, the 60s timer |
| `src/render.rs` | Board / players / status composition |
