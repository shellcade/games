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
use shellcade_kit::{Lifecycle, CTX_FEAT_CHARACTER};

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
            // Opt in to arcade player characters (kit v2.9.0): every roster
            // member's Player.character arrives populated, and the render
            // places each player's tile beside their name.
            ctx_features: CTX_FEAT_CHARACTER,
            // Touch deck chips (kit v2.10.0): the board is played with the
            // digit keys, so declare all nine — on a phone they render as a
            // tappable 1-9 chip grid mirroring the cells.
            controls: &[
                ControlDecl { input: Input::Char('1'), label: "1" },
                ControlDecl { input: Input::Char('2'), label: "2" },
                ControlDecl { input: Input::Char('3'), label: "3" },
                ControlDecl { input: Input::Char('4'), label: "4" },
                ControlDecl { input: Input::Char('5'), label: "5" },
                ControlDecl { input: Input::Char('6'), label: "6" },
                ControlDecl { input: Input::Char('7'), label: "7" },
                ControlDecl { input: Input::Char('8'), label: "8" },
                ControlDecl { input: Input::Char('9'), label: "9" },
            ],
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
