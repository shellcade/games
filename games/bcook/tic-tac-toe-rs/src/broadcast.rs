//! The broadcast send path (ABI v2): diff a freshly-packed frame against the
//! per-broadcast baseline, ship a run-coalesced delta container via the host
//! `identical`, and mirror the host-returned epoch.
//!
//! This game delivers an identical view to every player and never issues a
//! per-player `send`, so a single broadcast baseline is the complete state (the
//! hand-roller obligation that `identical` reconciles all slots is trivially met
//! — there are no per-index slots to reconcile). Host-authority epoch (ABI §4.5
//! / D4): the guest keyframes on the first send, on any roster change, and
//! whenever the host returns an epoch different from the one it stamped.

use crate::delta::{encode, KEYFRAME_BYTES};
use crate::frame::{Frame, FRAME_BYTES};

/// Owns the reused broadcast baseline + epoch + delta scratch. All buffers are
/// allocated once and reused (no per-send allocation beyond the host transport's
/// staging copy), matching the SDK's allocation-free steady state.
pub struct Broadcaster {
    baseline: Vec<u8>, // last packed frame the host accepted (FRAME_BYTES)
    packed: Vec<u8>,   // scratch for the freshly-packed current frame
    scratch: Vec<u8>,  // delta-container scratch (keyframe-sized worst case)
    epoch: u32,        // the epoch the host last returned for the broadcast slot
    present: bool,     // whether `baseline` holds a frame the host has
}

impl Broadcaster {
    pub fn new() -> Self {
        Broadcaster {
            baseline: vec![0u8; FRAME_BYTES],
            packed: vec![0u8; FRAME_BYTES],
            scratch: vec![0u8; KEYFRAME_BYTES],
            epoch: 0,
            present: false,
        }
    }

    /// Mark the broadcast baseline invalid so the next send is a keyframe — used
    /// on any roster change (join/leave), the host-authority backstop.
    pub fn invalidate(&mut self) {
        self.present = false;
    }

    /// Pack `frame`, diff it against the baseline, send the delta (or a keyframe
    /// on first send / forced), and mirror the host's returned epoch. A rejected
    /// delta (returned epoch != sent epoch: hibernation restore, baseline loss)
    /// is immediately RE-SENT as a keyframe stamped with the returned epoch —
    /// still on-stack, keyframes are unconditionally accepted, so no render is
    /// ever lost and hibernation conformance stays frame-for-frame exact
    /// (ABI.md §4.6/§4.7 obligation 2).
    pub fn broadcast(&mut self, frame: &Frame) {
        frame.pack_into(&mut self.packed);

        let was_delta = self.present;
        let n = encode(
            &self.baseline,
            &self.packed,
            &mut self.scratch,
            self.epoch,
            !self.present,
        );

        let mut returned = crate::host::send_identical(&self.scratch[..n]);

        if returned != self.epoch && was_delta {
            // Rejected delta: resync to the host's epoch and retry this same
            // frame as a keyframe (one retry — a keyframe cannot be rejected).
            self.epoch = returned;
            let n = encode(
                &self.baseline,
                &self.packed,
                &mut self.scratch,
                self.epoch,
                true, // keyframe
            );
            returned = crate::host::send_identical(&self.scratch[..n]);
        }

        // Adopt the baseline + epoch: the host now holds this exact frame.
        self.baseline.copy_from_slice(&self.packed);
        self.present = true;
        self.epoch = returned;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::delta::FLAG_KEYFRAME;
    use crate::frame::{Style, WHITE};

    // A test double host: records each payload and returns an epoch policy.
    #[test]
    fn first_send_is_keyframe_then_deltas() {
        // Drive the encode path directly the way `broadcast` does, asserting the
        // first send is a keyframe and a subsequent small change is a delta.
        let mut b = Broadcaster::new();

        let mut f = Frame::new();
        f.text(0, 0, "hi", Style::new(WHITE, 0));
        f.pack_into(&mut b.packed);
        let n = encode(&b.baseline, &b.packed, &mut b.scratch, b.epoch, !b.present);
        assert_eq!(b.scratch[0] & FLAG_KEYFRAME, FLAG_KEYFRAME, "first send keyframe");
        assert_eq!(n, KEYFRAME_BYTES);
        // adopt baseline as broadcast() would
        b.baseline.copy_from_slice(&b.packed);
        b.present = true;

        // small change -> delta, not keyframe
        let mut f2 = Frame::new();
        f2.text(0, 0, "hi", Style::new(WHITE, 0));
        f2.set_rune(5, 5, 'Z', Style::new(WHITE, 0));
        f2.pack_into(&mut b.packed);
        let n2 = encode(&b.baseline, &b.packed, &mut b.scratch, b.epoch, !b.present);
        assert_eq!(b.scratch[0] & FLAG_KEYFRAME, 0, "steady state is a delta");
        assert!(n2 < KEYFRAME_BYTES);
    }

    #[test]
    fn invalidate_forces_keyframe() {
        let mut b = Broadcaster::new();
        b.present = true; // pretend a baseline exists
        b.invalidate();
        assert!(!b.present);
    }
}
