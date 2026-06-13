# Scratchies — design spec

A single-player-at-heart casino game wrapped in a social newsagent: wander into
a corner store, browse the ticket stands, buy an instant scratch-it, and peel
the latex off panel by panel until you know whether it paid. Original name, art,
and ticket themes — no real lottery brand, name, or asset is reused anywhere
(the *spaceterm* precedent for homage games).

- **Slug:** `scratchies` · **Namespace:** `games/bcook/scratchies/`
- **Players:** 1–6, shared shop floor · **Lifecycle:** ephemeral · **Language:** Go (TinyGo / wasip1)
- **Kit:** shellcade-kit v2.10.0+ (per-player frames, durable KV wallet, touch-deck control chips, lobby/floor pattern)
- Screens referenced as **AB-1 … AB-11** are in [ARTBOARDS.md](ARTBOARDS.md).
- Sibling casino games this borrows conventions from: `pokies` (durable wallet,
  bust→rebuy, "Credits" leaderboard), `blackjack`, `roulette`.

---

## 1. Design pillars

1. **The scratch is the moment.** Pressure and payoff both come from the slow
   reveal — moving the coin, peeling one panel, seeing the third 7 land. Every
   ticket is *already* a winner or loser the instant you buy it (real scratchies
   are pre-printed); scratching is honest reveal theatre, never a skill check.
2. **A shop, not a menu.** You don't pick a game from a list — you walk up to a
   counter, look over the stands, and choose a ticket. The newsagent is the
   lobby (AB-1, AB-2).
3. **Many tickets, four engines.** Variety comes from *data* — 16 themed tickets
   over four price tiers — riding on four reusable mechanic engines. Adding a
   ticket is a data record, not new code.
4. **Honest odds.** Win frequency and return-to-player sit in real-scratchie
   territory (~1 in 4 any win; ~60–68% RTP). The house edge is real; the bust
   safety-net (rebuy to 1000) keeps it a toy, not a trap.
5. **Social, not competitive.** The floor is shared for the *vibe* — patrons at
   the counter, a big-win ticker — but every card is the buyer's own. No
   player's scratch affects another's.

## 2. Glossary

| Term | Meaning |
|---|---|
| **Counter** | The zoomed-out newsagent view: four ticket stands + flavour (AB-1). The floor's shared space. |
| **Stand** | One price tier's rack ($1 / $2 / $5 / $10). Zoom in to browse its tickets (AB-2). |
| **Ticket** | One buyable scratch-it: a data record of theme + price + mechanic + palette + prize table. 16 ship in v1 (§6). |
| **Panel** | One scratch-off cell on a card. Hidden (`▓`) until scratched, then shows its symbol/number/amount. |
| **Mechanic** | One of four reveal+resolve engines (§5) a ticket is built on. |
| **Outcome** | The card's predetermined result (a prize in credits, or no win), drawn at purchase before any panel is scratched (§7). |
| **Wallet** | The player's durable credit balance in KV. Starts 1000, resets to 1000 on bust (§8). |
| **Big win** | A win at/above the room-wide announce threshold; flashes on every patron's ticker (§9, AB-11). |

## 3. Game flow

```
COUNTER ──pick a stand──▶ STAND (browse) ──buy──▶ CARD (scratch) ──▶ RESULT
   ▲  ▲                      │   │                                     │
   │  └──────[q] back────────┘   └──[q] back to counter                │
   │                                                                   │
   └────────────[ENTER] buy again / [q] back to counter◀───────────────┘

  out of credits at a purchase ──▶ BUST → wallet rebuys to 1000 (AB-10)
```

- **COUNTER (AB-1):** zoomed-out store. Cursor (`◂ ▸`) moves between the four
  stands; **ENTER** zooms into the highlighted stand. Wallet shown top-right;
  big-win ticker along the bottom.
- **STAND (AB-2):** the tickets at that price, each with name, top prize, and
  printed odds. Cursor selects a ticket; **ENTER** buys it (credits deducted
  immediately, like buying it off the rack). **q** returns to the counter.
