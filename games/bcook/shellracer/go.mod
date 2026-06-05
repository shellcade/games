module shellracer.shellcade.example

go 1.25.0

require github.com/shellcade/kit/v2 v2.0.0

require (
	github.com/extism/go-pdk v1.1.3 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/term v0.43.0 // indirect
)

// TODO(add-frame-diffing-v2): replace with the released pin once kit v2.0.0 ships
// (`go get github.com/shellcade/kit/v2@v2.0.0`); this local-dev replace is
// temporary scaffolding against the frame-diffing kit worktree.
replace github.com/shellcade/kit/v2 => ../../../../kit
