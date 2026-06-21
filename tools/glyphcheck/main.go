// Command glyphcheck is a width-contract lint for the shellcade game catalog.
//
// # The corruption class it guards
//
// The kit frame helpers SetWide (a rune) and SetGraphemeWide (a grapheme
// cluster) write a single glyph into TWO terminal columns and reserve the cell
// to its right as a continuation cell. That is only correct when the glyph
// actually renders two columns wide.
// The contract the platform, runewidth, uniseg, x/ansi and real terminals all
// agree on is East Asian Width: a base code point of width Wide (W) or
// Fullwidth (F) is the only safe input. Pass a base code point whose EAW is
// anything else (Narrow, Halfwidth, Ambiguous, Neutral) and a viewer that
// renders it one column wide desyncs every column to its right for the rest of
// the row — the "pokies production corruption" class (the keycap 7️⃣ shipped a
// contested-width base, U+0037, and corrupted the reel layout in production;
// see games/bcook/pokies/layout.go, which now pins U+FF17 FULLWIDTH SEVEN).
//
// # What it checks
//
// glyphcheck parses every Go game's source (parser only — NO type checking, NO
// module download, so it runs offline in fork CI) and finds calls to the
// wide-glyph helpers SetWide and SetGraphemeWide. For each call it resolves the
// glyph argument to its possible literal values when it can do so purely
// syntactically:
//
//   - a string or char (rune) literal passed directly;
//   - an identifier bound to a package-level const/var that is a string literal;
//   - an index into a package-level map var whose values are string literals
//     (e.g. faceArt[s] in pokies — every map value is checked).
//
// For each resolved literal it takes the FIRST code point (the base of the
// cluster — combining marks and variation selectors that follow do not change
// the base's width class) and flags it when its EAW is not W/F. Empty literals
// are flagged too (a wide helper needs a glyph).
//
// Arguments it cannot resolve syntactically (a struct field, a function result,
// a slice element — e.g. scratchies' p.Reveal, which the game guards at runtime
// with its own isWideGlyph check) are reported as "unverified" and do NOT fail
// the build: this linter is sound (no false failures) at the cost of not
// catching dynamic violations. That residual is exactly what the kit's
// `shellcade-kit check` closes once it ships and the catalog pins a kit that
// carries it: that check inspects the BUILT artifact's rendered frames, so it
// sees the runtime width of every cell regardless of how the glyph was
// computed. When that lands and is pinned (it is NOT in the pinned v2.12.1),
// this tool and its embedded EAW table (eaw.go) retire in its favour. Until
// then this is the self-contained, network-free merge gate.
//
// # Usage
//
//	go run ./tools/glyphcheck [path ...]   # default: ./games
//
// Exit status is non-zero if any resolvable violation is found. "unverified"
// notes are printed to stderr but never fail the run. Pass -v to also print
// every call the linter cleared.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// wideHelpers are the kit frame methods that write a glyph into two columns and
// so require a W/F base code point. Both kit width-2 writers are covered:
//
//   - SetWide(row, col, r rune, st)             — single rune into 2 columns;
//   - SetGraphemeWide(row, col, cluster string, st) — grapheme cluster into 2.
//
// (See the kit's internal/game/grid.go: both reserve a Cont=true continuation
// cell, so a sub-2-column base desyncs the row identically.) Matched on the
// SELECTOR name only (f.SetWide(...)), independent of the receiver's spelling.
var wideHelpers = map[string]bool{
	"SetWide":         true,
	"SetGraphemeWide": true,
}

// glyphArgIndex is the positional index of the glyph argument for each helper.
// Both signatures put the glyph at arg 2 (0-based): (row, col, glyph, style).
// SetWide's glyph is a rune literal/const; SetGraphemeWide's is a string. Both
// resolve through the same string/char-literal machinery (see resolveStrings).
var glyphArgIndex = map[string]int{
	"SetWide":         2,
	"SetGraphemeWide": 2,
}

