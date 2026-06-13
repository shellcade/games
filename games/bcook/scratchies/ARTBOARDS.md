# Scratchies — artboards

Twelve screens covering the full loop — counter, stand, the four ticket
mechanics, results, the bust safety-net, and a scrolling big card — drawn at
the kit's fixed **80×24** frame. Trailing blank space is trimmed (the right edge of the canvas
is implied); every line is ≤ 80 columns. Companion to [SPEC.md](SPEC.md) —
boards are referenced there as AB-1 … AB-11, and the layouts mirror
`render.go` (run `go run . -smoke smoke.yaml` for live shots).

Color/style key (monochrome here; styles via `kit.Style`):

| Element | Style |
|---|---|
| Latex wear `▓▓`→`▒▒`→`░░`→revealed | dim grey, lightening per rub (3→2→1 left) |
| Coin cursor / `◀ coin` | bold white ring on the focused panel |
| Revealed cash `$5` / numbers | white |
| A landed match / found symbol `★ ✦MATCH` | green |
| `BUST` panel | red (only ends a *losing* find-symbol card) |
| Win banner `✦ ✦ ✦ … ✦ ✦ ✦` | green, flashes ~0.5 s on reveal |
| `BIG WIN` banner + room ticker flash | amber |
| Wallet `◉ you · N cr` | bold; dips red for ~0.3 s when a purchase deducts |
| Stand price tags `$1 $2 $5 $10` | yellow |
| Selection (double border `╔═╗`) | bold white |
| Scroll rail `▲ ░ ▓ ▼` (thumb = `▓`) | dim; thumb bold white |

Per-player frames (`r.Send`): the counter chrome and big-win ticker are shared,
but the stand you're browsing and the card you're scratching are yours alone.


## AB-1 · The counter — zoomed out

```text
 THE CORNER NEWSAGENT                                       ◉ you · 1,000 cr
───────────────────────────────────────────────────────────────────────────────
   ░ MAGS ░   ░ PAPERS ░          ★ INSTANT SCRATCH-ITS ★          ▤ DRINKS ▤

   ╔═ $1 ═══════╗  ┌─ $2 ───────┐  ┌─ $5 ───────┐  ┌─ $10 ──────┐
   ║ ▒▒▒   ▒▒▒  ║  │ ▓▓▓   ▓▓▓  │  │ ░░░   ░░░  │  │ ███   ███  │
   ║ ▒▒▒   ▒▒▒  ║  │ ▓▓▓   ▓▓▓  │  │ ░░░   ░░░  │  │ ███   ███  │
   ║ LUCKY 7s…  ║  │ GOLD RUSH… │  │ DIAMOND… │  │ PLATINUM…   │
   ║ 4 tickets  ║  │ 4 tickets  │  │ 4 tickets  │  │ 4 tickets  │
   ╚════════════╝  └────────────┘  └────────────┘  └────────────┘
        ▲ here
                                      ◍ alan            ◎ matt
                                  (browsing $5)      (scratching)

───────────────────────────────────────────────────────────────────────────────
   ◉ you  ·  g'day — pick a stand and have a crack
 ◂ ▸ choose a stand        [ENTER] step up to it        [q] leave the shop
```

- The lobby of the game: a newsagent counter with four ticket stands by price.
  `◂ ▸` slides the selection (double border) between stands; **ENTER** zooms in
  (AB-2). Wallet sits top-right.
- Other patrons (`◍ alan`, `◎ matt`) stand at whichever stand they're browsing —
  character tiles via `CtxFeatCharacter`, the salvo/pokies floor convention.
  Their cards are private; only their presence and big wins are shared.
- The bottom strip is the **big-win ticker** (here just a greeting); it scrolls
  room-wide hits during play (AB-11).


## AB-2 · The $1 stand — browse & buy

