# `game.toml` schema

Every game in the catalog ships a `game.toml` at the root of its directory:

    games/<shellcade-username>/<name>/game.toml

CI validates it on every PR
([`.github/scripts/validate_game_toml.py`](.github/scripts/validate_game_toml.py)).
A live example is
[`games/example/tic-tac-toe/game.toml`](games/example/tic-tac-toe/game.toml).

## The slug is the path

A game's **slug is its catalog path**: `<shellcade-username>/<name>`. The
platform composes that namespaced slug from the directory it lives in —
`game.toml` carries the **bare name only** and never a slash. Names are unique
**per author** (two authors may each have a `pong`; one author may not have two).

## Fields

| Field          | Required | Type           | Rule                                                                 |
| -------------- | -------- | -------------- | ------------------------------------------------------------------- |
| `name`         | yes      | string         | `[a-z0-9-]{1,32}`, no slash; **must equal the directory name**       |
| `display_name` | yes      | string         | non-empty; the human-facing lobby title                             |
| `description`  | yes      | string         | non-empty; **≤ 200 characters**                                     |
| `license`      | yes      | string         | one of the [allowlist](#license-allowlist)                          |
| `players.min`  | yes      | integer        | `1 ≤ min`                                                            |
| `players.max`  | yes      | integer        | `min ≤ max ≤ 8`                                                      |
| `tags`         | no       | list of string | each `[a-z0-9-]{1,32}`                                              |

`players.min` and `players.max` live under a `[players]` table and must satisfy
`1 ≤ min ≤ max ≤ 8` — the platform's player-count caps.

### License allowlist

Source is required and must be licensed so the arcade can build and host it.
Declare one of:

- `MIT`
- `Apache-2.0`
- `BSD-3-Clause`
- `MPL-2.0`
- `Unlicense`

## Example

```toml
name = "tic-tac-toe"
display_name = "Tic-Tac-Toe"
description = "Classic noughts and crosses on an 80x24 board."
license = "MIT"
tags = ["board", "two-player", "classic"]

[players]
min = 2
max = 2
```

This lives at `games/example/tic-tac-toe/game.toml`, so the platform-composed
slug is `example/tic-tac-toe`.

## Validate locally

The validator is stdlib-only on Python 3.11+ (`tomllib`); older interpreters
need `tomli`.

    python3 .github/scripts/validate_game_toml.py games/<you>/<name>
