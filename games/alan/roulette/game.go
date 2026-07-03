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
		ShortDescription: "Gather round one American double-zero wheel. Spread your chips and watch it spin.",
		MinPlayers:       1,
		MaxPlayers:       6,
		Tags:             []string{"roulette", "casino", "betting", "american"},

		// A casino game: players wager their account-wide platform Credits. The
		// richest wager is a straight-up (35:1), so a winning stake returns
		// stake*(35+1) = stake*36 and no board can pay more than 36x its stake.
		Kind:                kit.GameKindCasino,
		MaxPayoutMultiplier: 36,

		// Player characters (seat tiles) + Credits (the host owns every balance;
		// this game calls Wager/Settle/Buyback/Balance).
		CtxFeatures: kit.CtxFeatCharacter | kit.CtxFeatCredits,

		// A casual social table: when everyone leaves, the room closes — no
		// hibernation snapshot, no Resume-menu entry.
		Lifecycle: kit.LifecycleEphemeral,

		QuickModeLabel:    "Quick table",
		SoloModeLabel:     "Solo table",
		PrivateInviteLine: "Friends pull up a chair when they enter the code.",

		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Credits",
			Direction:   kit.HigherBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Integer,
		},

		// Touch deck chips (kit v2.10.0): the betting modifiers beyond the
		// canonical vocabulary. Cursor movement and bet placement stay on
		// the canonical arrows + Confirm.
		Controls: []kit.ControlDecl{
			kit.RuneControl('+', "BET+"),
			kit.RuneControl('-', "BET-"),
			kit.RuneControl('c', "CLEAR"),
			kit.RuneControl('r', "READY"),
			kit.KeyControl(kit.KeyBackspace, "UNDO"),
		},
	}
}

// NewRoom returns the per-room behavior.
func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return newRoom(cfg, svc)
}
