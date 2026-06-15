package main

import "testing"

// TestCatalogRTP asserts that every ticket's theoretical RTP sits inside its
// tier band (SPEC §7).  A mistuned prize table fails loudly here before it
// can ship.
func TestCatalogRTP(t *testing.T) {
	t.Helper()
	for _, tk := range tickets {
		tk := tk // capture
		t.Run(tk.Slug, func(t *testing.T) {
			rtp, _ := tk.stats()
			lo, hi := rtpBand(tk.Price)
			if rtp < lo || rtp > hi {
				t.Errorf("%s ($%d): RTP=%.4f outside [%.2f, %.2f]",
					tk.Slug, tk.Price, rtp, lo, hi)
			}
		})
	}
}

// TestCatalogHitRate asserts that every ticket's any-win probability sits in
// its tier band (SPEC §7).
func TestCatalogHitRate(t *testing.T) {
	for _, tk := range tickets {
		tk := tk
		t.Run(tk.Slug, func(t *testing.T) {
			_, hr := tk.stats()
			lo, hi := hitRateBand(tk.Price)
			if hr < lo || hr > hi {
				t.Errorf("%s ($%d): hitRate=%.4f (1 in %.2f) outside [%.3f, %.3f]",
					tk.Slug, tk.Price, hr, 1/hr, lo, hi)
			}
		})
	}
}

// TestCatalogTopPrize checks that the largest prize in each ticket's table
// matches the §6 headline jackpot.
func TestCatalogTopPrize(t *testing.T) {
	want := map[string]int{
		"lucky-7s":        10000,
		"coin-toss":       10000,
		"cherry-pop":      10000,
		"tinnie-tripler":  12000,
		"gold-rush":       25000,
		"lucky-numbers":   25000,
		"croc-cash":       25000,
		"double-trouble":  30000,
		"diamond-mine":    100000,
		"lotto-lanes":     100000,
		"treasure-hunt":   100000,
		"mega-multiplier": 120000,
		"platinum-sevens": 250000,
		"fortune-50":      250000,
		"outback-riches":  250000,
		"cash-explosion":  300000,
		"lucky-lines":     10000,
		"mega-lines":      250000,
		"cashword":        100000,
		"mega-crossword":  250000,
		"quick-bingo":     25000,
		"bingo-bonanza":   100000,
		"showdown":        10000,
		"dealers-bluff":   25000,
		"triple-word":     100000,
		"word-jackpot":    250000,
	}
	for _, tk := range tickets {
		tk := tk
		t.Run(tk.Slug, func(t *testing.T) {
			if len(tk.Prizes) == 0 {
				t.Fatal("empty prize table")
			}
			top := tk.Prizes[len(tk.Prizes)-1].Credits
			if exp, ok := want[tk.Slug]; ok {
				if top != exp {
					t.Errorf("%s: top prize=%d, want %d", tk.Slug, top, exp)
				}
			} else {
				t.Errorf("unknown slug %q — add to want map", tk.Slug)
			}
		})
	}
}

// TestCatalogPrizesAscending checks that every ticket's prize table is
// strictly non-decreasing in Credits (required by §7 and TestCatalogShape).
func TestCatalogPrizesAscending(t *testing.T) {
	for _, tk := range tickets {
		tk := tk
		t.Run(tk.Slug, func(t *testing.T) {
			prev := 0
			for i, row := range tk.Prizes {
				if row.Credits < prev {
					t.Errorf("%s: prize row %d (%d credits) < previous (%d) — table not ascending",
						tk.Slug, i, row.Credits, prev)
				}
				prev = row.Credits
			}
		})
	}
}

// ---- band helpers -------------------------------------------------------

// rtpBand returns the [lo, hi] RTP band for a given ticket price (SPEC §7).
func rtpBand(price int) (lo, hi float64) {
	switch price {
	case 1:
		return 0.58, 0.62
	case 2:
		return 0.60, 0.64
	case 5:
		return 0.63, 0.66
	case 10:
		return 0.65, 0.68
	default:
		return 0, 1 // unknown tier: don't fail
	}
}

// hitRateBand returns the [lo, hi] any-win frequency for a ticket price.
// Targets from SPEC §7: ≈1/3.9 (±0.02) for $1, ≈1/3.7 for $2, etc.
func hitRateBand(price int) (lo, hi float64) {
	switch price {
	case 1:
		return 0.236, 0.276 // target 1/3.9 ≈ 0.256
	case 2:
		return 0.250, 0.290 // target 1/3.7 ≈ 0.270
	case 5:
		return 0.266, 0.306 // target 1/3.5 ≈ 0.286
	case 10:
		return 0.283, 0.323 // target 1/3.3 ≈ 0.303
	default:
		return 0, 1
	}
}
