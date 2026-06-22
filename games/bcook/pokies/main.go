// Pokies — a no-winner social slot machine for the shellcade arcade. Each
// player gets their own 3-reel cabinet; cabinets render side-by-side on the
// fixed 80x24 canvas. A wake-driven port of the native pokies game to the
// shellcade wasm ABI: the reel animation is a clock derived from CallContext
// time, reel landings are one-shot deadlines held in guest memory, odds are
// admin-tunable via config, and the durable wallet uses the casino kv pattern
// (balance summed, peak max-merged) with a peak-ranked leaderboard.
//
// The machine is a 5-reel, 243-ways pokie (a shared resident lounge floor; sit
// at one of six themed machines). Features layer onto a single weighted strip:
//   - 243 WAYS: a symbol pays its left-aligned run (adjacent reels from reel 0,
//     any rows), credited pays[len] × the product of per-reel counts.
//   - WILD (👑) substitutes for any paying symbol within a run.
//   - SCATTER (🎁) counts anywhere in the 5x3 window; 3+ award free spins that
//     auto-play at no cost and retrigger.
//   - After a base-game win the player gambles it on a Red/Black (x2) or Suit
//     (x4) double-up ladder, or takes the win.
//
// RTP stays exact in closed form: because the ways win is a SUM over symbols,
// each symbol's expected win depends only on its per-reel count marginal
// (E[win_s] = pay3·a³·z + pay4·a⁴·z + pay5·a⁵), so no strip⁵ enumeration is
// needed; a Monte-Carlo test cross-checks it. compileVariant gates total RTP and
// retrigger convergence. The gamble is a fair deal (RTP-neutral).
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
