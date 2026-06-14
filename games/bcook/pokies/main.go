// Pokies — a no-winner social slot machine for the shellcade arcade. Each
// player gets their own 3-reel cabinet; cabinets render side-by-side on the
// fixed 80x24 canvas. A wake-driven port of the native pokies game to the
// shellcade wasm ABI: the reel animation is a clock derived from CallContext
// time, reel landings are one-shot deadlines held in guest memory, odds are
// admin-tunable via config, and the durable wallet uses the casino kv pattern
// (balance summed, peak max-merged) with a peak-ranked leaderboard.
//
// Aussie machine features layer onto the single weighted virtual strip:
//   - WILD (👑) substitutes on the payline; an all-wild line pays the top prize.
//   - SCATTER (🎁) counts anywhere in the 3x3 window; 3+ award free spins that
//     auto-play at no cost and retrigger.
//   - After a base-game win the player gambles it on a Red/Black (x2) or Suit
//     (x4) double-up ladder, or takes the win.
//
// RTP stays exact: stats() enumerates strip³ and folds free spins in via a
// closed form; compileVariant gates total RTP and retrigger convergence. The
// gamble is a fair deal (RTP-neutral).
//
// Build (dev profile):
//
//	tinygo build -o game.wasm -opt=1 -no-debug -gc=leaking \
//	  -target wasip1 -buildmode=c-shared .
//
// Native dev loop:
//
//	go run .
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }
