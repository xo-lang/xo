# Website (xolang.dev)

Last verified: 2026-10-10

The site is a separate repo (decision 0139),
[github.com/xo-lang/xolang.dev](https://github.com/xo-lang/xolang.dev),
locally at `~/godev/claude-space/xolang/website`. Its `README.md` has the
full layout.

Live at https://xolang.dev since 2026-10-10: GitHub Pages from Actions,
custom domain `xolang.dev`, HTTPS enforced (certificate approved, expires
2027-01-08, renewed by GitHub). DNS at Namecheap: A records on `@` to
185.199.108.153, .109, .110, .111.153 and `www` CNAME `xo-lang.github.io.`;
`http://` and `www` redirect 301 to `https://xolang.dev/`. The other four
domains (decision 0135) still show Namecheap's default 302 to `www`; their
301 redirects to `https://xolang.dev` are pending.

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
