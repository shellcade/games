# shellcade games — the arcade catalog

Games hosted on [shellcade.com](https://shellcade.com) live here. **Submitting
a game is a pull request.**

## Submit your game

1. Build it with the [kit](https://github.com/shellcade/kit) — start with
   [GUIDE.md](https://github.com/shellcade/kit/blob/main/GUIDE.md):

       shellcade-kit new mygame && cd mygame && go run .

2. When `shellcade-kit check mygame.wasm` passes, open a PR adding:

       games/<your-github-username>/<game-slug>/
       ├── game.toml          # name, slug, players, description
       ├── main.go …          # your game's source (source is required)
       └── go.mod

3. CI verifies: the path matches **your** GitHub username, the game builds
   (TinyGo), and `shellcade-kit check` passes — the same gate the arcade runs.
4. A maintainer reviews and merges. Merge = accepted into the catalog;
   an arcade operator then flips it live.

**Going live requires linking your GitHub account to your shellcade account**
(in the arcade: `ssh shellcade.com` → User → *Link GitHub* — a github.com/login/device
code flow). That link is what puts your handle on the lobby page and makes you
accountable for what players run.

## Rules

- Source is required and licensed for the arcade to build and host (MIT or
  compatible; declare in `game.toml`).
- One directory per game, under your own username. CI rejects PRs that touch
  other authors' games.
- Keep artifacts out of the repo — CI builds the `.wasm` it publishes.
