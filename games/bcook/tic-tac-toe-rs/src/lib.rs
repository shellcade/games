//! tic-tac-toe-rs — a shellcade catalog game written in RUST, implemented from
//! the public shellcade ABI (kit/ABI.md) alone. It is a behavioral port of the
//! Go `tic-tac-toe` game in this same catalog and exists to measure how hard
//! cross-language guest support is.
//!
//! This is ABI v2: 24-byte grapheme cells and a frame-delta container as the
//! frame payload of `identical` (the steady state ships run-coalesced deltas;
//! the keyframe form is the bootstrap/full-frame). The delta path is hand-rolled
//! per ABI.md §4.5 (host-authority epoch, keyframe on first send / rejection /
//! roster change) and byte-aligned with the `bcook/diff-rs` reference encoder.
//!
//! Transport is Extism (ABI.md §preamble): the kernel memory/IO plumbing comes
//! from extism-pdk, the host functions are raw wasm imports (ABI.md §3, §7),
//! and the 8 entry points are bare `extern "C"` exports returning i32 (0 = ok).

mod broadcast;
mod delta;
mod frame;
mod game;
mod host;
mod render;
mod wire;

mod exports {
    use crate::game::Room;
    use crate::host::{read_input, write_output};
    use crate::wire::{decode_ctx, Buf};

    const ABI_VERSION: u32 = 2;

    // Per-room state. One plugin instance == one room (ABI §1), and callbacks
    // are serial, so a single mutable global holds the entire room state. It is
    // lazily constructed on first access (HashMap::new is not const).
    static mut ROOM: Option<Room> = None;

    #[allow(static_mut_refs)]
    fn room() -> &'static mut Room {
        // SAFETY: callbacks are invoked serially per room (ABI §1); there is
        // never concurrent access to ROOM.
        unsafe {
            if ROOM.is_none() {
                ROOM = Some(Room::new());
            }
            ROOM.as_mut().unwrap()
        }
    }

    /// shellcade_abi: u32 ABI major version (little-endian).
    #[no_mangle]
    pub extern "C" fn shellcade_abi() -> i32 {
        write_output(&ABI_VERSION.to_le_bytes());
        0
    }

    /// meta: packed Meta (§4.2). Mirrors the Go game's Meta exactly.
    #[no_mangle]
    pub extern "C" fn meta() -> i32 {
        let mut w = Buf::new();
        w.str("tic-tac-toe-rs"); // slug (== directory name)
        w.str("Tic-Tac-Toe (Rust)"); // name
        w.str("Classic two-player noughts and crosses; first to three in a row wins.");
        w.u16(2); // minPlayers
        w.u16(2); // maxPlayers
        w.u16(3); // tagCount
        w.str("board");
        w.str("two-player");
        w.str("classic");
        w.str("Quick match"); // quickModeLabel
        w.str(""); // soloModeLabel (default)
        w.str("Share the code; your opponent joins your board."); // privateInviteLine
        w.u8(0); // hasLeaderboard = false
        write_output(&w.b);
        0
    }

    /// start: Ctx.
    #[no_mangle]
    pub extern "C" fn start() -> i32 {
        let input = read_input();
        let (ctx, _) = decode_ctx(&input);
        room().on_start(&ctx);
        0
    }

    /// join: Ctx ‖ u32 playerIdx.
    #[no_mangle]
    pub extern "C" fn join() -> i32 {
        let input = read_input();
        let (ctx, mut r) = decode_ctx(&input);
        let player_idx = r.u32() as usize;
        room().on_join(&ctx, player_idx);
        0
    }

    /// leave: Ctx ‖ u32 playerIdx (departed player is the final roster entry).
    #[no_mangle]
    pub extern "C" fn leave() -> i32 {
        let input = read_input();
        let (ctx, mut r) = decode_ctx(&input);
        let player_idx = r.u32() as usize;
        room().on_leave(&ctx, player_idx);
        0
    }

    /// input: Ctx ‖ u32 playerIdx ‖ u8 kind ‖ u32 rune ‖ u8 key.
    #[no_mangle]
    pub extern "C" fn input() -> i32 {
        let inbytes = read_input();
        let (ctx, mut r) = decode_ctx(&inbytes);
        let player_idx = r.u32() as usize;
        let kind = r.u8();
        let rune = r.u32();
        let _key = r.u8();
        room().on_input(&ctx, player_idx, kind, rune);
        0
    }

    /// wake: Ctx. The host heartbeat; drives the turn timeout.
    #[no_mangle]
    pub extern "C" fn wake() -> i32 {
        let input = read_input();
        let (ctx, _) = decode_ctx(&input);
        room().on_wake(&ctx);
        0
    }

    /// close: room teardown. Linear memory is reclaimed by instance destruction.
    #[no_mangle]
    pub extern "C" fn close() -> i32 {
        let _ = read_input();
        0
    }
}
