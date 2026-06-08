//! tic-tac-toe-rs — a shellcade catalog game written in RUST, on the
//! `shellcade-kit` SDK crate. The hand-rolled ABI plumbing this game once
//! carried (frame packing, wire codecs, raw host imports, delta encoding, the
//! baseline/epoch broadcaster) now lives in the SDK — what remains is the
//! game: seating, turns, win/draw/forfeit, solo play, and rendering.
//!
//!   cargo test                                        # game logic, natively
//!   cargo build --release --target wasm32-wasip1      # the arcade artifact
//!   shellcade-kit check target/wasm32-wasip1/release/tic_tac_toe_rs.wasm
#![forbid(unsafe_code)]

mod game;
mod render;

use shellcade_kit::prelude::*;
use shellcade_kit::Lifecycle;

struct TicTacToe;

impl Game for TicTacToe {
    fn meta(&self) -> Meta {
        Meta {
            slug: "tic-tac-toe-rs", // == directory name
            name: "Tic-Tac-Toe (Rust)",
            short_description: "Classic noughts and crosses; first to three in a row wins. Solo or head-to-head.",
            min_players: 2,
            max_players: 2,
            tags: &["board", "two-player", "classic"],
            // A casual social room: when everyone leaves, the room closes —
            // no hibernation snapshot, no Resume-menu entry (kit v2.7.0).
            lifecycle: Lifecycle::Ephemeral,
            quick_mode_label: "Quick match",
            solo_mode_label: "Solo: play both sides",
            private_invite_line: "Share the code; your opponent joins your board.",
            ..Meta::DEFAULT
        }
    }
    fn new_room(&self, cfg: &RoomConfig) -> Box<dyn Handler> {
        // A Solo room (lobby's solo option, capacity 1) seats its one player
        // on both marks; everything else is the head-to-head game.
        Box::new(game::TttRoom::new(cfg.mode == Mode::Solo))
    }
}

shellcade_kit::shellcade_game!(TicTacToe);
