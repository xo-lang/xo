# 0303: The native HTTP/1.1 client keeps connections alive in a pool (Go's http.Transport)

Status: Proposed (Xo Lang 3, 2026-10-21; stub, filled in on its branch)

## Context

The native std/http client opens a connection for every HTTP/1.1
request and closes it after the response; the Go backend's client (Go's
http.Transport) keeps idle connections and reuses them.

## Decision

(to follow)

## Consequences

(to follow)

## Alternatives

(to follow)
