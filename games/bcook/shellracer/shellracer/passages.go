package shellracer

import (
	_ "embed"
	"math/rand"
	"strings"
)

// passagesRaw is the native passage corpus, copied verbatim (tab-separated
// "difficulty\ttext" lines). TinyGo embeds string data fine, so the corpus
// ships in the artifact unchanged.
//
//go:embed passages.txt
var passagesRaw string

type passage struct {
	Text       string
	Difficulty string
}

var passages []passage

func init() {
	for _, line := range strings.Split(passagesRaw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		passages = append(passages, passage{Difficulty: parts[0], Text: parts[1]})
	}
}

// pickPassage selects a passage deterministically from the room's RNG so the
// choice is reproducible (room-seeded; hibernation-stable).
func pickPassage(r *rand.Rand) passage {
	if len(passages) == 0 {
		return passage{Text: "the quick brown fox jumps over the lazy dog", Difficulty: "easy"}
	}
	return passages[r.Intn(len(passages))]
}
