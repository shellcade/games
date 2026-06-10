#!/usr/bin/env python3
"""Static checks for a catalog game directory (no manifest — metadata lives in
the artifact and is asserted separately via `shellcade-kit meta`).

Usage: validate_game_dir.py games/<username>/<name>

Enforces:
  - path shape: exactly games/<username>/<name>, BOTH segments matching
    ^[a-z0-9-]{1,32}$ — the workflows iterate these paths in shell, so the
    shape is a checked invariant, not a convention
  - a standalone-module marker present: go.mod (Go guest) or Cargo.toml (Rust
    guest). The artifact's meta is the source of truth either way; this is just
    a "the sources build as their own module" sanity check.
  - LICENSE present, first line matching the allowlist:
      MIT, Apache-2.0, BSD-3-Clause, MPL-2.0, Unlicense
  - smoke.yaml present (the scripted-screens contract; `shellcade-kit smoke`
    validates the schema and runs it — this just asserts the file exists)
  - no committed build artifacts (*.wasm)
"""
import os
import re
import sys

ALLOWLIST = [
    (re.compile(r"MIT License", re.I), "MIT"),
    (re.compile(r"Apache License\s*$|Apache License,? Version 2\.0", re.I), "Apache-2.0"),
    (re.compile(r"BSD 3-Clause", re.I), "BSD-3-Clause"),
    (re.compile(r"Mozilla Public License,? (Version )?2\.0", re.I), "MPL-2.0"),
    (re.compile(r"free and unencumbered software", re.I), "Unlicense"),
]


def err(msg: str) -> None:
    print(f"::error::{msg}")
    sys.exit(1)


def main() -> None:
    if len(sys.argv) != 2:
        err("usage: validate_game_dir.py games/<username>/<name>")
    d = sys.argv[1].rstrip("/")

    # The whole path shape is the contract: games/<username>/<name> with both
    # segments bare names. The owner segment is fork-controlled too (it comes
    # from PR file paths), so it gets the same charset check as the game name.
    # Anchor on the trailing segments so authors can pass "$(pwd)" locally.
    parts = d.split("/")
    if len(parts) < 3 or parts[-3] != "games":
        err(f"{d}: game directory must be games/<username>/<name>")
    owner, name = parts[-2], parts[-1]
    if not re.fullmatch(r"[a-z0-9-]{1,32}", owner):
        err(f"{d}: username directory {owner!r} must match [a-z0-9-]{{1,32}}")
    if not re.fullmatch(r"[a-z0-9-]{1,32}", name):
        err(f"{d}: game directory name {name!r} must match [a-z0-9-]{{1,32}}")
    # A game is a standalone module in its source language: go.mod for a Go
    # guest, Cargo.toml for a Rust guest. The built artifact (and its meta) is
    # the real contract; this just asserts the sources stand on their own.
    module_markers = ("go.mod", "Cargo.toml")
    if not any(os.path.isfile(os.path.join(d, m)) for m in module_markers):
        err(f"{d}: no module marker — need go.mod (Go) or Cargo.toml (Rust)")

    lic = os.path.join(d, "LICENSE")
    if not os.path.isfile(lic):
        err(f"{d}/LICENSE missing — allowlist: MIT, Apache-2.0, BSD-3-Clause, MPL-2.0, Unlicense")
    with open(lic, encoding="utf-8", errors="replace") as f:
        head = "\n".join(f.read().splitlines()[:5])
    spdx = next((tag for pat, tag in ALLOWLIST if pat.search(head)), None)
    if spdx is None:
        err(f"{d}/LICENSE not recognized — allowlist: MIT, Apache-2.0, BSD-3-Clause, MPL-2.0, Unlicense")

    # The smoke contract: every game ships a smoke.yaml (scripted screens for
    # PR previews). Schema + run validation happens in `shellcade-kit smoke`;
    # presence is the static gate.
    if not os.path.isfile(os.path.join(d, "smoke.yaml")):
        err(f"{d}/smoke.yaml missing — every game ships a smoke script (see SCHEMA.md)")

    for root, _dirs, files in os.walk(d):
        for fn in files:
            if fn.endswith(".wasm"):
                err(f"{os.path.join(root, fn)}: built artifacts are never committed — CI builds what ships")

    print(f"ok: {d} (license={spdx}, module + smoke.yaml + sources present)")


if __name__ == "__main__":
    main()
