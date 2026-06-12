package main

import kit "github.com/shellcade/kit/v2"

// Game is the module registry entry: static metadata plus the room factory.
// The bare slug "spaceterm" is namespaced to "bcook/spaceterm" by the platform
// from the catalog path.
type Game struct{}

func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "spaceterm",
		Name:             "Spaceterm",
		ShortDescription: "Frantic co-op bridge duty: orders fly, timers drain, and the dial you need is on someone else's panel. Shout, hail, survive.",
		MinPlayers:       1,
		MaxPlayers:       6,
		Tags:             []string{"co-op", "party", "real-time", "frantic", "crew"},

		// Crew roster tiles wear each member's arcade glyph and colours.
		CtxFeatures: kit.CtxFeatCharacter,

		// A casual party ship: when everyone beams out, the room closes.
		Lifecycle: kit.LifecycleEphemeral,

		QuickModeLabel:    "Crew up",
		SoloModeLabel:     "Solo shift",
		PrivateInviteLine: "Crewmates beam aboard when they enter the code.",

		// Touch deck chips (kit v2.10.0): the eight panel hotkeys plus HAIL.
		// ('q' is reserved by the canonical vocabulary as Back, so the panel
		// block sits one column right: W E R T over S D F G.)
		Controls: []kit.ControlDecl{
			kit.RuneControl('w', "W"), kit.RuneControl('e', "E"),
			kit.RuneControl('r', "R"), kit.RuneControl('t', "T"),
			kit.RuneControl('s', "S"), kit.RuneControl('d', "D"),
			kit.RuneControl('f', "F"), kit.RuneControl('g', "G"),
			kit.RuneControl('h', "HAIL"),
		},

		// Sectors cleared: the whole crew banks the same number — co-op all
		// the way down. The board keeps each player's best run.
		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Sectors",
			Direction:   kit.HigherBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Integer,
		},
	}
}

func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return newRoom(cfg, svc)
}
