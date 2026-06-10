# Content policy — what the arcade hosts, and how to get something taken down

Everything in this repo runs on shared terminals in front of other people.
Merging a PR means the arcade builds your source, runs it sandboxed, and
renders whatever it draws to every player in the room — so the catalog has a
content bar, a way to report a game, and a takedown path that does not wait
for a pull request.

## What a game may not be

A game (its rendered content, its text, its name, its README — anything a
player or reviewer sees) may not:

- contain or generate **illegal content**, or sexual content involving minors
  (instant, permanent removal);
- **target people**: harassment, hate, or doxxing of players, authors, or
  anyone else — including content smuggled in via player handles or stored
  state and replayed to other players;
- **deceive players**: phishing for credentials or codes (no game ever needs a
  player to paste an SSH key, a link code, or a password), fake platform UI,
  or impersonating another author's game or namespace;
- **abuse the host**: attempt to escape or probe the sandbox, exfiltrate other
  players' data, or grief shared terminals (the platform filters dangerous
  escape sequences, but trying is itself a violation);
- be **spam**: placeholder games, ad vehicles, or namespace squatting.

Edgy is fine; the arcade is not a kids' walled garden. The line is harm to
people, deception, and abuse of the machine your game is a guest on.

## Reporting a game

- **Public report** (most cases): open an issue on this repo titled
  `report: <author>/<game>` — say what you saw, in which game, and how to
  reproduce it (a room, a sequence of inputs, a screenshot).
- **Private report** (doxxing, sexual content, sandbox escapes, anything that
  shouldn't be amplified by a public issue): use this repo's
  **Security → Report a vulnerability** (GitHub private reporting), which
  reaches the maintainers without publishing the details.

Reports about a *vulnerable dependency* in a game (the nightly vulncheck
workflow also catches these) follow the same path: issue first; private
report if it's exploitable.

## What happens — the takedown path

1. **Offline first, repo second.** Arcade operators can pull any game from the
   live arcade immediately — the same switch that flips a game live flips it
   off, fleet-wide, without waiting for this repo. A credible report of
   illegal content, deception, or sandbox abuse gets the game taken offline
   while it's investigated.
2. **The catalog follows.** If the report holds, the game's directory is
   removed (or reverted to a prior clean version) by maintainer PR, and the
   report issue records the outcome. If it doesn't hold, the game goes back
   live and the issue says why.
3. **Authors are told.** The author is pinged on the report issue (or
   contacted directly for private reports) and can respond there — a fix PR
   from the author is the preferred resolution for anything short of the
   instant-removal tier.
4. **Repeat or egregious violations cost the namespace**: the author's
   shellcade↔GitHub link stops authorizing publishes, and existing games may
   be removed.

## Appeals

Authors who believe a takedown was wrong comment on the report issue (or open
one referencing it). A maintainer who wasn't the original decider reviews.

---

This policy covers the catalog and what games do at runtime. The *technical*
contract a game must meet is [SCHEMA.md](SCHEMA.md); how to submit is the
[README](README.md).
