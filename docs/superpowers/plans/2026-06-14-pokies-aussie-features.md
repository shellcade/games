# Pokies — Aussie Machine Features Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add wild substitutions, scatter-triggered free spins, and a Gamble (double-up) ladder to the existing pokies machine, keeping the 5-cabinet floor and the single-strip exact-RTP model.

**Architecture:** Two new symbols (WILD 👑, SCATTER 🎁) join the virtual strip, distributed evenly across it (approach B). Line scoring becomes wild-aware; scatter scoring reads the full 3×3 window and triggers auto-played free spins (with retrigger). After a base-game line win the player gambles it Red/Black (×2) or by Suit (×4) on a ladder, or takes it. All RTP stays exact: `stats()` enumerates `strip³`, folds free spins in via a closed form, and `compileVariant` gates total RTP and retrigger convergence. The gamble is a fair deal (RTP-neutral).

**Tech Stack:** Go 1.25, shellcade kit v2.9.0, `kittest` harness. TinyGo `wasip1` build for publish (CI builds the wasm).

**Working dir:** `games/bcook/pokies` inside worktree `.claude/worktrees/pokies-aussie-features`. Run all `go` commands from `games/bcook/pokies`.

**Conventions:** TDD — failing test first, minimal code, green, commit. Each task ends green. Keep per-render allocations zero (see `alloc_test.go`).

---

## File map

- `variant.go` — symbols, `specialOrder`, `distribute`, `buildStripFrom`, wild-aware `payout`, `scatterAward`, `topMultiplier`, scatter/gamble doc+runtime types, extended `stats()` (struct), new compile gates, summary surfacing.
- `gamble.go` *(new)* — `gambleState`, card deal, ladder, take/lose resolution.
- `freespins.go` *(new)* — free-spin award/auto-play/retrigger/credit helpers.
- `room.go` — `machine` feature fields, `OnInput` mode routing, `OnWake` free-spin auto-play, `settleSpin` branch (trigger / gamble / plain), ticker for feature triggers.
- `layout.go` — gold free-spin border + `FREE N` readout, gamble selector (owner) / compact indicator (others), scatter/wild faces, per-mode controls line.
- `config.go` — extended schema + default doc + feature defaults.
- `*_test.go` — new tests + updates to index-hardcoded tests (made index-agnostic).

> Split into `gamble.go`/`freespins.go` keeps `room.go` focused; if a helper is trivial it may stay in `room.go` — decide by size while implementing.

---

## Phase A — Symbols, faces, and strip distribution (approach B)

### Task A1: Add WILD + SCATTER symbols and faces

**Files:**
- Modify: `variant.go` (symbol consts, `symbolByName`, `specialOrder`)
- Modify: `layout.go` (`faceArt`)
- Test: `variant_test.go` *(new file)* and existing `pokies_test.go`

- [ ] **Step 1: Failing test** — append to a new `variant_test.go`:

```go
package main

import "testing"

func TestSpecialSymbolsRegistered(t *testing.T) {
	if symbolByName["W"] != symWild {
		t.Errorf("W should map to symWild")
	}
	if symbolByName["S"] != symScatter {
		t.Errorf("S should map to symScatter")
	}
	if faceArt[symWild] == "" || faceArt[symScatter] == "" {
		t.Errorf("wild/scatter need reel art")
	}
}
```

- [ ] **Step 2: Run, expect FAIL** — `go test ./... -run TestSpecialSymbolsRegistered` → undefined `symWild`.

- [ ] **Step 3: Implement** — in `variant.go`:

```go
const (
	symBlank   symbol = '-'
	sym7       symbol = '7'
	symDollar  symbol = '$'
	symStar    symbol = '*'
	symBar     symbol = 'B'
	symCherry  symbol = 'C'
	symWild    symbol = 'W' // substitutes on the payline
	symScatter symbol = 'S' // counts anywhere; triggers free spins
)

// stripOrder is the regular (grouped) symbols. WILD/SCATTER are NOT here — they
// are distributed across the strip by specialOrder so scatters spread naturally.
var stripOrder = []symbol{sym7, symDollar, symStar, symBar, symCherry}

// specialOrder is the distribution order of the special symbols (deterministic).
var specialOrder = []symbol{symWild, symScatter}

var symbolByName = map[string]symbol{
	"7": sym7, "$": symDollar, "*": symStar, "B": symBar, "C": symCherry,
	"W": symWild, "S": symScatter,
}
```

In `layout.go` `faceArt`, add (single-codepoint, unambiguously East-Asian **Wide** — same discipline as 💎/🔔/🍒):

```go
	symWild:    "\U0001F451", // 👑 crown — wild
	symScatter: "\U0001F381", // 🎁 gift — scatter
```

- [ ] **Step 4: Run, expect PASS** — `go test ./... -run TestSpecialSymbolsRegistered`.

- [ ] **Step 5: Commit** — `git add -A && git commit -m "feat(pokies): register WILD + SCATTER symbols and reel art"`

---

### Task A2: Even strip distribution (`distribute`)

**Files:**
- Modify: `variant.go` (`distribute`, `buildStripFrom`)
- Test: `variant_test.go`

- [ ] **Step 1: Failing test**:

```go
func TestDistributeSpacesEvenly(t *testing.T) {
	base := []symbol{sym7, sym7, sym7, sym7, sym7, sym7} // 6 regulars
	got := distribute(base, symScatter, 2)
	if len(got) != 8 {
		t.Fatalf("len = %d, want 8", len(got))
	}
	// 2 scatters across 8 slots land at positions 0 and 4 (k*n/count).
	var pos []int
	for i, s := range got {
		if s == symScatter {
			pos = append(pos, i)
		}
	}
	if len(pos) != 2 || pos[0] != 0 || pos[1] != 4 {
		t.Fatalf("scatter positions = %v, want [0 4]", pos)
	}
	// All base symbols preserved (6 sevens remain).
	sevens := 0
	for _, s := range got {
		if s == sym7 {
			sevens++
		}
	}
	if sevens != 6 {
		t.Fatalf("sevens = %d, want 6 preserved", sevens)
	}
}

func TestDistributeZeroCountIsNoop(t *testing.T) {
	base := []symbol{sym7, symBar}
	got := distribute(base, symWild, 0)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (no-op)", len(got))
	}
}
```

- [ ] **Step 2: Run, expect FAIL** — undefined `distribute`.

- [ ] **Step 3: Implement** in `variant.go`:

```go
// distribute inserts `count` copies of sym into base at evenly spaced positions,
// returning a strip of length len(base)+count. Deterministic (no RNG): the k-th
// special occupies output position k*n/count (n = total length), so specials are
// spread across the strip rather than clumped, and every base symbol is kept in
// order. count <= 0 returns base unchanged.
func distribute(base []symbol, sym symbol, count int) []symbol {
	if count <= 0 {
		return base
	}
	n := len(base) + count
	out := make([]symbol, 0, n)
	bi, placed := 0, 0
	for i := 0; i < n; i++ {
		if placed < count && i == placed*n/count {
			out = append(out, sym)
			placed++
		} else {
			out = append(out, base[bi])
			bi++
		}
	}
	return out
}
```

Rewrite `buildStripFrom` to group regulars, then distribute specials:

```go
// buildStripFrom lays out the strip: regular symbols grouped in stripOrder, then
// WILD and SCATTER distributed evenly across the result (specialOrder). The whole
// layout is a pure function of weights, so a seeded room reproduces draws.
func buildStripFrom(weights map[symbol]int) []symbol {
	var strip []symbol
	for _, s := range stripOrder {
		for i := 0; i < weights[s]; i++ {
			strip = append(strip, s)
		}
	}
	for _, s := range specialOrder {
		strip = distribute(strip, s, weights[s])
	}
	return strip
}
```

- [ ] **Step 4: Run, expect PASS** — `go test ./... -run TestDistribute`.

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): distribute WILD/SCATTER evenly across the strip"`

---

## Phase B — Wild-aware line scoring

### Task B1: Wild substitution in `payout` + `topMultiplier`

**Files:**
- Modify: `variant.go` (`payout`, add `topMult` cache + `topMultiplier`, set in `compileVariant`)
- Test: `variant_test.go`

- [ ] **Step 1: Failing test**:

