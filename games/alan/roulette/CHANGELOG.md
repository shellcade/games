# Changelog

## Player characters

Each player's character now renders beside their name in the seat strip under
the table, next to their chip-colour swatch.

## Felt table makeover

Reworked the table so it reads like a real felt, and made room for a full house.

- The betting layout now fills the width as one green felt surface inside a
  wooden rail — white grid lines, red/black number boxes, green zeros — with the
  dozen and even-money rows tucked up snug and a row of felt breathing above and
  below.
- Players are seated in narrow stacked columns (name / chips / status) beneath
  the table, spread evenly so a full six-player table still breathes; the recent
  winning numbers moved up next to the title.
- Added a **5** chip so a player ground down to a few dollars can still place a
  bet (and bust into the re-buy) rather than getting stuck unable to afford one —
  the default chip stays 10.

## American double-zero wheel

Switched the table from the single-zero European wheel to the American
double-zero wheel.

- Added the **00** pocket (38 pockets now; house edge 5.26%) and the
  American-only zero bets: straight 00, the 0-00 split, the 00-2-3 trio, and the
  five-number **top line** (0-00-1-2-3, 6:1). Dropped the European-only 0-1/0-2/
  0-3 splits, the 0-2-3 trio, and the four-number basket.
- The two green zeros are boxed in the left margin as a vertical lane — 0, the
  0-00 split line, 00 — so up/down steps cleanly between them.
- The wheel now spins in a panel below the board (the felt and everyone's
  locked-in chips stay visible) instead of taking over the screen; once the
  ball lands, the winning number and every outside bet it pays flash, then hold
  solid through the payout — chips always drawn on top.
- The round reveal leads with your net result ("up 200 (bet 400, back 600)"),
  and vertical navigation steps through the dozen and even-money rows instead
  of skipping them.

## Initial release

A shared-table European roulette wheel — one wheel, everyone bets together.

- A timed betting window, a single shared spin, then payouts, on repeat. When
  every seated player readies up the wheel spins early after a short grace beat.
- The full single-zero felt: straight, split, street, corner, six-line, the
  zero trios and basket, plus the dozen, column, and even-money outside bets,
  with standard payouts (35:1 down to 1:1).
- Cursor betting on a bordered felt: the cursor lands on numbers, the lines
  between them (splits), and the intersections (corners/lines), with live
  coverage highlighting and chips drawn right where you place them.
- A dedicated spinning screen — a big flashing pocket readout over a
  decelerating wheel track — and a table roster showing each player's stake and
  round result.
- Durable bankroll (start 1,000, re-buy on bust) feeding a peak-chips
  leaderboard. Up to six players per table; plays fine solo.
