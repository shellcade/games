//! Host transport: the kernel I/O plumbing (extism-pdk) plus the shellcade host
//! functions declared as RAW wasm imports (ABI.md §3, §7). The PDK's
//! #[host_fn] macro corrupts scalar args, so the host functions are imported
//! directly here.
//!
//! This module is wasm-only. Under a native target (`cargo test`) the extism
//! kernel symbols do not exist, so the whole transport is replaced by inert
//! stubs — the pure game/frame/delta logic is what the native tests exercise,
//! exactly as `bcook/diff-rs` tests its encoder natively.

// ---- wasm transport --------------------------------------------------------

#[cfg(target_arch = "wasm32")]
mod imp {
    use extism_pdk::extism;
    use extism_pdk::Memory;

    #[link(wasm_import_module = "extism:host/user")]
    extern "C" {
        /// identical(ptr frame) -> i64 epoch: deliver one delta container to
        /// every player. In v2 the return carries the authoritative u32 epoch in
        /// its low 32 bits (upper 32 reserved-zero; read only the low 32).
        fn identical(frame_off: u64) -> i64;
        /// set_input_context(i64 ctx): 0 Nav, 1 Command, 2 Text.
        fn set_input_context(ctx: i64);
        /// end(ptr result): settle the room exactly once.
        fn end(result_off: u64);
        /// log(i64 level, ptr msg): 0 debug, 1 info, 2 warn, 3 error.
        fn log(level: i64, msg_off: u64);
    }

    pub fn read_input() -> Vec<u8> {
        unsafe { extism::load_input() }
    }

    pub fn write_output(b: &[u8]) {
        let mem = Memory::from_bytes(b).expect("alloc output");
        mem.set_output();
    }

    /// Broadcast one delta container to every player (host `identical`) and
    /// return the authoritative epoch (low 32 bits of the i64 return).
    pub fn send_identical(payload: &[u8]) -> u32 {
        let mem = Memory::from_bytes(payload).expect("alloc payload");
        let off = mem.offset();
        let ret = unsafe { identical(off) };
        mem.free();
        ret as u64 as u32 // low 32 bits = epoch; upper 32 reserved-zero
    }

    pub fn set_ctx(ctx: i64) {
        unsafe { set_input_context(ctx) };
    }

    pub fn end_room(result: &[u8]) {
        let mem = Memory::from_bytes(result).expect("alloc result");
        let off = mem.offset();
        unsafe { end(off) };
        mem.free();
    }

    #[allow(dead_code)]
    pub fn host_log(msg: &str) {
        let mem = Memory::from_bytes(msg).expect("alloc log");
        let off = mem.offset();
        unsafe { log(1, off) };
        mem.free();
    }
}

// ---- native stubs (cargo test) ---------------------------------------------

#[cfg(not(target_arch = "wasm32"))]
mod imp {
    pub fn read_input() -> Vec<u8> {
        Vec::new()
    }
    pub fn write_output(_b: &[u8]) {}
    /// Native stub: no host, so model a host that always accepts the delta and
    /// echoes the epoch the guest sent unchanged (epoch in the payload's bytes
    /// 1..5). This keeps the guest's baseline/epoch bookkeeping exercisable in a
    /// native test without an Extism runtime.
    pub fn send_identical(payload: &[u8]) -> u32 {
        if payload.len() >= 5 {
            u32::from_le_bytes([payload[1], payload[2], payload[3], payload[4]])
        } else {
            0
        }
    }
    pub fn set_ctx(_ctx: i64) {}
    pub fn end_room(_result: &[u8]) {}
    #[allow(dead_code)]
    pub fn host_log(_msg: &str) {}
}

pub use imp::*;
