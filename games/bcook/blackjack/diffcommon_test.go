package main

// Shared capture helpers for the frame-diffing benchmark study (see
// diffcapture_test.go for the per-game drive). Serializes the host->guest wire
// payloads (packed per ABI.md §4.3) into a compact, lossless sequence file that
// the kit/internal/diffbench package replays.

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	kit "github.com/shellcade/kit"
	"github.com/shellcade/kit/kittest"
)

// seqOutDir is where captured sequences are written: the kit diffbench
// testdata directory. Override with DIFFBENCH_TESTDATA when the kit checkout is
// elsewhere; the default points at the sibling frame-diffing kit worktree.
func seqOutDir() string {
	if d := os.Getenv("DIFFBENCH_TESTDATA"); d != "" {
		return d
	}
	return "/Users/bcook/dev/shellcade/.worktrees/frame-diffing/kit/internal/diffbench/testdata"
}

// dcPackCell writes one packed 16-byte cell, matching wire.PutCell / ABI.md §4.3.
func dcPackCell(buf []byte, i int, c kit.Cell) {
	o := i * 16
	binary.LittleEndian.PutUint32(buf[o:], uint32(c.Rune))
	if c.FG.IsSet() {
		r, g, b := c.FG.RGBVals()
		buf[o+4], buf[o+5], buf[o+6], buf[o+7] = 1, r, g, b
	} else {
		buf[o+4], buf[o+5], buf[o+6], buf[o+7] = 0, 0, 0, 0
	}
	if c.BG.IsSet() {
		r, g, b := c.BG.RGBVals()
		buf[o+8], buf[o+9], buf[o+10], buf[o+11] = 1, r, g, b
	} else {
		buf[o+8], buf[o+9], buf[o+10], buf[o+11] = 0, 0, 0, 0
	}
	buf[o+12] = uint8(c.Attr)
	if c.Cont {
		buf[o+13] = 1
	} else {
		buf[o+13] = 0
	}
	buf[o+14], buf[o+15] = 0, 0
}

// dcPackFrame packs a full 24x80 frame into a fresh 30720-byte slice.
func dcPackFrame(f *kit.Frame) []byte {
	buf := make([]byte, 24*80*16)
	i := 0
	for row := 0; row < kit.Rows; row++ {
		for col := 0; col < kit.Cols; col++ {
			dcPackCell(buf, i, f.Cells[row][col])
			i++
		}
	}
	return buf
}

// dcWriteSeq serializes a packed-frame sequence in a compact, LOSSLESS on-disk
// form: each frame is stored as the set of 16-byte cells that differ from the
// previous frame (the first frame diffs against an all-zero frame). The
// diffbench loader reconstructs the exact full 30720-byte frames. This keeps
// committed testdata small (an idle/identical frame costs ~6 bytes) without
// distorting any benchmark, which always operates on the reconstructed frames.
//
// Format: magic "FSEQ", u32 version=2, u32 frameCount, then per frame:
//   u16 changedCount, then changedCount * (u16 cellIndex + 16 packed bytes).
func dcWriteSeq(t *testing.T, name string, frames [][]byte) {
	t.Helper()
	if err := os.MkdirAll(seqOutDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	out := make([]byte, 0, 12+len(frames)*64)
	out = append(out, 'F', 'S', 'E', 'Q')
	out = binary.LittleEndian.AppendUint32(out, 2)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(frames)))
	prev := make([]byte, 30720)
	for _, fr := range frames {
		if len(fr) != 30720 {
			t.Fatalf("frame is %d bytes, want 30720", len(fr))
		}
		var idxs []uint16
		for i := 0; i < 24*80; i++ {
			o := i * 16
			same := true
			for k := 0; k < 16; k++ {
				if fr[o+k] != prev[o+k] {
					same = false
					break
				}
			}
			if !same {
				idxs = append(idxs, uint16(i))
			}
		}
		out = binary.LittleEndian.AppendUint16(out, uint16(len(idxs)))
		for _, idx := range idxs {
			out = binary.LittleEndian.AppendUint16(out, idx)
			out = append(out, fr[int(idx)*16:int(idx)*16+16]...)
		}
		copy(prev, fr)
	}
	p := filepath.Join(seqOutDir(), name+".fseq")
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s: %d frames, %d bytes on disk", p, len(frames), len(out))
}

// dcRecorder collects, in send order, every frame the room emits to any player.
type dcRecorder struct {
	r      *kittest.Room
	counts map[string]int // accountID -> frames already drained
	frames [][]byte
}

func dcNewRecorder(r *kittest.Room) *dcRecorder {
	return &dcRecorder{r: r, counts: map[string]int{}}
}

// drain appends any newly-sent frames (across all players, in roster order) as
// packed payloads. This mirrors the wire: one send == one wire payload.
func (rec *dcRecorder) drain() {
	for _, p := range rec.r.Players {
		fs := rec.r.Frames[p.AccountID]
		seen := rec.counts[p.AccountID]
		for ; seen < len(fs); seen++ {
			rec.frames = append(rec.frames, dcPackFrame(fs[seen]))
		}
		rec.counts[p.AccountID] = seen
	}
}

func dcPlayer(id string) kit.Player {
	return kit.Player{AccountID: id, Handle: id, Kind: kit.KindMember, Conn: "conn-" + id}
}

func dcRuneIn(r rune) kit.Input { return kit.Input{Kind: kit.InputRune, Rune: r} }
func dcKeyIn(k kit.Key) kit.Input {
	return kit.Input{Kind: kit.InputKey, Key: k}
}
