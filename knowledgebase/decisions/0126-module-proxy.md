# 0126: Module proxy protocol and XO_PROXY

Status: Accepted (2026-10-10), implemented. Builds the optional proxy of
decision 0086. Code: `internal/mod/fetch.go` (sources, `Download`,
`Versions`), `internal/mod/proxy.go` (client, `ProxyHandler`,
`ZipModule`), `cmd/xo/mod.go` (`xo mod proxy serve`). Tests:
`internal/mod/proxy_test.go` (TestProxy, TestProxyUntrusted,
TestProxyServe), `cmd/xo/proxy_test.go` (TestModProxy). Spec: core 4.2.

## Context

Decision 0086 fetches every module version with git from its location
and names "an optional proxy" as a later service. Teams want one: a
cache that keeps building when an upstream host is down or a repository
disappears, fewer clones on CI, and a single egress point. Go's GOPROXY
protocol is known, simple (static files over HTTP), and its fallback
list semantics are already understood by the people who will run Xo.

## Decision

- `XO_PROXY` is a list as Go's GOPROXY: proxy URLs (`http://` or
  `https://`), `direct` (git, decision 0086), and `off` (no fetching).
  After a comma the next source is tried only when the previous one
  answered 404 or 410; after a pipe, on any error. Default `direct`:
  there is no public Xo proxy, so nothing leaves the machine unless a
  team sets one. `XO_NOPROXY` (comma separated path globs, matched
  against as many leading path elements as the pattern has, as Go's
  GOPRIVATE) always goes direct.
- Protocol, read only, below the base URL, path and version escaped as
  in the module cache (`!` plus the lower case letter):
  `<path>/@v/list` (versions, one per line), `<path>/@v/<v>.mod` (the
  xo.mod), `<path>/@v/<v>.zip` (the tree, every file under
  `<path>@<v>/`, as Go's zips), `<path>/@v/<v>.lock` (the two xo.lock
  lines). Go's `.info` and `@latest` are left out: Xo has no
  pseudo-versions and computes latest from the list.
- The zip holds exactly the tree a git fetch produces (nested modules
  pruned), so the xo.lock hash does not depend on the source; the
  client refuses entries outside the prefix, non regular files, paths
  leaving the module, case-insensitive duplicates, and more than 500 MB.
- A proxy is untrusted. The fetched tree goes through the same checks
  as a git fetch: it must declare the module path, it must match
  xo.lock on every load (XO1304), and with a checksum log it is
  verified on first fetch (decision 0124). The `.lock` answer only
  catches a proxy that contradicts itself (XO1302).
- `xo mod proxy serve [--addr] [--fetch] [<cache dir>]` serves a module
  cache directory (the XO_MODCACHE layout, default the user's cache);
  zips are built on request, deterministically (sorted, fixed times).
  `--fetch` fills misses from git (always direct, so proxies cannot
  loop): a team's caching proxy. It binds 127.0.0.1:7070 by default.
- Problems: a version no source has, `off`, or a bad zip is XO1302 (its
  meaning, "cannot be fetched", is unchanged; its description now
  names proxies); a malformed `XO_PROXY` is XO1314.

## Consequences

- Builds work from a cache that outlives upstream repositories; a CI
  fleet can point at one proxy and keep git out of the build image.
- The proxy cannot change what is built without being caught: xo.lock
  (and the checksum log) pin the content.
- `xo serve` forwarding is skipped when any of XO_PROXY, XO_NOPROXY,
  XO_SUMDB, XO_NOSUMDB is set, as for the other module variables.

## Alternatives

- Git mirrors only (XO_GIT_REWRITE, already there): needs a git server
  and full histories; no zip cache, no fallback list.
- Go's protocol byte for byte (`.info`, `@latest`, Go's zip hash):
  Xo's hash is over the extracted tree (core 4.3), and pseudo-versions
  do not exist; the extra endpoints would be dead weight.
- Proxy authentication, `file://` proxies, and serving the checksum log
  through the proxy (Go's `/sumdb/` passthrough): not built; see the
  open items in the toolchain notes.
