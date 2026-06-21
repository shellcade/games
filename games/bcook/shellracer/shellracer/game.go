// Package shellracer is a multiplayer race to type a passage faster and more
// accurately, ported from the native shellcade cabinet to the wasm game ABI.
//
// The native game leaned on engine timers (After/Every), an OnTick simulation
// callback, SetPhase joinability, and per-viewer OnFrame snapshots. The lean
// ABI has none of those: every clock is a deadline held in guest memory and
// compared against r.Now() on the host heartbeat (OnWake), phases are internal
// state (joinability is host-derived), and per-viewer frames are composed and
// r.Send-ed per member. The decisive logic — server-authoritative validation,
// finish ranking, the anti-cheat flag — is preserved exactly.
//
// The behavior lives in this importable package so kittest can drive the
// Handler directly; the module's main + exports wire kit.Main/kit.Run to Game{}.
package shellracer

import kit "github.com/shellcade/kit/v2"

// Game is the shellracer registry entry: static metadata plus the per-room
// factory. The native meta declared min 1 / max 5. The meta slug is the BARE
// name ("shellracer"); the host composes the <author>/<name> namespace from the
// catalog path (the native game carried the full "bcook/shellracer" slug, which
// the lean ABI/host rejects — it composes the namespace itself).
type Game struct{}

// Meta returns the static game metadata. The native game declared no
// LeaderboardSpec on its meta yet still fed a board; the lean ABI requires the
// board be declared to feed it, so the WPM board is declared here (higher is
// better, each account's best is kept).
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "shellracer",
		Name:             "Shell Racer",
		ShortDescription: "Race opponents by typing a passage faster and more accurately.",
		MinPlayers:       1,
		MaxPlayers:       5,
		Tags:             []string{"typing", "race"},

		// A real-time typing race with no mid-game state worth resuming: when
		// everyone leaves, the room closes — no hibernation snapshot, no Resume entry.
		Lifecycle: kit.LifecycleEphemeral,

		QuickModeLabel:    "Quick race",
		SoloModeLabel:     "Solo practice",
		PrivateInviteLine: "The race begins when a second player joins.",

		// Per-member arcade characters (kit v2.9.0): each racer's tile
		// renders right before their name on the racer strip and the
		// results table.
		CtxFeatures: kit.CtxFeatCharacter,

		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "WPM",
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
