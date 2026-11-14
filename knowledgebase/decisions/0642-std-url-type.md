# 0642: std/url gets Go's URL type

Status: Proposed (Xo Lang 3, 2026-11-14)

## Context

std/url's features row (net/url) is partial: escaping and Values are
done (decision 0279), but there is no full URL type. std/http's Url
holds scheme, host, port, path, a single valued query and userinfo, with
no fragment, opaque form, raw path or relative references. Decision
0616's order puts the features axis before testing and performance
evidence (lead, 2026-11-14).

## Decision

To be filled: a port of Go's url.URL into std/url as `Url` (parse,
parse_request_uri, display, resolve_reference, is_abs, hostname, port,
query, escaped_path, join_path, Userinfo, redacted), with Go's tests,
and how it relates to http.Url.

## Consequences

To be filled.

## Alternatives

To be filled.
