// Stacked — a competitive falling-blocks battler for shellcade.
//
// Steer falling pieces into your 10-wide well, clear rows for points, and weld
// rivals shut. Clearing two or more rows at once fires garbage rows at whoever
// is currently stacked the tallest — so the leader always has a target on their
// back. Top out and you're eliminated; the last well standing wins the match.
//
// Solo is first-class score attack: garbage creeps in on a timer that keeps
// accelerating while gravity speeds up by level, and the run ends when you top
// out. Your best score rides the leaderboard.
//
// The piece set is original — a custom mix of four-, three-, and five-cell
// shapes with their own names (no guideline trade dress).
//
// Dev loop:  go run .            (plays in this terminal; -seats N for hot-seat)
//
//	Artifact:  tinygo build -opt=1 -no-debug -gc=leaking \
//	               -o stacked.wasm -target wasip1 -buildmode=c-shared .
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }

// Game is the module registry entry: static metadata plus the room factory.
type Game struct{}

// Meta returns the static game metadata shown in the arcade. The bare slug
// "stacked" is namespaced to "bcook/stacked" by the platform from the catalog
// path; this Meta carries the bare name only.
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "stacked",
		Name:             "Stacked",
		ShortDescription: "Falling-blocks battle: clear rows, dump garbage on the leader, last well standing wins.",
		MinPlayers:       1,
		MaxPlayers:       6,
		Tags:             []string{"puzzle", "battle", "arcade"},

		QuickModeLabel:    "Quick battle",
		SoloModeLabel:     "Score attack",
		PrivateInviteLine: "Friends drop into your match when they enter the code.",

		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Score",
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
