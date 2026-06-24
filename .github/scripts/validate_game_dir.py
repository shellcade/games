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
  - no committed build artifacts of any kind
"""
import re
import stat
import subprocess
import sys
from pathlib import Path
from typing import Optional

ALLOWLIST = [
    (re.compile(r"MIT License", re.I), "MIT"),
    (re.compile(r"Apache License\s*$|Apache License,? Version 2\.0", re.I), "Apache-2.0"),
    (re.compile(r"BSD 3-Clause", re.I), "BSD-3-Clause"),
    (re.compile(r"Mozilla Public License,? (Version )?2\.0", re.I), "MPL-2.0"),
    (re.compile(r"free and unencumbered software", re.I), "Unlicense"),
]

BUILD_DIRS = {"target", "smoke-out", "shots", "dist", "build"}
EXECUTABLE_EXTS = {".exe", ".dll", ".dylib", ".so"}
NATIVE_MAGIC = {
    b"\x7fELF": "ELF executable",
    b"MZ": "PE executable",
    b"\xfe\xed\xfa\xce": "Mach-O executable",
    b"\xfe\xed\xfa\xcf": "Mach-O executable",
    b"\xce\xfa\xed\xfe": "Mach-O executable",
    b"\xcf\xfa\xed\xfe": "Mach-O executable",
    b"\xca\xfe\xba\xbe": "Mach-O universal binary",
    b"\xbe\xba\xfe\xca": "Mach-O universal binary",
}


def err(msg: str) -> None:
    print(f"::error::{msg}")
    sys.exit(1)


def git_root(path: Path) -> Optional[Path]:
    try:
        out = subprocess.check_output(
            ["git", "-C", str(path), "rev-parse", "--show-toplevel"],
            stderr=subprocess.DEVNULL,
            text=True,
        ).strip()
    except (subprocess.CalledProcessError, FileNotFoundError):
        return None
    return Path(out) if out else None


def tracked_files(d: Path) -> list[Path]:
    root = git_root(d)
    if root is None:
        return [p for p in d.rglob("*") if p.is_file()]
    rel = d.resolve().relative_to(root.resolve()).as_posix()
    try:
        out = subprocess.check_output(
            ["git", "-C", str(root), "ls-files", "--", rel],
            stderr=subprocess.DEVNULL,
            text=True,
        )
    except subprocess.CalledProcessError as exc:
        err(f"{d}: could not list tracked files: {exc}")
    files = []
    for line in out.splitlines():
        p = root / line
        if p.is_file():
            files.append(p)
    return files


def artifact_reason(path: Path, game_dir: Path) -> Optional[str]:
    rel = path.resolve().relative_to(game_dir.resolve())
    parts = rel.parts
    if any(part in BUILD_DIRS for part in parts[:-1]):
        return "file under build-output directory"
    if path.suffix == ".wasm":
        return "wasm build artifact"
    if path.suffix in EXECUTABLE_EXTS:
        return "native build artifact"
    try:
        head = path.read_bytes()[:4]
    except OSError:
        head = b""
    for magic, reason in NATIVE_MAGIC.items():
        if head.startswith(magic):
            return reason
    try:
        mode = path.stat().st_mode
    except OSError:
        return None
    if mode & (stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH):
        if path.name not in {"LICENSE"}:
            return "executable-bit build artifact"
    return None


def main() -> None:
    if len(sys.argv) != 2:
        err("usage: validate_game_dir.py games/<username>/<name>")
    d = Path(sys.argv[1].rstrip("/"))

    # The whole path shape is the contract: games/<username>/<name> with both
    # segments bare names. The owner segment is fork-controlled too (it comes
    # from PR file paths), so it gets the same charset check as the game name.
    # Anchor on the trailing segments so authors can pass "$(pwd)" locally.
    parts = d.parts
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
    if not any((d / m).is_file() for m in module_markers):
        err(f"{d}: no module marker — need go.mod (Go) or Cargo.toml (Rust)")

    lic = d / "LICENSE"
    if not lic.is_file():
        err(f"{d}/LICENSE missing — allowlist: MIT, Apache-2.0, BSD-3-Clause, MPL-2.0, Unlicense")
    with lic.open(encoding="utf-8", errors="replace") as f:
        head = "\n".join(f.read().splitlines()[:5])
    spdx = next((tag for pat, tag in ALLOWLIST if pat.search(head)), None)
    if spdx is None:
        err(f"{d}/LICENSE not recognized — allowlist: MIT, Apache-2.0, BSD-3-Clause, MPL-2.0, Unlicense")

    # The smoke contract: every game ships a smoke.yaml (scripted screens for
    # PR previews). Schema + run validation happens in `shellcade-kit smoke`;
    # presence is the static gate.
    if not (d / "smoke.yaml").is_file():
        err(f"{d}/smoke.yaml missing — every game ships a smoke script (see SCHEMA.md)")

    for path in tracked_files(d):
        reason = artifact_reason(path, d)
        if reason is not None:
            err(f"{path}: committed build artifact rejected ({reason}) — CI builds what ships")

    print(f"ok: {d} (license={spdx}, module + smoke.yaml + sources present)")


if __name__ == "__main__":
    main()
