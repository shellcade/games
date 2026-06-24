#!/usr/bin/env python3
import os
import stat
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("validate_game_dir.py")


class ValidateGameDirTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        subprocess.run(["git", "init"], cwd=self.root, check=True, stdout=subprocess.DEVNULL)
        self.game = self.root / "games" / "alice" / "breakout"
        self.game.mkdir(parents=True)
        (self.game / "go.mod").write_text("module shellcade.games/alice/breakout\n", encoding="utf-8")
        (self.game / "LICENSE").write_text("MIT License\n", encoding="utf-8")
        (self.game / "smoke.yaml").write_text("seed: 1\nseats: 1\nsteps: []\n", encoding="utf-8")
        (self.game / "main.go").write_text("package main\n", encoding="utf-8")

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def git_add(self) -> None:
        subprocess.run(["git", "add", "."], cwd=self.root, check=True, stdout=subprocess.DEVNULL)

    def validate(self) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(SCRIPT), str(self.game)],
            cwd=self.root,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
        )

    def assert_rejected(self, filename: str, content: bytes, expected: str, executable: bool = False) -> None:
        path = self.game / filename
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content)
        if executable:
            path.chmod(path.stat().st_mode | stat.S_IXUSR)
        self.git_add()
        result = self.validate()
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn(expected, result.stdout)

    def test_valid_game_passes(self) -> None:
        self.git_add()
        result = self.validate()
        self.assertEqual(result.returncode, 0, result.stdout)

    def test_wasm_artifact_is_rejected(self) -> None:
        self.assert_rejected("game.wasm", b"\x00asm", "wasm build artifact")

    def test_elf_artifact_is_rejected(self) -> None:
        self.assert_rejected("breakout", b"\x7fELFpayload", "ELF executable")

    def test_mach_o_artifact_is_rejected(self) -> None:
        self.assert_rejected("breakout", b"\xcf\xfa\xed\xfepayload", "Mach-O executable")

    def test_pe_artifact_is_rejected(self) -> None:
        self.assert_rejected("breakout.exe", b"MZpayload", "native build artifact")

    def test_executable_bit_artifact_is_rejected(self) -> None:
        self.assert_rejected("breakout", b"native payload", "executable-bit build artifact", executable=True)

    def test_build_output_directory_is_rejected(self) -> None:
        self.assert_rejected("target/wasm32-wasip1/release/game.wasm", b"\x00asm", "build-output directory")

    def test_ignored_untracked_local_outputs_are_ignored(self) -> None:
        (self.root / ".gitignore").write_text("*.wasm\ntarget/\n", encoding="utf-8")
        self.git_add()
        (self.game / "game.wasm").write_bytes(b"\x00asm")
        target = self.game / "target" / "wasm32-wasip1" / "release"
        target.mkdir(parents=True)
        (target / "game.wasm").write_bytes(b"\x00asm")
        result = self.validate()
        self.assertEqual(result.returncode, 0, result.stdout)


if __name__ == "__main__":
    unittest.main()
