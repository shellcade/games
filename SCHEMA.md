# The game directory contract

A catalog game is a directory at `games/<shellcade-username>/<game-name>/`.
There is **no manifest file**: the game's metadata lives in its code (the
`Meta()` the kit requires anyway) and is read from the built artifact with
`shellcade-kit meta game.wasm` — CI asserts it agrees with the path, so
nothing is declared twice and nothing can drift.

## Required

| File | Rule |
|---|---|
| `go.mod` | Every game is a **standalone Go module** (any module path; `require github.com/shellcade/kit`) |
| source | Builds with the pinned TinyGo profile and passes `shellcade-kit check` (the same harness the arcade runs) |
| `LICENSE` | One of: **MIT, Apache-2.0, BSD-3-Clause, MPL-2.0, Unlicense** |

The **directory name** is the game's bare name — `[a-z0-9-]{1,32}`, no slash —
and MUST equal the `slug` your artifact's meta reports (`shellcade-kit meta`).
Your game's platform identity is the path: `<shellcade-username>/<game-name>`.
Player bounds (`minPlayers`/`maxPlayers` in your meta) must sit within the
platform's 1..8.

Built artifacts (`*.wasm`) are **never committed** — CI builds what ships.

## Optional

| File | Rule |
|---|---|
| `CHANGELOG.md` | The top `## ` section is folded into each release's notes — say what changed for players and reviewers. No versioning machinery: releases are auto-numbered `<owner>-<name>-vN` and pinned by content digest |
| `README.md` | For humans browsing the catalog |

## Validating locally

```sh
cd games/<you>/<game>
go mod tidy
tinygo build -opt=1 -no-debug -gc=leaking -o game.wasm -target wasip1 -buildmode=c-shared .
shellcade-kit check game.wasm   # full conformance, the merge gate
shellcade-kit meta game.wasm    # what the platform will read
python3 ../../../.github/scripts/validate_game_dir.py "$(pwd)"
```
