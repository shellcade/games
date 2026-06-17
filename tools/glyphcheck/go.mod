// glyphcheck is a standalone, stdlib-only tool module (go/parser + go/ast).
// It intentionally has NO dependencies so `go run ./tools/glyphcheck` works
// offline in fork CI with no module download. See main.go for the lint and its
// convergence with the kit's `shellcade-kit check`.
module github.com/shellcade/games/tools/glyphcheck

go 1.24
