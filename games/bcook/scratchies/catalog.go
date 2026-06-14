package main

import kit "github.com/shellcade/kit/v2"

// catalog.go — the 16-ticket catalog (SPEC §6).
//
// Prize tables are tuned so stats() lands in each tier's RTP / hit-rate band
// (SPEC §7):
//
//   $1  hitRate ≈ 1/3.9 (±0.02),  RTP 58–62 %
//   $2  hitRate ≈ 1/3.7 (±0.02),  RTP 60–64 %
//   $5  hitRate ≈ 1/3.5 (±0.02),  RTP 63–66 %
//   $10 hitRate ≈ 1/3.3 (±0.02),  RTP 65–68 %
//
// Each table has 9 rows ascending in Credits; the top row is the headline
// jackpot (SPEC §6).  Only catalog.go and catalog_test.go are modified from the
// starter; ticket structure (slugs, names, prices, mechanics, grid knobs) is
// unchanged.

// stats computes the exact theoretical RTP and any-win hit rate under
// drawOutcome's sequential semantics.
func (t *Ticket) stats() (rtp, hitRate float64) {
	miss := 1.0
	for _, row := range t.Prizes {
		if row.OneIn <= 0 {
			continue
		}
		p := miss * (1.0 / float64(row.OneIn))
		hitRate += p
		rtp += p * float64(row.Credits) / float64(t.Price)
		miss *= 1.0 - 1.0/float64(row.OneIn)
	}
	return rtp, hitRate
}

// ---- prize-table helpers -----------------------------------------------
//
// Each tier shares a common 9-row ladder shape; only the prize scale and
// jackpot differ.  Rows are ascending in Credits.
//
// Tuning knobs (OneIn values) were iterated until every ticket cleared its
// tier band; see catalog_test.go for the assertions.

// tier1Table builds a $1-tier prize ladder topped at `top` credits.
// hitRate ≈ 0.267 (1 in 3.74),  RTP ≈ 0.60–0.61.
func tier1Table(top int) PrizeTable {
	return PrizeTable{
		{Credits: 1, OneIn: 7},
		{Credits: 2, OneIn: 10},
		{Credits: 5, OneIn: 28},
		{Credits: 10, OneIn: 80},
		{Credits: 20, OneIn: 500},
		{Credits: 50, OneIn: 2500},
		{Credits: 100, OneIn: 12000},
		{Credits: top / 10, OneIn: 280000},
		{Credits: top, OneIn: 1400000},
	}
}

// tier2Table builds a $2-tier prize ladder topped at `top` credits.
// hitRate ≈ 0.267 (1 in 3.74),  RTP ≈ 0.605–0.607.
func tier2Table(top int) PrizeTable {
	return PrizeTable{
		{Credits: 2, OneIn: 7},
		{Credits: 4, OneIn: 10},
		{Credits: 10, OneIn: 28},
		{Credits: 20, OneIn: 80},
		{Credits: 40, OneIn: 500},
		{Credits: 100, OneIn: 2500},
		{Credits: 200, OneIn: 12000},
		{Credits: top / 10, OneIn: 280000},
		{Credits: top, OneIn: 1400000},
	}
}

// tier5Table builds a $5-tier prize ladder topped at `top` credits.
// hitRate ≈ 0.286 (1 in 3.50),  RTP ≈ 0.644–0.647.
func tier5Table(top int) PrizeTable {
	return PrizeTable{
		{Credits: 5, OneIn: 7},
		{Credits: 10, OneIn: 8},
		{Credits: 25, OneIn: 32},
		{Credits: 50, OneIn: 65},
		{Credits: 100, OneIn: 650},
		{Credits: 250, OneIn: 2500},
		{Credits: 500, OneIn: 12000},
		{Credits: top / 10, OneIn: 280000},
		{Credits: top, OneIn: 1400000},
	}
}

// tier10Table builds a $10-tier prize ladder topped at `top` credits.
// hitRate ≈ 0.306 (1 in 3.27),  RTP ≈ 0.664–0.667.
func tier10Table(top int) PrizeTable {
	return PrizeTable{
		{Credits: 10, OneIn: 6},
		{Credits: 20, OneIn: 8},
		{Credits: 50, OneIn: 32},
		{Credits: 100, OneIn: 65},
		{Credits: 200, OneIn: 500},
		{Credits: 500, OneIn: 2500},
		{Credits: 1000, OneIn: 12000},
		{Credits: top / 10, OneIn: 280000},
		{Credits: top, OneIn: 1400000},
	}
}

var themeDefault = Theme{Accent: kit.Yellow, Symbol: kit.Green}

