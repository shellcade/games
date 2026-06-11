package main

import kit "github.com/shellcade/kit/v2"

// Game is the chess registry entry: static metadata plus the per-room factory.
type Game struct{}

// Meta returns the static game metadata. The bare slug "chess" is namespaced to
// "alan/chess" by the platform from the catalog path; this Meta carries the bare
// name only.
//
// Strictly two-player: unlike the native game (which also offered a 1-player
// untimed hot-seat), the catalog entry declares MinPlayers 2 so the matchmaker
// always pairs a real opponent and the blitz clock always applies.
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "chess",
		Name:             "Chess",
		ShortDescription: "A two-player chess duel — pair up, beat the clock, mate the king.",
		MinPlayers:       2,
		MaxPlayers:       2,
		Tags:             []string{"chess", "strategy", "board"},

		// Player characters: each player's tile renders beside their name on
		// the side-panel player lines.
		CtxFeatures: kit.CtxFeatCharacter,

		// Chess-appropriate lobby mode labels — the generic defaults don't fit a
		// turn-based duel.
		QuickModeLabel:    "Quick match",
		SoloModeLabel:     "Solo: play both sides",
		PrivateInviteLine: "Your opponent takes the other colour when they enter the code.",

		// Touch deck chips (kit v2.10.0): the inputs beyond the canonical
		// vocabulary — letter commands and the Backspace selection-cancel —
		// surfaced as tappable chips on devices without a physical keyboard.
		// Movement and select stay on the canonical arrows + Confirm.
		Controls: []kit.ControlDecl{
			kit.RuneControl('r', "RESIGN"),
			kit.RuneControl('d', "DRAW"),
			kit.RuneControl('y', "YES"),
			kit.RuneControl('n', "NO"),
			kit.KeyControl(kit.KeyBackspace, "CANCEL"),
		},
	}
}

// NewRoom returns the per-room behavior.
func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return newRoom(cfg, svc)
}