```text
 $1 STAND · pick a ticket                                   ◉ you · 1,000 cr
───────────────────────────────────────────────────────────────────────────────
   ▸ LUCKY 7s         match three equal amounts     top 10,000     1 in 3.9
     COIN TOSS        match the lucky numbers       top 10,000     1 in 3.9
     CHERRY POP       find three cherries           top 10,000     1 in 3.9
     TINNIE TRIPLER   find a prize, then multiply   top 12,000     1 in 3.9

   ┌─ LUCKY 7s ──────────────────────────────────────┐
   │  ▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒   │
   │  Scratch the 3×3 grid of cash amounts. Uncover   │
   │  three of the same amount to win it.             │
   │                                                  │
   │  cost  $1            any win  ~1 in 3.9          │
   └──────────────────────────────────────────────────┘

───────────────────────────────────────────────────────────────────────────────
   ◍ alan scored 5,000 on GOLD RUSH!
 ▲ ▼ choose ticket        [ENTER] buy $1        [q] back to counter
```

- The four tickets at this price, each with a one-line mechanic blurb, top
  prize, and printed any-win odds. Cursor (`▸`) picks; the preview card below
  shows the highlighted ticket's latex art and the rules.
- **ENTER** buys: credits deduct immediately (the wallet ticks `1,000 → 999`
  with a brief red dip) and the fresh card opens (AB-3). **q** returns to AB-1.
- A real big win from another patron has scrolled into the ticker.


## AB-3 · Fresh card — Lucky 7s (match-3), all latex

```text
 LUCKY 7s · $1 · match three                                ◉ you · 999 cr
───────────────────────────────────────────────────────────────────────────────

           ┌──────────────────────────────────┐
           │   ┌────┐    ┌────┐    ┌────┐      │
           │   │▓▓▓▓│    │░░░░│    │▒▒▒▒│  ◀ coin
           │   └────┘    └────┘    └────┘      │
           │   ┌────┐    ┌────┐    ┌────┐      │
           │   │▒▒▒▒│    │▓▓▓▓│    │░░░░│      │
           │   └────┘    └────┘    └────┘      │
           │   ┌────┐    ┌────┐    ┌────┐      │
           │   │░░░░│    │▒▒▒▒│    │▓▓▓▓│      │
           │   └────┘    └────┘    └────┘      │
           └──────────────────────────────────┘
          fresh card — heavier latex (▓) takes more rubs

───────────────────────────────────────────────────────────────────────────────
 ←↑↓→ move coin    [SPACE] scratch    [a] scratch all    [q] leave it
```

- The bought ticket, all panels latexed but at **mixed starting wear**: each is
  seeded to need 1–3 rubs, so `░░░░` panels lift in one rub, `▓▓▓▓` panels take
  three. The coin-cursor rings the focused panel; arrows/`hjkl` move it, **SPACE**
  rubs it down a stage (SPEC §4).
- The outcome was drawn at purchase (SPEC §7) — this card already knows it pays
  5, regardless of which panels are stubborn. Scratch order and depth are yours
  to enjoy; **`a`** scratches all and skips to the verdict.


## AB-4 · Mid-scratch — two of a kind found

```text
 LUCKY 7s · $1 · match three                                ◉ you · 999 cr
───────────────────────────────────────────────────────────────────────────────

           ┌──────────────────────────────────┐
           │   ┌────┐    ┌────┐    ┌────┐      │
           │   │ $5 │    │ $1 │    │▒▒▒▒│  ◀ coin (rub again)
           │   └────┘    └────┘    └▒▒▒▒┘      │
           │   ┌────┐    ┌────┐    ┌────┐      │
           │   │ $5 │    │░░░░│    │▓▓▓▓│      │
           │   └────┘    └────┘    └────┘      │
           │   ┌────┐    ┌────┐    ┌────┐      │
           │   │▓▓▓▓│    │ $2 │    │▒▒▒▒│      │
           │   └────┘    └────┘    └────┘      │
           └──────────────────────────────────┘
            two $5 so far — the coin panel needs another rub

───────────────────────────────────────────────────────────────────────────────
 ←↑↓→ move coin    [SPACE] scratch    [a] scratch all    [q] leave it
```

- Mid-rub: revealed cells (`$5 $1 $5 $2`) sit beside latex at different wear —
  `░░░░` (one rub to go), `▒▒▒▒` (two), `▓▓▓▓` (untouched, three). The coin is on
  a `▒▒▒▒` panel, so one more SPACE lightens it to `░░░░`, another reveals it.
