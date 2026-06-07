package main

// The player-count budget gates: every edition of BONEYARD must support the
// declared 100-player room on production hardware. These run native (kittest),
// which cannot measure wasm speed directly — but it measures the two things
// that transfer exactly:
//
//   1. ALLOCATIONS. Production builds use -gc=leaking, so allocs/wake IS the
//      permanent leak rate, identical native or wasm.
//   2. RELATIVE CPU. The wasm+Fly multiplier over native was calibrated by the
//      platform load spike (~3-5x wasm, ~3-4x Fly): the native wall-time
//      budgets below carry that headroom to the 100ms heartbeat.
//
// They are TESTS, not benchmarks: CI fails when a change breaks the budget.
// (BenchmarkWake100 exists alongside for profiling.)

import (
	"runtime"
	"sort"
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

const benchPlayers = 100

// benchRoom builds a 100-player room with delvers spread across the MVP band,
// floors generated and populated — the shape of a busy week.
func benchRoom(t testing.TB) (*kittest.Room, *room, []kit.Player) {
	players := make([]kit.Player, benchPlayers)
	for i := range players {
		players[i] = kit.Player{AccountID: "acct-" + itoa(i), Handle: "bot" + itoa(i), Kind: kit.KindMember, Conn: "c-" + itoa(i)}
	}
	tr := kittest.NewRoom(players...)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	for _, p := range players {
		rm.OnJoin(tr, p)
	}
	for f := 1; f <= maxMVP; f++ {
		rm.floorAt(f)
	}
	i := 0
	for _, p := range players {
		d := rm.delvers[p.AccountID]
		d.floor = 1 + i%maxMVP
		f := rm.world.at(d.floor)
		d.x, d.y = f.upX, f.upY
		d.reveal(f)
		i++
	}
	return tr, rm, players
}

// driveWakes runs n 100ms wakes with a realistic input mix (~1.5 moves/sec per
// player ⇒ 15 inputs per wake) and returns per-wake wall times.
func driveWakes(tr *kittest.Room, rm *room, players []kit.Player, n int) []time.Duration {
	moves := []rune{'h', 'j', 'k', 'l'}
	durs := make([]time.Duration, 0, n)
	for w := 0; w < n; w++ {
		for i := 0; i < benchPlayers*3/20; i++ { // 15 inputs per 100ms wake
			p := players[(w*15+i)%benchPlayers]
			rm.OnInput(tr, p, kit.Input{Kind: kit.InputRune, Rune: moves[(w+i)%4]})
		}
		tr.Advance(100 * time.Millisecond)
		st := time.Now()
		rm.OnWake(tr)
		durs = append(durs, time.Since(st))
	}
	return durs
}

// GATE 1 — wake wall-time at 100 players. Native budgets carry ~30x headroom
// to the wasm+Fly 100ms heartbeat (and ~10x slack for noisy CI runners).
func TestWakeBudget100Players(t *testing.T) {
	if testing.Short() {
		t.Skip("budget gate: skipped under -short")
	}
	tr, rm, players := benchRoom(t)
	driveWakes(tr, rm, players, 20) // warmup
	durs := driveWakes(tr, rm, players, 200)
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	p50, p95 := durs[100], durs[190]
	t.Logf("wake @100p: p50=%v p95=%v max=%v", p50, p95, durs[199])
	if p50 > 2*time.Millisecond {
		t.Fatalf("wake p50 %v breaches the 2ms native budget (≈100ms heartbeat on prod after wasm+Fly multipliers)", p50)
	}
	if p95 > 8*time.Millisecond {
		t.Fatalf("wake p95 %v breaches the 8ms native budget", p95)
	}
}

// GATE 2 — steady-state allocations: an idle wake (no inputs) must allocate
// (almost) nothing, because under -gc=leaking every allocation is forever.
// The bound covers monster-wander message-free ticks; it is deliberately
// tight — raising it is a design decision, not a tweak.
func TestSteadyStateWakeAllocs(t *testing.T) {
	tr, rm, players := benchRoom(t)
	driveWakes(tr, rm, players, 20) // settle
	allocs := testing.AllocsPerRun(50, func() {
		tr.Advance(100 * time.Millisecond)
		rm.OnWake(tr)
	})
	t.Logf("idle wake allocs/op: %.1f", allocs)
	if allocs > 60 {
		t.Fatalf("idle wake allocates %.1f/op — permanent growth at 10 wakes/sec under -gc=leaking (budget 60)", allocs)
	}
}

// GATE 3 — death must not ALLOCATE: production runs -gc=leaking, where every
// byte ever allocated is permanent — native retained-heap can't see that (the
// native GC frees the orphan), so the gate measures TotalAlloc per death. The
// corpse record itself is intentional permanence (~100B); the budget rides
// above it but far below a stranded fog array (5.6KB/floor) or a rebuilt
// delver. A busy week is hundreds of deaths; every byte here is resident-room
// memory AND checkpoint size, forever.
func TestDeathAllocBudget(t *testing.T) {
	tr, rm, players := benchRoom(t)
	driveWakes(tr, rm, players, 10)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	const deaths = 50
	for i := 0; i < deaths; i++ {
		d := rm.delvers[players[i%benchPlayers].AccountID]
		d.floor = 2 + i%5
		rm.floorAt(d.floor)
		rm.die(tr, d, "the budget gate")
	}
	runtime.ReadMemStats(&after)
	perDeath := int64(after.TotalAlloc-before.TotalAlloc) / deaths
	t.Logf("allocated: %d bytes/death (permanent under -gc=leaking)", perDeath)
	if perDeath > 2*1024 {
		t.Fatalf("%d bytes allocated per death — permanent at -gc=leaking; a busy week strands megabytes in the resident room and its checkpoints (budget 2KB)", perDeath)
	}
}

// BenchmarkWake100 is the profiling companion (not a gate): -bench it to see
// where wake time goes; -benchmem to watch the allocation profile.
func BenchmarkWake100(b *testing.B) {
	tr, rm, players := benchRoom(b)
	driveWakes(tr, rm, players, 20)
	b.ReportAllocs()
	b.ResetTimer()
	moves := []rune{'h', 'j', 'k', 'l'}
	for w := 0; w < b.N; w++ {
		for i := 0; i < 15; i++ {
			p := players[(w*15+i)%benchPlayers]
			rm.OnInput(tr, p, kit.Input{Kind: kit.InputRune, Rune: moves[(w+i)%4]})
		}
		tr.Advance(100 * time.Millisecond)
		rm.OnWake(tr)
	}
}

// GATE 4 — kit torch economics stay exact: LANTERN's 480t at 0.6x is 800t
// effective (design §7) — more than BLADE's 600 despite the lower face value.
func TestKitTorchEconomics(t *testing.T) {
	burnAll := func(k *kitDef) int {
		d := &delver{}
		d.applyKit(k)
		n := 0
		for d.torch > 0 && n < 2000 {
			d.burn(1)
			n++
		}
		return n
	}
	blade, lantern := burnAll(&kits[0]), burnAll(&kits[1])
	if blade != 600 {
		t.Fatalf("BLADE burns out after %d units, want 600", blade)
	}
	if lantern != 800 {
		t.Fatalf("LANTERN burns out after %d units, want 800 (480/0.6)", lantern)
	}
}
