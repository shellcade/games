//! Tic-Tac-Toe room logic — a faithful behavioral port of the Go catalog game
//! (../tic-tac-toe/game.go). All state lives in one global `Room` in linear
//! memory (the ABI's state model). Seats are keyed by ACCOUNT ID so the room
//! survives hibernation (connection tokens change; account ids do not).

use std::collections::HashMap;

use crate::broadcast::Broadcaster;
use crate::render;
use crate::wire::{encode_result, Ctx, Member, Ranking, STATUS_DNF, STATUS_FINISHED};

// turnTimeout: how long a player has to move before forfeiting (60s in nanos).
const TURN_TIMEOUT_NANOS: i64 = 60_000_000_000;

// Marks; 0 is an empty cell.
pub const EMPTY: u8 = 0;
pub const MARK_X: u8 = b'X';
pub const MARK_O: u8 = b'O';

// The eight three-in-a-row lines over the 0..8 cell indices.
const WIN_LINES: [[usize; 3]; 8] = [
    [0, 1, 2], [3, 4, 5], [6, 7, 8], // rows
    [0, 3, 6], [1, 4, 7], [2, 5, 8], // cols
    [0, 4, 8], [2, 4, 6], // diagonals
];

/// One match. Mirrors the Go `room` struct.
pub struct Room {
    pub players: HashMap<String, Member>, // account id -> last-seen member (names)
    pub board: [u8; 9],
    pub x_id: String,
    pub o_id: String,
    pub turn: u8,
    pub moves: i32,
    pub over: bool,
    pub winner_id: String, // "" on draw or while playing
    pub deadline: Option<i64>, // current turn's forfeit deadline (unix nanos)
    started: bool,
    // v2 transparent frame diffing: the broadcast baseline + epoch state. Every
    // render diffs against this and ships a delta container via `identical`.
    broadcaster: Broadcaster,
}

impl Room {
    pub fn new() -> Self {
        Room {
            players: HashMap::new(),
            board: [EMPTY; 9],
            x_id: String::new(),
            o_id: String::new(),
            turn: MARK_X,
            moves: 0,
            over: false,
            winner_id: String::new(),
            deadline: None,
            started: false,
            broadcaster: Broadcaster::new(),
        }
    }

    // ---- callbacks ---------------------------------------------------------

    /// OnStart: set Nav input context, X moves first.
    pub fn on_start(&mut self, _ctx: &Ctx) {
        crate::host::set_ctx(0); // CtxNav
        self.turn = MARK_X;
        self.started = true;
        // The Go game does not render in OnStart (it renders on join); match that.
    }

    /// OnJoin: seat the first two joiners as X then O. A re-join just re-renders.
    pub fn on_join(&mut self, ctx: &Ctx, player_idx: usize) {
        // Roster change: invalidate the broadcast baseline so the next render is
        // a keyframe (the guest-side backstop to the host's epoch authority).
        self.broadcaster.invalidate();
        let p = match ctx.members.get(player_idx) {
            Some(m) => m.clone(),
            None => return,
        };
        let id = p.account_id.clone();
        self.players.insert(id.clone(), p);

        if id != self.x_id && id != self.o_id {
            if self.x_id.is_empty() {
                self.x_id = id.clone();
            } else if self.o_id.is_empty() {
                self.o_id = id.clone();
            }
        }

        if !self.over && self.both_seated() && self.deadline.is_none() {
            self.deadline = Some(ctx.now_unix_nanos + TURN_TIMEOUT_NANOS);
        }
        self.render(ctx);
    }

    /// OnLeave: a seated player leaving mid-game forfeits; otherwise clean up.
    pub fn on_leave(&mut self, ctx: &Ctx, player_idx: usize) {
        // Roster change: invalidate the broadcast baseline (next render keyframes).
        self.broadcaster.invalidate();
        if self.over {
            return;
        }
        let leaver = match ctx.members.get(player_idx) {
            Some(m) => m.account_id.clone(),
            None => return,
        };
        if leaver != self.x_id && leaver != self.o_id {
            return; // a non-seated viewer left.
        }
        let winner = if leaver == self.o_id {
            self.x_id.clone()
        } else {
            self.o_id.clone()
        };
        if winner.is_empty() {
            // The only seated player left before an opponent arrived: clear the seat.
            if leaver == self.x_id {
                self.x_id.clear();
            } else {
                self.o_id.clear();
            }
            self.deadline = None;
            self.players.remove(&leaver);
            self.render(ctx);
            return;
        }
        self.settle_forfeit(ctx, &winner, &leaver);
    }

