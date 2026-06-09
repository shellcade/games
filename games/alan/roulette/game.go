package main

import kit "github.com/shellcade/kit/v2"

// Game is the roulette registry entry: static metadata plus the per-room
// factory. One shared wheel per room; players take seats at the same felt.
type Game struct{}

// Meta returns the static game metadata. The Slug is the BARE name; the platform
// composes the namespaced "alan/roulette" from the catalog path, so Meta never
// carries a slash.
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "roulette",
		Name:             "Roulette",
		ShortDescription: "Gather round one wheel — place your chips across the felt and watch it spin.",
		MinPlayers:       1,
		MaxPlayers:       6,
		Tags:             []string{"roulette", "casino", "betting"},

		// A casual social table: when everyone leaves, the room closes — no
		// hibernation snapshot, no Resume-menu entry.
		Lifecycle: kit.LifecycleEphemeral,

		QuickModeLabel:    "Quick table",
		SoloModeLabel:     "Solo table",
		PrivateInviteLine: "Friends pull up a chair when they enter the code.",

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
