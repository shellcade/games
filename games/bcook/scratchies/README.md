# Scratchies

Duck into the corner newsagent, browse the ticket stands, buy an instant
scratch-it, and peel the latex off panel by panel. **16 tickets** across four
price tiers ($1 / $2 / $5 / $10) ride on **four scratch-card games**; every card
is already a winner or loser the moment you buy it — scratching is honest reveal
theatre, just like the real thing.

A casino-style game for the [shellcade](https://shellcade.com) arcade. 1–6
players share one newsagent counter; each browses and scratches their own cards,
and big wins flash on a shared ticker.

## How to play

1. **Counter** — `◂ ▸` pick one of the four price stands, **ENTER** to step up.
2. **Stand** — `▲ ▼` choose a ticket, **ENTER** to buy it (credits deducted).
3. **Scratch** — move the coin with the arrows, **SPACE** to rub the focused
   panel (each panel takes a hidden 1–3 rubs), or **`a`** to scratch the whole
   card at once. Big cards scroll as the coin reaches the edge.
4. **Collect** — a win credits your wallet; **ENTER** buys another, **`q`** heads
   back. Run out of credits and the newsagent spots you a fresh 1,000.

## The four games

| Game | How you win |
|---|---|
| **Match-3 cash** | Uncover three equal cash amounts to win that amount |
| **Key-number match** | Match any of your numbers to the winning numbers up top; matches sum |
| **Multiplier** | Reveal a prize, then a multiplier (up to 2×–20×) that boosts it |
| **Find-the-symbol** | Find three target emoji (🍒 🐊 💎 💰) — but mind the `BUST` |

## The catalog

| $1 | $2 | $5 | $10 |
|---|---|---|---|
| Lucky 7s · match-3 | Gold Rush · match-3 | Diamond Mine · match-3 | Platinum Sevens · match-3 |
| Coin Toss · key-num | Lucky Numbers · key-num | Lotto Lanes · key-num | Fortune 50 · key-num |
| Cherry Pop · 🍒 find | Croc Cash · 🐊 find +BUST | Treasure Hunt · 💎 find | Outback Riches · 💰 find +BUST |
| Tinnie Tripler · ×3 | Double Trouble · ×5 | Mega Multiplier · ×10 | Cash Explosion · ×20 |

Higher tiers add more panels (and scroll), bigger jackpots ($1 → 10,000 credits
up to $10 → 250,000), and slightly friendlier odds.

## Odds & credits

Odds sit in real-scratchie territory — **any-win ≈ 1 in 3.3–3.9**, **RTP ≈
58–68%** rising with price. Each ticket carries a prize table; `stats()` computes
its exact theoretical RTP and hit-rate, asserted in tests so a mistuned table
can't ship.

Your **wallet** is the shared casino pattern: a durable balance starting at
1,000 credits (ticket price = dollars 1:1), reset to 1,000 if you ever bust. The
**Credits** leaderboard ranks your all-time peak.

Representative table — **Lucky 7s** ($1, match-3), prize in credits · per-card odds:

| Prize | 1 in | | Prize | 1 in |
|---|---|---|---|---|
| 1 | 7 | | 50 | 2,500 |
| 2 | 10 | | 100 | 12,000 |
| 5 | 28 | | 1,000 | 280,000 |
| 10 | 80 | | 10,000 | 1,400,000 |
| 20 | 500 | | *(else)* | no win |

## Controls

| Input | Action |
|---|---|
| arrows / `hjkl` | move the cursor (stand · ticket · coin) — scrolls big cards |
| ENTER | step into a stand · buy · buy again |
| SPACE | rub the focused panel |
| `a` | scratch all → resolve |
| `q` / Esc | back one level |

## Design & development

- Design spec and screen mockups: [SPEC.md](SPEC.md), [ARTBOARDS.md](ARTBOARDS.md).
- Built on `github.com/shellcade/kit/v2`. Native dev loop: `go run .`
  (`-seats 3` for hot-seat, `-smoke smoke.yaml -smoke-out shots/` for shots).
- Tests: `go test ./...` — engine win/loss invariants, odds bands, and an
  end-to-end pass over all 16 tickets.

MIT licensed — see [LICENSE](LICENSE).