- **CARD (AB-3…AB-8):** the bought ticket, all panels hidden. Scratch per §4.
- **RESULT (AB-5, AB-9):** once enough panels are revealed to settle the
  outcome (or all are scratched), the card resolves: a win flashes and credits
  the wallet; a loss shows "no win this time." **ENTER** buys the same ticket
  again (if affordable); **q** returns to the stand/counter.

## 4. Scratching — the core interaction

A card is a grid of panels rendered as latex (`▓▓`) until revealed. The player
holds a virtual coin:

- **Arrows / `hjkl`** move the coin-cursor over the grid; the focused panel is
  ringed.
- **SPACE** rubs the focused panel once. Each panel is seeded to need **1–3
  rubs** to come off, but the latex always shows **fully opaque** (`▓▓`) until
  the final rub pops it open — you can't tell a 1-rub panel from a stubborn
  3-rub one by looking, so every panel is its own small suspense. The revealing
  rub flashes the symbol / number / amount. Rub depth is cosmetic — independent
  of whether the panel wins (§7).
- **`a`** = **SCRATCH ALL**: wears every remaining panel fully through at once
  (the real-card "scratch all" affordance) and goes straight to resolution.
  Always available.

Because the outcome is predetermined (§7), scratch *order* is cosmetic — you
can hunt panel-by-panel for the tension, or mash `a` to skip to the verdict. A
card auto-resolves the moment the revealed panels make the result certain (e.g.
the third matching 7 appears), even with panels still latexed; the rest reveal
on resolution. Touch decks scratch by tapping a panel directly (§10).

### 4.1 Scrolling big cards (AB-12)

Higher-tier cards have more panels than fit the visible area — grids stay ≤ 6
columns (always inside 80 cols) but grow in *rows*, so the $5 and $10 tiers
render in a **vertical scrolling viewport** (~4 cell-rows tall). The viewport
follows the coin:

- Moving the coin past the top/bottom visible edge **scrolls** the grid to keep
  it in view. `[` / `]` (or PgUp/PgDn) page a viewport at a time.
- A **scroll rail** on the right edge (`▲ … ▼` with a position thumb) shows how
  much card lies off-screen and where you are in it.
- The **result banner pins below the rule**, so the verdict and running prompt
  ("two $5 so far…") are always visible regardless of scroll position.
- **SCRATCH-ALL** reveals the whole grid and snaps the viewport to the
  resolving panels (the winning line, or a `BUST`), banner pinned.

$1/$2 cards fit without scrolling; the rail and paging simply don't appear.

## 5. The four mechanics

Each mechanic is one engine: given a drawn **outcome** (§7) and the room RNG, it
(a) lays panels out to *display* that outcome consistently, and (b) resolves the
card. Each ticket names which mechanic it uses plus its theme + prize table.

