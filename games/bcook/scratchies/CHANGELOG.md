# Changelog

## 0.2.0

- Five new scratch-card games (10 new tickets, 26 total across nine games):
  **Lucky Lines** (three in a row/column/diagonal), **Cashword** (complete words
  from a letter bank), **Quick Bingo** (mark a line of called numbers),
  **Showdown** (beat the house column by column), and **Triple Word** (spell a
  bonus word; a 3× tile triples it).
- Fixed a match-3 spoiler: the winning amount no longer turns green until the
  card resolves. Multiplier panels are now reachable with both right and down.

## 0.1.0

Initial release.

- Newsagent shop: a counter of four price stands ($1/$2/$5/$10), browse-and-buy,
  and a per-player state machine (counter → stand → card → result), with a
  durable 1,000-credit wallet, rebuy-on-bust, and a "Credits" peak leaderboard.
- Four scratch-card engines: match-3 cash, key-number match, multiplier, and
  find-the-symbol (emoji targets, optional BUST). Outcomes are predetermined at
  purchase and displayed honestly regardless of scratch order.
- 16 themed tickets with prize tables tuned to per-tier RTP / odds bands.
- Scratch feel: a coin cursor, 1–3 hidden rubs per panel (latex always shows
  fully opaque), scratch-all, and a vertical scrolling viewport for the big
  $5/$10 cards.
- 1–6 player shared shop floor with a room-wide big-win ticker.