type violation struct {
	pos   token.Position
	call  string
	glyph string // the offending literal (for the message)
	base  rune
	why   string
}

type unverified struct {
	pos  token.Position
	call string
	expr string
}

func main() {
	verbose := flag.Bool("v", false, "print every wide-glyph call the linter cleared")
	flag.Parse()
	roots := flag.Args()
	if len(roots) == 0 {
		roots = []string{"games"}
	}

	pkgs, err := collectPackages(roots)
	if err != nil {
		fmt.Fprintln(os.Stderr, "glyphcheck:", err)
		os.Exit(2)
	}

	var violations []violation
	var unverifieds []unverified
	cleared := 0
	for _, p := range pkgs {
		v, u, c := p.lint()
		violations = append(violations, v...)
		unverifieds = append(unverifieds, u...)
		cleared += c
	}

	sort.Slice(unverifieds, func(i, j int) bool { return unverifieds[i].pos.String() < unverifieds[j].pos.String() })
	for _, u := range unverifieds {
		fmt.Fprintf(os.Stderr, "%s: note: %s(%s) glyph is not a static literal — width unverified here (kit `check` covers it at runtime)\n",
			u.pos, u.call, u.expr)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "glyphcheck: cleared %d wide-glyph call(s) with W/F base code points\n", cleared)
	}

	if len(violations) == 0 {
		fmt.Printf("glyphcheck: ok — %d wide-glyph call(s) verified, %d unverified (dynamic), 0 violations\n", cleared, len(unverifieds))
		return
	}

	sort.Slice(violations, func(i, j int) bool { return violations[i].pos.String() < violations[j].pos.String() })
	for _, v := range violations {
		fmt.Fprintf(os.Stderr, "%s: error: %s passed %q (base U+%04X, %s) — wide-glyph helpers require a base code point with East Asian Width W or F; this desyncs every column to its right\n",
			v.pos, v.call, v.glyph, v.base, v.why)
	}
	fmt.Fprintf(os.Stderr, "glyphcheck: FAIL — %d width-contract violation(s)\n", len(violations))
	os.Exit(1)
}

// pkg is one Go package (a single game directory's .go files in one dir),
// parsed together so package-level const/var lookups resolve across files.
type pkg struct {
	fset  *token.FileSet
	files []*ast.File
	// package-level declarations, indexed by name.
	strConst map[string]string   // const/var bound to a string literal
	mapVar   map[string][]string // map var -> all string-literal values
}

func collectPackages(roots []string) ([]*pkg, error) {
	dirs := map[string]bool{}
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				dirs[filepath.Dir(path)] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	var pkgs []*pkg
	sorted := make([]string, 0, len(dirs))
	for d := range dirs {
		sorted = append(sorted, d)
	}
	sort.Strings(sorted)
	for _, dir := range sorted {
		p, err := parseDir(dir)
		if err != nil {
			return nil, err
		}
		if p != nil {
			pkgs = append(pkgs, p)
		}
	}
	return pkgs, nil
}

func parseDir(dir string) (*pkg, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	p := &pkg{fset: fset, strConst: map[string]string{}, mapVar: map[string][]string{}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			// A file that does not parse is the game's own problem; surface it
			// rather than silently skipping (a half-parsed package could hide a
			// violation). Build CI catches it too, but a clear message helps.
			return nil, fmt.Errorf("parse %s: %w", filepath.Join(dir, e.Name()), err)
		}
		p.files = append(p.files, f)
	}
	if len(p.files) == 0 {
		return nil, nil
	}
	p.indexDecls()
	return p, nil
}

// indexDecls records package-level const/var string literals and string-valued
// map literals so glyph arguments that reference them resolve syntactically.
func (p *pkg) indexDecls() {
	for _, f := range p.files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || (gd.Tok != token.CONST && gd.Tok != token.VAR) {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					switch v := vs.Values[i].(type) {
					case *ast.BasicLit:
						if s, ok := litString(v); ok {
							p.strConst[name.Name] = s
						}
					case *ast.CompositeLit:
						if vals, ok := mapStringValues(v); ok {
							p.mapVar[name.Name] = vals
						}
					}
				}
			}
		}
	}
}