```go
func TestWildCompletesLine(t *testing.T) {
	v := defaultVariant() // 7=500 $=150 *=55 B=10, top=500
	cases := []struct {
		name  string
		reels [3]symbol
		want  int
	}{
		{"7 W 7 pays as 777", [3]symbol{sym7, symWild, sym7}, 500},
		{"W W B pays as BBB", [3]symbol{symWild, symWild, symBar}, 10},
		{"W W W pays top", [3]symbol{symWild, symWild, symWild}, 500},
		{"W $ 7 no line", [3]symbol{symWild, symDollar, sym7}, 0},
		{"scatter on line breaks combo", [3]symbol{sym7, symScatter, sym7}, 0},
		{"W C W is cherries (pays 0)", [3]symbol{symWild, symCherry, symWild}, 0},
		{"plain 777 still 500", [3]symbol{sym7, sym7, sym7}, 500},
		{"plain no-match", [3]symbol{sym7, symDollar, symStar}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := v.payout(c.reels); got != c.want {
				t.Fatalf("payout(%v) = %d, want %d", c.reels, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run, expect FAIL** (wilds currently treated as a normal symbol → `7 W 7` returns 0).

- [ ] **Step 3: Implement** — add `topMult int` to the `variant` struct; in `compileVariant` after building `triples` set `v.topMult = topMultiplier(triples)`. Replace `payout`:

```go
// topMultiplier returns the largest three-of-a-kind multiplier in the paytable
// (what an all-wild line, and the wild's own top line, pays).
func topMultiplier(triples map[symbol]int) int {
	max := 0
	for _, m := range triples {
		if m > max {
			max = m
		}
	}
	return max
}

// payout returns the bet multiplier for the three center (payline) faces. Wilds
// substitute to complete the best three-of-a-kind; an all-wild line pays the top
// multiplier. A scatter on the payline does not form a line. Only a completed
// three-of-a-kind whose symbol is in the paytable pays.
func (v *variant) payout(center [3]symbol) int {
	anchor := symBlank
	wilds := 0
	for _, s := range center {
		switch {
		case s == symWild:
			wilds++
		case s == symScatter:
			return 0 // scatters never form a line
		default:
			if anchor == symBlank {
				anchor = s
			} else if s != anchor {
				return 0 // two distinct regular symbols
			}
		}
	}
	if wilds == 3 {
		return v.topMult
	}
	return v.triples[anchor] // wilds + matching regulars complete the triple
}
```

- [ ] **Step 4: Run, expect PASS** — `go test ./... -run 'TestWildCompletesLine|TestPayout'`. (Existing `TestPayout` cases have no wilds, so they still pass.)

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): wild substitution on the payline"`

---

## Phase C — Scatter scoring + RTP math

### Task C1: Scatter doc/runtime types + `scatterAward`

**Files:**
- Modify: `variant.go` (`scatterEntry`, `gambleConfig` on `oddsVariant`; `variant.scatter`, `variant.gamble`; `scatterAward`; window helper)
- Test: `variant_test.go`

- [ ] **Step 1: Failing test**:

```go
func TestScatterAwardThresholds(t *testing.T) {
	v := &variant{scatter: []scatterEntry{{Count: 5, Spins: 25}, {Count: 4, Spins: 15}, {Count: 3, Spins: 8}}}
	// helper: build a 3x3 window [reel][row] with `n` scatters placed.
	win := func(n int) (w [3][3]symbol) {
		for reel := 0; reel < 3; reel++ {
			for row := 0; row < 3; row++ {
				w[reel][row] = sym7
			}
		}
		placed := 0
		for reel := 0; reel < 3 && placed < n; reel++ {
			for row := 0; row < 3 && placed < n; row++ {
				w[reel][row] = symScatter
				placed++
			}
		}
		return w
	}
	for _, c := range []struct{ scatters, spins int }{
		{2, 0}, {3, 8}, {4, 15}, {5, 25}, {7, 25},
	} {
		if got := v.scatterAward(win(c.scatters)); got != c.spins {
			t.Errorf("%d scatters -> %d spins, want %d", c.scatters, got, c.spins)
		}
	}
}
```

- [ ] **Step 2: Run, expect FAIL** — undefined `scatterEntry` / `scatterAward`.

- [ ] **Step 3: Implement** — add to `oddsVariant`:

```go
type oddsVariant struct {
	Name     string         `json:"name"`
	Weights  map[string]int `json:"weights"`
	Paytable []payEntry     `json:"paytable"`
	Scatter  []scatterEntry `json:"scatter,omitempty"`
	Gamble   *gambleConfig  `json:"gamble,omitempty"`
}

// scatterEntry: `count` scatters anywhere in the 3x3 window award `spins` free
// spins (highest matching count wins).
type scatterEntry struct {
	Count int `json:"count"`
	Spins int `json:"spins"`
}

// gambleConfig caps the double-up ladder.
type gambleConfig struct {
	MaxRungs int `json:"maxRungs"` // gambles allowed before auto-take
	MaxWin   int `json:"maxWin"`   // at-risk ceiling that forces auto-take
}
```

Add `scatter []scatterEntry` and `gamble gambleConfig` to the runtime `variant` struct. Add:

```go
// scatterWindow is the [reel][row] face grid for stop indices i,j,k.
func scatterWindow(strip []symbol, idx [3]int) (w [3][3]symbol) {
	for reel := 0; reel < 3; reel++ {
		col := windowAt(strip, idx[reel]) // [top, center, bottom]
		w[reel] = col
	}
	return w
}

// scatterAward counts scatters across all nine visible cells and returns the free
// spins for the highest matching threshold (0 if below the lowest, or no table).
// v.scatter is kept sorted by Count descending at compile time.
func (v *variant) scatterAward(w [3][3]symbol) int {
	count := 0
	for reel := 0; reel < 3; reel++ {
		for row := 0; row < 3; row++ {
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
```

- [ ] **Step 4: Run, expect PASS** — `go test ./... -run TestScatterAward`.

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): scatter window counting + award table"`

---

### Task C2: Extended `stats()` with free-spin RTP

**Files:**
- Modify: `variant.go` (`variantStats`, rewrite `stats()`)
- Modify: `pokies_test.go` (update the two `stats()` callers)
- Test: `variant_test.go`

- [ ] **Step 1: Failing test** — exact small-variant check of the closed form:

```go
func TestStatsFoldsFreeSpins(t *testing.T) {
	// Strip with one scatter so trigger probability is hand-checkable is hard;
	// instead assert the relationship totalRTP == lineRTP*(1+t*m) holds and that
	// a scatter table raises total RTP above line RTP.
	v, err := compileVariant(oddsVariant{
		Name:     "fs",
		Weights:  map[string]int{"7": 2, "C": 8, "S": 3},
		Paytable: []payEntry{{Faces: "7", Multiplier: 30}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s := v.stats()
	if s.TriggerRate <= 0 {
		t.Fatalf("trigger rate = %v, want > 0 with scatters present", s.TriggerRate)
	}
	if s.TotalRTP <= s.LineRTP {
		t.Fatalf("total RTP %.4f should exceed line RTP %.4f", s.TotalRTP, s.LineRTP)
	}
	want := s.LineRTP * (1 + s.TriggerRate*s.AvgFreeSpins)
	if diff := want - s.TotalRTP; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("closed form mismatch: total=%.9f want=%.9f", s.TotalRTP, want)
	}
}
```

- [ ] **Step 2: Run, expect FAIL** — `stats()` returns 2 values / no `variantStats`.

- [ ] **Step 3: Implement** — replace `stats()`:

```go
// variantStats is the exact theoretical profile of a variant, enumerated over all
// strip^3 equally-likely outcomes.
type variantStats struct {
	LineRTP      float64 // mean line payout per spin (bet multiples)
	HitFreq      float64 // share of outcomes paying a line
	TriggerRate  float64 // t: share of outcomes triggering free spins
	AvgFreeSpins float64 // m: expected free spins per trigger, incl. retrigger
	TotalRTP     float64 // LineRTP * (1 + t*m): base + free-spin EV
}

// stats enumerates all strip^3 outcomes (independent uniform reels) for the exact
// line RTP and the scatter trigger rate, then folds free spins into total RTP via
// the branching-process closed form. Free spins pay line RTP at no cost and
// retrigger at rate t: m = avgAward / (1 - t*avgAward).
func (v *variant) stats() variantStats {
	n := len(v.strip)
	if n == 0 {
		return variantStats{}
	}
	lineTotal, hits, triggers, spinsAwarded := 0, 0, 0, 0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			for k := 0; k < n; k++ {
				center := [3]symbol{v.strip[i], v.strip[j], v.strip[k]}
				if p := v.payout(center); p > 0 {
					lineTotal += p
					hits++
				}
				if award := v.scatterAward(scatterWindow(v.strip, [3]int{i, j, k})); award > 0 {
					triggers++
					spinsAwarded += award
				}
			}
		}
	}
	outcomes := n * n * n
	st := variantStats{
		LineRTP: float64(lineTotal) / float64(outcomes),
		HitFreq: float64(hits) / float64(outcomes),
	}
	st.TriggerRate = float64(triggers) / float64(outcomes)
	if triggers > 0 {
		avgAward := float64(spinsAwarded) / float64(triggers)
		denom := 1 - st.TriggerRate*avgAward
		if denom > 0 {
			st.AvgFreeSpins = avgAward / denom
		}
	}
	st.TotalRTP = st.LineRTP * (1 + st.TriggerRate*st.AvgFreeSpins)
	return st
}
```

