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
       ├── go.mod             # every game is a standalone module
       ├── main.go …          # your game's source (source is required)
       ├── LICENSE            # MIT, Apache-2.0, BSD-3-Clause, MPL-2.0, or Unlicense
       └── CHANGELOG.md       # optional: top section becomes the release notes

   **Your slug is the path.** The catalog identity is
   `<your-shellcade-username>/<game-name>`, composed from the directory the
   game lives in. There is no manifest: metadata lives in your game's `Meta()`
   and CI reads it from the built artifact (`shellcade-kit meta`) — the bare
   name it reports (`[a-z0-9-]`, no slash) must equal the directory name; the
   platform adds your namespace. Names are unique per author. See
   [SCHEMA.md](SCHEMA.md) for the directory contract (validated by CI).

   PRs are authorized against your in-arcade GitHub link via the shellcade API:
   CI asks shellcade.com whether your PR's GitHub author is linked to the
   shellcade username in the path, and only your own `games/<your-handle>/`
   namespace is yours to publish (admins may publish anywhere). If you haven't
   linked yet, the check posts the linking steps on your PR. While the API is
   being rolled out, the check is skipped and maintainer review is the gate.

4. CI also verifies: the directory meets the [contract](SCHEMA.md), the game
   builds (TinyGo), `shellcade-kit check` passes — the same gate the arcade
   runs — and the artifact's own metadata agrees with the path.
5. A maintainer reviews and merges. Merge = accepted into the catalog;
   an arcade operator then flips it live, attributed to your handle.

## Rules

- Source is required and licensed for the arcade to build and host (MIT or
  compatible; a `LICENSE` file from the allowlist).
- One directory per game, under your own username. CI rejects PRs that touch
  other authors' games.
- Keep artifacts out of the repo — CI builds the `.wasm` it publishes.
- Hosted content must meet the [content policy](CONTENT_POLICY.md) — it also
  documents how to report a game and how takedowns work (operators can pull a
  game offline immediately; the catalog entry follows by PR).
