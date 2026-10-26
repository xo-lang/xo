# net/http.ReadRequest is not the server layer

**Observed** 2026-10-26.

## Symptom

A differential of http1's request head parser (`parse_head`) against
`net/http.ReadRequest` on the same raw bytes reports the parser refusing
heads Go reads. Three classes, all in the same shape: a header name with a
space (`X Y: v`, `X-Fold : v`, a trailing space), a head without `Host`, and
a Host folded into `example.com v`. Moving the fuzz to a valid Host and a
token-only name oracle took 300 seeded cases from 125 failures to zero.

## Cause

`parse_head` is the server's parser (its `head_of` is the port of
`net/http`'s `conn.readRequest` plus `httpguts` checks), and `ReadRequest`
alone is the client-shaped reader. The extra server checks are:

- `Host` must be present for HTTP/1.1 and well formed (`400 missing
  required Host header`, `400 malformed Host header`);
- a header name must be a token (`400 invalid header name`); textproto's
  reader takes a name with a space and keeps it.

Comparing `parse_head` with `ReadRequest` therefore compares two layers,
and every server-only check reads as a parser divergence.

## The response side

A differential of `parse_response` against `net/http.ReadResponse` hit the
same trap from the other end (`internal/stdtest/httprespfuzz_test.go`): two
of `RespHead`'s fields are deliberately not Go's public ones.

- `clen` is readTransfer's `realLength`, not `resp.ContentLength`: a HEAD
  response, a 1xx, 204 or 304 has 0 there even when Content-Length says 5,
  and chunked is -1.
- `close` is `shouldClose`, not `resp.Close`: Go also sets `resp.Close` for
  an unbounded body, while the port keeps framing apart (`Exchange.keep`).

Both differed in the first 300 seeded cases and read as parser
divergences; the oracle now derives them from the head the way the port
does. A folded `Connection` (a continuation line after it) is a third:
the token list must come from the folded field (`close "q"` is not
`close`), so the oracle reads the block with textproto rather than line by
line.

## Fix

In a differential oracle, apply the server's rules on the Go side too:
reject a name whose bytes are not tchars (`httpguts.ValidHeaderFieldName`),
and keep the generated heads' `Host` valid and unfoldable (a fixed
`Host: example.com`, and a dummy field after it so an obs-fold leader or an
empty line does not land on Host). `internal/stdtest/httpheadfuzz_test.go`
does both, with the reason written at each site. The server's Host and name
validation itself stays a separate row: to test it, drive Go's real server
over loopback rather than `ReadRequest`.
