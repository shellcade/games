package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

// symbol is the single-byte logical ID of a slot face: the key the odds
// variant JSON uses for weights/paytable (admin-surface compatible) and the
// element a reel strip is built from. What a face *looks like* is presentation
// only — faceArt in layout.go maps each symbol to its width-2 emoji cluster,
// drawn via the kit v2 grapheme cells.
type symbol byte

const (
	symBlank   symbol = '-' // neutral face shown before the first spin
	sym7       symbol = '7'
	symDollar  symbol = '$'
	symStar    symbol = '*'
	symBar     symbol = 'B'
	symCherry  symbol = 'C'
	symWild    symbol = 'W' // substitutes for any paying symbol within a ways run
	symScatter symbol = 'S' // counts anywhere in the window; triggers free spins
)

// The machine is a 5-reel, 3-row grid scored under 243 ways (3^5): a symbol pays
// its left-aligned run (adjacent reels from reel 0, any rows, wild substituting),
// credited pays[len] × the product of the per-reel counts ("ways").
const (
	numReels = 5
	visRows  = 3
)

// Validation bounds for an odds variant. They are wide on purpose — this is play
// money, so the bounds catch fat-fingered mistakes (a zeroed weight set, a
// negative multiplier, an absurd strip) rather than enforcing real-money policy.
// These mirror the native arcade admin surface so a config written there is
// accepted/rejected identically here.
const (
	maxStops   = 64   // cap on stops per reel (strip length) — bounds the strip³ enumeration
	minRTP     = 0.10 // 10% — catches a paytable accidentally zeroed out
	maxRTP     = 1.50 // 150% — catches a fat-fingered jackpot that makes the machine a printer
	rtpEpsilon = 1e-9
)

// defaultGamble is the compiled-in ladder cap used when a variant omits a gamble
// block: up to 5 doubles, then auto-take, with a generous credit ceiling.
var defaultGamble = gambleConfig{MaxRungs: 5, MaxWin: 1_000_000}

// stripOrder is the regular (grouped) symbols laid out in a stable order, so a
// seeded RNG reproduces draws across runs regardless of map iteration order.
// WILD and SCATTER are NOT here — they are distributed across the strip
// (specialOrder) so scatters spread naturally rather than clumping.
var stripOrder = []symbol{sym7, symDollar, symStar, symBar, symCherry}

// specialOrder is the deterministic distribution order of the special symbols
// (wild, then scatter) interleaved into the grouped base strip.
var specialOrder = []symbol{symWild, symScatter}

// symbolByName maps a face byte to its JSON weight/paytable key.
var symbolByName = map[string]symbol{
	"7": sym7, "$": symDollar, "*": symStar, "B": symBar, "C": symCherry,
	"W": symWild, "S": symScatter,
}

func nameOfSymbol(s symbol) string { return string(rune(s)) }

// oddsVariant is the on-the-wire JSON document stored under the "odds-variant"
// config key (the same document the native arcade admin area writes): a mini PAR
// sheet. weights map each reel symbol to its number of stops on the (single)
// virtual strip; paytable lists three-of-a-kind payouts top-down (first match
// wins).
type oddsVariant struct {
	Name     string         `json:"name"`
	Weights  map[string]int `json:"weights"`
	Paytable []payEntry     `json:"paytable"`
	Scatter  []scatterEntry `json:"scatter,omitempty"`
	Gamble   *gambleConfig  `json:"gamble,omitempty"`
}

// payEntry is one paytable row: a left-aligned run of `faces` (wild substituting)
// pays pay3 / pay4 / pay5 × the per-run `ways` for runs of length 3 / 4 / 5.
type payEntry struct {
	Faces string `json:"faces"`
	Pay3  int    `json:"pay3"`
	Pay4  int    `json:"pay4"`
	Pay5  int    `json:"pay5"`
}

// scatterEntry: `count` scatters anywhere in the 3x3 window award `spins` free
// spins. Highest matching count wins.
type scatterEntry struct {
	Count int `json:"count"`
	Spins int `json:"spins"`
}

// gambleConfig caps the double-up ladder: at most MaxRungs doubles, and an
// at-risk win reaching MaxWin forces an auto-take.
type gambleConfig struct {
	MaxRungs int `json:"maxRungs"`
	MaxWin   int `json:"maxWin"`
}

