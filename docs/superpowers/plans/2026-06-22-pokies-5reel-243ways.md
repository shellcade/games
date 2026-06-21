# Pokies — 5-reel 243-ways engine (PR3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the seated machine's 3-reel single-line engine with a 5×3, **243-ways** Aristocrat-style engine — wild substitution, scatter free spins, and the gamble ladder all carried over — with **exact closed-form RTP** and **6 themed PAR sheets**, one bound per lounge machine.

**Architecture:** Reels go 3→5 (`numReels`), each drawing i.i.d. from the variant's single weighted strip. A symbol pays its **left-aligned run** (adjacent reels from reel 0, any rows, wild substituting), credited `ways × multiplier(runLen)` where `ways` = the product of per-reel counts; the spin total is the **sum over symbols**. Because it's a sum, RTP is exact in closed form from each symbol's per-reel count marginal (`E[win_s] = pay3·a³·z + pay4·a⁴·z + pay5·a⁵`), validated by a Monte-Carlo cross-check. Scatter trigger rate comes from convolving the per-reel scatter-count distribution across 5 reels.

**Tech Stack:** Go 1.25, shellcade kit v2.14.0, `kittest`. TinyGo wasip1 for publish.

**Working dir:** `games/bcook/pokies` in worktree `.claude/worktrees/pokies-aussie-features` (branch `bcook/pokies-lounge`). Run `go` from there. Module is offline-capable: use `GOPROXY=off` for go commands.

**Conventions:** TDD — failing test, minimal code, green, commit. `gofmt -w *.go` before commits. Keep the suite green at each phase boundary. The seated cabinet is the only place reels render (the floor uses character tiles); `glyphcheck` must stay at 0 violations.

> **Builds on** PR2 (the resident lounge). The 6 lounge machines currently all bind `defaultVariant()`; this PR gives each its own themed 5-reel variant.

---

## Math reference (the contract every task upholds)

For a variant with a single weighted strip:
- **`countFor(s, window)`** = number of cells in a reel's 3-cell window equal to `s` **or** WILD (wild substitutes for any paying symbol `s`).
- **243-ways spin payout** over a settled `[numReels][visRows]symbol` window: for each paying symbol `s`, walk reels left-to-right; the **run** is the leading reels with `countFor(s,·) ≥ 1`; if `runLen ≥ 3`, add `pays[s][runLen-3] × Π(countFor over the run)`. Total = sum over symbols. (Scatter/wild are not "paying symbols" in this loop.)
- **Per-reel marginals** (enumerate the strip once): `a_s = E[countFor(s, window)]`, `z_s = P(countFor(s,·)=0)`.
- **Exact line RTP** (linearity of expectation; reels i.i.d.): `Σ_s ( pays[s][0]·a_s³·z_s + pays[s][1]·a_s⁴·z_s + pays[s][2]·a_s⁵ )`.
- **Scatter**: per-reel scatter-count distribution `q[0..3]`; convolve 5× → `dist[0..15]` = P(total scatters = k). `t = Σ_{k≥thr} dist[k]`; `avgAward = Σ_k dist[k]·award(k) / t`. Free spins fold in unchanged: `m = avgAward/(1 − t·avgAward)`, `RTP_total = RTP_line·(1 + t·m)`.

---

## File map

- `variant.go` — `numReels`/`visRows`; `payEntry` gains `pay3/pay4/pay5`; `variant.pays`; `countFor`; `waysPayout`; generalized `scatterWindow`/`scatterAward` to `numReels`; closed-form `stats()`; new compile gates; paytable display data.
- `room.go` — `spinState`/`machine` reel arrays 3→`numReels`; `startSpin`/`landReel`/`settleSpin` over `numReels`; settle uses `waysPayout`.
- `layout.go` — new seated 5×3 cabinet render (`drawSeated`) replacing the 3-reel `drawCard` in `composeSeated`; per-symbol ways paytable; `grid()` → `[numReels][visRows]`.
- `themes.go` *(new)* — 6 named 5-reel PAR sheets + `themeVariants()`; the floor binds `themes[i] = themeVariants()[i]`.
- `config.go` — schema for `pay3/4/5` + the rest.
- tests — `variant_test.go`, `ways_test.go` *(new, incl. Monte-Carlo)*, plus rewrites of the 3-reel assertions across `pokies_test.go`/`character_test.go`/`freespins_test.go`/`gamble_test.go`.

