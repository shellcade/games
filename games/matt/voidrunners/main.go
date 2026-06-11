// Voidrunners — an SSH arcade space shooter for shellcade.
//
// Asteroids-style flight on the fixed 80x24 terminal canvas: rotate, thrust
// with momentum, and fire. Floating craters drift through the arena to be shot
// (they break apart) or dodged (they kill you). It's free-for-all PvP — every
// player can blast every other player — in an endless arena where you respawn
// forever and chase your all-time best kill count on the leaderboard.
//
// Dev loop:  go run .            (plays in this terminal; -seats N for hot-seat)
//
//	Artifact:  tinygo build -opt=1 -no-debug -gc=leaking \
//	               -o voidrunners.wasm -target wasip1 -buildmode=c-shared .
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }

// Game is the module registry entry: static metadata plus the room factory.
type Game struct{}

// Meta returns the static game metadata shown in the arcade. The bare slug
// "voidrunners" is namespaced to "matt/voidrunners" by the platform
// from the catalog path; this Meta carries the bare name only.
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "voidrunners",
		Name:             "Voidrunners",
		ShortDescription: "Fly, dodge floating craters, and blast rival pilots in a free-for-all arena.",
		MinPlayers:       1,
		MaxPlayers:       6,
		Tags:             []string{"action", "shooter", "pvp", "arcade"},

		// Player characters: each pilot's tile renders beside their name on
		// the scoreboard, visible to every player.
		CtxFeatures: kit.CtxFeatCharacter,

		QuickModeLabel:    "Quick dogfight",
		SoloModeLabel:     "Target practice",
		PrivateInviteLine: "Friends join your arena when they enter the code.",

		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Kills",
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