- [ ] **Step 4: Update existing callers** — in `pokies_test.go` `TestDefaultVariantRTPIsAroundSeventyFivePercent`:

```go
func TestDefaultVariantRTPIsAroundSeventyFivePercent(t *testing.T) {
	s := defaultVariant().stats()
	if s.TotalRTP < 0.70 || s.TotalRTP > 0.80 {
		t.Fatalf("default total RTP = %.4f, want within [0.70, 0.80]", s.TotalRTP)
	}
	if s.HitFreq <= 0 || s.HitFreq > 0.20 {
		t.Fatalf("default hit frequency = %.4f, want a small positive share", s.HitFreq)
	}
}
```

(The `compileVariant` RTP-gate caller is updated in Task C3.)

- [ ] **Step 5: Run** — `go test ./... -run TestStatsFoldsFreeSpins` PASS. `TestDefaultVariantRTPIsAroundSeventyFivePercent` may FAIL until the default is tuned in Task C4 — that is expected; note it and proceed.

- [ ] **Step 6: Commit** — `git commit -am "feat(pokies): fold free-spin EV into exact total RTP"`

---

### Task C3: Compile gates — parse scatter/gamble, convergence, total-RTP band

**Files:**
- Modify: `variant.go` (`compileVariant`)
- Test: `variant_test.go`

- [ ] **Step 1: Failing test**:

```go
func TestCompileRejectsRunawayRetrigger(t *testing.T) {
	// Huge scatter weight + large award -> t*maxAward >= 1 (non-converging).
	_, err := compileVariant(oddsVariant{
		Name:     "runaway",
		Weights:  map[string]int{"7": 1, "S": 30},
		Paytable: []payEntry{{Faces: "7", Multiplier: 5}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 50}},
	})
	if err == nil {
		t.Fatal("expected rejection of a non-converging retrigger feature")
	}
}

func TestCompileSortsScatterDescAndDefaultsGamble(t *testing.T) {
	v, err := compileVariant(oddsVariant{
		Name:     "ok",
		Weights:  map[string]int{"7": 2, "C": 8, "S": 2},
		Paytable: []payEntry{{Faces: "7", Multiplier: 30}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}, {Count: 5, Spins: 25}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if v.scatter[0].Count != 5 {
		t.Fatalf("scatter table not sorted desc: %+v", v.scatter)
	}
	if v.gamble.MaxRungs != defaultGamble.MaxRungs || v.gamble.MaxWin != defaultGamble.MaxWin {
		t.Fatalf("gamble defaults not applied: %+v", v.gamble)
	}
}
```

- [ ] **Step 2: Run, expect FAIL** — undefined `defaultGamble`, no rejection.

- [ ] **Step 3: Implement** — add near the bounds consts:

```go
// defaultGamble is the compiled-in ladder cap used when a variant omits a gamble
// block: up to 5 doubles, then auto-take, with a generous credit ceiling.
var defaultGamble = gambleConfig{MaxRungs: 5, MaxWin: 1_000_000}
```

In `compileVariant`, after building `triples` and before/with the RTP check:

```go
	// Scatter table: validate, then sort by Count descending (scatterAward scans
	// it high-to-low).
	scatter := make([]scatterEntry, 0, len(doc.Scatter))
	for _, se := range doc.Scatter {
		if se.Count < 3 {
			return nil, fmt.Errorf("scatter trigger count %d must be at least 3", se.Count)
		}
		if se.Spins < 1 {
			return nil, fmt.Errorf("scatter award %d free spins must be positive", se.Spins)
		}
		scatter = append(scatter, se)
	}
	sort.SliceStable(scatter, func(i, j int) bool { return scatter[i].Count > scatter[j].Count })

	gamble := defaultGamble
	if doc.Gamble != nil {
		if doc.Gamble.MaxRungs < 1 || doc.Gamble.MaxWin < 1 {
			return nil, fmt.Errorf("gamble caps must be positive")
		}
		gamble = *doc.Gamble
	}

	v := &variant{name: doc.Name, strip: strip, triples: triples, scatter: scatter, gamble: gamble}
	v.topMult = topMultiplier(triples)
	v.payRowsCache, v.payLabels = compilePayTable(triples)

	// Convergence guard: the worst-case (largest) award must satisfy t*maxAward < 1
	// so the retrigger series converges (no money printer).
	st := v.stats()
	maxAward := 0
	for _, se := range scatter {
		if se.Spins > maxAward {
			maxAward = se.Spins
		}
	}
	if st.TriggerRate*float64(maxAward) >= 1-rtpEpsilon {
		return nil, fmt.Errorf("free-spin retrigger does not converge (t*maxAward = %.3f >= 1)",
			st.TriggerRate*float64(maxAward))
	}
	if st.TotalRTP < minRTP-rtpEpsilon || st.TotalRTP > maxRTP+rtpEpsilon {
		return nil, fmt.Errorf("theoretical RTP %.1f%% is outside the allowed [%.0f%%, %.0f%%]",
			st.TotalRTP*100, minRTP*100, maxRTP*100)
	}
	return v, nil
```

Remove the old `rtp, _ := v.stats()` block and the old `v := &variant{...}` / payRows assignment it replaced (this task supersedes them). Keep `compileVariant`'s earlier weight/strip validation unchanged.

- [ ] **Step 4: Run, expect PASS** — `go test ./... -run 'TestCompile'` (including existing `TestCompileVariantRejectsOutOfBounds`).

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): compile gates for scatter table, gamble caps, retrigger convergence, total RTP"`

---

### Task C4: Tune the default variant to ship features live

**Files:**
- Modify: `variant.go` (`defaultDoc`)
- Modify: `config.go` (`defaultVariantJSON`)
- Modify: `pokies_test.go` (`TestDefaultVariantTuning`, `weightSummary` if changed)
- Modify: `variant.go` (`weightSummary` to include W/S)
- Test: existing + `variant_test.go`

- [ ] **Step 1: Decide + verify tuning empirically.** Goal: total RTP ∈ [0.70, 0.80] with wild + scatter live, keeping headline multipliers (`7=500 $=150 *=55 B=10`, cherry pays 0) so the jackpot tests stay meaningful. Starting point to iterate from:

```go
func defaultDoc() oddsVariant {
	return oddsVariant{
		Name: "Default",
		Weights: map[string]int{
			"7": 1, "$": 2, "*": 3, "B": 5, "C": 10, "W": 1, "S": 3,
		},
		Paytable: []payEntry{
			{Faces: "7", Multiplier: 500},
			{Faces: "$", Multiplier: 150},
			{Faces: "*", Multiplier: 55},
			{Faces: "B", Multiplier: 10},
		},
		Scatter: []scatterEntry{
			{Count: 3, Spins: 8},
			{Count: 4, Spins: 15},
			{Count: 5, Spins: 25},
		},
		Gamble: &gambleConfig{MaxRungs: 5, MaxWin: 1_000_000},
	}
}
```

Run a scratch check (then delete it):

```go
func TestDumpDefaultStats(t *testing.T) { // temporary
	s := defaultVariant().stats()
	t.Logf("strip=%d line=%.4f hit=%.4f t=%.4f m=%.3f total=%.4f",
		len(defaultVariant().strip), s.LineRTP, s.HitFreq, s.TriggerRate, s.AvgFreeSpins, s.TotalRTP)
}
```