---

## Phase A — Ways engine core (variant.go, isolated & testable)

### Task A1: `numReels`, paytable per run-length, `countFor`

**Files:** Modify `variant.go`; Test: `ways_test.go` *(new)*.

- [ ] **Step 1: Failing test** (`ways_test.go`):

```go
package main

import "testing"

func TestCountForWildSubstitutes(t *testing.T) {
	if numReels != 5 || visRows != 3 {
		t.Fatalf("want 5 reels x 3 rows, got %dx%d", numReels, visRows)
	}
	cases := []struct {
		w    [visRows]symbol
		s    symbol
		want int
	}{
		{[visRows]symbol{sym7, sym7, symBar}, sym7, 2},
		{[visRows]symbol{sym7, symWild, symBar}, sym7, 2}, // wild counts toward 7
		{[visRows]symbol{symWild, symWild, symWild}, symBar, 3},
		{[visRows]symbol{symScatter, symCherry, symBar}, sym7, 0},
	}
	for _, c := range cases {
		if got := countFor(c.s, c.w); got != c.want {
			t.Errorf("countFor(%c, %v) = %d, want %d", rune(c.s), c.w, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run** `GOPROXY=off go test ./... -run TestCountForWildSubstitutes` → FAIL (undefined).

- [ ] **Step 3: Implement** — in `variant.go` add near the symbol consts:

```go
// The machine is a 5-reel, 3-row grid scored under 243 ways.
const (
	numReels = 5
	visRows  = 3
)
```

Replace `payEntry` and add `countFor`:

```go
// payEntry is one paytable row: a left-aligned run of `faces` (wild substituting)
// pays pay3 / pay4 / pay5 × the per-run `ways` for runs of length 3 / 4 / 5.
type payEntry struct {
	Faces string `json:"faces"`
	Pay3  int    `json:"pay3"`
	Pay4  int    `json:"pay4"`
	Pay5  int    `json:"pay5"`
}

// countFor counts symbol s in a reel's visible window, with WILD substituting for
// the (regular, paying) symbol s.
func countFor(s symbol, w [visRows]symbol) int {
	n := 0
	for _, x := range w {
		if x == s || x == symWild {
			n++
		}
	}
	return n
}
```

> Do NOT delete the old `triples`-based `payout` yet — room.go still calls it; it is removed in Task B2. The package must still compile.

- [ ] **Step 4: Run** → PASS. **Step 5: Commit** `git commit -am "feat(pokies): 5-reel constants, per-run-length paytable, countFor"`

### Task A2: `waysPayout`

**Files:** Modify `variant.go` (add `pays` field + `waysPayout`); Test: `ways_test.go`.

- [ ] **Step 1: Failing test**:

```go
// wv builds a variant whose pays map is set directly (strip irrelevant here).
func waysVariant(pays map[symbol][3]int) *variant { return &variant{pays: pays} }

// win5 builds a 5x3 window from five 3-row reels.
func win5(reels ...[visRows]symbol) (w [numReels][visRows]symbol) {
	for i := 0; i < numReels && i < len(reels); i++ {
		w[i] = reels[i]
	}
	return w
}

