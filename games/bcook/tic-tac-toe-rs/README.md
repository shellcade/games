# Tic-Tac-Toe (Rust)

Classic two-player noughts and crosses on the 80×24 canvas: the first two
joiners take X and O (roster order), play cells `1`–`9` on your turn, three in a
row wins, a full board draws. Leaving mid-game forfeits to your opponent, and a
turn left idle for 60s forfeits on the wake heartbeat.

This is a **Rust** catalog game — a behavioral port of the Go
[`tic-tac-toe`](../tic-tac-toe) in this same catalog, implemented from the
public shellcade ABI (`kit/ABI.md`) alone. It exists to prove the ABI is
language-neutral: a guest in any language that speaks the wire format is a
first-class arcade game. It targets **ABI v1** — fixed 16-byte cells, a
full-frame `identical` broadcast on every change.

## How it talks to the host

- **Transport is Extism.** Kernel I/O (input/output regions, the memory
  allocator) comes from `extism-pdk`; the §3 host functions are declared as
  **raw wasm imports** in `src/host.rs` — *not* the PDK `#[host_fn]` macro,
  which corrupts scalar args (ABI.md §7).
- **Exports** are the eight bare `#[no_mangle] pub extern "C" fn name() -> i32`
  entry points (`src/lib.rs`), returning `0` for ok.
- The crate builds as a **`cdylib` for `wasm32-wasip1`** (a WASI reactor).

## Build & verify

```sh
cargo build --release --target wasm32-wasip1
W=target/wasm32-wasip1/release/tic_tac_toe_rs.wasm
shellcade-kit check "$W"   # full conformance, incl. hibernation determinism
shellcade-kit meta  "$W"
```

The built `.wasm` is never committed; CI builds what ships.

## Layout

| File | Role |
|---|---|
| `src/lib.rs` | The 8 ABI exports + the per-room global state |
| `src/host.rs` | Raw host-function imports + Extism kernel plumbing |
| `src/wire.rs` | LE encode/decode, CallContext / Meta / Result codecs (ABI §4) |
| `src/frame.rs` | The 24×80×16-byte cell grid and its packed wire form (ABI §4.3) |
| `src/render.rs` | Board / players / status composition |
| `src/game.rs` | Room logic: seating, turns, win/draw/forfeit, the 60s timer |
