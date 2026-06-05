# shellcade games — the arcade catalog

Games hosted on [shellcade.com](https://shellcade.com) live here. **Submitting
a game is a pull request.**

## Submit your game

1. Build it with the [kit](https://github.com/shellcade/kit) — start with
   [GUIDE.md](https://github.com/shellcade/kit/blob/main/GUIDE.md):

       shellcade-kit new mygame && cd mygame && go run .

2. **Link your GitHub account to your shellcade account** (one-time):
   `ssh shellcade.com` → User → *Link GitHub*. You claim your GitHub username
   and prove it with an SSH key GitHub knows — instant if you use the same key
   for both, otherwise one command (`ssh link@shellcade.com <code>`). The link
   is what authorizes your catalog namespace and puts your handle on the lobby.

3. When `shellcade-kit check mygame.wasm` passes, open a PR adding:

       games/<your-shellcade-username>/<game-name>/
       ├── game.toml          # name, display_name, players, description, license
       ├── main.go …          # your game's source (source is required)
       └── go.mod

   **Your slug is the path.** The catalog identity is
   `<your-shellcade-username>/<game-name>`, composed from the directory the
   game lives in. `game.toml` carries the **bare name only** (`[a-z0-9-]`, no
   slash) and it must match the directory name; the platform adds your
   namespace. Names are unique per author. See [SCHEMA.md](SCHEMA.md) for the
   full `game.toml` schema (validated by CI).

   Add yourself to `authors.toml` (`<shellcade-username> = "<github-login>"`)
   in the same PR if this is your first game — a maintainer verifies the link
   in-arcade before merging. (This file is interim; an API will replace it.)

4. CI verifies: the path's shellcade username maps to **your** GitHub login,
   `game.toml` matches the [schema](SCHEMA.md), the game builds (TinyGo), and
   `shellcade-kit check` passes — the same gate the arcade runs.
5. A maintainer reviews and merges. Merge = accepted into the catalog;
   an arcade operator then flips it live, attributed to your handle.

## Rules

- Source is required and licensed for the arcade to build and host (MIT or
  compatible; declare in `game.toml`).
- One directory per game, under your own username. CI rejects PRs that touch
  other authors' games.
- Keep artifacts out of the repo — CI builds the `.wasm` it publishes.
