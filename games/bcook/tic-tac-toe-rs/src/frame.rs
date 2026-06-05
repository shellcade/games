//! The fixed 80x24 cell grid and its packed wire encoding (ABI.md §4.3,
//! v2 24-byte grapheme cell), mirroring the kit SDK's authoring Frame. Writes
//! outside the grid are clamped (never errored). A composed Frame packs to
//! exactly 46080 bytes.
//!
//! v2 cell anchor layout (24 bytes, little-endian):
//!   u32 rune @0 | u32 cp2 @4 | u32 cp3 @8        base + extra grapheme code points
//!   u8 fgSet,fgR,fgG,fgB @12 | u8 bgSet,bgR,bgG,bgB @16
//!   u8 attr @20 | u8 cont @21 | u16 pad @22 (zero)
//!
//! Canonical-zero rule: unused cp slots and pad MUST be zero, so cell equality
//! is exactly a 24-byte memcmp (load-bearing for the delta diff and hibernation
//! byte-identity). `pack_into` is the normative enforcer — it always writes
//! pad = 0 and zeroes any unset cp slot regardless of the in-memory Cell.

pub const ROWS: usize = 24;
pub const COLS: usize = 80;
/// v2 grapheme cell width.
pub const CELL_BYTES: usize = 24;
pub const FRAME_CELLS: usize = ROWS * COLS; // 1920
pub const FRAME_BYTES: usize = FRAME_CELLS * CELL_BYTES; // 46080

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

/// One drawable cell. `cp2`/`cp3` carry the extra code points of a grapheme
/// cluster (0 = unused); single-code-point content leaves them zero.
#[derive(Clone, Copy)]
pub struct Cell {
    pub rune: u32,
    pub cp2: u32,
    pub cp3: u32,
    pub fg: Color,
    pub bg: Color,
    pub attr: u8,
    pub cont: bool,
}

impl Cell {
    const fn blank() -> Self {
        Cell {
            rune: ' ' as u32,
            cp2: 0,
            cp3: 0,
            fg: Color::unset(),
            bg: Color::unset(),
            attr: 0,
            cont: false,
        }
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
        self.cells[i] = Cell {
            rune: r as u32,
            cp2: 0,
            cp3: 0,
            fg: st.fg,
            bg: st.bg,
            attr: st.attr,
            cont: false,
        };
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
    /// ABI.md §4.3 v2 anchor layout: u32 rune @0, u32 cp2 @4, u32 cp3 @8,
    /// fgSet+rgb @12, bgSet+rgb @16, attr @20, cont @21, u16 pad=0 @22. This is
    /// the canonical-zero enforcer: pad and unset cp slots are always written
    /// zero, regardless of the in-memory Cell.
    pub fn pack_into(&self, dst: &mut [u8]) {
        debug_assert!(dst.len() >= FRAME_BYTES);
        for (i, cell) in self.cells.iter().enumerate() {
            let o = i * CELL_BYTES;
            dst[o..o + 4].copy_from_slice(&cell.rune.to_le_bytes());
            dst[o + 4..o + 8].copy_from_slice(&cell.cp2.to_le_bytes());
            dst[o + 8..o + 12].copy_from_slice(&cell.cp3.to_le_bytes());
            if cell.fg.set {
                dst[o + 12] = 1;
                dst[o + 13] = cell.fg.r;
                dst[o + 14] = cell.fg.g;
                dst[o + 15] = cell.fg.b;
            } else {
                dst[o + 12] = 0;
                dst[o + 13] = 0;
                dst[o + 14] = 0;
                dst[o + 15] = 0;
            }
            if cell.bg.set {
                dst[o + 16] = 1;
                dst[o + 17] = cell.bg.r;
                dst[o + 18] = cell.bg.g;
                dst[o + 19] = cell.bg.b;
            } else {
                dst[o + 16] = 0;
                dst[o + 17] = 0;
                dst[o + 18] = 0;
                dst[o + 19] = 0;
            }
            dst[o + 20] = cell.attr;
            dst[o + 21] = if cell.cont { 1 } else { 0 };
            dst[o + 22] = 0; // pad (canonical zero)
            dst[o + 23] = 0;
        }
    }

    #[allow(dead_code)] // convenience packer; the hot path uses pack_into into a reused buffer
    pub fn pack(&self) -> Vec<u8> {
        let mut v = vec![0u8; FRAME_BYTES];
        self.pack_into(&mut v);
        v
    }
}
