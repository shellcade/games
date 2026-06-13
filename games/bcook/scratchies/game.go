package main

import kit "github.com/shellcade/kit/v2"

// Game is the shellcade entry type: static metadata plus a per-room factory.
type Game struct{}

// Meta declares the game to the platform (see SPEC §12).
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:              "scratchies",
		Name:              "Scratchies",
		ShortDescription:  "Duck into the newsagent, buy an instant scratch-it, and peel the latex off panel by panel. 16 tickets, four price tiers, one dream jackpot.",
		MinPlayers:        1,
		MaxPlayers:        6,
		Tags:              []string{"casino", "luck", "scratch-card", "instant-win", "solo"},
		CtxFeatures:       kit.CtxFeatCharacter,
		Lifecycle:         kit.LifecycleEphemeral,
		QuickModeLabel:    "Pop in",
		SoloModeLabel:     "Solo visit",
		PrivateInviteLine: "Mates pull up to the same counter when they enter the code.",
		Controls: []kit.ControlDecl{
			kit.RuneControl(' ', "SCRATCH"),
			kit.RuneControl('a', "SCRATCH ALL"),
			kit.KeyControl(kit.KeyEnter, "BUY"),
			kit.RuneControl('q', "BACK"),
		},
		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Credits",
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
