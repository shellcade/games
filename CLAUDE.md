# games — repo guide for Claude

The PUBLIC shellcade game catalog: third-party games live under
`games/<shellcade-username>/<game-slug>/` (the ARCADE-facing identity) and
arrive by pull request. `authors.toml` maps each shellcade username to its
linked GitHub login (interim — a shellcade.com API will replace it); CI
enforces that only the mapped login can touch a namespace.

## Hard rules

- **NEVER add shellcade-internal material here** — no OpenSpec artifacts, no
  internal specs/designs, no references to private packages or infra. Platform
  design lives in the private repo only.
- Never commit built `.wasm` artifacts; CI builds what gets published.
- A PR may only touch `games/<shellcade-username>/...` whose `authors.toml`
  entry maps to the PR author's GitHub login (CI enforces; maintainers can
  override for repo plumbing). First-game PRs add their own entry; verify the
  in-arcade link before merging it.
- Review bar: play it (`shellcade-kit play`), read it (it runs sandboxed but we
  host it), and check `game.toml` license + slug uniqueness.
