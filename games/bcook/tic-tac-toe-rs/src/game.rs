//! Tic-Tac-Toe room logic on the shellcade-kit SDK. Seats are keyed by
//! ACCOUNT ID so the room survives hibernation (connection tokens change;
//! account ids do not). Head-to-head seats the first two joiners as X and O;
//! a SOLO room (the lobby's Solo option, capacity 1) seats the one player on
//! BOTH marks and they alternate against themself. Frame delivery (baselines,
//! epochs, keyframes, retries) is the SDK's job — this file never touches the
//! wire.

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
/// can borrow both disjointly.
pub struct Match {
    pub players: HashMap<String, Player>, // account id -> last-seen player (names)
    pub solo: bool, // one player holds both seats and alternates marks
    pub board: [u8; 9],
    pub x_id: String,
    pub o_id: String,
    pub turn: u8,
    pub moves: i32,
    pub over: bool,
    pub winner_id: String,     // "" on draw or while playing
    pub winner_mark: u8,       // EMPTY on draw or while playing
    pub deadline: Option<i64>, // current turn's forfeit deadline (unix nanos)
}

impl Match {
    pub(crate) fn new(solo: bool) -> Self {
        Match {
            players: HashMap::new(),
            solo,
            board: [EMPTY; 9],
            x_id: String::new(),
            o_id: String::new(),
            turn: MARK_X,
            moves: 0,
            over: false,
            winner_id: String::new(),
            winner_mark: EMPTY,
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
    pub fn new(solo: bool) -> Self {
        TttRoom { m: Match::new(solo), frame: Frame::new() }
    }

    fn render(&mut self, r: &mut Room) {
        render::compose(&self.m, &mut self.frame);
        r.identical(&self.frame);
    }
}

impl Handler for TttRoom {
    /// Set Nav input context, X moves first. Render happens on join, not here.
    fn on_start(&mut self, r: &mut Room) {
        r.set_input_context(InputContext::Nav);
        self.m.turn = MARK_X;
    }

    /// Seat the joiner (head-to-head: first two take X then O; solo: the one
    /// player takes both marks). A re-join just re-renders. (Roster-change
    /// keyframing is the SDK's job now — no invalidate here.)
    fn on_join(&mut self, r: &mut Room, p: Player) {
        let id = p.account_id.clone();
        self.m.players.insert(id.clone(), p);
        self.m.seat(&id);

        // No idle-forfeit clock in solo: there is no opponent to protect.
        if !self.m.solo && !self.m.over && self.m.both_seated() && self.m.deadline.is_none() {
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
        if self.m.solo {
            // The solo player walked out. Before any move: clear the seats so
            // a peek-and-leave records nothing (mirrors the lone-seat cleanup
            // below). Mid-game: record the abandonment. Render either way —
            // a watching viewer should not keep a stale board.
            if self.m.moves == 0 {
                self.m.x_id.clear();
                self.m.o_id.clear();
                self.m.players.remove(&leaver);
                self.render(r);
                return;
            }
            self.m.over = true;
            self.m.deadline = None;
            self.render(r);
            self.end(r, &[(leaver, 0, 1, Status::Dnf)]);
            return;
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
        let mark = self.m.input_mark(&p.account_id);
        if mark == EMPTY {
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
            self.settle_win(r, mark);
            return;
        }
        if self.m.moves == 9 {
            self.settle_draw(r);
            return;
        }
        self.m.flip_turn();
        if !self.m.solo {
            self.m.deadline = Some(r.now_unix_nanos() + TURN_TIMEOUT_NANOS);
        }
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
        // Strictly greater-than: the deadline instant itself is still in time.
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

    fn settle_win(&mut self, r: &mut Room, mark: u8) {
        let winner_id = self.m.id_of(mark).to_string();
        self.m.over = true;
        self.m.winner_id = winner_id.clone();
        self.m.winner_mark = mark;
        self.m.deadline = None;
        self.render(r);
        if self.m.solo {
            // One person holds both seats: a single Finished row, not a
            // duplicate winner/loser pair against the same account.
            self.end(r, &[(winner_id, 1, 1, Status::Finished)]);
            return;
        }
        let loser_id = if self.m.x_id == winner_id {
            self.m.o_id.clone()
        } else {
            self.m.x_id.clone()
        };
        self.end(
            r,
            &[
                (winner_id, 1, 1, Status::Finished),
                (loser_id, 0, 2, Status::Finished),
            ],
        );
    }

    fn settle_draw(&mut self, r: &mut Room) {
        self.m.over = true;
        self.m.winner_id = String::new();
        self.m.winner_mark = EMPTY;
        self.m.deadline = None;
        self.render(r);
        if self.m.solo {
            self.end(r, &[(self.m.x_id.clone(), 0, 1, Status::Finished)]);
            return;
        }
        self.end(
            r,
            &[
                (self.m.x_id.clone(), 0, 1, Status::Finished),
                (self.m.o_id.clone(), 0, 1, Status::Finished),
            ],
        );
    }

    /// Head-to-head only: solo rooms never arm the forfeit clock and a solo
    /// leave settles in on_leave.
    fn settle_forfeit(&mut self, r: &mut Room, winner_id: &str, loser_id: &str) {
        self.m.over = true;
        self.m.winner_id = winner_id.to_string();
        self.m.winner_mark = if winner_id == self.m.x_id { MARK_X } else { MARK_O };
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

    /// seat assigns the joiner: in solo the one player takes BOTH seats and
    /// alternates marks; head-to-head the first two joiners take X then O.
    /// Already-seated ids (re-joins) are left alone.
    fn seat(&mut self, id: &str) {
        if id == self.x_id || id == self.o_id {
            return;
        }
        if self.solo {
            if self.x_id.is_empty() {
                self.x_id = id.to_string();
                self.o_id = id.to_string();
            }
        } else if self.x_id.is_empty() {
            self.x_id = id.to_string();
        } else if self.o_id.is_empty() {
            self.o_id = id.to_string();
        }
    }

    /// input_mark is the mark `id` may place RIGHT NOW: the current turn's
    /// mark when it is their move (in solo the one seated player owns both
    /// marks and alternates), EMPTY when not seated or out of turn.
    fn input_mark(&self, id: &str) -> u8 {
        if self.solo {
            return if self.mark_for(id) != EMPTY { self.turn } else { EMPTY };
        }
        let m = self.mark_for(id);
        if m == self.turn {
            m
        } else {
            EMPTY
        }
    }

    fn mark_for(&self, id: &str) -> u8 {
        if id == self.x_id {
            MARK_X
        } else if id == self.o_id {
            MARK_O
        } else {
            EMPTY
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

    /// Resolve an account id to its arcade character (populated for every
    /// roster member because the meta declares CTX_FEAT_CHARACTER); None for
    /// an id with no roster entry.
    pub fn character(&self, id: &str) -> Option<&Character> {
        self.players.get(id).map(|p| &p.character)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn win_detection_covers_rows_cols_diagonals() {
        let x = MARK_X;
        let won = |board: [u8; 9], mark: u8| {
            let mut m = Match::new(false);
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
        let mut m = Match::new(false);
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

    #[test]
    fn head_to_head_seating_and_turn_gating() {
        let mut m = Match::new(false);
        m.seat("a");
        assert!(!m.both_seated());
        m.seat("a"); // re-join is idempotent
        assert!(m.o_id.is_empty());
        m.seat("b");
        assert!(m.both_seated());
        m.seat("c"); // table full: a third joiner stays a viewer
        assert_eq!(m.input_mark("a"), MARK_X);
        assert_eq!(m.input_mark("b"), EMPTY); // not O's turn yet
        assert_eq!(m.input_mark("c"), EMPTY);
        m.flip_turn();
        assert_eq!(m.input_mark("a"), EMPTY);
        assert_eq!(m.input_mark("b"), MARK_O);
    }

    #[test]
    fn solo_seats_one_player_on_both_marks() {
        let mut m = Match::new(true);
        m.seat("a");
        assert!(m.both_seated());
        assert_eq!(m.x_id, "a");
        assert_eq!(m.o_id, "a");
        // The solo player places whichever mark is to move; a viewer never may.
        assert_eq!(m.input_mark("a"), MARK_X);
        m.flip_turn();
        assert_eq!(m.input_mark("a"), MARK_O);
        assert_eq!(m.input_mark("viewer"), EMPTY);
        // A later joiner can never displace the solo seats.
        m.seat("b");
        assert_eq!(m.x_id, "a");
        assert_eq!(m.o_id, "a");
        assert_eq!(m.input_mark("b"), EMPTY);
    }
}
