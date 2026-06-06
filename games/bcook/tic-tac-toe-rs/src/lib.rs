//! tic-tac-toe-rs — a shellcade catalog game written in RUST, on the
//! `shellcade-kit` SDK crate. It is a behavioral port of the Go `tic-tac-toe`
//! game in this same catalog; the hand-rolled ABI plumbing this game once
//! carried (frame packing, wire codecs, raw host imports, delta encoding, the
//! baseline/epoch broadcaster) now lives in the SDK — what remains is the
//! game: seating, turns, win/draw/forfeit, and rendering.
//!
//!   cargo test                                        # game logic, natively
//!   cargo build --release --target wasm32-wasip1      # the arcade artifact
//!   shellcade-kit check target/wasm32-wasip1/release/tic_tac_toe_rs.wasm
#![forbid(unsafe_code)]

mod game;
mod render;

use shellcade_kit::prelude::*;

struct TicTacToe;

impl Game for TicTacToe {
    fn meta(&self) -> Meta {
        Meta {
            slug: "tic-tac-toe-rs", // == directory name
            name: "Tic-Tac-Toe (Rust)",
            short_description: "Classic two-player noughts and crosses; first to three in a row wins.",
            min_players: 2,
            max_players: 2,
            tags: &["board", "two-player", "classic"],
            quick_mode_label: "Quick match",
            private_invite_line: "Share the code; your opponent joins your board.",
            ..Meta::DEFAULT
        }
    }
    fn new_room(&self, _cfg: &RoomConfig) -> Box<dyn Handler> {
        Box::new(game::TttRoom::new())
    }
}

shellcade_kit::shellcade_game!(TicTacToe);