`go test ./... -run TestDumpDefaultStats -v`. Adjust **cherry weight `C`** (raises non-paying frequency, lowers RTP) and **wild weight `W`** (raises RTP) until `total ∈ [0.72, 0.78]` comfortably inside the band. Record the final weights. Delete the scratch test.

- [ ] **Step 2: Mirror the final doc into `config.go`** `defaultVariantJSON` exactly (a test pins they compile identically — see Task G1). Include the `scatter` and `gamble` blocks.

- [ ] **Step 3: Update `weightSummary`** to include specials, and update `TestDefaultVariantTuning`:

```go
// in variant.go
func (v *variant) weightSummary() string {
	counts := map[symbol]int{}
	for _, s := range v.strip {
		counts[s]++
	}
	out := ""
	for _, s := range append(append([]symbol{}, stripOrder...), specialOrder...) {
		if out != "" {
			out += " "
		}
		out += fmt.Sprintf("%s:%d", nameOfSymbol(s), counts[s])
	}
	return out
}
```

```go
// in pokies_test.go — update to the tuned numbers (example shown; use ACTUALS)
func TestDefaultVariantTuning(t *testing.T) {
	v := defaultVariant()
	wantLen := 1 + 2 + 3 + 5 + 10 + 1 + 3 // = 25  (use the tuned weights)
	if len(v.strip) != wantLen {
		t.Fatalf("default strip length = %d, want %d", len(v.strip), wantLen)
	}
	if _, ok := v.triples[symCherry]; ok {
		t.Fatal("cherries must pay nothing in the default variant")
	}
	if v.topMult != 500 {
		t.Fatalf("topMult = %d, want 500", v.topMult)
	}
}
```

- [ ] **Step 4: Run** — `go test ./...`. Fix any index-hardcoded tests that now land on a special symbol (Task C5 covers `settleKnownFaces` / center-row test). `TestDefaultVariantRTPIsAroundSeventyFivePercent` must pass.

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): default variant ships wild + scatter, retuned to ~75% RTP"`

---

### Task C5: Make index-hardcoded tests strip-agnostic

**Files:**
- Modify: `pokies_test.go` (`settleKnownFaces`, `TestGridCenterRowIsThePayline`, any test asserting specific strip indices)
- Test: itself

The default strip composition changed, so tests that hardcode `stopIdx` indices (e.g. `{0,11,1}` expecting 7/C/$) may now land on WILD/SCATTER. Make them locate indices by scanning the strip.

- [ ] **Step 1: Replace `settleKnownFaces`** to find indices dynamically:

```go
// firstIdx returns the first strip position holding face s (fatal if absent).
func firstIdx(t *testing.T, strip []symbol, s symbol) int {
	t.Helper()
	for i, x := range strip {
		if x == s {
			return i
		}
	}
	t.Fatalf("strip has no %q", rune(s))
	return 0
}

func settleKnownFaces(t *testing.T, rm *room, r *kittest.Room, p kit.Player) {
	t.Helper()
	m := rm.machines[p.AccountID]
	m.bet = 10
	m.balance = startBalance - 10
	strip := rm.variant.strip
	i7, iC, iD := firstIdx(t, strip, sym7), firstIdx(t, strip, symCherry), firstIdx(t, strip, symDollar)
	m.spin = &spinState{
		startedAt: r.Now(),
		variant:   rm.variant,
		stopIdx:   [3]int{i7, iC, iD},
		final:     [3]symbol{sym7, symCherry, symDollar},
	}
	rm.settleSpin(r, p.AccountID)
	rm.render(r)
}
```

- [ ] **Step 2: Update `TestGridCenterRowIsThePayline`** to use any in-range indices (it already reads `m.reels` back, so just pick `{0,1,2}` and set `final` from the strip):

```go
	strip := rm.variant.strip
	m.spin = &spinState{startedAt: r.Now(), variant: rm.variant, stopIdx: [3]int{0, 1, 2}}
	m.spin.final = [3]symbol{strip[0], strip[1], strip[2]}
```

- [ ] **Step 3: Run, expect PASS** — `go test ./...` (whole suite green again).

- [ ] **Step 4: Commit** — `git commit -am "test(pokies): make strip-index tests composition-agnostic"`

---

## Phase D — Free spins state machine

### Task D1: `machine` feature fields + trigger on settle

**Files:**
- Modify: `room.go` (`machine` fields; `settleSpin` branch; constants)
- Create: `freespins.go`
- Test: `freespins_test.go` *(new)*

- [ ] **Step 1: Failing test**:

```go
package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// scatterVariant: a variant where landing all reels on a scatter index fills the
// window with scatters (≥3) to force a trigger.
func TestScatterTriggerStartsFreeSpins(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.bet = 50
	m.balance = startBalance - 50

	strip := rm.variant.strip
	sIdx := firstIdx(t, strip, symScatter)
	// Stop all three reels centered on a scatter: each reel window then shows at
	// least the center scatter; if neighbours are scatters too the count climbs.
	m.spin = &spinState{startedAt: r.Now(), variant: rm.variant,
		stopIdx: [3]int{sIdx, sIdx, sIdx}, final: [3]symbol{strip[sIdx], strip[sIdx], strip[sIdx]}}
	// Force the window to contain ≥3 scatters for a deterministic trigger.
	rm.settleSpin(r, p.AccountID)

	if rm.scatterCount(m) < 3 {
		t.Skip("default strip spacing put <3 scatters in this window; covered by unit test of scatterAward")
	}
	if m.freeSpins <= 0 {
		t.Fatalf("expected free spins to start, got %d", m.freeSpins)
	}
	if m.freeBet != 50 {
		t.Fatalf("freeBet = %d, want the triggering bet 50", m.freeBet)
	}
}
```

> Note: because distribution spreads scatters, a 3-aligned stop may not reach 3 in the window; the test skips if so (the deterministic trigger path is covered directly in Step 1b below). Prefer a constructed variant for determinism:

```go
func TestFreeSpinsAwardedDeterministically(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	// A scatter-dense variant: window is guaranteed ≥3 scatters at any stop.
	h.variant = mustCompile(t, oddsVariant{
		Name: "scatterland", Weights: map[string]int{"7": 1, "C": 5, "S": 6},
		Paytable: []payEntry{{Faces: "7", Multiplier: 20}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}},
	})
	h.OnStart(r)
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet, m.balance = 10, startBalance-10
	idx := firstIdx(t, h.variant.strip, symScatter)
	m.spin = &spinState{startedAt: r.Now(), variant: h.variant,
		stopIdx: [3]int{idx, idx, idx}, final: [3]symbol{symScatter, symScatter, symScatter}}
	h.settleSpin(r, p.AccountID)
	if m.freeSpins != 8 {
		t.Fatalf("freeSpins = %d, want 8", m.freeSpins)
	}
}
```

- [ ] **Step 2: Run, expect FAIL** — undefined `freeSpins` / `freeBet` / `scatterCount`.

- [ ] **Step 3: Implement** — add fields to `machine`:

```go
	freeSpins int      // remaining free spins (0 = base game)
	freeBet   int      // bet locked at trigger; free spins pay at this
	freeWin   int      // accumulated free-spin credits (for the banner)
	freeVar   *variant // variant pinned at trigger (free spins settle under it)
	nextFree  time.Time // earliest time the next auto free spin may start
	gamble    *gambleState
```

Add a free-spin gap constant near the timing consts:

```go
	freeSpinGap = 700 * time.Millisecond // pause between auto-played free spins
```

Create `freespins.go`:

```go
package main

import kit "github.com/shellcade/kit/v2"

// scatterCount returns the scatter count in the machine's last settled window
// (used for trigger/retrigger decisions and tests).
func (rm *room) scatterCount(m *machine) int {
	v := m.lastVariant()
	if v == nil || len(m.lastStrip) == 0 {
		return 0
	}
	n := 0
	w := scatterWindow(m.lastStrip, m.lastIdx)
	for reel := 0; reel < 3; reel++ {
		for row := 0; row < 3; row++ {
			if w[reel][row] == symScatter {
				n++
			}
		}
	}
	return n
}

// triggerFreeSpins awards free spins from the just-settled window under variant v
// (the spin's pinned variant). Returns the spins awarded (0 if none). Retrigger
// during free spins adds to the running count.
func (rm *room) triggerFreeSpins(r kit.Room, m *machine, v *variant, bet int) int {
	award := v.scatterAward(scatterWindow(m.lastStrip, m.lastIdx))
	if award == 0 {
		return 0
	}
	if m.freeSpins == 0 { // fresh feature
		m.freeBet = bet
		m.freeVar = v
		m.freeWin = 0
	}
	m.freeSpins += award
	return award
}
```

