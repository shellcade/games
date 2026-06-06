# Tic-Tac-Toe (Rust)

Classic two-player noughts and crosses on the 80×24 canvas: the first two
joiners take X and O (roster order), play cells `1`–`9` on your turn, three in a
row wins, a full board draws. Leaving mid-game forfeits to your opponent, and a
turn left idle for 60s forfeits on the wake heartbeat.

This is a **Rust** catalog game — a behavioral port of the Go
[`tic-tac-toe`](../tic-tac-toe) in this same catalog, implemented from the
public shellcade ABI (`kit/ABI.md`) alone. It exists to prove the ABI is
language-neutral: a guest in any language that speaks the wire format is a
first-class arcade game. It targets **ABI v2** — 24-byte grapheme cells and a
**frame-delta container** as the `identical` payload. The steady state ships a
run-coalesced delta (only the changed cells); the keyframe form is the
bootstrap / full-frame / worst case. The delta path is **hand-rolled** per
ABI.md §4.5 (a custom guest may emit the container directly): a single broadcast
baseline + the host-authoritative epoch, keyframing on the first send, any
roster change, and any host rejection. The encoder is byte-aligned with the
cross-verified [`bcook/diff-rs`](../diff-rs) reference.

## How it talks to the host

- **Transport is Extism.** Kernel I/O (input/output regions, the memory
  allocator) comes from `extism-pdk`; the §3 host functions are declared as
  **raw wasm imports** in `src/host.rs` — *not* the PDK `#[host_fn]` macro,
  which corrupts scalar args (ABI.md §7).
- **Exports** are the eight bare `#[no_mangle] pub extern "C" fn name() -> i32`
  entry points (`src/lib.rs`), returning `0` for ok. The v2 `identical` host
  import returns an `i64` whose low 32 bits carry the authoritative epoch.
- The crate builds as a **`cdylib` for `wasm32-wasip1`** (a WASI reactor) and
  additionally as an `rlib` so the pure game/frame/delta logic is unit-testable
  natively — the Extism transport is wasm-gated and stubbed off-wasm.

## Build & verify

```sh
cargo build --release --target wasm32-wasip1
W=target/wasm32-wasip1/release/tic_tac_toe_rs.wasm
shellcade-kit check "$W"   # full conformance, incl. hibernation determinism
shellcade-kit meta  "$W"
```

> **Note:** the currently *released* `shellcade-kit` binary speaks **ABI v1** and
> **cannot** validate this v2 artifact — it rejects the v2 `identical` import
> signature (`i64 -> i64` vs v1's `i64 -> void`). Validate v2 with the in-repo v2
> host; the build itself succeeds and `cargo test` exercises the codec natively.

`cargo test` runs the native logic suite (frame packing canonical-zero, delta
encode/round-trip, keyframe form, malformed-delta rejection, broadcast
keyframe/delta selection). The built `.wasm` is never committed; CI builds what
ships.

## Layout

| File | Role |
|---|---|
| `src/lib.rs` | The 8 ABI exports + the per-room global state |
| `src/host.rs` | Raw host-function imports + Extism kernel plumbing (wasm-gated; native stubs) |
| `src/wire.rs` | LE encode/decode, CallContext / Meta / Result codecs (ABI §4) |
| `src/frame.rs` | The 24×80×24-byte grapheme cell grid and its packed wire form (ABI §4.3) |
| `src/delta.rs` | The v2 frame-delta container: run-list encode, keyframe form, budget fallback (ABI §4.5) |
| `src/broadcast.rs` | The broadcast send path: baseline diff, epoch mirror, keyframe-on-reject |
| `src/render.rs` | Board / players / status composition |
| `src/game.rs` | Room logic: seating, turns, win/draw/forfeit, the 60s timer |
