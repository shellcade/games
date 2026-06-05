//! The fixed 80x24 cell grid and its packed wire encoding (ABI.md §4.3),
//! mirroring the kit SDK's authoring Frame. Writes outside the grid are
//! clamped (never errored). A composed Frame packs to exactly 30720 bytes.

pub const ROWS: usize = 24;
pub const COLS: usize = 80;
pub const CELL_BYTES: usize = 16;
pub const FRAME_CELLS: usize = ROWS * COLS; // 1920
pub const FRAME_BYTES: usize = FRAME_CELLS * CELL_BYTES; // 30720

/// An optional truecolor value; `None`-like (unset) maps to the terminal
/// default. The standard palette below matches the kit canvas constants.
#[derive(Clone, Copy, PartialEq, Eq)]
pub struct Color {
    pub set: bool,
    pub r: u8,
    pub g: u8,
    pub b: u8,
}

impl Color {
    pub const fn unset() -> Self {
        Color { set: false, r: 0, g: 0, b: 0 }
    }
    pub const fn rgb(r: u8, g: u8, b: u8) -> Self {
        Color { set: true, r, g, b }
    }
    const fn gray(v: u8) -> Self {
        Color::rgb(v, v, v)
    }
}

// Standard palette (matches kit/internal/game/grid.go).
pub const WHITE: Color = Color::rgb(0xff, 0xff, 0xff);
pub const GREEN: Color = Color::rgb(0x55, 0xff, 0x55);
pub const YELLOW: Color = Color::rgb(0xff, 0xff, 0x55);
pub const CYAN: Color = Color::rgb(0x55, 0xff, 0xff);
pub const DIM_GRAY: Color = Color::gray(0x6c);

// Attribute bits (ABI.md §4.3): bit0 bold, bit1 dim, bit2 underline, bit3 reverse.
pub const ATTR_BOLD: u8 = 1 << 0;

/// Style bundles the styling applied when writing a rune.
#[derive(Clone, Copy)]
pub struct Style {
    pub fg: Color,
    pub bg: Color,
    pub attr: u8,
}

impl Style {
    pub const fn new(fg: Color, attr: u8) -> Self {
        Style { fg, bg: Color::unset(), attr }
    }
}

/// One drawable cell.
#[derive(Clone, Copy)]
pub struct Cell {
    pub rune: u32,
    pub fg: Color,
    pub bg: Color,
    pub attr: u8,
    pub cont: bool,
}

impl Cell {
    const fn blank() -> Self {
        Cell { rune: ' ' as u32, fg: Color::unset(), bg: Color::unset(), attr: 0, cont: false }
    }
}

/// The fixed 24x80 grid a game composes and broadcasts. Reused across renders.
pub struct Frame {
    pub cells: [Cell; FRAME_CELLS],
}

impl Frame {
    pub fn new() -> Self {
        Frame { cells: [Cell::blank(); FRAME_CELLS] }
    }

    fn in_bounds(row: i32, col: i32) -> bool {
        row >= 0 && (row as usize) < ROWS && col >= 0 && (col as usize) < COLS
    }

    /// Write one styled rune; out-of-bounds writes are clamped (dropped).
    pub fn set_rune(&mut self, row: i32, col: i32, r: char, st: Style) {
        if !Self::in_bounds(row, col) {
            return;
        }
        let i = (row as usize) * COLS + (col as usize);
        self.cells[i] = Cell { rune: r as u32, fg: st.fg, bg: st.bg, attr: st.attr, cont: false };
    }

    /// Write a string left-to-right, clamped to the row. Returns the next col.
    pub fn text(&mut self, row: i32, col: i32, s: &str, st: Style) -> i32 {
        let mut c = col;
        for ch in s.chars() {
            self.set_rune(row, c, ch, st);
            c += 1;
        }
        c
    }

    /// Write a string so it ends at col `end` (inclusive). Mirrors kit
    /// TextRight: start = end - len + 1, by char count.
    pub fn text_right(&mut self, row: i32, end: i32, s: &str, st: Style) {
        let n = s.chars().count() as i32;
        self.text(row, end - n + 1, s, st);
    }

    /// Pack the frame into `dst` (must be FRAME_BYTES). Matches wire.PutCell /
    /// ABI.md §4.3: u32 rune, fgSet+rgb, bgSet+rgb, attr, cont, u16 pad=0.
    pub fn pack_into(&self, dst: &mut [u8]) {
        debug_assert!(dst.len() >= FRAME_BYTES);
        for (i, cell) in self.cells.iter().enumerate() {
            let o = i * CELL_BYTES;
            dst[o..o + 4].copy_from_slice(&cell.rune.to_le_bytes());
            if cell.fg.set {
                dst[o + 4] = 1;
                dst[o + 5] = cell.fg.r;
                dst[o + 6] = cell.fg.g;
                dst[o + 7] = cell.fg.b;
            } else {
                dst[o + 4] = 0;
                dst[o + 5] = 0;
                dst[o + 6] = 0;
                dst[o + 7] = 0;
            }
            if cell.bg.set {
                dst[o + 8] = 1;
                dst[o + 9] = cell.bg.r;
                dst[o + 10] = cell.bg.g;
                dst[o + 11] = cell.bg.b;
            } else {
                dst[o + 8] = 0;
                dst[o + 9] = 0;
                dst[o + 10] = 0;
                dst[o + 11] = 0;
            }
            dst[o + 12] = cell.attr;
            dst[o + 13] = if cell.cont { 1 } else { 0 };
            dst[o + 14] = 0;
            dst[o + 15] = 0;
        }
    }

    pub fn pack(&self) -> Vec<u8> {
        let mut v = vec![0u8; FRAME_BYTES];
        self.pack_into(&mut v);
        v
    }
}
