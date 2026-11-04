# A proxied request without a body must not carry a body source

Status: Observed (2026-11-03, Xo Lang 1).

## Symptom

http_reverse_proxy_ws failed about 1 run in 100 under the full arm64
golden load on linux/arm64: the proxy logged `protocol error: Get
"http://127.0.0.1:PORT/bye": EOF` (a 502), or `dial tcp ...: connection
refused`, or the WebSocket client saw a bad handshake with no proxy log.
Alone it passed hundreds of times.

## Cause

http.reverse_proxy passed every incoming request's BodyReader to its
client (with_body_reader), also for a GET with no body. A request with a
body source is not replayable, so when the pooled connection it took had
just been closed by the target, the client's retry on a new connection
(decision 0303, Go's shouldRetryRequest) was skipped. It also went out
as `Transfer-Encoding: chunked` with an empty chunk, which Go never
sends for a bodiless request. Load decides whether the target's close
of the idle connection lands before the next request reuses it.

## Fix

The server marks a stream request it read no body for (http1.body_none:
Content-Length 0, or neither it nor chunked; BodyReader.empty), and the
proxy sends such a request with no body, as httputil sets
`outreq.Body = nil` when ContentLength is 0; std/rt's proxy does the
same with net/http's NoBody. Golden http_reverse_proxy_retry: a target
that closes a kept connection on the next request. When a proxy or
client forwards a body, ask whether an empty one should be none.

## Follow-up (2026-11-03, Observed): an upgrade needs its source

The first fix also dropped the source of a proxied upgrade (a WebSocket
GET has no body). Without a source the client makes no full duplex
exchange (no dsrv), so when the 101 tunnel's target side ends,
end_conn frees the connection (hclient.end_keep) while tunnel's
client-to-target task still writes on it: a use-after-free that
trapped on wasip2 ("unknown handle index", http_reverse_proxy_ws as a
command, 5 of 5) and passed silently on native, XO_RC_CHECK=1
included. An upgrade now keeps its source. Before dropping a request's
source, check what else its presence selects (here the duplex path and
end_shared).
