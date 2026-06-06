//! Tic-Tac-Toe room logic — a faithful behavioral port of the Go catalog game
//! (../tic-tac-toe/game.go), on the shellcade-kit SDK. Seats are keyed by
//! ACCOUNT ID so the room survives hibernation (connection tokens change;
//! account ids do not). Frame delivery (baselines, epochs, keyframes, retries)
//! is the SDK's job — this file never touches the wire.

use std::collections::HashMap;

use shellcade_kit::prelude::*;

use crate::render;

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

/// The pure match state, separate from the reused render frame so rendering
/// can borrow both disjointly. Mirrors the Go `room` struct.
pub struct Match {
    pub players: HashMap<String, Player>, // account id -> last-seen player (names)
    pub board: [u8; 9],
    pub x_id: String,
    pub o_id: String,
    pub turn: u8,
    pub moves: i32,
    pub over: bool,
    pub winner_id: String,     // "" on draw or while playing
    pub deadline: Option<i64>, // current turn's forfeit deadline (unix nanos)
}

impl Match {
    fn new() -> Self {
        Match {
            players: HashMap::new(),
            board: [EMPTY; 9],
            x_id: String::new(),
            o_id: String::new(),
            turn: MARK_X,
            moves: 0,
            over: false,
            winner_id: String::new(),
            deadline: None,
        }
    }
}

/// The Handler: match state + one reused Frame (allocation-free steady state).
pub struct TttRoom {
    m: Match,
    frame: Frame,
}

impl TttRoom {
    pub fn new() -> Self {
        TttRoom { m: Match::new(), frame: Frame::new() }
    }

    fn render(&mut self, r: &mut Room) {
        render::compose(&self.m, &mut self.frame);
        r.identical(&self.frame);
    }
}

impl Handler for TttRoom {
    /// Set Nav input context, X moves first. The Go game does not render in
    /// OnStart (it renders on join); match that.
    fn on_start(&mut self, r: &mut Room) {
        r.set_input_context(InputContext::Nav);
        self.m.turn = MARK_X;
    }

    /// Seat the first two joiners as X then O. A re-join just re-renders.
    /// (Roster-change keyframing is the SDK's job now — no invalidate here.)
    fn on_join(&mut self, r: &mut Room, p: Player) {
        let id = p.account_id.clone();
        self.m.players.insert(id.clone(), p);

        if id != self.m.x_id && id != self.m.o_id {
            if self.m.x_id.is_empty() {
                self.m.x_id = id.clone();
            } else if self.m.o_id.is_empty() {
                self.m.o_id = id;
            }
        }

        if !self.m.over && self.m.both_seated() && self.m.deadline.is_none() {
            self.m.deadline = Some(r.now_unix_nanos() + TURN_TIMEOUT_NANOS);
        }
        self.render(r);
    }

    /// A seated player leaving mid-game forfeits; otherwise clean up the seat.
    fn on_leave(&mut self, r: &mut Room, p: Player) {
        if self.m.over {
            return;
        }
        let leaver = p.account_id;
        if leaver != self.m.x_id && leaver != self.m.o_id {
            return; // a non-seated viewer left.
        }
        let winner = if leaver == self.m.o_id {
            self.m.x_id.clone()
        } else {
            self.m.o_id.clone()
        };
        if winner.is_empty() {
            // The only seated player left before an opponent arrived: clear the seat.
            if leaver == self.m.x_id {
                self.m.x_id.clear();
            } else {
                self.m.o_id.clear();
            }
            self.m.deadline = None;
            self.m.players.remove(&leaver);
            self.render(r);
            return;
        }
        self.settle_forfeit(r, &winner, &leaver);
    }

    /// Place a mark for the current player. Out-of-turn, non-digit, and
    /// occupied-cell input is ignored (no re-render — render on change only).
    fn on_input(&mut self, r: &mut Room, p: Player, input: Input) {
        if self.m.over || !self.m.both_seated() {
            return;
        }
        let mark = self.m.mark_for(&p.account_id);
        if mark == 0 || mark != self.m.turn {
            return; // not seated, or not their turn.
        }
        let Input::Char(c @ '1'..='9') = input else {
            return;
        };
        let cell = (c as u32 - '1' as u32) as usize;
        if self.m.board[cell] != EMPTY {
            return; // occupied.
        }

        self.m.board[cell] = mark;
        self.m.moves += 1;

        if self.m.has_won(mark) {
            let w = self.m.id_of(mark).to_string();
            self.settle_win(r, &w);
            return;
        }
        if self.m.moves == 9 {
            self.settle_draw(r);
            return;
        }
        self.m.flip_turn();
        self.m.deadline = Some(r.now_unix_nanos() + TURN_TIMEOUT_NANOS);
        self.render(r);
    }