Add a `lastVariant()` helper on `machine` (returns `m.freeVar` if in free spins else nil — or store last settle variant). Simpler: add field `lastVar *variant` set in `settleSpin`; `lastVariant()` returns it.

Wire into `settleSpin` (see Task D2 for the full branch). For this task, the minimal change: after computing the window in settle, call `triggerFreeSpins` and set `freeBet`.

- [ ] **Step 4: Run, expect PASS** — `go test ./... -run TestFreeSpinsAwardedDeterministically`.

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): scatter trigger awards free spins"`

---

### Task D2: `settleSpin` branch — trigger vs gamble vs plain; free-spin credit

**Files:**
- Modify: `room.go` (`settleSpin`, extract helpers)
- Modify: `freespins.go`
- Test: `freespins_test.go`

- [ ] **Step 1: Failing test** — free-spin win credits at the locked bet and decrements:

```go
func TestFreeSpinWinCreditsAtLockedBetNoCharge(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.variant = mustCompile(t, oddsVariant{
		Name: "fs", Weights: map[string]int{"7": 4, "C": 6, "S": 2},
		Paytable: []payEntry{{Faces: "7", Multiplier: 20}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 5}},
	})
	h.OnStart(r)
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet = 10
	// Enter free spins by hand: 3 spins left, locked bet 50.
	m.freeSpins, m.freeBet, m.freeVar = 3, 50, h.variant
	m.balance = 1000
	i7 := firstIdx(t, h.variant.strip, sym7)
	m.spin = &spinState{startedAt: r.Now(), variant: h.variant,
		stopIdx: [3]int{i7, i7, i7}, final: [3]symbol{sym7, sym7, sym7}} // 20x
	h.settleSpin(r, p.AccountID)

	if m.balance != 1000+50*20 { // credited at the LOCKED bet, no deduction
		t.Fatalf("balance = %d, want %d", m.balance, 1000+50*20)
	}
	if m.freeSpins != 2 {
		t.Fatalf("freeSpins = %d, want 2 (decremented)", m.freeSpins)
	}
	if m.gamble != nil {
		t.Fatal("gamble must not be offered during free spins")
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement** — restructure `settleSpin`. Replace the body after `m.spin = nil; m.spun = true` with a branch. Pseudocode → concrete:

```go
func (rm *room) settleSpin(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil || m.spin == nil {
		return
	}
	m.reels = m.spin.final
	m.lastIdx = m.spin.stopIdx
	v := m.spin.variant
	if v == nil {
		v = defaultVariant()
	}
	m.lastStrip = v.strip
	m.lastVar = v
	wasFree := m.freeSpins > 0
	bet := m.bet
	if wasFree {
		bet = m.freeBet
	}
	m.spin = nil
	m.spun = true

	win := bet * v.payout(m.reels)

	if wasFree {
		m.freeSpins--
		m.freeWin += win
		rm.creditWin(r, id, win, false) // credit, no gamble, no rebuy-on-zero
		rm.triggerFreeSpins(r, m, v, bet) // retrigger adds spins
		if win >= bet*tickerMult {
			rm.announce(r, id, win)
		}
		if m.freeSpins == 0 {
			rm.endFreeSpins(r, id) // flash the feature total
		}
		rm.scheduleNextFree(r, m)
		return
	}

	// Base game. A spin can both pay a line and trigger free spins; if it
	// triggers, credit the line win directly (no gamble) and start the feature.
	if award := rm.triggerFreeSpins(r, m, v, bet); award > 0 {
		rm.creditWin(r, id, win, false)
		rm.announce(r, id, 0) // "X hit FREE SPINS!" (see announce free-spin form)
		rm.scheduleNextFree(r, m)
		return
	}

	if win > 0 {
		rm.enterGamble(r, m, win) // hold the win on the ladder
		m.flash = ""
		return
	}

	rm.creditWin(r, id, 0, true) // no win: rebuy check + clear flash
}
```

Add `creditWin` (centralizes balance, peak, leaderboard, rebuy, flash) in `room.go`:

```go
// creditWin adds `win` to the balance, updates peak + leaderboard, sets the WIN
// flash, and (when allowZeroRebuy) re-buys a busted machine. Used by base wins
// taken, free-spin wins, and the no-win settle.
func (rm *room) creditWin(r kit.Room, id string, win int, allowZeroRebuy bool) {
	m := rm.machines[id]
	if m == nil {
		return
	}
	m.balance += win
	if m.balance > m.highScore {
		m.highScore = m.balance
	}
	switch {
	case allowZeroRebuy && m.balance <= 0:
		m.balance = rebuyAmount
		m.flash = "RE-BUY"
	case win > 0:
		m.flash = fmt.Sprintf("WIN! +%d", win)
	}
	m.flashUntil = r.Now().Add(flashDur)
	rm.clampBet(m)
	if p, ok := rm.names[id]; ok {
		rm.persistWallet(r, p, m.balance, m.highScore)
		if m.highScore > m.postedPeak {
			m.postedPeak = m.highScore
			r.Post(kit.Result{Rankings: []kit.PlayerResult{{
				Player: p, Metric: m.highScore, Status: kit.StatusFinished,
			}}})
		}
	}
}
```

Move the existing ticker logic into `announce(r, id, win)` — when `win == 0` emit the free-spins banner (`"%s hit FREE SPINS!"`), else the big-win banner (existing text). Add `m.lastVar *variant` field. Add stubs `scheduleNextFree`, `endFreeSpins` (filled in D3) and `enterGamble` (Phase E) — for this task `enterGamble` can be a temporary that just calls `creditWin(win, true)` so the test compiles; it is replaced in Phase E. **Mark the stub with a `// TODO(Phase E)` and a test in E supersedes it.**

> The existing `settleSpin` tests (`TestSettleCreditsJackpot`, `TestBustRebuys…`, `TestBigWinPushesTicker`, `TestSmallWinDoesNotPushTicker`, leaderboard) assume a win credits immediately. With gamble holding base wins, those will change. **Update them in Task E3** to take-the-win first. Until then they may fail — note and proceed, OR temporarily keep `enterGamble` crediting directly so they stay green until E3. Prefer the temporary direct-credit stub so the suite stays green between tasks.

- [ ] **Step 4: Run, expect PASS** — `go test ./... -run TestFreeSpinWinCreditsAtLockedBetNoCharge` (and the rest still green with the direct-credit stub).

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): settle branches into free-spin/gamble/plain paths"`

---

### Task D3: Auto-play free spins from `OnWake`

**Files:**
- Modify: `room.go` (`OnWake`), `freespins.go` (`scheduleNextFree`, `endFreeSpins`, `autoFreeSpin`)
- Test: `freespins_test.go`

- [ ] **Step 1: Failing test** — driving wakes auto-plays the whole feature to completion:

```go
func TestFreeSpinsAutoPlayToCompletion(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.variant = mustCompile(t, oddsVariant{
		Name: "fs", Weights: map[string]int{"7": 3, "C": 9}, // no scatter -> no retrigger
		Paytable: []payEntry{{Faces: "7", Multiplier: 20}},
	})
	h.OnStart(r)
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.freeSpins, m.freeBet, m.freeVar = 3, 10, h.variant
	m.balance = 1000

	// Advance well past 3 * (settle + gap).
	for i := 0; i < 30; i++ {
		r.Advance(300 * time.Millisecond)
		h.OnWake(r)
	}
	if m.freeSpins != 0 {
		t.Fatalf("freeSpins = %d, want 0 after auto-play", m.freeSpins)
	}
	if m.spin != nil {
		t.Fatal("no spin should be in flight after the feature ends")
	}
}
```

- [ ] **Step 2: Run, expect FAIL** (free spins never auto-start).

- [ ] **Step 3: Implement** — in `OnWake`, after the existing per-machine landing loop and before `rm.render(r)`, add auto-play:

```go
		// Auto-play free spins: when none is in flight, the flash has shown, and
		// the inter-spin gap has elapsed, roll the next free spin for free.
		if m.spin == nil && m.freeSpins > 0 && now.After(m.nextFree) {
			rm.autoFreeSpin(r, id)
		}
```

In `freespins.go`:

