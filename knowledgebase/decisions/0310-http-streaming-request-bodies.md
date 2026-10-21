# 0310: Streaming request bodies in std/http

Status: Accepted (owner, 2026-10-21, within decision 0276: std follows
Go's standard library; Xo Lang 3; stub, filled in on its branch)

## Context

A std/http handler gets the whole request body in memory
(`Request.body`, capped by `XO_HTTP_MAX_BODY`); Go's handlers read
`r.Body` as a stream. Uploads larger than memory, and multipart parts
read as they arrive, need the stream.

## Decision

(to follow)

## Consequences

(to follow)

## Alternatives

(to follow)
