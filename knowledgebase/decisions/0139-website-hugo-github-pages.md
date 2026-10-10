# 0139: Website on Hugo and GitHub Pages

Status: Accepted (2026-10-10). Chosen by the owner (design and Hugo).

## Context
Xo needs a public site at `xolang.dev` (decision 0135): a home page, docs,
the spec, release notes, and a way for agents to read all of it. The owner
compared five mockups (three styles, then two logo pairs) and picked the dark,
terminal style "Night Watch" with the soft fox pair: the alert head
(`03-soft-style-v1`) and the curled sleeping fox (`07-curled`).

## Decision
- **Repository**: the site lives in its own repo, locally
  `~/godev/claude-space/xolang/website` (to be pushed as
  `github.com/xo-lang/xolang.dev`), not in this one.
- **Generator**: Hugo v0.167.0, built from source with `go install`
  (Homebrew has no bottle for macOS 27). No theme; own layouts and one
  stylesheet. Hugo is a single Go binary, builds the site in about 30 ms,
  and needs no Node toolchain.
- **Hosting**: GitHub Pages through Actions (`.github/workflows/pages.yml`),
  `static/CNAME` is `xolang.dev`.
- **Content from this repo**: `scripts/sync-content.py` copies
  `spec/core.md`, `spec/std.md`, and each `## vX.Y.Z` section of
  `CHANGELOG.md` (one blog post per release) into the site, with front
  matter. Generated files are committed there, so the site builds without
  access to this repo.
- **For agents**: `/llms.txt` indexes the docs, and every page is also
  served as raw Markdown at `<page>/index.md`.
- **Brand on the site**: the title is the text "Xo Language" (no logo in
  the header or footer); Finan appears in the hero and the "zero errors"
  section; an orange announcement bar (`params.announcement` in
  `hugo.toml`) carries important notices. Xo is introduced as pronounced
  "ex-oh".

## Consequences
- After a spec or CHANGELOG change that should be public, run
  `python3 scripts/sync-content.py` in the site repo and push it.
- The spec is now rendered by a Markdown engine: a wrapped line inside a
  code span that starts with an HTML tag name (`<header>`) becomes raw HTML
  (see gotcha spec-markdown-html-tag-line-start).
- Pending: push the site repo, enable Pages (Actions source) and the custom
  domain, the four redirect domains (0135), and the `xo-lang/xo` repo the
  docs link to.
- Later: Xo syntax highlighting (Hugo's Chroma has no Xo lexer; code
  blocks are plain), a page per diagnostic code, a WebAssembly playground.

## Alternatives
- **Astro Starlight**: strong docs features (search, sidebar), but a Node
  toolchain for a Go project.
- **Hand written HTML**: no build step, but the spec and release notes
  would be copied by hand.
- **Site inside this repo** (`website/`): one repo, but site commits would
  mix with compiler history and every site push would run the compiler CI.
- Mockups not chosen: "Oasis" (light editorial), "Facets" (bold angular),
  and the geometric fox pair.