func TestWaysPayout(t *testing.T) {
	col := func(a, b, c symbol) [visRows]symbol { return [visRows]symbol{a, b, c} }
	blank := col(symBar, symBar, symBar) // no 7
	v := waysVariant(map[symbol][3]int{sym7: {10, 50, 250}})

	// 7 on reels 0,1,2 (one each), reel 3 has none -> run length 3, ways=1, pay3=10.
	w := win5(col(sym7, symBar, symBar), col(sym7, symBar, symBar), col(sym7, symBar, symBar), blank, blank)
	if got := v.waysPayout(w); got != 10 {
		t.Errorf("3-of-a-kind 1-way = %d, want 10", got)
	}
	// counts 2,1,1 on first three reels -> ways=2 -> 2*pay3=20.
	w = win5(col(sym7, sym7, symBar), col(sym7, symBar, symBar), col(sym7, symBar, symBar), blank, blank)
	if got := v.waysPayout(w); got != 20 {
		t.Errorf("ways=2 three-of-a-kind = %d, want 20", got)
	}
	// run of 5, all single -> pay5=250.
	r := col(sym7, symBar, symBar)
	if got := v.waysPayout(win5(r, r, r, r, r)); got != 250 {
		t.Errorf("5-of-a-kind = %d, want 250", got)
	}
	// only 2 reels -> no pay.
	if got := v.waysPayout(win5(r, r, blank, blank, blank)); got != 0 {
		t.Errorf("2-of-a-kind = %d, want 0", got)
	}
	// wild completes the run: reel1 is a wild -> still a 3-run.
	w = win5(r, col(symWild, symBar, symBar), r, blank, blank)
	if got := v.waysPayout(w); got != 10 {
		t.Errorf("wild-completed run = %d, want 10", got)
	}
}
```

- [ ] **Step 2: Run** → FAIL. **Step 3: Implement** — add `pays map[symbol][3]int` to the `variant` struct (near `triples`), and:

```go
// waysPayout scores a settled 5x3 window under 243 ways: each paying symbol pays
// its left-aligned run (wild substituting), credited pays[s][len-3] × the product
// of the per-reel counts ("ways"); the spin total is the sum over symbols.
func (v *variant) waysPayout(w [numReels][visRows]symbol) int {
	total := 0
	for _, s := range stripOrder {
		p := v.pays[s]
		if p == ([3]int{}) {
			continue
		}
		runLen, ways := 0, 1
		for reel := 0; reel < numReels; reel++ {
			c := countFor(s, w[reel])
			if c == 0 {
				break
			}
			runLen++
			ways *= c
		}
		if runLen >= 3 {
			total += p[runLen-3] * ways
		}
	}
	return total
}
```

- [ ] **Step 4: Run** → PASS. **Step 5: Commit** `git commit -am "feat(pokies): 243-ways payout (left-run, wild sub, ways product)"`

### Task A3: scatter over 5 reels + closed-form `stats()`

**Files:** Modify `variant.go` (`scatterWindow`/`scatterAward` to `numReels`; rewrite `stats()`); Modify callers in `variant.go`; Test: `ways_test.go`.

- [ ] **Step 1: Failing test** — closed form matches the math contract and folds free spins:

```go
func TestStatsClosedFormWays(t *testing.T) {
	v, err := compileVariant(oddsVariant{
		Name:    "w",
		Weights: map[string]int{"7": 6, "C": 20, "S": 3},
		Paytable: []payEntry{{Faces: "7", Pay3: 10, Pay4: 40, Pay5: 200}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s := v.stats()
	if s.LineRTP <= 0 || s.TotalRTP < s.LineRTP {
		t.Fatalf("bad stats: %+v", s)
	}
	if s.TriggerRate <= 0 {
		t.Fatalf("scatters present but trigger rate %v", s.TriggerRate)
	}
	// closed-form identity for the free-spin fold.
	want := s.LineRTP * (1 + s.TriggerRate*s.AvgFreeSpins)
	if d := want - s.TotalRTP; d > 1e-9 || d < -1e-9 {
		t.Fatalf("fold mismatch: total=%.9f want=%.9f", s.TotalRTP, want)
	}
}
```

- [ ] **Step 2: Run** → FAIL.

- [ ] **Step 3: Implement** — generalize the window helpers to `numReels` and rewrite `stats()`:

```go
// scatterWindow is the [reel][row] grid for the given per-reel stop indices.
func scatterWindow(strip []symbol, idx [numReels]int) (w [numReels][visRows]symbol) {
	for reel := 0; reel < numReels; reel++ {
		w[reel] = windowAt(strip, idx[reel])
	}
	return w
}

// scatterAward counts scatters across all numReels*visRows cells and returns the
// free spins for the highest matching threshold (v.scatter sorted desc).
func (v *variant) scatterAward(w [numReels][visRows]symbol) int {
	count := 0
	for reel := 0; reel < numReels; reel++ {
		for row := 0; row < visRows; row++ {
			if w[reel][row] == symScatter {
				count++
			}
		}
	}
	for _, se := range v.scatter {
		if count >= se.Count {
			return se.Spins
		}
	}
	return 0
}

// reelMarginal returns a = E[countFor(s, window)] and z = P(countFor==0) over all
// strip stops (one reel; reels are i.i.d.).
func (v *variant) reelMarginal(s symbol) (a, z float64) {
	n := len(v.strip)
	if n == 0 {
		return 0, 1
	}
	sum, zeros := 0, 0
	for i := 0; i < n; i++ {
		c := countFor(s, windowAt(v.strip, i))
		sum += c
		if c == 0 {
			zeros++
		}
	}
	return float64(sum) / float64(n), float64(zeros) / float64(n)
}

// scatterTotalDist returns P(total scatters across numReels reels = k), k=0..numReels*visRows.
func (v *variant) scatterTotalDist() []float64 {
	n := len(v.strip)
	per := make([]float64, visRows+1)
	if n == 0 {
		per[0] = 1
	} else {
		for i := 0; i < n; i++ {
			w := windowAt(v.strip, i)
			c := 0
			for _, x := range w {
				if x == symScatter {
					c++
				}
			}
			per[c] += 1.0 / float64(n)
		}
	}
	dist := []float64{1}
	for r := 0; r < numReels; r++ {
		next := make([]float64, len(dist)+visRows)
		for i, pi := range dist {
			for k, pk := range per {
				next[i+k] += pi * pk
			}
		}
		dist = next
	}
	return dist
}

func (v *variant) stats() variantStats {
	st := variantStats{}
	// Line RTP: exact closed form (sum over symbols; linearity of expectation).
	hit := 0.0
	for _, s := range stripOrder {
		p := v.pays[s]
		if p == ([3]int{}) {
			continue
		}
		a, z := v.reelMarginal(s)
		st.LineRTP += float64(p[0])*a*a*a*z + float64(p[1])*a*a*a*a*z + float64(p[2])*a*a*a*a*a
		// hit ≈ P(this symbol forms a >=3 run); enough for a sanity metric.
		hit += (a / float64(visRows)) // rough; replaced by exact below if needed
	}
	st.HitFreq = hit / float64(len(stripOrder)+1) // bounded sanity metric, not asserted tightly
	// Scatter trigger + average award from the convolved distribution.
	if len(v.scatter) > 0 {
		dist := v.scatterTotalDist()
		minThr := v.scatter[len(v.scatter)-1].Count // smallest threshold (sorted desc)
		var probSum, awardSum float64
		for k := minThr; k < len(dist); k++ {
			probSum += dist[k]
			awardSum += dist[k] * float64(v.awardForCount(k))
		}
		st.TriggerRate = probSum
		if probSum > 0 {
			avgAward := awardSum / probSum
			if d := 1 - st.TriggerRate*avgAward; d > 0 {
				st.AvgFreeSpins = avgAward / d
			}
		}
	}
	st.TotalRTP = st.LineRTP * (1 + st.TriggerRate*st.AvgFreeSpins)
	return st
}

// awardForCount returns the free spins a total scatter count earns (highest match).
func (v *variant) awardForCount(count int) int {
	for _, se := range v.scatter {
		if count >= se.Count {
			return se.Spins
		}
	}
	return 0
}
```

> `HitFreq` is now a loose sanity metric (no longer the exact strip³ hit rate). The default-RTP test (Task E) asserts only that it is a small positive number; if a precise hit frequency is wanted later, compute `1 − Π_s P(no ≥3 run)` — out of scope here.

- [ ] **Step 4: Run** → FAIL until `compileVariant` builds `pays` (Task A4). Note + proceed (do A4 next).

### Task A4: `compileVariant` — build `pays`, gates over closed-form RTP

**Files:** Modify `variant.go` (`compileVariant`, `defaultDoc`, `topMultiplier` removal); Test: `ways_test.go` + existing.

- [ ] **Step 1:** Rewrite the paytable-building section of `compileVariant`. Replace the `triples` loop with:

```go
	pays := map[symbol][3]int{}
	for _, pe := range doc.Paytable {
		sym, ok := symbolByName[pe.Faces]
		if !ok {
			return nil, fmt.Errorf("unknown symbol %q in paytable", pe.Faces)
		}
		if pe.Pay3 < 0 || pe.Pay4 < 0 || pe.Pay5 < 0 {
			return nil, fmt.Errorf("symbol %q has a negative multiplier", pe.Faces)
		}
		if _, dup := pays[sym]; !dup {
			pays[sym] = [3]int{pe.Pay3, pe.Pay4, pe.Pay5}
		}
	}
```

Set `v.pays = pays` (drop `v.triples`, `v.topMult`, and `topMultiplier`). Keep the scatter/gamble parsing and the convergence + band gates, but compute them from the new `stats()` (the gate code already reads `st.TriggerRate`, `st.TotalRTP`; keep `maxAward` from the scatter table). Rebuild the paytable display cache from `pays` (Task C handles the on-screen format; here just compute `payRowsCache`/`payLabels` from `pays` — see Task C for the exact display, or temporarily set them to nil and fill in Task C).

- [ ] **Step 2:** Update `defaultDoc()` to a 5-reel doc (final weights tuned in Task E; start with):

```go
func defaultDoc() oddsVariant {
	return oddsVariant{
		Name:    "Default",
		Weights: map[string]int{"7": 1, "$": 2, "*": 3, "B": 6, "C": 13, "W": 1, "S": 2},
		Paytable: []payEntry{
			{Faces: "7", Pay3: 50, Pay4: 200, Pay5: 1000},
			{Faces: "$", Pay3: 20, Pay4: 80, Pay5: 400},
			{Faces: "*", Pay3: 10, Pay4: 40, Pay5: 150},
			{Faces: "B", Pay3: 5, Pay4: 15, Pay5: 60},
		},
		Scatter: []scatterEntry{{Count: 3, Spins: 8}, {Count: 4, Spins: 15}, {Count: 5, Spins: 25}},
		Gamble:  &gambleConfig{MaxRungs: 5, MaxWin: 1_000_000},
	}
}
```

- [ ] **Step 3: Run** `GOPROXY=off go test ./... -run 'TestStatsClosedFormWays|TestWaysPayout|TestCountFor'` → PASS. Other tests (3-reel ones) FAIL to compile against the new payEntry — that's Task E. Confirm `go build ./...` compiles (room.go still uses old `payout` — keep a temporary `payout` shim if needed: `func (v *variant) payout(c [3]symbol) int { return 0 }` is NOT acceptable; instead leave the real old `payout` until B2 swaps room.go, then delete it). **Decision:** keep the old `payout` method intact through Phase A so the package compiles, even though it now reads a removed `triples`. → Therefore: DO NOT remove `triples` in A4; instead keep `triples` populated in parallel for the legacy `payout` until B2. Simplest: in A4 build BOTH `pays` (new) and keep the old `triples` block; delete `triples`+`payout` in B2.

- [ ] **Step 4: Commit** `git commit -am "feat(pokies): compile ways paytable + closed-form RTP gates"`

### Task A5: Monte-Carlo cross-check (validates the closed form)

**Files:** Test only: `ways_test.go`.

- [ ] **Step 1: Add** a sampling test that mirrors the real spin (5 i.i.d. reel stops) and compares mean payout to `stats().LineRTP`:

```go
func TestClosedFormMatchesSampling(t *testing.T) {
	v := defaultVariant()
	st := v.stats()
	rng := newTestRand(1) // deterministic; see helper note
	const N = 300000
	total := 0
	for n := 0; n < N; n++ {
		var w [numReels][visRows]symbol
		for r := 0; r < numReels; r++ {
			w[r] = windowAt(v.strip, rng.Intn(len(v.strip)))
		}
		total += v.waysPayout(w)
	}
	got := float64(total) / float64(N)
	rel := (got - st.LineRTP) / st.LineRTP
	if rel > 0.05 || rel < -0.05 {
		t.Fatalf("sampled line RTP %.4f vs closed-form %.4f (rel %.3f) — closed form wrong", got, st.LineRTP, rel)
	}
}
```

> **Helper note:** use `math/rand`'s `rand.New(rand.NewSource(1))` for `newTestRand` (test-only; the wasm build never seeds). Confirm the import is allowed in tests (it is). If `windowAt` is unexported and takes `(strip, idx)`, call it directly — it already exists.

- [ ] **Step 2: Run** → PASS (the closed form is exact, so this passes comfortably). If it FAILS, the closed form has a bug — fix `stats()`/`waysPayout`, do not loosen the tolerance below 5%.

- [ ] **Step 3: Commit** `git commit -am "test(pokies): Monte-Carlo cross-check of closed-form ways RTP"`

---

## Phase B — Wire 5 reels into the room

### Task B1: `spinState`/`machine` reel arrays → `numReels`

**Files:** Modify `room.go`; Test: existing (compile).

- [ ] **Step 1:** Change `spinState.stopIdx [3]int`→`[numReels]int`, `final [3]symbol`→`[numReels]symbol`; `machine.reels [3]symbol`→`[numReels]symbol`, `lastIdx [3]int`→`[numReels]int`. Change `landed int` to range 0..numReels.

- [ ] **Step 2:** `startSpin` rolls `numReels` reels:

```go
	s := &spinState{startedAt: r.Now(), variant: v}
	for i := 0; i < numReels; i++ {
		s.stopIdx[i] = r.Rand().Intn(len(v.strip))
		s.final[i] = v.strip[s.stopIdx[i]]
	}
```

`landReel` and the OnWake landing loop: change the `i < 3` bounds and `m.spin.landed >= 3` to `numReels`. The staggered deadline `reelStopBase + i*reelStopStep` is unchanged (now i=0..4).

- [ ] **Step 3: Run** `GOPROXY=off go build ./...` → must compile (settle still calls old `payout` on `m.reels` which is now `[numReels]` — that call breaks). Swap it in B2; for now a compile error here is expected. Proceed to B2 in the same commit.

### Task B2: `settleSpin` scores via `waysPayout`; delete legacy line code

**Files:** Modify `room.go` (`settleSpin`, `triggerFreeSpins` window), `variant.go` (delete `triples`, `payout`, `topMultiplier`, `compilePayTable` if line-only).

- [ ] **Step 1:** In `settleSpin`, replace the line-payout with the ways window. Where it currently does `win := bet * v.payout(m.reels)`:

```go
	win := bet * v.waysPayout(scatterWindow(v.strip, m.lastIdx))
```

(`m.lastIdx` is the settled `[numReels]int`; `scatterWindow` builds the `[numReels][visRows]symbol`.) The scatter trigger already uses `scatterWindow(m.lastStrip, m.lastIdx)` via `triggerFreeSpins`; ensure that helper now uses the `[numReels]int` lastIdx (it does after B1).

- [ ] **Step 2:** Delete `triples` from the `variant` struct, the old `payout`, `topMultiplier`, and the now-unused `compilePayTable`/`payRow` if they were line-only. Rebuild `payRowsCache`/`payLabels` from `pays` for the seated paytable (Task C defines the format; here produce a per-symbol row).

- [ ] **Step 3: Run** `GOPROXY=off go build ./...` compiles. Many tests still fail (3-reel literals) — Phase E. Confirm the package builds and `ways_test.go` passes. **Commit** `git commit -am "feat(pokies): settle 5 reels via 243-ways; drop legacy line payout"`

---

## Phase C — Seated 5×3 cabinet render

### Task C1: `grid()` and the seated reel window → 5×3

**Files:** Modify `layout.go`; Test: `pokies_test.go` (geometry).

- [ ] **Step 1:** Change `grid(m)` to return `[numReels][visRows]symbol` indexed `[reel][row]` (it currently returns `[3][3]` as `[row][reel]` — flip to `[reel][row]` for clarity and size to `numReels`). Update `spinStrip`/`idleStrip`/`rollWindow` usage to fill all `numReels` reels.

- [ ] **Step 2:** Replace the seated cabinet. In `composeSeated`, instead of the 15-col `drawCard`, call a new `drawSeated(f, m, v)` that draws a centered 5×3 reel window full-width. The reel window is `numReels` width-2 faces with 1-col gaps: width = `numReels*2 + (numReels-1)` = 14 inner cols; frame it in a box, center on 80 cols. Rows: title already on row 0; reels around rows 4–8; readouts (HI/BAL/BET or FREE) and the gamble overlay below; the ways paytable underneath. Reuse the existing face drawing (`SetGraphemeWide(faceArt[s])`), the gamble overlay (`drawGamble`), and the free-spin gold treatment. Keep all writes on-canvas (clamped).

- [ ] **Step 3:** Provide a geometry helper the tests can use, mirroring the old `soloFaceCol`: e.g. `seatedReelCol(reel int) int` returning the frame column of reel `reel`'s face. Pin it with a render test asserting the five center faces land where expected and that the box framing is correct.

- [ ] **Step 4: Run** the new seated render test → PASS. **Commit** `git commit -am "feat(pokies): seated 5x3 cabinet render"`

### Task C2: ways paytable display

**Files:** Modify `variant.go` (build `payRowsCache`/`payLabels` from `pays`), `layout.go` (`drawPaytableFor`).

- [ ] **Step 1:** Define the per-symbol paytable row: the face plus its `5:pay5 4:pay4 3:pay3` (highest first). Build the cache in `compileVariant`. Update `drawPaytableFor` to render one row per paying symbol (face art + the three numbers), centered, clamped to the canvas. Add a render test asserting the top symbol's `pay5` shows.

- [ ] **Step 2: Run** → PASS. **Commit** `git commit -am "feat(pokies): 243-ways paytable display"`

---

## Phase D — Themes

### Task D1: `themes.go` — 6 PAR sheets, bound per machine

**Files:** Create `themes.go`; Modify `room.go` (`newRoom` theme binding); Test: `themes_test.go` *(new)*.

- [ ] **Step 1: Failing test**:

```go
func TestThemesCompileInBand(t *testing.T) {
	vs := themeVariants()
	if len(vs) != 6 {
		t.Fatalf("themes = %d, want 6", len(vs))
	}
	names := map[string]bool{}
	for _, v := range vs {
		if v == nil {
			t.Fatal("nil theme variant — a PAR sheet failed to compile")
		}
		s := v.stats()
		if s.TotalRTP < 0.70 || s.TotalRTP > 0.95 {
			t.Errorf("theme %q total RTP %.3f outside [0.70,0.95]", v.name, s.TotalRTP)
		}
		names[v.name] = true
	}
	if len(names) != 6 {
		t.Errorf("theme names not distinct: %v", names)
	}
}
```

- [ ] **Step 2: Implement** `themes.go` with `themeDocs() []oddsVariant` (6 named docs: Lucky 7s, Gem Rush, Bells, Cherry Pop, Crown, Gift Drop — each distinct weights/pay3-4-5/scatter/gamble) and `themeVariants() []*variant` compiling them (panic on failure, like `defaultVariant`). Tune each into `[0.70, 0.95]` using a scratch dump test (delete after). In `room.go` `newRoom`, set `themes := themeVariants()` instead of all-default; keep `len(themes)==len(fmachines)` (6 machines, 6 themes — assert/align).

- [ ] **Step 3: Run** → PASS. **Commit** `git commit -am "feat(pokies): six themed 5-reel PAR sheets, one per lounge machine"`

---

## Phase E — Config, test rewrites, tuning, finish

### Task E1: config schema for pay3/4/5

**Files:** Modify `config.go`; Test: `config_test.go`.

- [ ] **Step 1:** Update `oddsVariantSchema` paytable items to `{faces enum, pay3≥0, pay4≥0, pay5≥0}` (required), and set `defaultVariantJSON` to mirror the tuned `defaultDoc` exactly (the existing identity test `TestDeclaredDefaultMatchesCompiledDefault` pins this — update it to compare `pays` instead of `triples`).

- [ ] **Step 2: Run** the config tests → PASS. **Commit** `git commit -am "feat(pokies): config schema for 243-ways paytable"`

### Task E2: rewrite the 3-reel tests for 5-reel ways

**Files:** Modify `pokies_test.go`, `variant_test.go`, `character_test.go`, `freespins_test.go`, `gamble_test.go`.

- [ ] **Step 1:** Mechanically migrate every test that uses 3-reel constructs:
  - `[3]symbol{...}` reel literals and `spinState{stopIdx:[3]int, final:[3]symbol}` → `[numReels]…`. For "force a win" setups, set all/needed reels to the paying symbol; assert `bet * waysPayout(window)` rather than the old `payout`.
  - `v.payout(...)` / `v.triples` references → `v.waysPayout(window)` / `v.pays`.
  - `TestDefaultVariantTuning`: update strip length expectation and `pays[sym7]` presence; drop `topMult`.
  - `TestSettleCreditsJackpot`, `TestBigWinPushesTicker`, `TestNewPeak…`, `TestMidSpinVariantStability`, `TestSpinSettles…`, `TestReelsLandLeftToRightStaggered`, `TestGridScrollsAsTheClockAdvances`: build a 5-reel winning/known landing; keep the seat-first `seatAt0` already present; reels land at i=0..4 (the last reel due at `reelStopBase+4*reelStopStep`).
  - Reel-face render tests (`TestReelFacesRenderAsWideGraphemes` etc.): retarget to the new `seatedReelCol` geometry from Task C1 and assert all five faces.
  - `cleanIdx` and `firstIdx` helpers stay; window construction uses `scatterWindow(strip, [numReels]int{...})`.
  - Free-spin/gamble tests: update window construction to `[numReels]` and `waysPayout`-based wins; mechanics are unchanged.

- [ ] **Step 2: Run** `GOPROXY=off go test ./... -count=1` until FULLY green. **Commit** `git commit -am "test(pokies): migrate the suite to 5-reel 243-ways"`

### Task E3: tune the default RTP, then verify the whole thing

**Files:** Modify `variant.go`/`config.go` (final default weights+pays); verify.

- [ ] **Step 1:** With a scratch dump test, print `defaultVariant().stats()` and adjust the default `pays`/weights so `TotalRTP ∈ [0.85, 0.92]` (5-reel ways games run richer than the 3-reel ~73%; pick a target and pin the test band). Update `TestDefaultVariantRTP…` to the chosen band. Re-mirror `defaultVariantJSON`. Delete the scratch test.

- [ ] **Step 2: Verify:** `gofmt -w *.go`; `GOPROXY=off go test ./... -count=1` green; `GOPROXY=off go vet ./...` clean; `GOPROXY=off go build ./...`; `tools/glyphcheck` 0 violations; TinyGo wasip1 c-shared build succeeds; render a `TestDumpFloorView`-style seated frame to eyeball the 5×3 cabinet (delete after).

- [ ] **Step 3:** Update `main.go` package doc (5 reels / 243 ways) and `smoke.yaml` if reel timing changed. **Commit** `git commit -am "feat(pokies): tune default 5-reel RTP; docs + smoke"`

### Task E4: finish

- [ ] **Step 1:** Full suite green, vet clean, native + wasm + glyphcheck pass.
- [ ] **Step 2:** Use `superpowers:finishing-a-development-branch` to push `bcook/pokies-lounge` (already PR #77) — this lands on the existing lounge PR, OR open a dedicated PR if the branch was split. Summarize the 5-reel 243-ways engine, the closed-form RTP (+ Monte-Carlo proof), and the 6 themes.

---

## Self-review checklist (author)

- **Spec coverage (§A of the design):** 5×3 grid + 243 ways (A1–A2), wild sub (A1 `countFor`), scatter over 15 (A3), closed-form exact RTP + convergence/band gates (A3–A4), Monte-Carlo cross-check (A5), free spins/gamble carry-over (B2 reuses existing), seated 5×3 layout (C1), ways paytable (C2), 6 themes (D1), config schema (E1). ✓
- **Placeholder scan:** the only deferred numbers are the empirically-tuned theme/default weights (D1/E3) and the exact seated geometry (C1) — each has a concrete "dump-then-pin" method, not a vague TODO. ✓
- **Type consistency:** `numReels`/`visRows`; `payEntry{Faces,Pay3,Pay4,Pay5}`; `variant.pays map[symbol][3]int`; `countFor(symbol,[visRows]symbol)`; `waysPayout([numReels][visRows]symbol)`; `scatterWindow(strip,[numReels]int)→[numReels][visRows]symbol`; `reelMarginal`/`scatterTotalDist`/`awardForCount`; `themeVariants()`. Consistent across tasks. ✓
- **Exactness:** line RTP is a sum over symbols ⇒ closed form is exact by linearity of expectation regardless of wild cross-symbol correlation; A5 proves it numerically. Determinism preserved (i.i.d. reels from `r.Rand()`, strip-derived marginals). ✓
