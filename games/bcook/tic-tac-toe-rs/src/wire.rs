//! Wire encodings (ABI.md §4): the little-endian append encoder, the
//! bounds-checked decoder, the CallContext decode, and the Meta / Result
//! encoders. Target-agnostic (no host calls), mirroring kit/wire.

// ---- little-endian append encoder ------------------------------------------

pub struct Buf {
    pub b: Vec<u8>,
}

impl Buf {
    pub fn new() -> Self {
        Buf { b: Vec::new() }
    }
    pub fn u8(&mut self, v: u8) {
        self.b.push(v);
    }
    pub fn u16(&mut self, v: u16) {
        self.b.extend_from_slice(&v.to_le_bytes());
    }
    pub fn u32(&mut self, v: u32) {
        self.b.extend_from_slice(&v.to_le_bytes());
    }
    pub fn i64(&mut self, v: i64) {
        self.b.extend_from_slice(&v.to_le_bytes());
    }
    /// str: u16 length || UTF-8 bytes (ABI.md §preamble).
    pub fn str(&mut self, s: &str) {
        let bytes = s.as_bytes();
        let n = bytes.len().min(0xffff);
        self.u16(n as u16);
        self.b.extend_from_slice(&bytes[..n]);
    }
}

// ---- bounds-checked decoder (matches wire.Rd: short reads degrade to zero) --

pub struct Rd<'a> {
    b: &'a [u8],
    off: usize,
    bad: bool,
}

impl<'a> Rd<'a> {
    pub fn new(b: &'a [u8]) -> Self {
        Rd { b, off: 0, bad: false }
    }
    fn ok(&mut self, n: usize) -> bool {
        if self.bad || self.off + n > self.b.len() {
            self.bad = true;
            return false;
        }
        true
    }
    pub fn u8(&mut self) -> u8 {
        if !self.ok(1) {
            return 0;
        }
        let v = self.b[self.off];
        self.off += 1;
        v
    }
    pub fn u16(&mut self) -> u16 {
        if !self.ok(2) {
            return 0;
        }
        let v = u16::from_le_bytes([self.b[self.off], self.b[self.off + 1]]);
        self.off += 2;
        v
    }
    pub fn u32(&mut self) -> u32 {
        if !self.ok(4) {
            return 0;
        }
        let mut a = [0u8; 4];
        a.copy_from_slice(&self.b[self.off..self.off + 4]);
        self.off += 4;
        u32::from_le_bytes(a)
    }
    pub fn i64(&mut self) -> i64 {
        if !self.ok(8) {
            return 0;
        }
        let mut a = [0u8; 8];
        a.copy_from_slice(&self.b[self.off..self.off + 8]);
        self.off += 8;
        i64::from_le_bytes(a)
    }
    pub fn string(&mut self) -> String {
        let n = self.u16() as usize;
        if !self.ok(n) {
            return String::new();
        }
        let s = String::from_utf8_lossy(&self.b[self.off..self.off + n]).into_owned();
        self.off += n;
        s
    }
}

// ---- CallContext (§4.1) ----------------------------------------------------

#[derive(Clone)]
pub struct Member {
    pub handle: String,
    pub account_id: String,
    pub kind: u8, // 0 guest, 1 member
}

pub struct Ctx {
    pub now_unix_nanos: i64,
    pub members: Vec<Member>,
}

/// Decode the CallContext prefix and return it plus the reader positioned at
/// the trailing per-export args (e.g. playerIdx for join/leave/input).
pub fn decode_ctx<'a>(input: &'a [u8]) -> (Ctx, Rd<'a>) {
    let mut r = Rd::new(input);
    let now = r.i64(); // nowUnixNanos
    r.i64(); // seed
    r.u8(); // seedSet
    r.u8(); // mode
    r.u16(); // capacity
    r.u16(); // minPlayers
    let n = r.u16() as usize; // memberCount
    let mut members = Vec::with_capacity(n);
    for _ in 0..n {
        let handle = r.string();
        let account_id = r.string();
        let _conn = r.string();
        let kind = r.u8();
        members.push(Member { handle, account_id, kind });
    }
    r.u8(); // settled
    (Ctx { now_unix_nanos: now, members }, r)
}

// ---- Result (§4.4) ---------------------------------------------------------

pub const STATUS_FINISHED: u8 = 0;
pub const STATUS_DNF: u8 = 1;

pub struct Ranking {
    pub player_idx: u32,
    pub metric: i64,
    pub rank: u16,
    pub status: u8,
}

pub fn encode_result(rankings: &[Ranking]) -> Vec<u8> {
    let mut w = Buf::new();
    w.u16(rankings.len() as u16);
    for rk in rankings {
        w.u32(rk.player_idx);
        w.i64(rk.metric);
        w.u16(rk.rank);
        w.u8(rk.status);
    }
    w.b
}