    /// OnInput: place a mark for the current player. Out-of-turn, non-digit, and
    /// occupied-cell input is ignored (no re-render — render on change only).
    pub fn on_input(&mut self, ctx: &Ctx, player_idx: usize, kind: u8, rune: u32) {
        if self.over || !self.both_seated() {
            return;
        }
        let id = match ctx.members.get(player_idx) {
            Some(m) => m.account_id.clone(),
            None => return,
        };
        let mark = self.mark_for(&id);
        if mark == 0 || mark != self.turn {
            return; // not seated, or not their turn.
        }
        // kind 0 = printable rune.
        if kind != 0 || rune < '1' as u32 || rune > '9' as u32 {
            return;
        }
        let cell = (rune - '1' as u32) as usize;
        if self.board[cell] != EMPTY {
            return; // occupied.
        }

        self.board[cell] = mark;
        self.moves += 1;

        if self.has_won(mark) {
            let w = self.id_of(mark).to_string();
            self.settle_win(ctx, &w);
            return;
        }
        if self.moves == 9 {
            self.settle_draw(ctx);
            return;
        }
        self.flip_turn();
        self.deadline = Some(ctx.now_unix_nanos + TURN_TIMEOUT_NANOS);
        self.render(ctx);
    }

    /// OnWake: forfeit the current mover if their turn deadline has passed.
    pub fn on_wake(&mut self, ctx: &Ctx) {
        if self.over || !self.both_seated() {
            return;
        }
        let deadline = match self.deadline {
            Some(d) => d,
            None => return,
        };
        // Go uses r.Now().After(deadline): strictly greater-than.
        if ctx.now_unix_nanos > deadline {
            let loser = self.id_of(self.turn).to_string();
            let winner = if loser == self.x_id {
                self.o_id.clone()
            } else {
                self.x_id.clone()
            };
            self.settle_forfeit(ctx, &winner, &loser);
        }
    }

    // ---- settling ----------------------------------------------------------

    fn settle_win(&mut self, ctx: &Ctx, winner_id: &str) {
        self.over = true;
        self.winner_id = winner_id.to_string();
        self.deadline = None;
        self.render(ctx);
        let loser_id = if self.x_id == winner_id {
            self.o_id.clone()
        } else {
            self.x_id.clone()
        };
        self.end(
            ctx,
            &[
                (winner_id.to_string(), 1, 1, STATUS_FINISHED),
                (loser_id, 0, 2, STATUS_FINISHED),
            ],
        );
    }

    fn settle_draw(&mut self, ctx: &Ctx) {
        self.over = true;
        self.winner_id = String::new();
        self.deadline = None;
        self.render(ctx);
        self.end(
            ctx,
            &[
                (self.x_id.clone(), 0, 1, STATUS_FINISHED),
                (self.o_id.clone(), 0, 1, STATUS_FINISHED),
            ],
        );
    }

    fn settle_forfeit(&mut self, ctx: &Ctx, winner_id: &str, loser_id: &str) {
        self.over = true;
        self.winner_id = winner_id.to_string();
        self.deadline = None;
        self.render(ctx);
        self.end(
            ctx,
            &[
                (winner_id.to_string(), 1, 1, STATUS_FINISHED),
                (loser_id.to_string(), 0, 2, STATUS_DNF),
            ],
        );
    }

    /// end builds a Result mapping each account id to its CURRENT roster index
    /// (the host scopes results against this callback's roster) and settles.
    fn end(&self, ctx: &Ctx, rows: &[(String, i64, u16, u8)]) {
        let mut rankings = Vec::with_capacity(rows.len());
        for (id, metric, rank, status) in rows {
            rankings.push(Ranking {
                player_idx: self.roster_index(ctx, id),
                metric: *metric,
                rank: *rank,
                status: *status,
            });
        }
        crate::host::end_room(&encode_result(&rankings));
    }

    /// roster_index resolves an account id to its index in the current
    /// callback's roster (the leaver is delivered as the final entry, so it is
    /// present). Falls back to 0 if absent, matching kit encodeResult.
    fn roster_index(&self, ctx: &Ctx, id: &str) -> u32 {
        for (i, m) in ctx.members.iter().enumerate() {
            if m.account_id == id {
                return i as u32;
            }
        }
        0
    }

    // ---- board helpers -----------------------------------------------------

    pub fn both_seated(&self) -> bool {
        !self.x_id.is_empty() && !self.o_id.is_empty()
    }

    fn mark_for(&self, id: &str) -> u8 {
        if id == self.x_id {
            MARK_X
        } else if id == self.o_id {
            MARK_O
        } else {
            0
        }
    }

    fn id_of(&self, mark: u8) -> &str {
        if mark == MARK_X {
            &self.x_id
        } else {
            &self.o_id
        }
    }

    fn flip_turn(&mut self) {
        self.turn = if self.turn == MARK_X { MARK_O } else { MARK_X };
    }

    fn has_won(&self, mark: u8) -> bool {
        WIN_LINES.iter().any(|line| {
            self.board[line[0]] == mark
                && self.board[line[1]] == mark
                && self.board[line[2]] == mark
        })
    }

    /// display_name resolves a seated account id to its stored handle, with a
    /// "(guest)" marker for guests — mirrors Player.DisplayName.
    pub fn display_name(&self, id: &str) -> String {
        match self.players.get(id) {
            Some(m) if m.kind == 0 => format!("{} (guest)", m.handle),
            Some(m) => m.handle.clone(),
            None => String::new(),
        }
    }

    fn render(&mut self, ctx: &Ctx) {
        let frame = render::compose(self, ctx);
        self.broadcaster.broadcast(&frame);
    }
}
