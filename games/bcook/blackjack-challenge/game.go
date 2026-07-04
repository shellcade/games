package main

import kit "github.com/shellcade/kit/v2"

// Game is the blackjack-challenge registry entry: static metadata plus the
// per-room factory. The catalog slug is composed by the platform from the
// directory path (games/bcook/blackjack-challenge -> "bcook/blackjack-challenge");
// Meta carries the bare name.
type Game struct{}

// Meta returns the static game metadata for this Challenge-rules table.
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "blackjack-challenge",
		Name:             "Blackjack Challenge",
		ShortDescription: "Face-up dealer, ties lose - but your blackjack never does, paying 2:1 up to 5:1.",
		MinPlayers:       1,
		MaxPlayers:       5,
		Tags:             []string{"cards", "casino"},

		// A casual social room: when everyone leaves, the room closes —
		// no hibernation snapshot, no Resume-menu entry (kit v2.7.0).
		Lifecycle: kit.LifecycleEphemeral,

		// A casino-kind game (kit v2.16.0): players gamble their account-wide
		// platform Credits through the room's svc.Credits service — the host
		// owns every balance. MaxPayoutMultiplier is the settlement ceiling:
		// the top single-stake outcome is a Star Pairs pair of aces at 30:1,
		// which returns stake×(30+1) = 31× on that side stake; the mandatory
		// main bet only dilutes the per-seat aggregate, so 31 covers the
		// largest honest payout. CtxFeatCredits is declared alongside so a
		// credits-capable front end negotiates the encoding.
		Kind:                kit.GameKindCasino,
		MaxPayoutMultiplier: 31,

		// Per-member arcade characters (kit v2.9.0): every roster member
		// arrives with Player.Character populated, rendered as a one-cell
		// tile right before the player's name (seat rail + turn waits).
		CtxFeatures: kit.CtxFeatCharacter | kit.CtxFeatCredits,

		QuickModeLabel:    "Join a table",
		SoloModeLabel:     "Heads-up vs dealer",
		PrivateInviteLine: "Friends take a seat when they enter the code.",

		// The board is a peak account-wide-credits metric: after each settle the
		// seat's fresh platform balance (svc.Credits.Balance) is compared to its
		// high-water mark and Posted on a new personal best (see room.go
		// postPeak). BestResult keeps each account's highest observed balance.
		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Credits",
			Direction:   kit.HigherBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Integer,
		},

		// Touch deck chips (kit v2.10.0): every input beyond the canonical
		// vocabulary (arrows/Confirm/Back, which the deck always provides) needs
		// a chip so it is reachable on touch. The turn actions and the betting
		// side-bet keys are all letter commands. Betting itself drives stake
		// (Up/Down), the backed seat (Left/Right), and place (Confirm) off the
		// canonical arrows; only P/B need declaring.
		Controls: []kit.ControlDecl{
			kit.RuneControl('h', "HIT"),
			kit.RuneControl('s', "STAND"),
			kit.RuneControl('d', "DOUBLE"),
			// P splits a pair on a turn AND loops the Star Pairs side bet during
			// betting — one rune, so the chip carries both meanings.
			kit.RuneControl('p', "SPLIT/PAIRS"),
			// Betting only: B loops the behind bet on the focused seat.
			kit.RuneControl('b', "BEHIND"),
		},
	}
}

// NewRoom returns the per-room behavior.
func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return newRoom(cfg, svc)
}
