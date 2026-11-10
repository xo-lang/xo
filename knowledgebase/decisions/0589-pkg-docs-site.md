# 0589: pkg.xolang.org, package documentation for std and published modules

Status: Accepted (owner, 2026-11-10)

## Context

Go has three places for its ecosystem: go.dev (the language site),
pkg.go.dev (documentation of every published package, for people), and
proxy.golang.org with sum.golang.org (services for the go command). gopls's
hover links each symbol to its pkg.go.dev page.

Xo has the first and third: xolang.dev (docs, tour, playground, and one page
per std module under /docs/stdlib/) and, since decision 0568, the module
proxy, index and checksum log on xolang.org (proxy.xolang.org,
index.xolang.org, sum.xolang.org). Published modules have no documentation
site: a reader of github.com/xo-lang/aws, azure or gcp (v0.1.0 with Xo
0.46.0) reads the source or the README. The owner asked (2026-11-10) for a
pkg.go.dev equivalent after seeing gopls's hover link; xo lsp is gaining a
hover link to the std pages in the meantime.

`xo doc [--json] [--all] [module]` already renders a module's doc comments,
and decision 0588 merges std's declarations and bodies into one file per
module, so std and user modules document the same way.

## Decision (proposed)

1. **Name: pkg.xolang.org** (revised 2026-11-10). The first draft chose
   pkg.xolang.dev to keep people-facing pages under the .dev brand, but
   xolang.dev stays on Namecheap DNS (owner decision), and an R2 custom
   domain, a Worker route and Cloudflare's cache rules need the zone on
   Cloudflare. xolang.org is already a Cloudflare zone (0568), so
   pkg.xolang.org is served like proxy.xolang.org with no new zone. The
   .dev name could still point at Cloudflare Pages through a Namecheap
   CNAME, but Pages caps a deployment's file count (20,000 on the Free
   plan), which a site with a page per module version outgrows.
2. **What it shows**, per module version: the module's doc, its packages'
   exported types, functions, methods, constants and their doc comments
   (from `xo doc --json`), the signature with effects and errors, links to
   the source lines at the tagged commit, the versions list, the imports and
   importers known to the index, the license (detected from the module's
   LICENSE file; no license, no rendered docs, as pkg.go.dev does), and
   the `xo get` line. Std is a module like any other, versioned by Xo
   release (pkg.xolang.org/std/charset@0.46.0), and /docs/stdlib on
   xolang.dev redirects to it.
3. **How it is built:** static pages. A builder reads index.xolang.org's
   feed for new versions, fetches each zip through proxy.xolang.org
   (verified against sum.xolang.org), runs `xo doc --json` from the
   released xo in a sandbox with no network, and renders HTML plus a
   search index. Built pages are written to an R2 bucket served on
   pkg.xolang.org through Cloudflare, like the proxy. The builder never
   runs a module's code (doc extraction parses and checks only).
4. **Where the builder runs: a scheduled GitHub Actions workflow in a
   public repository** (owner, 2026-11-10), on GitHub-hosted runners, which
   are free for public repositories; never the OCI host (hosting only,
   0568) and not Cloudflare Containers (Workers Paid, $5 a month minimum,
   kept as the fallback). A new public repository (xo-lang/pkgsite, created 2026-11-10 by the lead
   at the owner's request; Actions' default token is read only) holds the builder and the
   workflow, so xo-lang/xo's history and permissions stay separate.
   - Two jobs: `build` has no secrets and no write token; it reads the
     index feed, fetches and verifies each new version, runs the released
     xo's `xo doc --json` and renders the pages into an artifact. `publish`
     downloads the artifact and writes it to the R2 bucket with a token
     scoped to that bucket only, stored as an Actions secret. Untrusted
     module source is only parsed in `build`, never where the token is.
   - Triggers: `schedule` (every 30 minutes) and `workflow_dispatch`;
     never `pull_request_target`, and pull requests from forks get no
     secrets. Actions are pinned by commit hash.
   - GitHub disables scheduled workflows in a repository with no activity
     for 60 days: the workflow commits its last-built index position (or
     a heartbeat) so the repository stays active, and a missed run is
     caught up from the index position on the next one.
   - Scheduled runs can start late under load; a delay of up to an hour
     before a new version's page appears is acceptable.
5. **Editors:** xo lsp's hover link points at pkg.xolang.org for std and
   for any module the index knows; user code in the open workspace gets no
   link.
6. **Takedowns and abuse** follow 0568's policy and contact
   (modules@xolang.org): a module removed from the proxy is removed here.
7. **Stages:** (a) std only, generated at each release from the release's
   std, replacing /docs/stdlib; (b) published modules from the index, with
   xo-lang/aws, azure and gcp first; (c) search across modules and symbols;
   (d) importers and the "used by" count.

## Consequences

- Module authors get documentation pages without doing anything but
  tagging a version, which rewards doc comments.
- One more public service to run: the builder repository and its
  workflow, an R2 bucket, DNS
  for pkg.xolang.org (a record in the existing Cloudflare zone), and a release step for std's pages.
- xolang.dev/docs/stdlib's URLs change; redirects keep old links working.
- `xo doc --json` becomes a public format, versioned with a schema field.

## Alternatives

- **Keep std pages on xolang.dev and nothing for modules:** no cost, but
  third-party modules stay undocumented.
- **pkg.xolang.dev:** keeps people-facing pages under one brand, but
  xolang.dev stays on Namecheap DNS, so only a Cloudflare Pages CNAME could
  serve it (file cap above).
- **A path on the main site (xolang.dev/pkg/...):** one site, but the main
  site's Hugo build would depend on the module index and grow with every
  published module.
- **Render on request (a Worker running xo doc per hit):** no build queue,
  but slow first hits and a heavier sandbox on the request path.

## Open questions for the owner

- Accept the name and the stages.
- The builder's home (decision item 4).
- Whether pkg.xolang.org also hosts the module search that `xo search`
  might call later.