// tickets is the live catalog. Four mechanics × four tiers, each its own theme.
var tickets = []Ticket{
	// --- $1 ---
	{Slug: "lucky-7s", Name: "Lucky 7s", Price: 1, Mechanic: MechMatch3, Theme: themeDefault, Cols: 3, Rows: 3, Prizes: tier1Table(10000)},
	{Slug: "coin-toss", Name: "Coin Toss", Price: 1, Mechanic: MechKeyNum, Theme: themeDefault, Cols: 3, Rows: 2, WinNumbers: 2, Prizes: tier1Table(10000)},
	{Slug: "cherry-pop", Name: "Cherry Pop", Price: 1, Mechanic: MechFind, Theme: themeDefault, Cols: 3, Rows: 3, Symbol: "🍒", Prizes: tier1Table(10000)},
	{Slug: "tinnie-tripler", Name: "Tinnie Tripler", Price: 1, Mechanic: MechMult, Theme: themeDefault, Cols: 1, Rows: 2, MaxMult: 3, Prizes: tier1Table(12000)},

	// --- $2 ---
	{Slug: "gold-rush", Name: "Gold Rush", Price: 2, Mechanic: MechMatch3, Theme: themeDefault, Cols: 4, Rows: 4, Prizes: tier2Table(25000)},
	{Slug: "lucky-numbers", Name: "Lucky Numbers", Price: 2, Mechanic: MechKeyNum, Theme: themeDefault, Cols: 3, Rows: 3, WinNumbers: 3, Prizes: tier2Table(25000)},
	{Slug: "croc-cash", Name: "Croc Cash", Price: 2, Mechanic: MechFind, Theme: themeDefault, Cols: 4, Rows: 3, Symbol: "🐊", HasBust: true, Prizes: tier2Table(25000)},
	{Slug: "double-trouble", Name: "Double Trouble", Price: 2, Mechanic: MechMult, Theme: themeDefault, Cols: 1, Rows: 2, MaxMult: 5, Prizes: tier2Table(30000)},

	// --- $5 ---
	{Slug: "diamond-mine", Name: "Diamond Mine", Price: 5, Mechanic: MechMatch3, Theme: themeDefault, Cols: 5, Rows: 5, Prizes: tier5Table(100000)},
	{Slug: "lotto-lanes", Name: "Lotto Lanes", Price: 5, Mechanic: MechKeyNum, Theme: themeDefault, Cols: 4, Rows: 4, WinNumbers: 4, Prizes: tier5Table(100000)},
	{Slug: "treasure-hunt", Name: "Treasure Hunt", Price: 5, Mechanic: MechFind, Theme: themeDefault, Cols: 5, Rows: 4, Symbol: "💎", Prizes: tier5Table(100000)},
	{Slug: "mega-multiplier", Name: "Mega Multiplier", Price: 5, Mechanic: MechMult, Theme: themeDefault, Cols: 1, Rows: 2, MaxMult: 10, Prizes: tier5Table(120000)},

	// --- $10 ---
	{Slug: "platinum-sevens", Name: "Platinum Sevens", Price: 10, Mechanic: MechMatch3, Theme: themeDefault, Cols: 6, Rows: 6, Prizes: tier10Table(250000)},
	{Slug: "fortune-50", Name: "Fortune 50", Price: 10, Mechanic: MechKeyNum, Theme: themeDefault, Cols: 4, Rows: 6, WinNumbers: 6, Prizes: tier10Table(250000)},
	{Slug: "outback-riches", Name: "Outback Riches", Price: 10, Mechanic: MechFind, Theme: themeDefault, Cols: 6, Rows: 5, Symbol: "💰", HasBust: true, Prizes: tier10Table(250000)},
	{Slug: "cash-explosion", Name: "Cash Explosion", Price: 10, Mechanic: MechMult, Theme: themeDefault, Cols: 1, Rows: 2, MaxMult: 20, Prizes: tier10Table(300000)},

	// --- five new game types (reuse the tuned tier tables, so they stay in band) ---
	// Lucky Lines — three equal amounts in a row/column/diagonal.
	{Slug: "lucky-lines", Name: "Lucky Lines", Price: 1, Mechanic: MechLines, Theme: themeDefault, Cols: 3, Rows: 3, Prizes: tier1Table(10000)},
	{Slug: "mega-lines", Name: "Mega Lines", Price: 10, Mechanic: MechLines, Theme: themeDefault, Cols: 5, Rows: 5, Prizes: tier10Table(250000)},
	// Cashword — scratch a letter bank; complete listed words.
	{Slug: "cashword", Name: "Cashword", Price: 5, Mechanic: MechCrossword, Theme: themeDefault, Cols: 4, Rows: 4, WordList: []string{"GOLD", "CASH", "LUCKY", "RICH", "COIN", "WIN"}, Prizes: tier5Table(100000)},
	{Slug: "mega-crossword", Name: "Mega Crossword", Price: 10, Mechanic: MechCrossword, Theme: themeDefault, Cols: 5, Rows: 4, WordList: []string{"JACKPOT", "FORTUNE", "RICHES", "GOLDEN", "MONEY", "PRIZE", "LUCKY", "CASH"}, Prizes: tier10Table(250000)},
	// Quick Bingo — reveal your card; complete a line of called numbers.
	{Slug: "quick-bingo", Name: "Quick Bingo", Price: 2, Mechanic: MechBingo, Theme: themeDefault, Cols: 5, Rows: 5, WinNumbers: 10, Prizes: tier2Table(25000)},
	{Slug: "bingo-bonanza", Name: "Bingo Bonanza", Price: 5, Mechanic: MechBingo, Theme: themeDefault, Cols: 5, Rows: 5, WinNumbers: 12, Prizes: tier5Table(100000)},
	// Showdown — beat the house column by column.
	{Slug: "showdown", Name: "Showdown", Price: 1, Mechanic: MechShowdown, Theme: themeDefault, Cols: 3, Rows: 2, Prizes: tier1Table(10000)},
	{Slug: "dealers-bluff", Name: "Dealer's Bluff", Price: 2, Mechanic: MechShowdown, Theme: themeDefault, Cols: 4, Rows: 2, Prizes: tier2Table(25000)},
	// Triple Word — spell listed bonus words; a 3× tile triples one.
	{Slug: "triple-word", Name: "Triple Word", Price: 5, Mechanic: MechTriple, Theme: themeDefault, Cols: 6, Rows: 4, WordList: []string{"WIN", "CASH", "GOLD", "LUCK", "RICH", "MONEY", "BONUS"}, Prizes: tier5Table(100000)},
	{Slug: "word-jackpot", Name: "Word Jackpot", Price: 10, Mechanic: MechTriple, Theme: themeDefault, Cols: 6, Rows: 6, WordList: []string{"WINNER", "RICHES", "GOLDEN", "BONUS", "MONEY", "LUCKY", "CASH"}, Prizes: tier10Table(250000)},
}