- Decoys (`$1`, `$2`) never reach three-of-a-kind on a losing layout — here the
  third `$5` is waiting under the latex. Scratch depth is cosmetic: a stubborn
  panel is no likelier to win (SPEC §4, §7).


## AB-5 · Winner reveal + collect

```text
 LUCKY 7s · $1 · ★ WINNER ★                                 ◉ you · 1,004 cr
───────────────────────────────────────────────────────────────────────────────

           ╔══════════════════════════════════╗
           ║   ┌────┐    ┌────┐    ┌────┐      ║
           ║   │ $5 │    │ $1 │    │ $5★│      ║
           ║   └────┘    └────┘    └────┘      ║
           ║   ┌────┐    ┌────┐    ┌────┐      ║
           ║   │ $5★│    │$20 │    │ $2 │      ║
           ║   └────┘    └────┘    └────┘      ║
           ║   ┌────┐    ┌────┐    ┌────┐      ║
           ║   │ $1 │    │ $2 │    │$10 │      ║
           ║   └────┘    └────┘    └────┘      ║
           ╚══════════════════════════════════╝
              ✦ ✦ ✦   three $5 — WON 5 CREDITS   ✦ ✦ ✦

───────────────────────────────────────────────────────────────────────────────
 [ENTER] buy another ($1)              [q] back to the $1 stand
```

- The third `$5` lands; the three winners get a green `★` and the card border
  flashes green for ~0.5 s. Wallet credits `999 → 1,004` (cost already paid, so
  net +4 on the trip). Remaining panels auto-reveal on resolution.
- **ENTER** re-buys the same ticket (if affordable); **q** back to the stand.


## AB-6 · Key number match — Lucky Numbers ($2)

```text
 LUCKY NUMBERS · $2 · match the winning numbers             ◉ you · 1,002 cr
───────────────────────────────────────────────────────────────────────────────
   WINNING NUMBERS     ┌────┐  ┌────┐  ┌────┐
                       │ 07 │  │ 23 │  │ 41 │
                       └────┘  └────┘  └────┘
   YOUR NUMBERS
      ┌───────────┐  ┌───────────┐  ┌───────────┐
      │ 07  ✦MATCH│  │ 12        │  │ 33        │
      │ pays  50  │  │ pays   4  │  │ pays   2  │
      └───────────┘  └───────────┘  └───────────┘
      ┌───────────┐  ┌───────────┐  ┌───────────┐
      │ 19        │  │ 41  ✦MATCH│  │ ▓▓        │
      │ pays   2  │  │ pays  25  │  │ ▓▓▓▓▓▓▓▓▓ │  ◀ coin
      └───────────┘  └───────────┘  └───────────┘
            two matches — 50 + 25 = WON 75 CREDITS

───────────────────────────────────────────────────────────────────────────────
 ←↑↓→ move coin    [SPACE] scratch    [a] scratch all    [q] leave it
```

- The three winning numbers sit up top. Each of your six panels hides a number
  and the prize it pays. A *your number* equal to any winning number lights
  green (`✦MATCH`) and pays its amount; matches **sum** (50 + 25 = 75).
- Losing layouts plant no overlap. Higher tiers (Lotto Lanes $5, Fortune 50
  $10) widen to more winning numbers and a bigger grid — more chances per card.


## AB-7 · Multiplier — Mega Multiplier ($5)

```text
 MEGA MULTIPLIER · $5 · find a prize, then multiply it      ◉ you · 1,015 cr
───────────────────────────────────────────────────────────────────────────────

                 YOUR PRIZE                  MULTIPLIER
               ┌────────────┐              ┌────────────┐
               │            │              │            │
               │    200     │      ×       │    10×     │
               │            │              │   ✦✦✦✦     │
               └────────────┘              └────────────┘

                       200   ×   10   =   2,000

                    ✦ ✦ ✦   B I G   W I N   ✦ ✦ ✦
                          WON 2,000 CREDITS

───────────────────────────────────────────────────────────────────────────────
 [ENTER] buy another ($5)              [q] back to the $5 stand
```

- Two reveals: a base prize, then the multiplier — the multiplier panel is the
  beat you scratch last. Final win = prize × multiplier, factored from the
  card's drawn outcome so it always lands clean.
