# games — repo guide for Claude

The PUBLIC shellcade game catalog: third-party games live under
`games/<shellcade-username>/<game-slug>/` (the ARCADE-facing identity) and
arrive by pull request. Namespace ownership is authorized against each author's
in-arcade GitHub link via the shellcade API (`authorize.yml` POSTs the PR
author + path to shellcade.com, which answers whether they may publish there);
maintainer review is the gate while that endpoint's secret is being rolled out.

## Hard rules

- **NEVER add shellcade-internal material here** — no OpenSpec artifacts, no
  internal specs/designs, no references to private packages or infra. Platform
  design lives in the private repo only.
- Never commit build artifacts; CI builds what gets published and rejects wasm, native executable, Rust `target/`, smoke output, and other build outputs.
- A PR may only touch `games/<shellcade-username>/...` whose shellcade account
  is linked to the PR author's GitHub login (the `authorize.yml` API check
  enforces this once its secret is set; until then, verify the in-arcade link
  during review). Admins may publish to any namespace.
- Review bar: play it (`shellcade-kit play`), read it (it runs sandboxed but we
  host it), and check `LICENSE`, `smoke.yaml`, metadata/path agreement, artifact cleanliness, and slug uniqueness.