```go
func (rm *room) scheduleNextFree(r kit.Room, m *machine) {
	m.nextFree = r.Now().Add(freeSpinGap)
}

func (rm *room) endFreeSpins(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil {
		return
	}
	if m.freeWin > 0 {
		m.flash = fmt.Sprintf("FEATURE +%d", m.freeWin)
		m.flashUntil = r.Now().Add(flashDur)
	}
	m.freeVar = nil
}

// autoFreeSpin rolls one free spin (no bet charged) under the pinned free-spin
// variant. The existing OnWake landing loop settles it via settleSpin's free path.
func (rm *room) autoFreeSpin(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil || m.spin != nil || m.freeSpins <= 0 {
		return
	}
	v := m.freeVar
	if v == nil {
		v = rm.variant
	}
	s := &spinState{startedAt: r.Now(), variant: v}
	for i := range s.final {
		s.stopIdx[i] = r.Rand().Intn(len(v.strip))
		s.final[i] = v.strip[s.stopIdx[i]]
	}
	m.spin = s
}
```

Guard `startSpin` and `OnInput` so a player cannot manually spin or change bet while `freeSpins > 0` (auto-play owns the reels): early-return from `startSpin` if `m.freeSpins > 0`, and in `OnInput` route to nothing for bet changes during free spins.

- [ ] **Step 4: Run, expect PASS** — `go test ./... -run TestFreeSpinsAutoPlay`.

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): auto-play free spins with retrigger from OnWake"`

---

## Phase E — Gamble ladder

### Task E1: Card deal + ladder resolution (pure logic)

**Files:**
- Create: `gamble.go`
- Test: `gamble_test.go` *(new)*

- [ ] **Step 1: Failing test** — fairness + ladder growth + cap:

```go
package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

func TestGambleColorIsFair(t *testing.T) {
	r := kittest.NewRoom(kittest.Player("alice"))
	red, n := 0, 20000
	for i := 0; i < n; i++ {
		if dealCardRed(r.Rand()) {
			red++
		}
	}
	frac := float64(red) / float64(n)
	if frac < 0.47 || frac > 0.53 {
		t.Fatalf("red fraction = %.3f, want ≈0.5", frac)
	}
}

func TestGambleSuitIsQuarter(t *testing.T) {
	r := kittest.NewRoom(kittest.Player("alice"))
	hits, n := 0, 20000
	for i := 0; i < n; i++ {
		if dealCardSuit(r.Rand()) == suitHearts {
			hits++
		}
	}
	frac := float64(hits) / float64(n)
	if frac < 0.22 || frac > 0.28 {
		t.Fatalf("hearts fraction = %.3f, want ≈0.25", frac)
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement** `gamble.go`:

```go
package main

import (
	"fmt"

	kit "github.com/shellcade/kit/v2"
)

// suit indices; hearts/diamonds are red, spades/clubs black.
const (
	suitSpades = iota
	suitHearts
	suitDiamonds
	suitClubs
)

// selector option indices (linear navigation; rendered over two rows).
const (
	selTake = iota
	selRed
	selBlack
	selSpades
	selHearts
	selDiamonds
	selClubs
	selCount
)

// gambleState holds an at-risk win on the double-up ladder.
type gambleState struct {
	atRisk int  // current win being gambled
	rungs  int  // doubles taken so far
	sel    int  // highlighted selector option (sel*)
	card   int  // last revealed suit (-1 = face down)
	last   bool // last guess result (for the reveal flash)
}

func dealCardSuit(rng kit.Rand) int { return rng.Intn(4) }
func dealCardRed(rng kit.Rand) bool {
	s := dealCardSuit(rng)
	return s == suitHearts || s == suitDiamonds
}
func suitIsRed(s int) bool { return s == suitHearts || s == suitDiamonds }
```

> Check `kit.Rand`'s exact type/name from `room.go` usage (`r.Rand().Intn`). Use the same type in the helper signatures.

- [ ] **Step 4: Run, expect PASS** — `go test ./... -run 'TestGamble(Color|Suit)'`.

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): gamble card deal primitives"`

---

### Task E2: `enterGamble`, input handling, take/lose

**Files:**
- Modify: `gamble.go` (`enterGamble`, `gambleInput`, `resolveGuess`, `takeWin`)
- Modify: `room.go` (`OnInput` mode routing)
- Test: `gamble_test.go`

- [ ] **Step 1: Failing tests**:

```go
func TestGambleWinDoublesAndLadders(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.balance = 1000
	rm.enterGamble(r, m, 100)
	if m.gamble == nil || m.gamble.atRisk != 100 {
		t.Fatalf("enterGamble did not hold the win")
	}
	// Pick the color the next card will be, so the guess wins.
	// Peek not possible; instead force sel to RED/BLACK by the dealt color.
	m.gamble.sel = selRed
	red := dealCardRedPeek(r) // deterministic peek helper (see impl note)
	if !red {
		m.gamble.sel = selBlack
	}
	rm.gambleConfirm(r, p.AccountID)
	if m.gamble == nil || m.gamble.atRisk != 200 {
		t.Fatalf("atRisk = %v, want doubled to 200", m.gamble)
	}
	if m.gamble.rungs != 1 {
		t.Fatalf("rungs = %d, want 1", m.gamble.rungs)
	}
}

func TestGambleTakeBanksWin(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.balance = 1000
	rm.enterGamble(r, m, 250)
	m.gamble.sel = selTake
	rm.gambleConfirm(r, p.AccountID)
	if m.gamble != nil {
		t.Fatal("take should clear the gamble")
	}
	if m.balance != 1250 {
		t.Fatalf("balance = %d, want 1250 (win banked)", m.balance)
	}
}

func TestGambleLossForfeits(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.balance = 1000
	rm.enterGamble(r, m, 100)
	// Guess the WRONG color deterministically.
	m.gamble.sel = selRed
	if dealCardRedPeek(r) {
		m.gamble.sel = selRed // card will be red -> guess black to lose
		m.gamble.sel = selBlack
	}
	// (Pick the losing option based on the peek.)
	rm.gambleConfirm(r, p.AccountID)
	if m.gamble != nil {
		t.Fatal("a loss should clear the gamble")
	}
	if m.balance != 1000 {
		t.Fatalf("balance = %d, want 1000 (win forfeited, nothing credited)", m.balance)
	}
}
```

> **Determinism note:** `kittest` seeds a fixed RNG. Rather than a real "peek" (which would consume the RNG), implement the test by reading the next card via a tiny indirection: expose `rm.dealNext(r)` used by `gambleConfirm`, and in tests call a seeded clone. Simplest robust approach: make `gambleConfirm` take the dealt suit as an argument in an internal form `resolveGuess(r, id, suit)` and have a thin `gambleConfirm` that deals then calls it; tests call `resolveGuess` with a chosen suit directly. Rewrite the three tests to call `rm.resolveGuess(r, p.AccountID, suit)` with explicit suits — deterministic, no peeking. Use that shape.

Rewritten deterministic tests use `resolveGuess(r, id, suit)`:

```go
func TestGambleWinDoublesAndLadders(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.balance = 1000
	rm.enterGamble(r, m, 100)
	m.gamble.sel = selRed
	rm.resolveGuess(r, p.AccountID, suitHearts) // hearts = red -> RED wins
	if m.gamble == nil || m.gamble.atRisk != 200 || m.gamble.rungs != 1 {
		t.Fatalf("after red win: %+v, want atRisk 200 rung 1", m.gamble)
	}
	m.gamble.sel = selSpades
	rm.resolveGuess(r, p.AccountID, suitSpades) // suit hit -> x4
	if m.gamble == nil || m.gamble.atRisk != 800 {
		t.Fatalf("after suit win: %+v, want atRisk 800", m.gamble)
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement** in `gamble.go`:

```go
func (rm *room) enterGamble(r kit.Room, m *machine, win int) {
	m.gamble = &gambleState{atRisk: win, sel: selTake, card: -1}
}

// gambleInput moves the selector or confirms. Called from OnInput when a gamble
// is active.
func (rm *room) gambleInput(r kit.Room, id string, act kit.Action) {
	m := rm.machines[id]
	if m == nil || m.gamble == nil {
		return
	}
	switch act {
	case kit.ActLeft, kit.ActUp:
		m.gamble.sel = (m.gamble.sel + selCount - 1) % selCount
	case kit.ActRight, kit.ActDown:
		m.gamble.sel = (m.gamble.sel + 1) % selCount
	case kit.ActConfirm:
		rm.gambleConfirm(r, id)
	}
}

