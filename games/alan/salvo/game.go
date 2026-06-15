package main

import kit "github.com/shellcade/kit/v2"

// Game is the module registry entry: static metadata plus the room factory. The
// bare slug "salvo" is namespaced to "alan/salvo" by the platform from the
// catalog path.
type Game struct{}

func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "salvo",
		Name:             "Salvo",
		ShortDescription: "Lob shells over destructible hills, read the wind, and blast your rivals off the map — turn-based tank artillery.",
		MinPlayers:       1,
		MaxPlayers:       6,
		Tags:             []string{"artillery", "tanks", "turn-based", "strategy", "destructible"},

		// Player characters: each tank wears its owner's glyph and colours.
		CtxFeatures: kit.CtxFeatCharacter,

		// A casual battlefield: when everyone leaves, the room closes.
		Lifecycle: kit.LifecycleEphemeral,

		QuickModeLabel:    "Quick battle",
		SoloModeLabel:     "Solo vs CPU",
		PrivateInviteLine: "Friends roll onto the battlefield when they enter the code.",

		// Touch deck chip (kit v2.10.0): weapon cycling is the one input
		// beyond the canonical vocabulary (aim/power/fire stay on the
		// arrows + Confirm).
		Controls: []kit.ControlDecl{
			kit.RuneControl('w', "WEAPON"),
		},

		// Career wins: every match victory bumps a durable counter, posted to a
		// shared leaderboard (higher is better, the board keeps your best total).
		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Wins",
			Direction:   kit.HigherBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Integer,
		},
	}
}

func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return newRoom(cfg, svc)
}
