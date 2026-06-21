// Paperdrift — a real-time multiplayer paper-glider run for the terminal,
// built on shellcade/kit. Trim your glider's nose with the arrow keys,
// dive to build speed, climb to bank height, thread the wall gaps, and ride
// thermals to stay aloft. Everyone launches together over the same sky and
// the furthest flight without crashing wins the round.
//
// Run it right now: go run .  (add -seats 2 to fly against yourself).
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }

// Game is the registry entry: static metadata plus the per-room factory.
type Game struct{}

func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "paperdrift",
		Name:             "Paperdrift",
		ShortDescription: "Trim a paper glider through gaps and thermals. The furthest flight wins.",
		MinPlayers:       1,
		MaxPlayers:       6,
		HeartbeatMS:      50, // real-time flight: 20 wakes/sec
		Tags:             []string{"arcade", "glider", "race"},

		QuickModeLabel:    "Quick flight",
		SoloModeLabel:     "Solo glide",
		PrivateInviteLine: "Gliders launch together - the furthest flight without crashing wins.",

		// A run without its pilots is meaningless; dispose abandoned rooms.
		Lifecycle: kit.LifecycleEphemeral,

		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Distance",
			Direction:   kit.HigherBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Integer,
		},
	}
}

func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler { return newRoom(cfg, svc) }