// lint walks every file's call expressions and checks wide-glyph helper calls.
func (p *pkg) lint() (vs []violation, us []unverified, cleared int) {
	for _, f := range p.files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !wideHelpers[sel.Sel.Name] {
				return true
			}
			name := sel.Sel.Name
			idx := glyphArgIndex[name]
			if idx >= len(call.Args) {
				return true
			}
			pos := p.fset.Position(call.Pos())
			arg := call.Args[idx]
			lits, resolved := p.resolveStrings(arg)
			if !resolved {
				us = append(us, unverified{pos: pos, call: name, expr: exprString(arg)})
				return true
			}
			for _, s := range lits {
				if base, why, bad := badBase(s); bad {
					vs = append(vs, violation{pos: pos, call: name, glyph: s, base: base, why: why})
				} else {
					cleared++
				}
			}
			return true
		})
	}
	return vs, us, cleared
}

// resolveStrings tries to resolve an expression to a set of string-literal
// values. Returns (values, true) when fully resolved syntactically, else
// (nil, false). It deliberately resolves only forms whose value is known with
// certainty from the AST — never guesses.
func (p *pkg) resolveStrings(e ast.Expr) ([]string, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if s, ok := litString(x); ok {
			return []string{s}, true
		}
	case *ast.Ident:
		if s, ok := p.strConst[x.Name]; ok {
			return []string{s}, true
		}
	case *ast.IndexExpr:
		// faceArt[s] — a constant index into a known map var checks ALL values
		// (we cannot know which key is selected, so every value must be safe).
		if id, ok := x.X.(*ast.Ident); ok {
			if vals, ok := p.mapVar[id.Name]; ok {
				return vals, true
			}
		}
	case *ast.ParenExpr:
		return p.resolveStrings(x.X)
	}
	return nil, false
}

// litString returns the Go-unquoted value of a string or char literal.
func litString(b *ast.BasicLit) (string, bool) {
	switch b.Kind {
	case token.STRING:
		s, err := strconv.Unquote(b.Value)
		if err != nil {
			return "", false
		}
		return s, true
	case token.CHAR:
		r, _, _, err := strconv.UnquoteChar(strings.Trim(b.Value, "'"), '\'')
		if err != nil {
			return "", false
		}
		return string(r), true
	}
	return "", false
}

// mapStringValues returns the string-literal values of a map composite literal
// whose every element value is a string/char literal. (nil, false) otherwise.
func mapStringValues(c *ast.CompositeLit) ([]string, bool) {
	mt, ok := c.Type.(*ast.MapType)
	if !ok {
		return nil, false
	}
	// Value type need not be named "string" syntactically, but every element
	// must BE a string/char literal for us to check it.
	_ = mt
	var out []string
	for _, el := range c.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			return nil, false
		}
		lit, ok := kv.Value.(*ast.BasicLit)
		if !ok {
			return nil, false
		}
		s, ok := litString(lit)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// badBase reports whether the first code point of s is an unsafe base for a
// wide-glyph helper (EAW not W/F), returning that base and a reason.
func badBase(s string) (rune, string, bool) {
	if s == "" {
		return 0, "empty glyph", true
	}
	rs := []rune(s)
	base := rs[0]
	if isWideOrFullwidth(base) {
		return base, "", false
	}
	return base, "East Asian Width is not Wide or Fullwidth", true
}

// exprString renders an expression compactly for diagnostics.
func exprString(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return exprString(x.X) + "." + x.Sel.Name
	case *ast.IndexExpr:
		return exprString(x.X) + "[" + exprString(x.Index) + "]"
	case *ast.CallExpr:
		return exprString(x.Fun) + "(…)"
	case *ast.BasicLit:
		return x.Value
	}
	return fmt.Sprintf("%T", e)
}