// variant is the compiled, validated runtime form: an ordered weighted strip
// (built in the stable stripOrder, so a seeded room reproduces outcomes for a
// given variant) plus a symbol→multiplier triple paytable.
type variant struct {
	name    string
	strip   []symbol
	pays    map[symbol][3]int // per symbol, the run-length 3/4/5 multipliers (absent = pays 0)
	scatter []scatterEntry    // free-spin trigger table, sorted by Count descending
	gamble  gambleConfig      // double-up ladder caps (defaults applied at compile)

	// Paytable display, computed ONCE at compile time (compilePayTable). The
	// paytable is static for a variant's life, but drawPaytable runs every render
	// per viewer — recomputing the sorted rows + labels there would leak a slice
	// and a sort.SliceStable on every wake under -gc=leaking. Cache them.
	payRowsCache []payRow
	payLabels    []string
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

// scatterWindow is the [reel][row] face grid when reels are stopped at the given
// per-reel landing indices.
func scatterWindow(strip []symbol, idx [numReels]int) (w [numReels][visRows]symbol) {
	for reel := 0; reel < numReels; reel++ {
		w[reel] = windowAt(strip, idx[reel]) // [top, center, bottom]
	}
	return w
}

// scatterAward counts scatters across all numReels*visRows visible cells and
// returns the free spins for the highest matching threshold (0 if below the
// lowest, or no table). v.scatter is kept sorted by Count descending at compile.
func (v *variant) scatterAward(w [numReels][visRows]symbol) int {
	count := 0
	for reel := 0; reel < numReels; reel++ {
		for row := 0; row < visRows; row++ {
			if w[reel][row] == symScatter {
				count++
			}
		}
	}
	return v.awardForCount(count)
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

// defaultDoc is the compiled-in tuning the machine uses when no config is stored
// (or a stored variant fails to parse): a 5-reel 243-ways PAR sheet with one wild
// and two scatters distributed per strip, free spins at 3/4/5 scatters
// (6/10/15), and a 5-rung gamble. Tuned to a ~84% total RTP (line + free-spin EV)
// in the modern-pokie range. Cherries are the blank (pay nothing).
func defaultDoc() oddsVariant {
	return oddsVariant{
		Name: "Default",
		Weights: map[string]int{
			"7": 1, "$": 2, "*": 3, "B": 5, "C": 30, "W": 1, "S": 2,
		},
		Paytable: []payEntry{
			{Faces: "7", Pay3: 10, Pay4: 30, Pay5: 100},
			{Faces: "$", Pay3: 6, Pay4: 20, Pay5: 60},
			{Faces: "*", Pay3: 4, Pay4: 12, Pay5: 36},
			{Faces: "B", Pay3: 2, Pay4: 6, Pay5: 16},
		},
		Scatter: []scatterEntry{
			{Count: 3, Spins: 6},
			{Count: 4, Spins: 10},
			{Count: 5, Spins: 15},
		},
		Gamble: &gambleConfig{MaxRungs: 5, MaxWin: 1_000_000},
	}
}

// defaultVariant compiles defaultDoc; a failure here is a programming bug.
func defaultVariant() *variant {
	v, err := compileVariant(defaultDoc())
	if err != nil {
		panic(fmt.Sprintf("pokies: default variant does not compile: %v", err))
	}
	return v
}

// parseVariant decodes a stored config blob into a compiled, validated variant.
func parseVariant(blob []byte) (*variant, error) {
	var doc oddsVariant
	if err := json.Unmarshal(blob, &doc); err != nil {
		return nil, fmt.Errorf("parse odds variant: %w", err)
	}
	return compileVariant(doc)
}

// compileVariant validates an odds document and builds its runtime strip +
// paytable. Validation bounds: every weight ≥ 0 with at least one positive,
// strip length within maxStops, every multiplier ≥ 0, and the computed
// theoretical RTP within [minRTP, maxRTP]. These match the native admin path so
// a config saved there compiles identically here.
func compileVariant(doc oddsVariant) (*variant, error) {
	weights := map[symbol]int{}
	positive := false
	for name, w := range doc.Weights {
		sym, ok := symbolByName[name]
		if !ok {
			return nil, fmt.Errorf("unknown symbol %q in weights", name)
		}
		if w < 0 {
			return nil, fmt.Errorf("symbol %q has a negative weight %d", name, w)
		}
		if w > 0 {
			positive = true
		}
		weights[sym] = w
	}
	if !positive {
		return nil, fmt.Errorf("at least one symbol must have a positive weight")
	}

	strip := buildStripFrom(weights)
	if len(strip) == 0 {
		return nil, fmt.Errorf("the reel strip is empty")
	}
	if len(strip) > maxStops {
		return nil, fmt.Errorf("the reel strip has %d stops, the cap is %d", len(strip), maxStops)
	}

	pays := map[symbol][3]int{}
	for _, pe := range doc.Paytable {
		sym, ok := symbolByName[pe.Faces]
		if !ok {
			return nil, fmt.Errorf("unknown symbol %q in paytable", pe.Faces)
		}
		if pe.Pay3 < 0 || pe.Pay4 < 0 || pe.Pay5 < 0 {
			return nil, fmt.Errorf("symbol %q has a negative multiplier", pe.Faces)
		}
		// First row for a symbol wins (top-down).
		if _, dup := pays[sym]; !dup {
			pays[sym] = [3]int{pe.Pay3, pe.Pay4, pe.Pay5}
		}
	}

	// Scatter trigger table: validate, then sort by Count descending (scatterAward
	// scans it high-to-low).
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

	v := &variant{name: doc.Name, strip: strip, pays: pays, scatter: scatter, gamble: gamble}
	v.payRowsCache, v.payLabels = compilePayTable(pays)

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
}

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

// variantStats is the exact theoretical profile of a variant under 243 ways,
// computed in closed form from the per-reel symbol marginals.
type variantStats struct {
	LineRTP      float64 // mean ways payout per spin (bet multiples)
	HitFreq      float64 // loose sanity metric: mean filled fraction of the grid
	TriggerRate  float64 // t: P(scatter count reaches the lowest threshold)
	AvgFreeSpins float64 // m: expected free spins per trigger, incl. retrigger
	TotalRTP     float64 // LineRTP * (1 + t*m): base + free-spin EV
}

// reelMarginal returns a = E[countFor(s, window)] and z = P(countFor==0) over all
// strip stops for one reel (reels are i.i.d.).
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

// scatterTotalDist returns P(total scatters across numReels reels = k), for k in
// 0..numReels*visRows, by convolving the per-reel scatter-count distribution.
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

// stats computes the exact theoretical profile under 243 ways. Line RTP is a sum
// over symbols, so by linearity of expectation it is exact from each symbol's
// per-reel count marginal (a, z): E[win_s] = pay3·a³·z + pay4·a⁴·z + pay5·a⁵
// (the run stops at reel L<5 with prob z; a run of 5 has no stopping reel). Scatter
// trigger uses the convolved total-count distribution; free spins fold in via the
// branching-process closed form m = avgAward/(1 − t·avgAward).
func (v *variant) stats() variantStats {
	st := variantStats{}
	filled := 0.0
	for _, s := range stripOrder {
		p := v.pays[s]
		a, z := v.reelMarginal(s)
		filled += a
		if p == ([3]int{}) {
			continue
		}
		a3 := a * a * a
		st.LineRTP += float64(p[0])*a3*z + float64(p[1])*a3*a*z + float64(p[2])*a3*a*a
	}
	// Loose sanity metric: average filled fraction of a single reel window across
	// the paying symbols (not an exact hit rate; only checked to be positive).
	st.HitFreq = filled / (float64(len(stripOrder)) * float64(visRows))

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

// weightSummary renders the strip weights as a stable "7:1 $:2 *:3 B:5 C:7"
// string (stripOrder, so the layout is deterministic).
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

// payRow is one paytable entry: a run of sym pays pays[len-3] × ways.
type payRow struct {
	sym  symbol
	pays [3]int // {pay3, pay4, pay5}
}

// compilePayTable builds the pay rows (highest 5-of-a-kind first, stable in
// stripOrder) and their " 5:.. 4:.. 3:.." labels ONCE per variant (called from
// compileVariant). drawPaytableFor reads the cached slices, allocating nothing
// per render.
func compilePayTable(pays map[symbol][3]int) (rows []payRow, labels []string) {
	for _, s := range stripOrder {
		if p := pays[s]; p != ([3]int{}) {
			rows = append(rows, payRow{s, p})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].pays[2] > rows[j].pays[2] })
	labels = make([]string, len(rows))
	for i, pr := range rows {
		labels[i] = fmt.Sprintf(" %d/%d/%d", pr.pays[2], pr.pays[1], pr.pays[0])
	}
	return rows, labels
}

// windowAt returns the three visible faces (top, center, bottom) when the strip
// is stopped with idx centered. Wraps around the strip.
func windowAt(strip []symbol, idx int) [3]symbol {
	n := len(strip)
	return [3]symbol{strip[(idx-1+n)%n], strip[idx], strip[(idx+1)%n]}
}

// rollWindow returns the visible faces for a reel still spinning, scrolled to the
// given animation offset (contiguous, so the wheel appears to roll).
func rollWindow(strip []symbol, offset int) [3]symbol {
	n := len(strip)
	o := ((offset % n) + n) % n
	return [3]symbol{strip[o], strip[(o+1)%n], strip[(o+2)%n]}
}