// gambleConfirm acts on the highlighted option: TAKE banks; a guess deals a card.
func (rm *room) gambleConfirm(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil || m.gamble == nil {
		return
	}
	if m.gamble.sel == selTake {
		rm.takeWin(r, id)
		return
	}
	rm.resolveGuess(r, id, dealCardSuit(r.Rand()))
}

// resolveGuess settles the highlighted guess against a dealt suit: a correct
// Red/Black doubles (x2), a correct Suit quadruples (x4); a wrong guess forfeits.
// A win advances the ladder unless a cap is hit, which auto-takes.
func (rm *room) resolveGuess(r kit.Room, id string, suit int) {
	m := rm.machines[id]
	if m == nil || m.gamble == nil {
		return
	}
	g := m.gamble
	g.card = suit
	win, mult := false, 0
	switch g.sel {
	case selRed:
		win, mult = suitIsRed(suit), 2
	case selBlack:
		win, mult = !suitIsRed(suit), 2
	case selSpades, selHearts, selDiamonds, selClubs:
		win, mult = suit == suitOf(g.sel), 4
	}
	if !win {
		m.flash = "GAMBLED AWAY"
		m.flashUntil = r.Now().Add(flashDur)
		m.gamble = nil
		rm.creditWin(r, id, 0, true) // rebuy check; nothing credited
		return
	}
	g.atRisk *= mult
	g.rungs++
	cap := rm.gambleCap(m)
	if g.rungs >= cap.MaxRungs || g.atRisk >= cap.MaxWin {
		rm.takeWin(r, id)
	}
}

// takeWin banks the at-risk win through the normal credit path (peak, leaderboard,
// big-win ticker), then clears the gamble.
func (rm *room) takeWin(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil || m.gamble == nil {
		return
	}
	win := m.gamble.atRisk
	m.gamble = nil
	rm.creditWin(r, id, win, true)
	if win >= m.bet*tickerMult {
		rm.announce(r, id, win)
	}
}

func suitOf(sel int) int { return sel - selSpades } // selSpades..selClubs -> 0..3

func (rm *room) gambleCap(m *machine) gambleConfig {
	if m.lastVar != nil {
		return m.lastVar.gamble
	}
	return defaultGamble
}
```

In `room.go` `OnInput`, route by mode:

```go
func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	m := rm.machines[p.AccountID]
	if m == nil {
		return
	}
	act := kit.Resolve(in, kit.CtxNav)
	switch {
	case m.gamble != nil:
		rm.gambleInput(r, p.AccountID, act)
	case m.freeSpins > 0:
		// reels are auto-played; ignore bet/spin during the feature
	default:
		switch act {
		case kit.ActUp:
			rm.adjustBet(m, +1)
		case kit.ActDown:
			rm.adjustBet(m, -1)
		case kit.ActConfirm:
			rm.startSpin(r, p)
		}
	}
	rm.render(r)
}
```

Replace the temporary `enterGamble` stub from D2 with this real one.

- [ ] **Step 4: Run, expect PASS** — `go test ./... -run TestGamble`.

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): gamble ladder — guess, double, take, forfeit, cap"`

---

### Task E3: Update base-win tests for the gamble hold

**Files:**
- Modify: `pokies_test.go` (`TestSettleCreditsJackpot`, `TestBustRebuysPreservingHighScore`, `TestBigWinPushesTicker`, `TestSmallWinDoesNotPushTicker`, `TestNewPeakPostsToLeaderboard`, `TestNoPeakDoesNotPost`, `TestSpinSettlesToPayoutOverWake`)

Base-game wins now enter the gamble holding the win; they credit on TAKE. Update each win-expecting test to take the win after settle.

- [ ] **Step 1: Add a helper** to `pokies_test.go`:

```go
// takeIfGambling banks a held base-game win so balance/leaderboard assertions
// see the credited amount.
func takeIfGambling(rm *room, r *kittest.Room, id string) {
	if m := rm.machines[id]; m != nil && m.gamble != nil {
		m.gamble.sel = selTake
		rm.gambleConfirm(r, id)
	}
}
```

- [ ] **Step 2: Update each test** — after `rm.settleSpin(...)` (or `settle(...)`), insert `takeIfGambling(rm, r, p.AccountID)` before asserting balance/flash/ticker/posts. For `TestSpinSettlesToPayoutOverWake`, call it after `settle`. For loss/no-win tests (`TestBustRebuys…`, `TestNoPeakDoesNotPost`) no change is needed (no win → no gamble). For `TestSmallWinDoesNotPushTicker` (10× = win) add the take, then assert no ticker. Keep `TestBigWinPushesTicker`: the big-win ticker now fires on TAKE — assert after `takeIfGambling`.

Example (`TestSettleCreditsJackpot`):

```go
	rm.settleSpin(r, p.AccountID)
	takeIfGambling(rm, r, p.AccountID)
	if m.balance != startBalance-50+25000 { ... }
```

- [ ] **Step 3: Run, expect PASS** — `go test ./...` whole suite green.

- [ ] **Step 4: Commit** — `git commit -am "test(pokies): base wins bank through the gamble hold"`

---

## Phase F — Rendering

### Task F1: Free-spin readout + gold border

**Files:**
- Modify: `layout.go` (`drawCard` border style, `body`/status for `FREE N`)
- Test: `pokies_test.go`

- [ ] **Step 1: Failing test**:

```go
func TestFreeSpinCabinetShowsFreeCount(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.freeSpins, m.freeBet = 7, 50
	rm.render(r)
	if !frameContains(r, p, "FREE 7") {
		t.Error("free-spin cabinet should show FREE 7")
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement** — in `drawCard`, when `m.freeSpins > 0` use a gold border style (`stBordFree = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}`) and render the BET line as `FREE` + count. Add a `stBordFree` style; in the body section:

```go
	if m.freeSpins > 0 {
		rm.body(f, top+8, col, "FREE", m.freeSpins)
	} else {
		rm.body(f, top+8, col, "BET", m.bet)
	}
```

Choose the border style before drawing borders:

```go
	bord, nameSt := stBordDim, stName
	switch {
	case m.freeSpins > 0:
		bord = stBordFree
	case own:
		bord, nameSt = stBordOwn, stNameOwn
	}
```

- [ ] **Step 4: Run, expect PASS.**

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): gold free-spin cabinet with FREE count"`

---

### Task F2: Gamble selector (owner) + compact indicator (others)

**Files:**
- Modify: `layout.go` (`status`/new `drawGamble`, controls line)
- Test: `pokies_test.go`

- [ ] **Step 1: Failing test**:

```go
func TestGambleOwnerSeesSelectorOthersSeeIndicator(t *testing.T) {
	a, b := kittest.Player("anna"), kittest.Player("bert")
	rm, r := newGame(t, a, b)
	rm.OnJoin(r, a)
	rm.OnJoin(r, b)
	ma := rm.machines[a.AccountID]
	ma.balance = 1000
	rm.enterGamble(r, ma, 150)
	rm.render(r)
	// Owner (anna) sees the interactive prompt.
	if !frameContains(r, a, "TAKE") || !frameContains(r, a, "RED") {
		t.Error("owner should see the gamble selector")
	}
	// Other viewer (bert) sees a compact indicator with the at-risk amount.
	if !frameContains(r, b, "150") {
		t.Error("other viewers should see the at-risk amount")
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement** — in `drawCard`, when `m.gamble != nil`, replace the reel screen / readout region with gamble UI:
  - **Owner** (`own`): draw the at-risk win, a face-down/revealed card, the option row `TAKE RED BLACK` and suit row `S H D C` (colored: red for H/D), highlighting `m.gamble.sel`. Render suits as **single letters** (ambiguous-width glyph avoidance), with `stRed = {FG: kit.Red}`/`stWhite`. Highlight = bold + inverse or a `>` caret.
  - **Others**: a compact `🎲 +<atRisk>` line in the status area.
  - Add a `controls` line variant: when gambling, the bottom hint reads `←/→ pick   SPACE lock/take`.

Keep all writes inside the 15-col cabinet footprint; reuse `SetGraphemeWide` only for the 🎲 indicator (unambiguously wide). Implement as a `drawGamble(f, col, top, m, own)` helper called from `drawCard` in place of the reel/readout block when `m.gamble != nil`.

- [ ] **Step 4: Run, expect PASS.**

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): per-viewer gamble selector and at-risk indicator"`

---

