package main

import "fmt"

// themes.go defines the six named 5-reel PAR sheets bound one-per-machine on the
// lounge floor. Each is a distinct odds variant (weights / pays / scatter /
// gamble) tuned into the modern-pokie RTP band; logical symbols and face art are
// shared across themes (themes differ by odds + name). One machine = one theme.

// themeDocs returns the six PAR-sheet documents in machine order. Each compiles
// to a total RTP in roughly [0.83, 0.92] (verified by TestThemesCompileInBand).
func themeDocs() []oddsVariant {
	scatter := []scatterEntry{{Count: 3, Spins: 6}, {Count: 4, Spins: 10}, {Count: 5, Spins: 15}}
	gamble := &gambleConfig{MaxRungs: 5, MaxWin: 1_000_000}
	return []oddsVariant{
		{
			Name:    "Lucky 7s", // high-variance, top-symbol focus (~86%)
			Weights: map[string]int{"7": 1, "$": 2, "*": 3, "B": 5, "C": 30, "W": 1, "S": 2},
			Paytable: []payEntry{
				{Faces: "7", Pay3: 13, Pay4: 42, Pay5: 147},
				{Faces: "$", Pay3: 6, Pay4: 21, Pay5: 63},
				{Faces: "*", Pay3: 4, Pay4: 13, Pay5: 36},
				{Faces: "B", Pay3: 2, Pay4: 6, Pay5: 16},
			},
			Scatter: scatter, Gamble: gamble,
		},
		{
			Name:    "Gem Rush", // mid-symbol focus, frequent (~87%)
			Weights: map[string]int{"7": 1, "$": 3, "*": 4, "B": 5, "C": 29, "W": 1, "S": 2},
			Paytable: []payEntry{
				{Faces: "7", Pay3: 8, Pay4: 24, Pay5: 80},
				{Faces: "$", Pay3: 5, Pay4: 14, Pay5: 44},
				{Faces: "*", Pay3: 3, Pay4: 9, Pay5: 26},
				{Faces: "B", Pay3: 1, Pay4: 4, Pay5: 12},
			},
			Scatter: scatter, Gamble: gamble,
		},
		{
			Name:    "Bells", // low-variance, frequent small wins (~83%)
			Weights: map[string]int{"7": 1, "$": 2, "*": 4, "B": 6, "C": 29, "W": 1, "S": 2},
			Paytable: []payEntry{
				{Faces: "7", Pay3: 7, Pay4: 21, Pay5: 70},
				{Faces: "$", Pay3: 5, Pay4: 14, Pay5: 42},
				{Faces: "*", Pay3: 2, Pay4: 8, Pay5: 23},
				{Faces: "B", Pay3: 1, Pay4: 3, Pay5: 10},
			},
			Scatter: scatter, Gamble: gamble,
		},
		{
			Name:    "Cherry Pop", // scatter-rich free-spin theme (~89%)
			Weights: map[string]int{"7": 1, "$": 2, "*": 3, "B": 5, "C": 28, "W": 1, "S": 3},
			Paytable: []payEntry{
				{Faces: "7", Pay3: 8, Pay4: 24, Pay5: 80},
				{Faces: "$", Pay3: 5, Pay4: 14, Pay5: 45},
				{Faces: "*", Pay3: 2, Pay4: 8, Pay5: 24},
				{Faces: "B", Pay3: 2, Pay4: 4, Pay5: 11},
			},
			Scatter: []scatterEntry{{Count: 3, Spins: 5}, {Count: 4, Spins: 8}, {Count: 5, Spins: 12}},
			Gamble:  gamble,
		},
		{
			Name:    "Crown", // wild-leaning, big swings (~91%)
			Weights: map[string]int{"7": 1, "$": 2, "*": 3, "B": 5, "C": 29, "W": 2, "S": 2},
			Paytable: []payEntry{
				{Faces: "7", Pay3: 5, Pay4: 16, Pay5: 57},
				{Faces: "$", Pay3: 3, Pay4: 10, Pay5: 30},
				{Faces: "*", Pay3: 2, Pay4: 6, Pay5: 16},
				{Faces: "B", Pay3: 1, Pay4: 3, Pay5: 8},
			},
			Scatter: scatter, Gamble: gamble,
		},
		{
			Name:    "Gift Drop", // balanced all-rounder (~87%)
			Weights: map[string]int{"7": 1, "$": 2, "*": 3, "B": 6, "C": 29, "W": 1, "S": 2},
			Paytable: []payEntry{
				{Faces: "7", Pay3: 7, Pay4: 22, Pay5: 72},
				{Faces: "$", Pay3: 4, Pay4: 14, Pay5: 43},
				{Faces: "*", Pay3: 3, Pay4: 9, Pay5: 26},
				{Faces: "B", Pay3: 1, Pay4: 4, Pay5: 12},
			},
			Scatter: scatter, Gamble: gamble,
		},
	}
}

// themeVariants compiles the six PAR sheets; a failure here is a programming bug
// (a theme PAR sheet that does not satisfy the RTP/convergence gates).
func themeVariants() []*variant {
	docs := themeDocs()
	vs := make([]*variant, len(docs))
	for i, doc := range docs {
		v, err := compileVariant(doc)
		if err != nil {
			panic(fmt.Sprintf("pokies: theme %q does not compile: %v", doc.Name, err))
		}
		vs[i] = v
	}
	return vs
}
