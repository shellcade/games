//! Host transport: the kernel I/O plumbing (extism-pdk) plus the shellcade host
//! functions declared as RAW wasm imports (ABI.md §3, §7). The PDK's
//! #[host_fn] macro corrupts scalar args, so the host functions are imported
//! directly here. This module is wasm-only; host-side `cargo test` never links it.

use extism_pdk::extism;
use extism_pdk::Memory;

#[link(wasm_import_module = "extism:host/user")]
extern "C" {
    /// identical(ptr frame): deliver one frame to every player.
    fn identical(frame_off: u64);
    /// set_input_context(i64 ctx): 0 Nav, 1 Command, 2 Text.
    fn set_input_context(ctx: i64);
    /// end(ptr result): settle the room exactly once.
    fn end(result_off: u64);
    /// log(i64 level, ptr msg): 0 debug, 1 info, 2 warn, 3 error.
    fn log(level: i64, msg_off: u64);
}

/// Read this export's input payload (the CallContext and any trailing args).
pub fn read_input() -> Vec<u8> {
    unsafe { extism::load_input() }
}

/// Store a byte slice as this export's output value.
pub fn write_output(b: &[u8]) {
    let mem = Memory::from_bytes(b).expect("alloc output");
    mem.set_output();
}

/// Broadcast one packed frame to every player (host `identical`).
pub fn send_identical(frame: &[u8]) {
    let mem = Memory::from_bytes(frame).expect("alloc frame");
    let off = mem.offset();
    unsafe { identical(off) };
    mem.free();
}

/// Set the room input context (Nav / Command / Text).
pub fn set_ctx(ctx: i64) {
    unsafe { set_input_context(ctx) };
}

/// Settle the room with a packed Result (§4.4).
pub fn end_room(result: &[u8]) {
    let mem = Memory::from_bytes(result).expect("alloc result");
    let off = mem.offset();
    unsafe { end(off) };
    mem.free();
}

/// Emit a guest log line at info level.
#[allow(dead_code)]
pub fn host_log(msg: &str) {
    let mem = Memory::from_bytes(msg).expect("alloc log");
    let off = mem.offset();
    unsafe { log(1, off) };
    mem.free();
}
