//! Rendering — a centered 3x3 board, a title, both players' names with marks,
//! and a status line. The view is identical for everyone, so the caller
//! broadcasts one composed frame with `identical` (the SDK's delta path does
//! the rest).

use shellcade_kit::prelude::*;

use crate::game::{Match, MARK_O, MARK_X};

// Styles.
fn st_title() -> Style { Style::new(WHITE, ATTR_BOLD) }
fn st_dim() -> Style { Style::new(DIM_GRAY, 0) }
fn st_grid() -> Style { Style::new(DIM_GRAY, 0) }
fn st_x() -> Style { Style::new(CYAN, ATTR_BOLD) }
fn st_o() -> Style { Style::new(YELLOW, ATTR_BOLD) }
fn st_empty() -> Style { Style::new(DIM_GRAY, 0) }
fn st_turn() -> Style { Style::new(WHITE, ATTR_BOLD) }
fn st_win() -> Style { Style::new(GREEN, ATTR_BOLD) }
fn st_draw() -> Style { Style::new(YELLOW, ATTR_BOLD) }
fn st_wait() -> Style { Style::new(DIM_GRAY, 0) }

// Board geometry: each cell 5 wide x 1 tall, separated by grid lines, centered.
const CELL_W: i32 = 5;
const BOARD_W: i32 = CELL_W * 3 + 2; // two vertical separators
const BOARD_COL: i32 = (COLS - BOARD_W) / 2;
const BOARD_ROW: i32 = 8;

/// compose rebuilds the full frame for the current match state into the
/// caller's reused Frame (clear + compose — the allocation-free steady state).
pub fn compose(rm: &Match, f: &mut Frame) {
    f.clear();

    // Title.
    let title = "TIC - TAC - TOE";
    f.text(1, (COLS - title.len() as i32) / 2, title, st_title());

    draw_players(rm, f);
    draw_board(rm, f);
    draw_status(rm, f);

    let hint = "Press 1-9 to place your mark";
    f.text(ROWS - 2, (COLS - hint.len() as i32) / 2, hint, st_dim());
}

fn style_for(mark: u8) -> Style {
    if mark == MARK_X {
        st_x()
    } else {
        st_o()
    }
}

fn draw_players(rm: &Match, f: &mut Frame) {
    let x_name = if rm.x_id.is_empty() { "(waiting)".to_string() } else { rm.display_name(&rm.x_id) };
    let o_name = if rm.o_id.is_empty() { "(waiting)".to_string() } else { rm.display_name(&rm.o_id) };

    let left = format!("X  {}", x_name);
    let right = format!("{}  O", o_name);
    f.text(4, 4, &left, style_for(MARK_X));
    f.text_right(4, COLS - 5, &right, style_for(MARK_O));

    if !rm.over && rm.both_seated() {
        if rm.turn == MARK_X {
            f.set_rune(4, 2, '>', st_turn());
        } else {
            f.set_rune(4, COLS - 3, '<', st_turn());
        }
    }
}

fn draw_board(rm: &Match, f: &mut Frame) {
    for cell in 0..9i32 {
        let rowq = cell / 3;
        let colq = cell % 3;
        let cr = BOARD_ROW + rowq * 2;
        let cc = BOARD_COL + colq * (CELL_W + 1);
        let mid = cc + CELL_W / 2;
        match rm.board[cell as usize] {
            MARK_X => f.set_rune(cr, mid, 'X', st_x()),
            MARK_O => f.set_rune(cr, mid, 'O', st_o()),
            _ => f.set_rune(cr, mid, (b'1' + cell as u8) as char, st_empty()),
        }
    }
    // Vertical separators between columns (after col 0 and col 1).
    for q in 1..=2i32 {
        let sep = BOARD_COL + q * (CELL_W + 1) - 1;
        for rowq in 0..3i32 {
            f.set_rune(BOARD_ROW + rowq * 2, sep, '|', st_grid());
        }
    }
    // Horizontal separators between rows.
    for q in 1..=2i32 {
        let sr = BOARD_ROW + q * 2 - 1;
        for c in 0..BOARD_W {
            f.set_rune(sr, BOARD_COL + c, '-', st_grid());
        }
        // Crosses where the lines meet.
        for j in 1..=2i32 {
            f.set_rune(sr, BOARD_COL + j * (CELL_W + 1) - 1, '+', st_grid());
        }
    }
}

fn draw_status(rm: &Match, f: &mut Frame) {
    let row = BOARD_ROW + 3 * 2 + 1;
    let (msg, st): (String, Style) = if !rm.both_seated() {
        ("Waiting for both players...".to_string(), st_wait())
    } else if rm.over && rm.winner_id.is_empty() {
        ("Draw!".to_string(), st_draw())
    } else if rm.over && rm.solo {
        // Solo: both seats are the same person — name the winning MARK.
        (format!("{} wins!", rm.winner_mark as char), st_win())
    } else if rm.over {
        (format!("{} wins!", rm.display_name(&rm.winner_id)), st_win())
    } else if rm.turn == MARK_X {
        ("X to move".to_string(), st_turn())
    } else {
        ("O to move".to_string(), st_turn())
    };
    let n = msg.chars().count() as i32;
    f.text(row, (COLS - n) / 2, &msg, st);
}
