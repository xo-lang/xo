# Website (xolang.dev)

Last verified: 2026-10-10

The site is a separate repo (decision 0139), locally at
`~/godev/claude-space/xolang/website`; not yet pushed (target
`github.com/xo-lang/xolang.dev`). Its `README.md` has the full layout.

| Task | Command (in the site repo) |
|---|---|
| Install Hugo | `CGO_ENABLED=0 GOBIN=$PWD/.tools/bin go install github.com/gohugoio/hugo@v0.167.0` (no Homebrew bottle on macOS 27) |
| Preview | `.tools/bin/hugo server` (http://localhost:1313) |
| Build | `.tools/bin/hugo --gc --minify` into `public/` |
| Pull spec and release notes from this repo | `python3 scripts/sync-content.py` (default `../../codelang`, or `$XO_REPO`) |
| Publish | push to `main`; `.github/workflows/pages.yml` deploys to GitHub Pages |
| Change the orange announcement bar | `params.announcement` in `hugo.toml` |

What comes from this repo: `spec/core.md`, `spec/std.md`, and each
`## vX.Y.Z (date)` section of `CHANGELOG.md` (one blog post per release).
Keep that heading format, and keep `<...>` code spans on one line in the
spec (gotcha spec-markdown-html-tag-line-start). Agents can read
`/llms.txt` and any page as Markdown at `<page>/index.md`.