    /// Forfeit the current mover if their turn deadline has passed.
    fn on_wake(&mut self, r: &mut Room) {
        if self.m.over || !self.m.both_seated() {
            return;
        }
        let Some(deadline) = self.m.deadline else {
            return;
        };
        // Go uses r.Now().After(deadline): strictly greater-than.
        if r.now_unix_nanos() > deadline {
            let loser = self.m.id_of(self.m.turn).to_string();
            let winner = if loser == self.m.x_id {
                self.m.o_id.clone()
            } else {
                self.m.x_id.clone()
            };
            self.settle_forfeit(r, &winner, &loser);
        }
    }
}

impl TttRoom {
    // ---- settling ----------------------------------------------------------

    fn settle_win(&mut self, r: &mut Room, winner_id: &str) {
        self.m.over = true;
        self.m.winner_id = winner_id.to_string();
        self.m.deadline = None;
        self.render(r);
        let loser_id = if self.m.x_id == winner_id {
            self.m.o_id.clone()
        } else {
            self.m.x_id.clone()
        };
        self.end(
            r,
            &[
                (winner_id.to_string(), 1, 1, Status::Finished),
                (loser_id, 0, 2, Status::Finished),
            ],
        );
    }

    fn settle_draw(&mut self, r: &mut Room) {
        self.m.over = true;
        self.m.winner_id = String::new();
        self.m.deadline = None;
        self.render(r);
        self.end(
            r,
            &[
                (self.m.x_id.clone(), 0, 1, Status::Finished),
                (self.m.o_id.clone(), 0, 1, Status::Finished),
            ],
        );
    }

    fn settle_forfeit(&mut self, r: &mut Room, winner_id: &str, loser_id: &str) {
        self.m.over = true;
        self.m.winner_id = winner_id.to_string();
        self.m.deadline = None;
        self.render(r);
        self.end(
            r,
            &[
                (winner_id.to_string(), 1, 1, Status::Finished),
                (loser_id.to_string(), 0, 2, Status::Dnf),
            ],
        );
    }

    /// Build an Outcome by resolving each account id against the CURRENT
    /// roster (the leaver is delivered as the final entry, so it is present) —
    /// the SDK maps each Player to its roster index on the wire.
    fn end(&mut self, r: &mut Room, rows: &[(String, i64, u16, Status)]) {
        let rankings = rows
            .iter()
            .filter_map(|(id, metric, rank, status)| {
                let player = r.members().iter().find(|m| m.account_id == *id).cloned()?;
                Some(PlayerResult { player, metric: *metric, rank: *rank, status: *status })
            })
            .collect();
        r.end(&Outcome { rankings });
    }
}

impl Match {
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

    /// Resolve a seated account id to its stored display name (the SDK Player
    /// carries the "(guest)" marker logic).
    pub fn display_name(&self, id: &str) -> String {
        self.players.get(id).map_or(String::new(), Player::display_name)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn win_detection_covers_rows_cols_diagonals() {
        let x = MARK_X;
        let won = |board: [u8; 9], mark: u8| {
            let mut m = Match::new();
            m.board = board;
            m.has_won(mark)
        };
        assert!(won([x, x, x, 0, 0, 0, 0, 0, 0], x)); // top row
        assert!(won([x, 0, 0, x, 0, 0, x, 0, 0], x)); // left col
        assert!(won([x, 0, 0, 0, x, 0, 0, 0, x], x)); // diagonal
        assert!(won([0, 0, x, 0, x, 0, x, 0, 0], x)); // anti-diagonal
        assert!(!won([x, x, 0, 0, 0, 0, 0, 0, 0], x));
        assert!(!won([x, x, x, 0, 0, 0, 0, 0, 0], MARK_O)); // other mark
    }

    #[test]
    fn turn_flip_alternates_and_seating_reads() {
        let mut m = Match::new();
        assert_eq!(m.turn, MARK_X);
        m.flip_turn();
        assert_eq!(m.turn, MARK_O);
        m.x_id = "a".into();
        assert!(!m.both_seated());
        m.o_id = "b".into();
        assert!(m.both_seated());
        assert_eq!(m.mark_for("a"), MARK_X);
        assert_eq!(m.mark_for("b"), MARK_O);
        assert_eq!(m.mark_for("c"), 0);
        assert_eq!(m.id_of(MARK_O), "b");
    }
}
