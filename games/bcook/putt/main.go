// Putt — an SSH arcade mini-golf game for shellcade.
//
// ASCII mini-golf on the fixed 80x24 terminal canvas. Nine handcrafted holes
// of increasing mischief: rotate an aim indicator with the arrows, hold space
// to charge a power meter, release to putt. The ball rolls with friction,
// bounces off the course walls (`/ \ | - =` and box characters), bogs down in
// sand (`:`), and a splash in the water (`~`) costs a stroke and resets you to
// where you putted from. Later holes add a spinning windmill arm to time.
//
// Everyone plays the SAME hole at once — no turn waiting. Your ball is bright
// reverse-video; rivals are dim ghost glyphs you pass right through. When every
// player holes out (or hits the par+4 stroke cap) a scorecard intermission
// shows, then the next hole. Solo is first-class: same course, no ghosts, pure
// stroke-count attack — your 9-hole total rides the leaderboard.
//
// Dev loop:  go run .            (plays in this terminal; -seats N for hot-seat)
//
//	Artifact:  tinygo build -opt=1 -no-debug -gc=leaking \
//	               -o putt.wasm -target wasip1 -buildmode=c-shared .
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }

// Game is the module registry entry: static metadata plus the room factory.
type Game struct{}

// Meta returns the static game metadata shown in the arcade. The bare slug
// "putt" is namespaced to "bcook/putt" by the platform from the catalog path;
// this Meta carries the bare name only.
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "putt",
		Name:             "Putt",
		ShortDescription: "ASCII mini-golf — 9 holes, everyone plays at once, lowest total wins.",
		MinPlayers:       1,
		MaxPlayers:       6,
		Tags:             []string{"sports", "golf", "casual", "party"},

		// Each golfer's identity on the course is their distinct color (ball +
		// scorecard row). The scorecard already places each player's character
		// tile via kit.CharacterCell when one is present, so opting into
		// CtxFeatCharacter is a one-line addition here when desired; it is left
		// off so the game is self-contained color-coded identity.

		QuickModeLabel:    "Quick 9",
		SoloModeLabel:     "Round of 9",
		PrivateInviteLine: "Friends tee off on your course when they enter the code.",

		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Strokes",
			Direction:   kit.LowerBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Integer,
		},
	}
}

// NewRoom returns the per-room behavior.
func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return newRoom(cfg, svc)
}