- This win clears the room-wide threshold (≥ 500 *and* ≥ 50× price), so it also
  fires the big-win ticker on every patron's frame (AB-11).
- The top multiplier climbs by tier — 3× (Tinnie Tripler $1) · 5× (Double
  Trouble $2) · 10× here · 20× (Cash Explosion $10) — same beat, bigger swing.


## AB-8 · Find-the-symbol — Croc Cash ($2, with BUST)

```text
 CROC CASH · $2 · find three crocs — mind the BUST!         ◉ you · 1,000 cr
───────────────────────────────────────────────────────────────────────────────
   FIND 3  ʕ•ᴥ•ʔ CROC  TO WIN                          PRIZE:  50

        ┌──────┐  ┌──────┐  ┌──────┐  ┌──────┐
        │ CROC │  │ fish │  │ ▓▓▓▓ │  │ CROC │
        └──────┘  └──────┘  └──────┘  └──────┘
        ┌──────┐  ┌──────┐  ┌──────┐  ┌──────┐
        │ ▓▓▓▓ │  │ CROC │  │ fish │  │░░░░░░│  ◀ coin
        └──────┘  └──────┘  └──────┘  └░░░░░░┘
        ┌──────┐  ┌──────┐  ┌──────┐  ┌──────┐
        │ fish │  │ ▓▓▓▓ │  │ BUST │  │ ▓▓▓▓ │
        └──────┘  └──────┘  └──────┘  └──────┘
         three crocs found — WON 50 CREDITS  (dodged the BUST!)

───────────────────────────────────────────────────────────────────────────────
 ←↑↓→ move coin    [SPACE] scratch    [a] scratch all    [q] leave it
```

- Press-your-luck: hunt for three `CROC`. A `BUST` panel (red) ends a *losing*
  card instantly with no win — so **scratch-all** is a real gamble here.
- Because the outcome was drawn at purchase, a winning card places exactly three
  crocs and keeps `BUST` off the path until resolution (SPEC §5.4, §7) — peeling
  carefully or mashing `a` reaches the same honest verdict.
- $5 **Treasure Hunt** drops the BUST for a gentler hunt; $10 **Outback Riches**
  keeps it, on a 20-panel grid.


## AB-9 · No win — better luck next time

```text
 GOLD RUSH · $2 · no win this time                          ◉ you · 998 cr
───────────────────────────────────────────────────────────────────────────────

           ┌──────────────────────────────────┐
           │   ┌────┐    ┌────┐    ┌────┐      │
           │   │ $2 │    │$10 │    │ $5 │      │
           │   └────┘    └────┘    └────┘      │
           │   ┌────┐    ┌────┐    ┌────┐      │
           │   │ $1 │    │ $2 │    │$20 │      │
           │   └────┘    └────┘    └────┘      │
           │   ┌────┐    ┌────┐    ┌────┐      │
           │   │$10 │    │ $5 │    │ $1 │      │
           │   └────┘    └────┘    └────┘      │
           └──────────────────────────────────┘
               no three-of-a-kind — no win

───────────────────────────────────────────────────────────────────────────────
 [ENTER] buy another ($2)              [q] back to the $2 stand
```

- The common case (~3 in 4): a fully-revealed card with no winning combination.
  No amount appears three times. Quiet, no flash — the card just settles.
- **ENTER** to go again, **q** back to the stand. The wallet already shows the
  cost paid (`998`).


## AB-10 · Bust → rebuy to 1,000

```text
 THE CORNER NEWSAGENT                                       ◉ you · 0 cr
───────────────────────────────────────────────────────────────────────────────


              ┌──────────────────────────────────────────┐
              │                                          │
              │            OUT OF CREDITS                │
              │                                          │
              │   the newsagent slides you a fresh       │
              │   twenty and a wink.                     │
              │                                          │
              │         ✦   + 1,000 CREDITS   ✦          │
              │                                          │
              └──────────────────────────────────────────┘


───────────────────────────────────────────────────────────────────────────────
 [ENTER] back to the counter — have another crack
```

