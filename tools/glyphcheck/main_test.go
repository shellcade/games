package main

import (
	"os"
	"path/filepath"
	"testing"
)

// lintSrc writes src as a single Go file into a temp package dir, parses it the
// same way the linter does, and returns the lint result.
func lintSrc(t *testing.T, src string) ([]violation, []unverified, int) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "game.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := parseDir(dir)
	if err != nil {
		t.Fatalf("parseDir: %v", err)
	}
	if p == nil {
		t.Fatal("parseDir returned nil package")
	}
	return p.lint()
}

func TestClearsWideFullwidthLiterals(t *testing.T) {
	// A fullwidth digit (U+FF17) and a wide emoji (U+1F340) are both W/F, so
	// both helpers clear. SetWide takes a rune literal; SetGraphemeWide a string.
	src := `package g
func draw(f F) {
	f.SetWide(0, 0, '７', st)
	f.SetGraphemeWide(0, 0, "🍀", st)
}
`
	vs, us, cleared := lintSrc(t, src)
	if len(vs) != 0 {
		t.Fatalf("expected no violations, got %v", vs)
	}
	if len(us) != 0 {
		t.Fatalf("expected no unverified, got %v", us)
	}
	if cleared != 2 {
		t.Fatalf("expected 2 cleared, got %d", cleared)
	}
}

func TestFlagsNonWideBaseLiterals(t *testing.T) {
	// The pokies class: U+0037 ('7') and U+2744 ('❄') are NOT W/F. Both helpers
	// must flag them. The keycap form "7️⃣" still has base U+0037.
	src := `package g
func draw(f F) {
	f.SetWide(0, 0, '7', st)
	f.SetGraphemeWide(0, 0, "7️⃣", st)
	f.SetWide(0, 0, '❄', st)
}
`
	vs, _, cleared := lintSrc(t, src)
	if len(vs) != 3 {
		t.Fatalf("expected 3 violations, got %d: %v", len(vs), vs)
	}
	if cleared != 0 {
		t.Fatalf("expected 0 cleared, got %d", cleared)
	}
	for _, v := range vs {
		if v.base != '7' && v.base != '❄' {
			t.Fatalf("unexpected flagged base U+%04X", v.base)
		}
	}
}

func TestResolvesPackageDecls(t *testing.T) {
	// A package-level rune const, a string const, and a string-valued map var
	// all resolve syntactically; every map value is checked.
	src := `package g
const glyph = '７'
const cluster = "🍀"
var faceArt = map[string]string{"a": "💎", "b": "x"}
func draw(f F) {
	f.SetWide(0, 0, glyph, st)
	f.SetGraphemeWide(0, 0, cluster, st)
	f.SetGraphemeWide(0, 0, faceArt[s], st)
}
`
	vs, us, _ := lintSrc(t, src)
	if len(us) != 0 {
		t.Fatalf("expected no unverified, got %v", us)
	}
	// faceArt has one bad value ("x" -> base 'x'); the two consts are W/F.
	if len(vs) != 1 || vs[0].base != 'x' {
		t.Fatalf("expected one violation on map value 'x', got %v", vs)
	}
}

func TestDynamicArgIsUnverifiedNotViolation(t *testing.T) {
	// A runtime-computed glyph (local var, struct field) cannot be resolved
	// statically: report it as unverified, never as a violation (soundness —
	// the kit's runtime `check` covers these).
	src := `package g
func draw(f F, p P) {
	var glyph rune = pick()
	f.SetWide(0, 0, glyph, st)
	f.SetGraphemeWide(0, 0, p.Reveal, st)
}
`
	vs, us, _ := lintSrc(t, src)
	if len(vs) != 0 {
		t.Fatalf("expected no violations for dynamic args, got %v", vs)
	}
	if len(us) != 2 {
		t.Fatalf("expected 2 unverified, got %d: %v", len(us), us)
	}
}
