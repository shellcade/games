package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// Determinism is the property the shared-bones fantasy rests on: the same
// (seed, depth) must produce byte-identical floors, every time, everywhere.
func TestGenDeterminism(t *testing.T) {
	for depth := 1; depth <= maxMVP; depth++ {
		a, b := genFloor(42, depth), genFloor(42, depth)
		if a.tiles != b.tiles || a.upX != b.upX || a.downX != b.downX {
			t.Fatalf("B%d: same seed produced different floors", depth)
		}
	}
	if genFloor(42, 3).tiles == genFloor(43, 3).tiles {
		t.Fatal("different seeds produced identical floors")
	}
	if genFloor(42, 3).tiles == genFloor(42, 4).tiles {
		t.Fatal("adjacent depths produced identical floors")
	}
}

// Every floor must be playable: stairs on open tiles, shrine depths carry a
// shrine adjacent to the down-stairs, and the down-stairs is reachable from
// the up-stairs (rooms are chain-connected by construction — verify anyway
// with a flood fill, the generator's contract).
func TestGenPlayable(t *testing.T) {
	for seed := int64(1); seed <= 25; seed++ {
		for depth := 1; depth <= maxMVP; depth++ {
			f := genFloor(seed, depth)
			if f.tiles[f.upY][f.upX] != tUp || f.tiles[f.downY][f.downX] != tDown {
				t.Fatalf("seed %d B%d: stairs not placed", seed, depth)
			}
			if hasShrine(depth) {
				if f.shrineX == 0 || f.tiles[f.shrineY][f.shrineX] != tShrine {
					t.Fatalf("seed %d B%d: shrine missing", seed, depth)
				}
				dx, dy := f.shrineX-f.downX, f.shrineY-f.downY
				if dx*dx+dy*dy != 1 {
					t.Fatalf("seed %d B%d: shrine not adjacent to down-stairs", seed, depth)
				}
			}
			if !reachable(f) {
				t.Fatalf("seed %d B%d: down-stairs unreachable from up-stairs", seed, depth)
			}
		}
	}
}

func reachable(f *floor) bool {
	var seen [floorH][floorW]bool
	stack := [][2]int{{f.upX, f.upY}}
	seen[f.upY][f.upX] = true
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if c[0] == f.downX && c[1] == f.downY {
			return true
		}
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := c[0]+d[0], c[1]+d[1]
			if f.open(nx, ny) && !seen[ny][nx] {
				seen[ny][nx] = true
				stack = append(stack, [2]int{nx, ny})
			}
		}
	}
	return false
}

// The canonical scaling operator (design §1) — determinism-critical, so the
// worked examples from the design are pinned as tests.
func TestScalingRounding(t *testing.T) {
	if got := scaled(12, hpScalar(12)); got != 20 { // 12 * 1.66 = 19.92
		t.Fatalf("goblin B12 HP = %d, want 20", got)
	}
	if got := scaled(6, hpScalar(5)); got != 7 { // 6 * 1.24 = 7.44
		t.Fatalf("kobold B5 HP = %d, want 7", got)
	}
}

// Combat replay determinism: two rooms with the same week seed and the same
// input script must produce IDENTICAL world states — every die roll comes
// from week-seed-derived actor PRNGs, never the wall clock. (The stage-5
// anti-cheat replays runs against exactly this property.)
func TestCombatReplayDeterminism(t *testing.T) {
	run := func() (string, int) {
		tr := kittest.NewRoom(bp("ada"))
		rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
		rm.OnStart(tr)
		rm.OnJoin(tr, bp("ada"))
		moves := []rune{'d', 's', 'a', 'w', 'd', 'd', 's', 's'}
		for i := 0; i < 400; i++ {
			tr.Advance(100 * time.Millisecond)
			rm.OnInput(tr, bp("ada"), kit.Input{Kind: kit.InputRune, Rune: moves[i%len(moves)]})
			rm.OnWake(tr)
		}
		// Fingerprint: every monster's position+HP and the delver's vitals.
		fp := ""
		for _, m := range rm.monsters {
			fp += m.sp.name + itoa(m.x) + "," + itoa(m.y) + ":" + itoa(m.hp) + ";"
		}
		d := rm.delvers["ada"]
		return fp + "|" + itoa(d.hp) + "," + itoa(d.x) + "," + itoa(d.y), d.hp
	}
	a, _ := run()
	b, _ := run()
	if a != b {
		t.Fatal("identical seeds + inputs diverged — combat is drawing non-deterministic randomness")
	}
}
