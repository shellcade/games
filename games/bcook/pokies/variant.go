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
	symWild    symbol = 'W' // substitutes on the payline to complete a triple
	symScatter symbol = 'S' // counts anywhere in the window; triggers free spins
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
}

// payEntry is one paytable row: three of `faces` pays `multiplier` × the bet.
type payEntry struct {
	Faces      string `json:"faces"`
	Multiplier int    `json:"multiplier"`
}

// variant is the compiled, validated runtime form: an ordered weighted strip
// (built in the stable stripOrder, so a seeded room reproduces outcomes for a
// given variant) plus a symbol→multiplier triple paytable.
type variant struct {
	name    string
	strip   []symbol
	triples map[symbol]int // three-of-a-kind multiplier per symbol (absent = pays 0)

	// Paytable display, computed ONCE at compile time (compilePayTable). The
	// paytable is static for a variant's life, but drawPaytable runs every render
	// per viewer — recomputing the sorted rows + " x%d" labels there leaked a
	// slice and a sort.SliceStable on every wake under -gc=leaking. Cache them.
	payRowsCache []payRow
	payLabels    []string
}

// payout returns the bet multiplier for three settled center faces under this
// variant: only a three-of-a-kind whose symbol is in the paytable pays.
func (v *variant) payout(reels [3]symbol) int {
	if reels[0] == reels[1] && reels[1] == reels[2] {
		return v.triples[reels[0]]
	}
	return 0
}

// defaultDoc is the compiled-in tuning the machine uses when no config is stored
// (or a stored variant fails to parse): the original strip weights
// (7:1 $:2 *:3 B:5 C:7) and paytable (500/150/55/10, cherries pay nothing), a
// high-variance ~75% RTP profile matching native pokies.
func defaultDoc() oddsVariant {
	return oddsVariant{
		Name: "Default",
		Weights: map[string]int{
			"7": 1, "$": 2, "*": 3, "B": 5, "C": 7,
		},
		Paytable: []payEntry{
			{Faces: "7", Multiplier: 500},
			{Faces: "$", Multiplier: 150},
			{Faces: "*", Multiplier: 55},
			{Faces: "B", Multiplier: 10},
		},
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

	triples := map[symbol]int{}
	for _, pe := range doc.Paytable {
		sym, ok := symbolByName[pe.Faces]
		if !ok {
			return nil, fmt.Errorf("unknown symbol %q in paytable", pe.Faces)
		}
		if pe.Multiplier < 0 {
			return nil, fmt.Errorf("symbol %q has a negative multiplier %d", pe.Faces, pe.Multiplier)
		}
		// Top-down, first match wins: keep the first multiplier for a symbol.
		if _, dup := triples[sym]; !dup {
			triples[sym] = pe.Multiplier
		}
	}

	v := &variant{name: doc.Name, strip: strip, triples: triples}
	v.payRowsCache, v.payLabels = compilePayTable(triples)
	rtp, _ := v.stats()
	if rtp < minRTP-rtpEpsilon || rtp > maxRTP+rtpEpsilon {
		return nil, fmt.Errorf("theoretical RTP %.1f%% is outside the allowed [%.0f%%, %.0f%%]",
			rtp*100, minRTP*100, maxRTP*100)
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

// stats enumerates all len(strip)³ equally-likely outcomes (each reel draws
// independently and uniformly from the strip) to compute the theoretical
// return-to-player (mean bet-multiplier credited) and the hit frequency (the
// share of outcomes that pay anything). strip³ is tiny (18³ = 5,832 by default,
// at most maxStops³), so this runs in well under a millisecond.
func (v *variant) stats() (rtp, hitFreq float64) {
	n := len(v.strip)
	if n == 0 {
		return 0, 0
	}
	total := 0
	hits := 0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			for k := 0; k < n; k++ {
				p := v.payout([3]symbol{v.strip[i], v.strip[j], v.strip[k]})
				total += p
				if p > 0 {
					hits++
				}
			}
		}
	}
	outcomes := n * n * n
	return float64(total) / float64(outcomes), float64(hits) / float64(outcomes)
}

// weightSummary renders the strip weights as a stable "7:1 $:2 *:3 B:5 C:7"
// string (stripOrder, so the layout is deterministic).
func (v *variant) weightSummary() string {
	counts := map[symbol]int{}
	for _, s := range v.strip {
		counts[s]++
	}
	out := ""
	for _, s := range stripOrder {
		if out != "" {
			out += " "
		}
		out += fmt.Sprintf("%s:%d", nameOfSymbol(s), counts[s])
	}
	return out
}

// payRow is one paytable entry: three of sym pays mult × the bet.
type payRow struct {
	sym  symbol
	mult int
}

// payRows returns the paying triples highest-multiplier first in a stable
// order, feeding the paytable strip under the cabinets. Stable sort keeps
// equal multipliers in stripOrder so the output never depends on map
// iteration order.
// compilePayTable builds the descending-by-multiplier pay rows and their
// " x%d" labels ONCE per variant (called from compileVariant). drawPaytable
// then reads the cached slices, allocating nothing per render.
func compilePayTable(triples map[symbol]int) (rows []payRow, labels []string) {
	// Range stripOrder (not the triples map) so iteration order is deterministic.
	for _, s := range stripOrder {
		if m := triples[s]; m > 0 {
			rows = append(rows, payRow{s, m})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].mult > rows[j].mult })
	labels = make([]string, len(rows))
	for i, pr := range rows {
		labels[i] = fmt.Sprintf(" x%d", pr.mult)
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
