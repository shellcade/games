package main

import kit "github.com/shellcade/kit"

// Game is the blackjack registry entry: static metadata plus the per-room
// factory. The catalog slug is composed by the platform from the directory path
// (games/bcook/blackjack -> "bcook/blackjack"); Meta carries the bare name.
type Game struct{}

// Meta returns the static game metadata (mirrors the native blackjack meta).
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "blackjack",
		Name:             "Blackjack",
		ShortDescription: "Take a seat at a shared dealer table — bet, hit, stand, and chase your high score.",
		MinPlayers:       1,
		MaxPlayers:       5,
		Tags:             []string{"cards", "casino"},

		QuickModeLabel:    "Join a table",
		SoloModeLabel:     "Heads-up vs dealer",
		PrivateInviteLine: "Friends take a seat when they enter the code.",

		// The native game is no-winner (it never End/Posts) and surfaced its
		// board via a custom KV-backed peak provider. The lean ABI has no custom
		// provider, so the board is declared here and fed with Post on a new
		// personal peak — the same high-water-mark metric (Chips), keyed off the
		// durable wallet's `peak` (see room.go persistWallet / postPeak).
		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Chips",
			Direction:   kit.HigherBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Integer,
		},
	}
}

// NewRoom returns the per-room behavior.
func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return newRoom(cfg, svc)
}
