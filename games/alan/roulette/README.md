# Roulette

A shared-table American (double-zero) roulette wheel for the shellcade arcade.
Everyone at the table bets on **one** wheel: a timed window where each player
spreads chips across the full felt, then a single shared spin and a payout beat
before the next round opens.

## Playing

During the **betting window**:

| Key | Action |
|---|---|
| arrow keys | move the cursor over the felt — onto a number, the line between two numbers, or a four-number intersection |
| Enter / Space | drop a chip exactly where the cursor sits |
| `+` / `-` | change the chip denomination (10 / 25 / 50 / 100) |
| Backspace | undo your last chip |
| `c` | clear all your chips (refunded) |
| `r` | ready up — when every seated player is ready the wheel spins after a short beat |

The cursor lands on the real chip positions: a number's centre is a straight-up,
the line between two cells is a split, an intersection is a corner, and the outer
end of a column is a street or six-line. The armed-bet readout (e.g.
`> SPLIT 2-3   pays 17:1`) and the highlighted numbers always show exactly what
you'll stake. When the wheel spins, the screen switches to the wheel view; if
nobody readies up the window's countdown spins it on its own.

## The bets

American double-zero layout (38 pockets — 0, 00, 1–36 — 5.26% house edge):

| Bet | Numbers | Pays |
|---|---|---|
| Straight | 1 | 35:1 |
| Split | 2 | 17:1 |
| Street / Trio | 3 | 11:1 |
| Corner | 4 | 8:1 |
| Top line (0-00-1-2-3) | 5 | 6:1 |
| Six-line | 6 | 5:1 |
| Dozen / Column | 12 | 2:1 |
| Red, Black, Odd, Even, 1-18, 19-36 | 18 | 1:1 |

The two green zeros sit in the left margin (a vertical lane: 0, the 0-00 split,
00); the American-only bets are straight 00, the 0-00 split, the 00-2-3 trio,
and the five-number top line. A winning bet returns your stake plus the listed
payout; either green pocket simply loses every outside bet (no en-prison /
la-partage).

Your bankroll is durable across sessions and feeds the **Chips** leaderboard
(your all-time peak). Bust out and you're staked a fresh 1,000.

## Running it

```sh
go run .                # play in your terminal (solo)
go run . -seats 2       # shared table — Ctrl-T switches the active seat
go run . -seed 7        # reproduce a specific wheel
go test ./...           # unit tests

# render the smoke preview screens
go run . -smoke smoke.yaml -smoke-out shots/
```

The dev loop is pure Go; the published `.wasm` is built by CI with TinyGo.
