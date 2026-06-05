# games — repo guide for Claude

The PUBLIC shellcade game catalog: third-party games live under
`games/<github-user>/<game-slug>/` and arrive by pull request.

## Hard rules

- **NEVER add shellcade-internal material here** — no OpenSpec artifacts, no
  internal specs/designs, no references to private packages or infra. Platform
  design lives in the private repo only.
- Never commit built `.wasm` artifacts; CI builds what gets published.
- A PR may only touch `games/<author>/...` where `<author>` is the PR author's
  GitHub login (CI enforces; maintainers can override for repo plumbing).
- Review bar: play it (`shellcade-kit play`), read it (it runs sandboxed but we
  host it), and check `game.toml` license + slug uniqueness.
