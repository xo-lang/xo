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
| Pull spec, release notes, and diagnostic code pages from this repo | `python3 scripts/sync-content.py` (default `../../codelang`, or `$XO_REPO`; builds `cmd/xo` for `xo explain --json` unless `$XO` is set) |
| Re-verify the tour's programs and record their output | `XO=path/to/xo scripts/check-examples.sh` (each `examples/tour/*` must check, run, and pass `xo test`; each `examples/mistakes/*` must fail `xo check`) |
| Publish | push to `main`; `.github/workflows/pages.yml` deploys to GitHub Pages |
| Change the orange announcement bar | `params.announcement` in `hugo.toml` |

What comes from this repo: `spec/core.md`, `spec/std.md`, each
`## vX.Y.Z (date)` section of `CHANGELOG.md` (one blog post per release),
and one page per diagnostic code (the spec's 10.6 table plus
`xo explain --json`, so explain texts are public; placeholders like
`<dir>` are escaped by the script). The tour (12 steps) is hand written in
the site repo; after a language change, rerun `check-examples.sh` there.
The source repo is announced as "not public yet" until
`params.sourcePublic = true` in `hugo.toml`.
Keep that heading format, and keep `<...>` code spans on one line in the
spec (gotcha spec-markdown-html-tag-line-start). Agents can read
`/llms.txt` and any page as Markdown at `<page>/index.md`.
