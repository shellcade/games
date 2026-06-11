// Meltdown — a co-op reactor-room panic for shellcade.
//
// The whole 80x24 terminal is a failing reactor ship: rooms and corridors
// drawn in box characters around a glowing core. One crew member per player
// runs the halls (arrows / hjkl) putting out faults that erupt at the
// stations and worsen if neglected — LEAKs to patch, FIREs to smother,
// JAMMED VALVEs to key open, and (with a full crew) BREACHes that take two
// bodies at once. Every unfixed fault eats the core's shared integrity, and
// faults spawn faster and faster, so the run ALWAYS ends. Your score is how
// long the crew survives.
//
// Solo is first-class: the same loop, faults scaled down for one engineer and
// no two-person faults. More crew raises the spawn rate only sub-linearly, so
// a bigger crew is an easier shift — bring friends.
//
// Dev loop:  go run .            (plays in this terminal; -seats N for hot-seat)
//
//	Artifact:  tinygo build -opt=1 -no-debug -gc=leaking \
//	               -o meltdown.wasm -target wasip1 -buildmode=c-shared .
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }

// Game is the module registry entry: static metadata plus the room factory.
type Game struct{}

// Meta returns the static game metadata shown in the arcade. The bare slug
// "meltdown" is namespaced to "bcook/meltdown" by the platform from the
// catalog path; this Meta carries the bare name only.
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "meltdown",
		Name:             "Meltdown",
		ShortDescription: "Keep a failing reactor alive: patch leaks, smother fires, and crack jammed valves before the core dies.",
		MinPlayers:       1,
		MaxPlayers:       6,
		Tags:             []string{"co-op", "action", "party", "survival"},

		// Player characters: each crew member's tile renders beside their name
		// on the roster and as their body in the reactor, visible to everyone.
		CtxFeatures: kit.CtxFeatCharacter,

		QuickModeLabel:    "Crew shift",
		SoloModeLabel:     "Lone engineer",
		PrivateInviteLine: "Friends join your reactor crew when they enter the code.",

		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Survival",
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
