package main

import kit "github.com/shellcade/kit/v2"

// Game is the module registry entry: static metadata plus the room factory. The
// bare slug "bytebreaker" is namespaced to "alan/bytebreaker" by the platform
// from the catalog path; this Meta carries the bare name only.
type Game struct{}

func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "bytebreaker",
		Name:             "Bytebreaker",
		ShortDescription: "Smash a neon wall of bytes with a bouncing bit — a one-more-go brick breaker for your terminal.",
		MinPlayers:       1,
		MaxPlayers:       6,
		Tags:             []string{"arcade", "breakout", "action", "retro"},

		// Player characters: each rival's tile renders beside their name in
		// the live-scores strip.
		CtxFeatures: kit.CtxFeatCharacter,

		// A casual arcade cabinet: each player runs their own board, and when
		// everyone leaves the room closes — no hibernation snapshot.
		Lifecycle: kit.LifecycleEphemeral,

		QuickModeLabel:    "Quick game",
		SoloModeLabel:     "Solo run",
		PrivateInviteLine: "Friends pull up a cabinet when they enter the code.",

		// One shared high-score board: each run posts its best, the board keeps
		// each account's best (higher is better).
		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Score",
			Direction:   kit.HigherBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Integer,
		},
	}
}

// NewRoom returns the per-room behaviour.
func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return newRoom(cfg, svc)
}