- Triggered when the wallet can't cover the cheapest stand you can reach
  (SPEC §8). No game-over: the balance resets to **1,000** with a beat, and
  **ENTER** returns to the counter (AB-1). The leaderboard keeps your peak.


## AB-11 · Shared big-win — room-wide ticker

```text
 THE CORNER NEWSAGENT                                       ◉ you · 1,000 cr
───────────────────────────────────────────────────────────────────────────────
   ░ MAGS ░   ░ PAPERS ░          ★ INSTANT SCRATCH-ITS ★          ▤ DRINKS ▤

   ┌─ $1 ───────┐  ┌─ $2 ───────┐  ╔═ $5 ═══════╗  ┌─ $10 ──────┐
   │ ▒▒▒   ▒▒▒  │  │ ▓▓▓   ▓▓▓  │  ║ ░░░   ░░░  ║  │ ███   ███  │
   │ ▒▒▒   ▒▒▒  │  │ ▓▓▓   ▓▓▓  │  ║ ░░░   ░░░  ║  │ ███   ███  │
   └────────────┘  └────────────┘  ╚════════════╝  └────────────┘
                                      ◍ alan ✦          ◎ matt
                    ╔════════════════════════════════════════════╗
                    ║   ✦ ✦   alan scored 2,000 on            ✦ ✦ ║
                    ║           MEGA MULTIPLIER!                  ║
                    ╚════════════════════════════════════════════╝
───────────────────────────────────────────────────────────────────────────────
   ◉ you 75 · ◍ alan 2,000 · ◎ matt 50 · ◍ alan 5,000 ··············
 ◂ ▸ choose a stand        [ENTER] step up to it        [q] leave the shop
```

- When any patron's win clears the threshold (SPEC §9), an amber banner pops
  over the counter for ~2 s on **every** frame, and the win joins the scrolling
  ticker along the bottom. `alan` glints (`✦`) at the $5 stand where he hit.
- Flavour only: it never shows a balance or touches your card. Solo play simply
  never populates the banner — the ticker shows only your own hits.


## AB-12 · Big card — scrolling viewport (Platinum Sevens $10, 6×6)

```text
 PLATINUM SEVENS · $10 · match three · BIG CARD             ◉ you · 990 cr
───────────────────────────────────────────────────────────────────────────────
  ✦ two $5 so far — keep digging                      rows 2–4 of 6      ▲
   ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐                             ▓
   │$10 │ │ $5 │ │$50 │ │ $2 │ │$20 │ │ $5 │                             ▓
   └────┘ └────┘ └────┘ └────┘ └────┘ └────┘                             ░
   ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐                             ░
   │ $5 │ │$100│ │ $2 │ │$20 │ │$10 │ │ $5 │                             ░
   └────┘ └────┘ └────┘ └────┘ └────┘ └────┘                             ░
   ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐                             ▼
   │$20 │ │ $2 │ │░░░░│ │$50 │ │$10 │ │ $5 │  ◀ coin — ↓ scrolls         ▼
   └────┘ └────┘ └░░░░┘ └────┘ └────┘ └────┘     down to row 5
   ──────────────────────────────────────────────────────────────────────
  ↑ row 1 above · rows 5–6 below
 ←↑↓→ move/scroll   [SPACE] scratch   [a] scratch all   [ [ ] ] page   [q] back
```

- The $10 card is **6×6** — taller than the ~3 visible cell-rows, so it scrolls.
  Here rows 2–4 are showing; the coin sits on the **bottom visible row**. Press
  **↓** and the coin holds that edge while the grid slides up to reveal row 5 —
  exactly the "scroll with the coin" feel. `[` / `]` page a viewport at a time.
- The **scroll rail** (right edge) marks position: the `▓` thumb sits high
  because you're near the top of a longer card; `▲`/`▼` show more lies off-screen
  in each direction.
- The running prompt (`two $5 so far…`) is **pinned below the rule**, so the
  verdict is readable no matter where you've scrolled. On **SCRATCH-ALL** the
  whole grid reveals and the viewport snaps to the winning line (or a `BUST`),
  banner pinned.
- $5 cards (Diamond Mine 5×5, Treasure Hunt 20, Lotto Lanes 16) scroll the same
  way with a shorter card; $1/$2 cards fit fully and show no rail.
