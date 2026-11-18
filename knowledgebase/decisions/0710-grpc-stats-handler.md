# 0710: std/grpc stats handlers (per-message and connection events)

Status: Accepted (xo-4c, 2026-11-17)

## Context

grpc-go's `stats.Handler` (`grpc.StatsHandler` on a server, `grpc.WithStatsHandler` on a client; v1.79.2
stats/stats.go) sees every call begin and end, each header, payload and trailer with its sizes, and each
connection open and close. OpenTelemetry's and OpenCensus's grpc instrumentation are built on it. std/grpc has
interceptors (decision 0703), `with_on_finish`, and message interceptors (decision 0709): a call's begin and end
and its messages, but no sizes, no header or trailer events and no connection events. map.txt marks
grpc.StatsHandler and grpc.WithStatsHandler partial (task #236).

## Decision

1. One event enum after grpc-go's event structs, with Xo values in place of Go's (a `Map[Str, Str]` for metadata,
   `Status?` for the error, `Instant` for times, the address as `Str`):

   ```
   enum Stats {
     Begin(call: Int, client: Bool, method: Str, client_stream: Bool, server_stream: Bool, at: Instant)
     OutHeader(call: Int, client: Bool, method: Str, header: Map[Str, Str], compression: Str, remote: Str)
     InHeader(call: Int, client: Bool, method: Str, header: Map[Str, Str], compression: Str, remote: Str, wire_length: Int)
     OutPayload(call: Int, client: Bool, length: Int, compressed_length: Int, wire_length: Int, at: Instant)
     InPayload(call: Int, client: Bool, length: Int, compressed_length: Int, wire_length: Int, at: Instant)
     OutTrailer(call: Int, client: Bool, trailer: Map[Str, Str])
     InTrailer(call: Int, client: Bool, trailer: Map[Str, Str], wire_length: Int)
     End(call: Int, client: Bool, status: Status?, trailer: Map[Str, Str], begin: Instant, end: Instant)
     ConnBegin(client: Bool, remote: Str, local: Str)
     ConnEnd(client: Bool, remote: Str, local: Str)
   }
   ```

   `call` is a number unique within the client or server: it stands in for the context grpc-go's TagRPC returns,
   so a handler can pair a call's events. grpc-go's `Payload any` is left out (a handler that needs the message
   uses a message interceptor, decision 0709); `DelayedPickComplete` and `IsTransparentRetryAttempt` are left out
   (no transparent retries); `FailFast` is left out (no wait-for-ready).

2. `Client.with_stats_handler(h: fn(Stats) -> () uses fx) -> Client uses fx` and
   `Server.with_stats_handler(h)`, effect laundered at registration as decision 0703's interceptors; several
   handlers run in the order added, as grpc-go's list. Events come in grpc-go's order: Begin, OutHeader,
   OutPayload..., InHeader, InPayload..., InTrailer, End on a client; Begin, InHeader, InPayload..., OutHeader,
   OutPayload..., OutTrailer, End on a server. A retried call (decision 0705) reports each attempt as its own
   Begin to End, as grpc-go.

3. Stages: stage 1 the call events on client and server. Stage 2 ConnBegin and ConnEnd, which need a connection
   open and close hook from std/http (client pool and server accept); Lang 3 reviews that hook.

## Consequences

- OpenTelemetry style metrics (call counts, sizes, latencies) and logs of headers can be written in Xo against
  the same events as grpc-go.
- Each event costs a closure call per handler; with no handler the call path is unchanged (one empty-list check).
- Wire lengths are the gRPC framed sizes (5-byte prefix plus the possibly compressed message), as grpc-go's;
  header wire lengths are std/http's HPACK-decoded sizes where grpc-go reports the frame's, documented in spec.

## Alternatives

- An interface with TagRPC, HandleRPC, TagConn and HandleConn, as grpc-go: Xo has no context to tag; the call
  number does the same job with one function.
- Widening message interceptors to carry sizes: they change messages, a stats handler only observes; grpc-go keeps
  the two apart too.
- Not planned, with the reason: leaves OpenTelemetry instrumentation impossible, the main use.
