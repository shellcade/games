# The game directory contract

A catalog game is a directory at `games/<shellcade-username>/<game-name>/`.
There is **no manifest file**: the game's metadata lives in its code (the
`Meta()` the kit requires anyway) and is read from the built artifact with
`shellcade-kit meta game.wasm` — CI asserts it agrees with the path, so
nothing is declared twice and nothing can drift.

## Required

| File | Rule |
|---|---|
| module marker | Every game is a **standalone module** in its source language: `go.mod` for a Go guest (any path; `require github.com/shellcade/kit`), or `Cargo.toml` for a Rust guest (a `cdylib` for `wasm32-wasip1`; implement the ABI from `ABI.md` — see `bcook/tic-tac-toe-rs`). The **built artifact** and its `meta` are the real contract; the language is your choice |
| source | Builds with its pinned toolchain profile (TinyGo dev profile, or `cargo build --release --target wasm32-wasip1`) and passes `shellcade-kit check` (the same harness the arcade runs) |
| `LICENSE` | One of: **MIT, Apache-2.0, BSD-3-Clause, MPL-2.0, Unlicense** |
| `smoke.yaml` | A deterministic smoke script (seed, seats, steps) that drives your game and names screen dumps — CI runs it on every PR (`shellcade-kit smoke`) and posts the screens as a visual preview comment. Smoke scripts drive at most **8 seats** (the runner clamps `minPlayers` to the seat count, so large-room games still pass smoke; large-room behavior is exercised by `check` and your budget tests). Schema + authoring guidance: [kit GUIDE.md "Smoke scripts"](https://github.com/shellcade/kit/blob/main/GUIDE.md#smoke-scripts-scripted-screens) |

The **directory name** is the game's bare name — `[a-z0-9-]{1,32}`, no slash —
and MUST equal the `slug` your artifact's meta reports (`shellcade-kit meta`).
Your game's platform identity is the path: `<shellcade-username>/<game-name>`.
Player bounds (`minPlayers`/`maxPlayers` in your meta) must sit within the
platform's 1..1024.

Built artifacts (`*.wasm`) are **never committed** — CI builds what ships.

## Optional

| File | Rule |
|---|---|
| `CHANGELOG.md` | The top `## ` section is folded into each release's notes — say what changed for players and reviewers. No versioning machinery: releases are auto-numbered `<owner>-<name>-vN` and pinned by content digest |
| `README.md` | For humans browsing the catalog |

## Validating locally

Go guest:

```sh
cd games/<you>/<game>
go mod tidy
tinygo build -opt=1 -no-debug -gc=leaking -o game.wasm -target wasip1 -buildmode=c-shared .
shellcade-kit check game.wasm   # full conformance, the merge gate
shellcade-kit meta game.wasm    # what the platform will read
shellcade-kit smoke .           # runs smoke.yaml, writes the shot files CI previews
python3 ../../../.github/scripts/validate_game_dir.py "$(pwd)"
```

(Go authors can also iterate natively without TinyGo: `go run . -smoke smoke.yaml`.)

Rust guest (cdylib, `wasm32-wasip1`):

```sh
cd games/<you>/<game>
cargo build --release --target wasm32-wasip1
W=target/wasm32-wasip1/release/<crate_name>.wasm   # crate name, underscores
shellcade-kit check "$W"        # full conformance, the merge gate
shellcade-kit meta "$W"         # what the platform will read
shellcade-kit smoke .           # builds + runs smoke.yaml, writes the shot files
python3 ../../../.github/scripts/validate_game_dir.py "$(pwd)"
```