### Task F3: Per-mode controls line + feature ticker text

**Files:**
- Modify: `layout.go` (`compose` bottom controls), `freespins.go`/`room.go` (`announce` free-spins text)
- Test: `pokies_test.go`

- [ ] **Step 1: Failing test**:

```go
func TestFreeSpinTriggerAnnouncesRoomWide(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.variant = mustCompile(t, oddsVariant{
		Name: "fs", Weights: map[string]int{"7": 1, "C": 5, "S": 6},
		Paytable: []payEntry{{Faces: "7", Multiplier: 20}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}},
	})
	h.OnStart(r)
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet, m.balance = 10, 990
	idx := firstIdx(t, h.variant.strip, symScatter)
	m.spin = &spinState{startedAt: r.Now(), variant: h.variant,
		stopIdx: [3]int{idx, idx, idx}, final: [3]symbol{symScatter, symScatter, symScatter}}
	h.settleSpin(r, p.AccountID)
	if !h.tickerActive(r.Now()) || !strings.Contains(h.ticker.text, "FREE SPINS") {
		t.Fatalf("ticker = %q, want a FREE SPINS announcement", h.ticker.text)
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement** — `announce(r, id, win)`:

```go
func (rm *room) announce(r kit.Room, id string, win int) {
	p, ok := rm.names[id]
	if !ok {
		return
	}
	text := fmt.Sprintf("%s hit a big win  +%d", p.DisplayName(), win)
	if win == 0 {
		text = fmt.Sprintf("%s hit FREE SPINS!", p.DisplayName())
	}
	rm.ticker = ticker{text: text, ch: p.Character, until: r.Now().Add(tickerDur)}
}
```

Replace the inline ticker construction in the old settle code with `announce`. In `compose`, vary the bottom controls line by the viewer's machine mode (gamble → `←/→ pick  SPACE lock`, free spins → `auto-playing free spins`, else the existing `Up/Down bet  SPACE spin  Esc leave`).

- [ ] **Step 4: Run, expect PASS** — `go test ./...`.

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): feature ticker + per-mode controls line"`

---

## Phase G — Config surface, allocations, docs, build

### Task G1: Extend the config schema + default-doc identity test

**Files:**
- Modify: `config.go` (`oddsVariantSchema`, `defaultVariantJSON`)
- Test: `config_test.go`

- [ ] **Step 1: Failing test** — default JSON compiles identical to `defaultDoc`, and schema accepts the new blocks:

```go
func TestDefaultJSONMatchesCompiledDefault(t *testing.T) {
	v, err := parseVariant([]byte(defaultVariantJSON))
	if err != nil {
		t.Fatalf("default JSON did not parse/compile: %v", err)
	}
	d := defaultVariant()
	if v.weightSummary() != d.weightSummary() || v.topMult != d.topMult {
		t.Fatalf("default JSON drifted from compiled default")
	}
	if len(v.scatter) != len(d.scatter) {
		t.Fatalf("scatter tables differ: %d vs %d", len(v.scatter), len(d.scatter))
	}
}
```

(Check whether `config_test.go` already has a near-identical test to extend rather than duplicate.)

- [ ] **Step 2: Run, expect FAIL** (until JSON includes scatter/gamble + W/S weights).

- [ ] **Step 3: Implement** — extend `oddsVariantSchema`:
  - `weights.properties` gains `"W"` and `"S"` (`{"type":"integer","minimum":0}`).
  - add `"scatter"` array (items `{count integer ≥3, spins integer ≥1}`) and `"gamble"` object (`{maxRungs ≥1, maxWin ≥1}`); both optional (not in `required`).
  - Set `defaultVariantJSON` to the exact tuned `defaultDoc` (Task C4), including `scatter` and `gamble`.
  - Update the `Description` to mention wilds, free spins, and gamble.

- [ ] **Step 4: Run, expect PASS** — `go test ./...`.

- [ ] **Step 5: Commit** — `git commit -am "feat(pokies): config schema for wilds, scatter free spins, gamble"`

---

### Task G2: Allocation discipline on new render paths

**Files:**
- Modify: `alloc_test.go` (extend), `layout.go` (cache any per-render slices)
- Test: `alloc_test.go`

- [ ] **Step 1: Read `alloc_test.go`** to learn the existing `testing.AllocsPerRun` pattern.

- [ ] **Step 2: Add an alloc test** rendering a machine in each mode (free spins, gamble) and asserting zero (or the existing budget) allocations per render:

```go
func TestNoAllocPerRenderInFeatureModes(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.gamble = &gambleState{atRisk: 100, sel: selRed, card: -1}
	if n := testing.AllocsPerRun(50, func() { rm.compose(p) }); n > allocBudget {
		t.Errorf("gamble compose allocs = %v, want <= %v", n, allocBudget)
	}
	m.gamble = nil
	m.freeSpins = 5
	if n := testing.AllocsPerRun(50, func() { rm.compose(p) }); n > allocBudget {
		t.Errorf("free-spin compose allocs = %v, want <= %v", n, allocBudget)
	}
}
```

(Use the existing budget constant/value from `alloc_test.go`; if compose uses `fmt.Sprintf`, that already allocates and the budget reflects it — match the established expectation.)

- [ ] **Step 3: Run** — fix any avoidable per-render allocations (precompute static label strings; avoid building slices in `drawGamble`). PASS.

- [ ] **Step 4: Commit** — `git commit -am "test(pokies): allocation budget for feature render paths"`

---

### Task G3: Smoke script, full suite, vet, native run, docs

**Files:**
- Modify: `smoke.yaml` (exercise a gamble + free spins if the format supports scripted inputs), `main.go` doc comment (mention features)
- Verify: build + tests

- [ ] **Step 1: Read `smoke.yaml`** and extend the scripted sequence to show a spin → win → gamble, and (if feasible) a scatter trigger. Keep within the format.

- [ ] **Step 2: Update `main.go` package doc** to note wild substitution, scatter free spins, and the gamble ladder.

- [ ] **Step 3: Full verification**:

```bash
go vet ./...
go test ./...
go build ./...           # native build sanity
```

Expected: vet clean, all tests pass, build succeeds.

- [ ] **Step 4: Native smoke (optional, manual)** — `go run .` and play a few spins to eyeball the cabinet, free spins, and gamble selector at 80×24. Confirm 👑/🎁 render width-2 and the gamble row aligns.

- [ ] **Step 5: Commit** — `git commit -am "chore(pokies): smoke script + docs for the new features"`

---

## Phase H — Finish

### Task H1: Self-review, RTP sanity, PR

- [ ] **Step 1: Re-run the whole suite** — `go test ./... -count=1` green.
- [ ] **Step 2: Print the final default profile** (temporary log or a kept `t.Logf` test) and confirm total RTP ∈ [0.72, 0.78], trigger rate sane (e.g. 1–5%), avg free spins reasonable.
- [ ] **Step 3: `go vet ./...`** clean.
- [ ] **Step 4: Update the design/plan docs** if anything diverged (e.g. suit letters instead of glyphs is already documented).
- [ ] **Step 5: Use `superpowers:finishing-a-development-branch`** to open the PR against `main` with a summary of the three features, the RTP-gate changes, and the width-discipline note (suits as letters).

---

## Self-review checklist (author)

- **Spec coverage:** wilds (B1), scatter free spins (C1–C4, D1–D3), gamble ladder incl. suit ×4 (E1–E2), RTP closed form + gates (C2–C3), config schema (G1), per-viewer gamble (F2), ticker/free-spin announce (F3), tests (every task), out-of-scope floor untouched. ✓
- **Placeholder scan:** the only deferred decisions are the empirically-tuned default weights (C4) and reading existing `alloc_test.go`/`smoke.yaml` formats (G2/G3) — each has a concrete method, not a vague TODO. The D2 `enterGamble` stub is explicitly temporary and superseded in E2. ✓
- **Type consistency:** `variantStats` fields (`LineRTP`, `HitFreq`, `TriggerRate`, `AvgFreeSpins`, `TotalRTP`) used consistently; `scatterAward([3][3]symbol)`, `scatterWindow(strip, [3]int)`, `selTake…selClubs` selector enum, `gambleState{atRisk,rungs,sel,card,last}`, `creditWin/announce/enterGamble/takeWin/resolveGuess` signatures match across tasks. ✓
- **Determinism:** `distribute` is RNG-free; gamble tests use `resolveGuess(suit)` not RNG peeking. ✓
