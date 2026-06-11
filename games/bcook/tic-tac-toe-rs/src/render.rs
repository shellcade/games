//! Rendering — a centered 3x3 board, a title, both players' names with marks
//! (each name preceded by the player's arcade character tile + one space),
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

// Plate names are capped so the two row-4 plates (mark + character tile +
// space + name each) can never meet in the middle of the 80-col canvas:
// left ends by col 8+24=32, right starts no earlier than col 72-24-2+1=47.
const NAME_MAX: usize = 24;

/// A display name clipped to the plate budget (char-counted, like Frame::text).
fn plate_name(rm: &Match, id: &str) -> String {
    rm.display_name(id).chars().take(NAME_MAX).collect()
}

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
    // Left plate: "X  " then (seated) character tile + space + name; an empty
    // seat keeps the original "(waiting)" placeholder with no tile.
    let col = f.text(4, 4, "X  ", style_for(MARK_X));
    if rm.x_id.is_empty() {
        f.text(4, col, "(waiting)", style_for(MARK_X));
    } else {
        if let Some(ch) = rm.character(&rm.x_id) {
            f.set(4, col, character_cell(ch));
        }
        f.text(4, col + 2, &plate_name(rm, &rm.x_id), style_for(MARK_X));
    }

    // Right plate: name + "  O" ending at COLS-5, with the character tile +
    // one space immediately before the name (tile, space, name — same reading
    // order as the left plate).
    f.text_right(4, COLS - 5, "  O", style_for(MARK_O));
    if rm.o_id.is_empty() {
        f.text_right(4, COLS - 8, "(waiting)", style_for(MARK_O));
    } else {
        let name = plate_name(rm, &rm.o_id);
        let name_col = COLS - 8 - name.chars().count() as i32 + 1;
        f.text(4, name_col, &name, style_for(MARK_O));
        if let Some(ch) = rm.character(&rm.o_id) {
            f.set(4, name_col - 2, character_cell(ch));
        }
    }

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
    if rm.over && !rm.winner_id.is_empty() && !rm.solo {
        // Head-to-head win: the winner's character tile + space + name,
        // centered as one unit (tile and message together span n+2 cols).
        let msg = format!("{} wins!", plate_name(rm, &rm.winner_id));
        let col = (COLS - (msg.chars().count() as i32 + 2)) / 2;
        if let Some(ch) = rm.character(&rm.winner_id) {
            f.set(row, col, character_cell(ch));
        }
        f.text(row, col + 2, &msg, st_win());
        return;
    }
    let (msg, st): (String, Style) = if !rm.both_seated() {
        ("Waiting for both players...".to_string(), st_wait())
    } else if rm.over && rm.winner_id.is_empty() {
        ("Draw!".to_string(), st_draw())
    } else if rm.over {
        // Solo: both seats are the same person — name the winning MARK.
        (format!("{} wins!", rm.winner_mark as char), st_win())
    } else if rm.turn == MARK_X {
        ("X to move".to_string(), st_turn())
    } else {
        ("O to move".to_string(), st_turn())
    };
    let n = msg.chars().count() as i32;
    f.text(row, (COLS - n) / 2, &msg, st);
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::game::Match;

    /// A seated member with an arcade character, as the SDK delivers when the
    /// meta declares CTX_FEAT_CHARACTER.
    fn member(id: &str, handle: &str, glyph: char, ink: (u8, u8, u8), bg: (u8, u8, u8)) -> Player {
        Player {
            account_id: id.to_string(),
            handle: handle.to_string(),
            kind: Kind::Member,
            character: Character {
                glyph: glyph.to_string(),
                ink_r: ink.0,
                ink_g: ink.1,
                ink_b: ink.2,
                bg_r: bg.0,
                bg_g: bg.1,
                bg_b: bg.2,
                fallback: b'@',
            },
            ..Player::default()
        }
    }

    fn seated_match() -> Match {
        let mut m = Match::new(false);
        m.players.insert(
            "a".into(),
            member("a", "Ana", '♞', (0xff, 0x00, 0x00), (0x00, 0x00, 0x40)),
        );
        m.players.insert(
            "b".into(),
            member("b", "Bob", '♛', (0x00, 0xff, 0x00), (0x40, 0x00, 0x00)),
        );
        m.x_id = "a".into();
        m.o_id = "b".into();
        m
    }

    fn row_text(f: &Frame, row: i32) -> String {
        (0..COLS).map(|c| f.get(row, c).rune).collect()
    }

    #[test]
    fn character_tile_renders_beside_each_name_plate() {
        let m = seated_match();
        let mut f = Frame::new();
        compose(&m, &mut f);

        // Left plate: "X  " at cols 4..6, Ana's tile at 7, a space, name at 9.
        let tile = f.get(4, 7);
        assert_eq!(tile.rune, '♞');
        assert_eq!(tile.fg, Color::rgb(0xff, 0x00, 0x00));
        assert_eq!(tile.bg, Color::rgb(0x00, 0x00, 0x40));
        assert_eq!(f.get(4, 8).rune, ' ');
        assert!(row_text(&f, 4).contains("♞ Ana"));

        // Right plate: "  O" ends at COLS-5 (col 75), the name ends at col 72,
        // and Bob's tile sits one space before the name.
        assert_eq!(f.get(4, COLS - 5).rune, 'O');
        let name_col = COLS - 8 - 3 + 1; // "Bob" is 3 chars -> col 70
        assert_eq!(f.get(4, name_col).rune, 'B');
        assert_eq!(f.get(4, name_col - 1).rune, ' ');
        let tile = f.get(4, name_col - 2);
        assert_eq!(tile.rune, '♛');
        assert_eq!(tile.fg, Color::rgb(0x00, 0xff, 0x00));
        assert_eq!(tile.bg, Color::rgb(0x40, 0x00, 0x00));
        assert!(row_text(&f, 4).contains("♛ Bob  O"));
    }

    #[test]
    fn winner_line_carries_the_winners_character_tile() {
        let mut m = seated_match();
        m.over = true;
        m.winner_id = "a".into();
        m.winner_mark = MARK_X;
        let mut f = Frame::new();
        compose(&m, &mut f);

        // "♞ Ana wins!" centered as one 11-col unit: tile at (80-11)/2 = 34.
        let row = 15; // BOARD_ROW + 3*2 + 1
        let tile = f.get(row, 34);
        assert_eq!(tile.rune, '♞');
        assert_eq!(tile.fg, Color::rgb(0xff, 0x00, 0x00));
        assert_eq!(f.get(row, 35).rune, ' ');
        assert!(row_text(&f, row).contains("♞ Ana wins!"));
    }

    #[test]
    fn long_names_truncate_and_keep_the_tile_adjacent_in_bounds() {
        let long = "x".repeat(40); // > NAME_MAX
        let mut m = Match::new(false);
        m.players
            .insert("a".into(), member("a", &long, '♟', (1, 2, 3), (4, 5, 6)));
        m.players
            .insert("b".into(), member("b", &long, '♜', (7, 8, 9), (1, 1, 1)));
        m.x_id = "a".into();
        m.o_id = "b".into();
        let mut f = Frame::new();
        compose(&m, &mut f);

        // Left: tile at 7, name clipped to NAME_MAX chars (cols 9..=32).
        assert_eq!(f.get(4, 7).rune, '♟');
        assert_eq!(f.get(4, 9 + NAME_MAX as i32 - 1).rune, 'x');
        // Clipped, not spilling past the budget.
        assert_eq!(f.get(4, 9 + NAME_MAX as i32).rune, ' ');
        // Right: clipped name ends at col 72; tile two cols before its start,
        // still right of the left plate (no mid-row collision).
        let name_col = COLS - 8 - NAME_MAX as i32 + 1; // col 49
        assert_eq!(f.get(4, name_col).rune, 'x');
        assert_eq!(f.get(4, name_col - 2).rune, '♜');
        assert!(name_col - 2 > 9 + NAME_MAX as i32);
    }
}