### 5.1 Match-3 cash (AB-3…AB-5, AB-12)
A grid of **cash amounts** — 3×3 ($1) · 4×4 ($2) · 5×5 ($5) · 6×6 ($10), the
$5/$10 boards scrolling (§4.1). Uncover **three equal amounts** → win that
amount. A losing card lays out so no amount appears three times; a winning card
seeds exactly one amount three times (the won prize) plus non-tripling decoys.
Bigger grids mean more decoys and a longer hunt, not better three-of-a-kind
odds (those stay the ticket's printed rate). The simple classic.

### 5.2 Key number match (AB-6)
A row of 2–6 **winning numbers** at the top (pre-revealed or scratched first),
then a grid of **your numbers** — 6 ($1) · 9 ($2) · 16 ($5) · 24 ($10), the
$5/$10 grids scrolling (§4.1) — each paired with a prize. Any *your number* that
matches a winning number wins its paired prize; multiple matches sum. Losers lay
out with no overlap; winners plant the matching number(s) on the prize(s) equal
to the outcome. More numbers at higher tiers = more chances per card.

### 5.3 Multiplier (AB-7)
Two zones: a **prize** panel (a base amount) and a **multiplier** panel. Final
win = base × multiplier — the multiplier reveal is the beat you scratch last.
Higher tiers raise both the prize range and the **top multiplier**: 3×
(Tinnie Tripler $1) · 5× (Double Trouble $2) · 10× (Mega Multiplier $5) · 20×
(Cash Explosion $10). The engine factors the card's drawn outcome into a
base×mult pair that's in-range for the ticket, so the product always lands clean.

### 5.4 Find-the-symbol (AB-8)
Press-your-luck on grids of 9 ($1) · 12 ($2) · 20 ($5) · 30 ($10) panels (the
$5/$10 grids scroll, §4.1). Panels hide theme symbols, rendered as width-2
**emoji** (`🍒` Cherry Pop · `🐊` Croc Cash · `💎` Treasure Hunt · `💰` Outback
Riches; decoys `🐟 ⭐ 🔔 🦀 🍀 🐸`); **find 3 of the target symbol** to win the prize printed alongside. Some tickets ($2 Croc Cash, $10
Outback Riches)
salt in a **BUST** symbol — revealing it ends the card immediately with no win,
so SCRATCH-ALL is a gamble. Loser layouts never reach 3 targets; winners place
exactly 3 (and keep BUST off the natural path until resolution).

## 6. Ticket catalog (v1 — 16 tickets)

Themes are original and silly-not-crude (repo content policy). Top prizes are in
credits (= dollars 1:1, §8). Grid/chances grow with price.

| Price | Ticket | Mechanic | Grid / chances | Top prize | Any-win odds (target) |
|---|---|---|---|---|---|
| $1 | **Lucky 7s** | Match-3 | 3×3 | 10,000 | ~1 in 3.9 |
| $1 | **Coin Toss** | Key number | 2 winning · 6 yours | 10,000 | ~1 in 3.9 |
| $1 | **Cherry Pop** | Find-symbol | 9 panels, find 3 cherries | 10,000 | ~1 in 3.9 |
| $1 | **Tinnie Tripler** | Multiplier | 1 prize × to 3× | 12,000 | ~1 in 3.9 |
| $2 | **Gold Rush** | Match-3 | 4×4 | 25,000 | ~1 in 3.7 |
| $2 | **Lucky Numbers** | Key number | 3 winning · 9 yours | 25,000 | ~1 in 3.7 |
| $2 | **Croc Cash** | Find-symbol +BUST | 12 panels, find 3 crocs | 25,000 | ~1 in 3.7 |
| $2 | **Double Trouble** | Multiplier | 1 prize × to 5× | 30,000 | ~1 in 3.7 |
| $5 | **Diamond Mine** | Match-3 *(scrolls)* | 5×5 | 100,000 | ~1 in 3.5 |
| $5 | **Lotto Lanes** | Key number *(scrolls)* | 4 winning · 16 yours | 100,000 | ~1 in 3.5 |
| $5 | **Treasure Hunt** | Find-symbol *(scrolls)* | 20 panels, find 3 gems | 100,000 | ~1 in 3.5 |
| $5 | **Mega Multiplier** | Multiplier | 1 prize × to 10× | 120,000 | ~1 in 3.5 |
| $10 | **Platinum Sevens** | Match-3 *(scrolls)* | 6×6 | 250,000 | ~1 in 3.3 |
| $10 | **Fortune 50** | Key number *(scrolls)* | 6 winning · 24 yours | 250,000 | ~1 in 3.3 |
| $10 | **Outback Riches** | Find-symbol +BUST *(scrolls)* | 30 panels, find 3 picks | 250,000 | ~1 in 3.3 |
| $10 | **Cash Explosion** | Multiplier | 1 prize × to 20× | 300,000 | ~1 in 3.3 |

That's 16 tickets — four mechanics × four tiers, each its own theme/palette. The
$5/$10 grid cards scroll (§4.1); the multiplier cards stay a compact two-panel
reveal at every tier. The catalog is a Go table of `Ticket` records; new tickets
are new rows.

## 7. Odds, RTP & the prize model

Each ticket carries a **prize table**: a list of `(prize_credits, probability)`
rows; the remaining probability is "no win". At **purchase** the engine draws
one outcome from this table with the room RNG (§13), *then* lays the card out to
display it. This makes cards predetermined, fair regardless of scratch order,
and reproducible from the seed.

A `stats()` enumerator (mirroring `pokies`' approach) sums `Σ prize·p` for the
exact theoretical **RTP** and `Σ p` for the **any-win frequency**, asserted by
unit tests to sit in the intended band per tier — so a typo in a table can't
silently ship a 200% machine.

**Tier bands (targets, tuned in playtest):**

| Tier | Any-win odds | RTP band |
|---|---|---|
| $1 | ~1 in 3.9 | 58–62% |
| $2 | ~1 in 3.7 | 60–64% |
| $5 | ~1 in 3.5 | 63–66% |
| $10 | ~1 in 3.3 | 65–68% |

**Representative table — Lucky 7s ($1, match-3)** (prize in credits · per-card odds):

| Prize | 1 in | | Prize | 1 in |
|---|---|---|---|---|
| 1 | 6 | | 50 | 2,000 |
| 2 | 18 | | 100 | 10,000 |
| 5 | 40 | | 1,000 | 250,000 |
| 10 | 120 | | 10,000 | 1,250,000 |
| 20 | 400 | | *(else)* | no win |

Any-win ≈ 1 in 3.87; RTP ≈ 0.58. Most wins return ≤ the ticket price or a few ×
— the long tail to 10,000 is the dream. Other tickets reshape this ladder to
their tier band; full tables live in code (`tickets.go`), one representative per
mechanic reproduced in the README.

## 8. Credits & wallet (the casino pattern)

Lifted directly from `pokies` so a player's balance feels like one casino across
games:

- **Durable wallet in KV:** `balance` (merge rule **sum**, the carryable
  bankroll) and `peak` (merge **max**). A fresh/guest/non-positive account seeds
  to **1000**.
- **Price = credits 1:1.** A $5 ticket costs 5 credits, deducted at purchase.
  Prizes pay in credits at face value (a $1 ticket's 10,000 top prize = 10,000
  credits — the rare jackpot, ~1 in 1.25M).
- **Bust → rebuy:** if balance can't cover the cheapest stand you can still
  reach (i.e. hits 0 / can't afford a $1), the wallet **resets to 1000** with a
  beat (AB-10). No game-over; the newsagent always spots you another twenty.
- **Leaderboard:** `MetricLabel: "Credits"`, `HigherBetter`, `BestResult`,
  `Integer` — posts on each new personal **peak** (a big win), identical to
  pokies/blackjack so the casino set shares one ladder shape.

## 9. Multiplayer — the shared floor

Like `pokies`' floor. Several patrons occupy one newsagent room:

- **Private frames** (`r.Send(player, …)`): each player browses, buys, and
  scratches their **own** card; the screen below the chrome is theirs alone.
- **Shared chrome:** the counter (AB-1) shows other patrons as figures at the
  stands (character tiles via `CtxFeatCharacter`); a **big-win ticker** runs
  along the bottom of every frame.
- **Big win (AB-11):** a win ≥ the announce threshold (e.g. ≥ 500 credits *and*
  ≥ 50× the ticket price) flashes room-wide: "`◉ alan scored 5,000 on Gold
  Rush!`". Pure flavour — never reveals another player's balance or affects
  their cards.
- Solo play is the same game with an empty counter; the ticker only ever shows
  your own hits.

## 10. Input map & touch-deck chips

| Input | Context | Action |
|---|---|---|
| arrows / `hjkl` | counter / stand / card | move cursor (stand · ticket · coin) |
| ↓ / ↑ at viewport edge | scrolling card (§4.1) | coin holds the edge; grid scrolls one row to reveal more |
| `[` / `]` (PgUp/PgDn) | scrolling card | page the viewport up / down |
| ENTER | counter / stand / result | zoom stand · buy ticket · buy again |
| SPACE | card | scratch focused panel |
| `a` | card | scratch all → resolve (viewport snaps to the result) |
| `q` / Esc | any | back one level (card→stand→counter→leave) |

Counter/stand/result run in `CtxNav`; card scratching runs in `CtxCommand` so
`a` and SPACE stay the game's (and `q` is the canonical Back). Declared
`Controls` give touch users chips: directional + **SCRATCH**, **SCRATCH ALL**,
**BUY**, **BACK**. On touch decks a panel is also **tap-to-scratch** directly,
which is the natural mobile gesture; the coin-cursor is the keyboard equivalent.

## 11. Screen layouts

All on the fixed **80×24** kit frame; exact mockups in ARTBOARDS.md:

- **AB-1** Counter (zoomed out) · **AB-2** Stand — browse & buy · **AB-3** Fresh
  card (match-3) · **AB-4** Mid-scratch with coin cursor · **AB-5** Winner reveal
  + collect · **AB-6** Key-number-match card · **AB-7** Multiplier card · **AB-8**
  Find-the-symbol (with BUST) · **AB-9** No-win result · **AB-10** Bust → rebuy ·
  **AB-11** Shared big-win ticker · **AB-12** Big card — scrolling viewport.

Shared chrome on every frame: title/location strip + wallet (row 0–1), the
working area (rows 3–21), big-win ticker + hint line (rows 22–23).

## 12. kit Meta (as implemented in game.go)

```go
func (Game) Meta() kit.GameMeta {
    return kit.GameMeta{
        Slug:             "scratchies",
        Name:             "Scratchies",
        ShortDescription: "Duck into the newsagent, buy an instant scratch-it, and peel the latex off panel by panel. 16 tickets, four price tiers, one dream jackpot.",
        MinPlayers:       1,
        MaxPlayers:       6,
        Tags:             []string{"casino", "luck", "scratch-card", "instant-win", "solo"},
        CtxFeatures:      kit.CtxFeatCharacter,
        Lifecycle:        kit.LifecycleEphemeral,
        QuickModeLabel:   "Pop in",
        SoloModeLabel:    "Solo visit",
        PrivateInviteLine: "Mates pull up to the same counter when they enter the code.",
        Controls: []kit.ControlDecl{
            kit.RuneControl(' ', "SCRATCH"),
            kit.RuneControl('a', "SCRATCH ALL"),
            kit.RuneControl('\r', "BUY"),
            kit.RuneControl('q', "BACK"),
        },
        Leaderboard: &kit.LeaderboardSpec{
            MetricLabel: "Credits",
            Direction:   kit.HigherBetter,
            Aggregation: kit.BestResult,
            Format:      kit.Integer,
        },
        // Admin-settable config (config.go): per-tier RTP band + announce
        // threshold, declared so the arcade's admin tools render a form.
        Config: configSpecs(),
    }
}
```

## 13. Determinism & smoke plan

All randomness (outcome draws, panel layouts, per-panel scratch depths, big-win
selection) flows from one room-seeded RNG, so `smoke.yaml` and bug replays are
byte-identical.

The shipped `smoke.yaml` drives two seats with a fixed seed: both pull up to the
counter → seat 0 zooms the $1 stand, buys **Lucky 7s**, scratches panel-by-panel
to a small win → seat 1 buys **Mega Multiplier** ($5), hits SCRATCH-ALL into a
big win that fires the room-wide ticker → seat 0 keeps buying $1s until busted →
wallet rebuys to 1000 (AB-10). Scripted keys are read off the seeded run so the
story replays exactly.

CI gates per SCHEMA.md: TinyGo wasip1 build, `shellcade-kit check`, meta slug ==
dir, MIT LICENSE file, smoke shots posted as PR previews.

## 14. Out of scope (v1)

- Trading, gifting, or a persistent collection/album of scratched tickets.
- "Second-chance" draws on losing tickets; ticket-of-the-week rotation.
- Animated scratch particles beyond a one-frame reveal flash per panel.
- Per-player difficulty/odds settings (odds are the ticket's, fixed and printed).
- Any real-money, purchase, or external-payment surface — credits are toy-only.

## 15. Open questions

1. **RTP bands** (§7) are first guesses; confirm the per-tier feel in playtest —
   a $1 that pays back too rarely feels mean, too often feels pointless.
2. **Bust threshold:** rebuy at exactly 0, or when you can't afford the cheapest
   stand ($1)? Latter avoids a stranded 0–credit dead-end; confirm it feels fair.
3. **Big-win threshold** (§9): ≥500 credits *and* ≥50× price — tune so the ticker
   is exciting, not constant, across a 6-patron floor.
4. **SCRATCH-ALL on BUST tickets:** mashing `a` on Croc Cash should still honour a
   pre-drawn win (BUST only ends *losing* find-symbol cards) — verify the reveal
   reads as fair and not like the game cheated you.
5. Counter art density: how much newsagent flavour (magazines, lotto sign, drink
   fridge) fits at 80×24 without crowding the four stands? Settle in AB-1.
