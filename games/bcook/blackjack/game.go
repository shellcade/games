package main

import kit "github.com/shellcade/kit/v2"

// Game is the blackjack registry entry: static metadata plus the per-room
// factory. The catalog slug is composed by the platform from the directory path
// (games/bcook/blackjack -> "bcook/blackjack"); Meta carries the bare name.
type Game struct{}

// Meta returns the static game metadata (mirrors the native blackjack meta).
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "blackjack",
		Name:             "Blackjack",
		ShortDescription: "Take a seat at a shared dealer table: bet, hit, stand, and chase your high score.",
		MinPlayers:       1,
		MaxPlayers:       5,
		Tags:             []string{"cards", "casino"},

		// A casual social room: when everyone leaves, the room closes —
		// no hibernation snapshot, no Resume-menu entry (kit v2.7.0).
		Lifecycle: kit.LifecycleEphemeral,

		// Per-member arcade characters (kit v2.9.0): every roster member
		// arrives with Player.Character populated, rendered as a one-cell
		// tile right before the player's name (seat rail + turn waits).
		CtxFeatures: kit.CtxFeatCharacter,

		QuickModeLabel:    "Join a table",
		SoloModeLabel:     "Heads-up vs dealer",
		PrivateInviteLine: "Friends take a seat when they enter the code.",

		// The native game is no-winner (it never End/Posts) and surfaced its
		// board via a custom KV-backed peak provider. The lean ABI has no custom
		// provider, so the board is declared here and fed with Post on a new
		// personal peak — the same high-water-mark metric (Chips), keyed off the
		// durable wallet's `peak` (see room.go persistWallet / postPeak).
		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Chips",
			Direction:   kit.HigherBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Integer,
		},

		// Touch deck chips (kit v2.10.0): every input beyond the canonical
		// vocabulary (arrows/Confirm/Back, which the deck always provides) needs
		// a chip so it is reachable on touch. The turn actions, the insurance
		// answers, and the betting side-bet keys are all letter commands. Betting
		// itself drives stake (Up/Down), the backed seat (Left/Right), and place
		// (Confirm) off the canonical arrows; only P/B need declaring.
		Controls: []kit.ControlDecl{
			kit.RuneControl('h', "HIT"),
			kit.RuneControl('s', "STAND"),
			kit.RuneControl('d', "DOUBLE"),
			// P splits a pair on a turn AND loops the Perfect Pairs side bet
			// during betting — one rune, so the chip carries both meanings.
			kit.RuneControl('p', "SPLIT/PAIRS"),
			kit.RuneControl('r', "SURRENDER"),
			kit.RuneControl('y', "YES"),
			kit.RuneControl('n', "NO"),
			// Betting only: B loops the behind bet on the focused seat (Left/Right
			// pick the seat); P (above) loops that seat's Perfect Pairs.
			kit.RuneControl('b', "BEHIND"),
		},
	}
}

// NewRoom returns the per-room behavior.
func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return newRoom(cfg, svc)
}
