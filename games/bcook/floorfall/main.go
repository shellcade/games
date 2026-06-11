// Floorfall — a last-one-standing crumbling-floors party game for shellcade.
//
// Hex-A-Gone-style survival on the fixed 80x24 terminal canvas. The arena is
// three stacked floor layers; you only see the one you stand on. Tiles degrade
// a beat AFTER you step off them (█ → ▓ → ░ → gone), and stepping onto a hole
// drops you to the same spot on the layer below. Fall through the bottom layer
// and you are out. Last one standing wins the round; then a fresh arena.
//
// Solo mode is first-class: an ambient "crumble wave" decays random tiles on
// its own and accelerates over time, so a solo run is survive-as-long-as-you-
// can — your best survival time rides the leaderboard. With 2+ players the wave
// is gentle and footsteps do the damage. No combat — all positioning.
//
// Dev loop:  go run .            (plays in this terminal; -seats N for hot-seat)
//
//	Artifact:  tinygo build -opt=1 -no-debug -gc=leaking \
//	               -o floorfall.wasm -target wasip1 -buildmode=c-shared .
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }

// Game is the module registry entry: static metadata plus the room factory.
type Game struct{}

// Meta returns the static game metadata shown in the arcade. The bare slug
// "floorfall" is namespaced to "bcook/floorfall" by the platform from the
// catalog path; this Meta carries the bare name only.
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "floorfall",
		Name:             "Floorfall",
		ShortDescription: "Outlast rivals on crumbling stacked floors — step off a tile and it falls away.",
		MinPlayers:       1,
		MaxPlayers:       6,
		Tags:             []string{"action", "arcade", "party", "last-one-standing"},

		// Player characters: each contestant's tile renders beside their name on
		// the scoreboard, visible to every player.
		CtxFeatures: kit.CtxFeatCharacter,

		QuickModeLabel:    "Quick scramble",
		SoloModeLabel:     "Outrun the crumble",
		PrivateInviteLine: "Friends join your arena when they enter the code.",

		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Survival",
			Direction:   kit.HigherBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Duration,
		},
	}
}

// NewRoom returns the per-room behavior.
func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return newRoom(cfg, svc)
}
