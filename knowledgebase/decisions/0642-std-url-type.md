# 0642: std/url gets Go's URL type

Status: Accepted (Xo Lang 3, 2026-11-14, on the lead's order to close
std/url's features gap before its Proven evidence)

## Context

std/url's features row (net/url) was partial: escaping and Values were
done (decision 0279), but there was no full URL type. std/http's Url
holds scheme, host, port, path, a single valued query and userinfo, with
no fragment, opaque form, raw path or relative references; the private
runtime parser behind it (std/native/llrt/neturl) is native only and
keeps what the server and client need. Decision 0616's order puts the
features axis before testing and performance evidence (lead,
2026-11-14): a feature push changes the source hash.

## Decision

std/url gets `Url`, `Userinfo` and `UrlErr`, a port of url.go of go1.26
(spec std 20.1):

- `parse`, `parse_request_uri`, `join_path`, and the methods `display`
  (String), `redacted`, `is_abs`, `escaped_path`, `escaped_fragment`,
  `parse_ref` (URL.Parse), `resolve_reference`, `query`, `request_uri`,
  `hostname`, `port`, `join_path`. Field names are Go's in snake case;
  `user` is `Userinfo?` and a password `Str?` (None is Go's password
  not set). The Url fields have defaults, so `Url{host: h}` builds one.
- Go's results and error texts. `UrlErr` displays as *url.Error
  (`parse "URL": reason`); join_path's path error has an empty `op`,
  since Go returns it bare.
- urlstrictcolons at Go 1.26's default (strict for http and https).
- Pure Xo in std/url.xo, which every backend runs (the Go backend
  through stdsync's generated file). It uses std/netip for IP literals
  and its own copy of path.Clean: code generated into the Go backend's
  runtime cannot call std/path, which that backend builds as a program
  package.
- The tests are Go's: scripts/genurltest copies net/url's sources and
  test file from the Go toolchain, lifts the tables Go declares inside
  test functions to package variables, and prints one Xo test per case
  with Go's answer (529 cases).

http.Url stays as it is: the server and client build it on every
request from the runtime parser, and its shape (a port as Int?, a
single valued query Map) is what handlers read. A program that needs
the full form parses `req.url.display()`, or the raw target, with
`url.parse`.

## Consequences

- std/url's features row is done, so its Proven evidence is rerun once
  at the new source hash (within decision 0616's grace).
- Two URL types exist, http.Url and url.Url; spec std 20.1 names the
  difference. Merging them would change every handler's field reads.
- url.Url.query drops a pair that does not decode, as URL.Query does;
  parse_query returns the error.

## Alternatives

- Expose the runtime parser (neturl) as std/url's type: native only, so
  the Go backend and wasm would lack it, and it drops the opaque form,
  the fragment's raw text and the user's escapes.
- Grow http.Url into the full type: every server request would build
  the larger value, and its field types would change under handlers.
- Leave the row partial: decision 0616 needs the features axis first.
