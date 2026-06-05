#!/usr/bin/env python3
"""Validate a game.toml against the catalog schema (see SCHEMA.md).

Usage: validate_game_toml.py games/<shellcade-username>/<game-name>

The directory's last path segment is the game's bare name; game.toml's
`name` field MUST equal it. The platform composes the namespaced slug
(<shellcade-username>/<name>) from the catalog path — game.toml never
carries a slash. Uniqueness is per author.

Stdlib-only on Python 3.11+ (tomllib). Falls back to the third-party
`tomli` shim on older interpreters so the script is runnable locally.
"""

import os
import re
import sys

try:
    import tomllib  # Python 3.11+
except ModuleNotFoundError:  # pragma: no cover - local dev fallback
    import tomli as tomllib

# Single source of truth for the schema, mirrored prose-side in SCHEMA.md.
NAME_RE = re.compile(r"^[a-z0-9-]{1,32}$")
LICENSES = {"MIT", "Apache-2.0", "BSD-3-Clause", "MPL-2.0", "Unlicense"}
PLAYER_MIN, PLAYER_MAX = 1, 8  # platform caps
DESC_MAX = 200


def fail(path, msg):
    # ::error:: makes the line a GitHub Actions annotation; harmless elsewhere.
    print(f"::error file={path}::{msg}")
    sys.exit(1)


def validate(game_dir):
    game_dir = game_dir.rstrip("/")
    toml_path = os.path.join(game_dir, "game.toml")
    dir_name = os.path.basename(game_dir)

    if not os.path.isfile(toml_path):
        fail(toml_path, f"{toml_path} is missing")

    try:
        with open(toml_path, "rb") as f:
            data = tomllib.load(f)
    except tomllib.TOMLDecodeError as e:
        fail(toml_path, f"game.toml is not valid TOML: {e}")

    # --- required fields present ---
    for key in ("name", "display_name", "description", "license", "players"):
        if key not in data:
            fail(toml_path, f"missing required field: {key}")

    # --- name: bare slug, matches the directory ---
    name = data["name"]
    if not isinstance(name, str) or not NAME_RE.match(name):
        fail(toml_path, f"name {name!r} must match [a-z0-9-] (1-32 chars), no slash")
    if name != dir_name:
        fail(
            toml_path,
            f"name {name!r} must equal the directory name {dir_name!r} "
            "(the slug is the catalog path; game.toml holds the bare name)",
        )

    # --- display_name ---
    display = data["display_name"]
    if not isinstance(display, str) or not display.strip():
        fail(toml_path, "display_name must be a non-empty string")

    # --- description ---
    desc = data["description"]
    if not isinstance(desc, str) or not desc.strip():
        fail(toml_path, "description must be a non-empty string")
    if len(desc) > DESC_MAX:
        fail(toml_path, f"description is {len(desc)} chars; max {DESC_MAX}")

    # --- license: allowlist ---
    lic = data["license"]
    if lic not in LICENSES:
        fail(
            toml_path,
            f"license {lic!r} not allowed; choose one of {sorted(LICENSES)}",
        )

    # --- players: min/max within platform caps ---
    players = data["players"]
    if not isinstance(players, dict) or "min" not in players or "max" not in players:
        fail(toml_path, "players must be a table with integer min and max")
    pmin, pmax = players["min"], players["max"]
    if not isinstance(pmin, int) or not isinstance(pmax, int):
        fail(toml_path, "players.min and players.max must be integers")
    if not (PLAYER_MIN <= pmin <= pmax <= PLAYER_MAX):
        fail(
            toml_path,
            f"players must satisfy {PLAYER_MIN} <= min <= max <= {PLAYER_MAX} "
            f"(got min={pmin}, max={pmax})",
        )

    # --- tags: optional list of bare slugs ---
    if "tags" in data:
        tags = data["tags"]
        if not isinstance(tags, list) or not all(isinstance(t, str) for t in tags):
            fail(toml_path, "tags must be a list of strings")
        for t in tags:
            if not NAME_RE.match(t):
                fail(toml_path, f"tag {t!r} must match [a-z0-9-] (1-32 chars)")

    print(f"ok: {toml_path} (name={name}, license={lic}, players={pmin}-{pmax})")


def main(argv):
    if len(argv) != 2:
        print("usage: validate_game_toml.py <game-dir>", file=sys.stderr)
        return 2
    validate(argv[1])
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
