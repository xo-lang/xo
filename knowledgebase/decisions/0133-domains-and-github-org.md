# 0133: Domains and GitHub org

Status: Accepted (2026-10-10). Domains bought and org registered by the
owner on 2026-10-10.

## Context
Xo needs a website and a public home for its code. The GitHub name
`xolang` belongs to an unrelated organization (created 2024-05-19), and
`xolang.com` is taken. The repo already uses `xo-lang` as its namespace:
the Go module path `github.com/xo-lang/xo` and the VS Code publisher
`xo-lang` (see [../infrastructure/namespaces.md](../infrastructure/namespaces.md)).

## Decision
- GitHub org: **`xo-lang`** (https://github.com/xo-lang), registered
  2026-10-10. The module path `github.com/xo-lang/xo` now points at a
  real org.
- Main website: **`xolang.dev`**. `.dev` is on the browsers' HSTS
  preload list, so the site, install scripts, and downloads are always
  served over HTTPS. Go uses the same split (`go.dev` with
  `github.com/golang`).
- Redirects to `xolang.dev`: `xolang.org`, `xo-lang.org`, `xo-lang.dev`,
  `xo-lang.com`. They catch both spellings and the common top level
  domains.

All five domains are registered at Namecheap, bought 2026-10-10.

## Consequences
- Docs, the README, and release notes link to `xolang.dev`; code and
  module paths use `github.com/xo-lang/...`.
- Pending: DNS and hosting for `xolang.dev`, and the redirects on the
  other four domains. Nothing is served yet.
- Pending: renewal reminders or auto renew on all five domains; a lapsed
  language domain is a supply chain risk (install scripts, vanity import
  paths).
- If Xo ever gets vanity import paths (`xolang.dev/...`), they belong on
  this domain; that is a separate decision.

## Alternatives
- `xo-lang.org` as the main site (Rust's pattern, `rust-lang.org` with
  `github.com/rust-lang`): matches the org exactly, but `.dev` forces
  HTTPS and reads as a developer tool. Kept as a redirect.
- `xolang.org` as the main site: plain HTTP stays possible; kept as a
  redirect.
- GitHub `xolang`: taken.
