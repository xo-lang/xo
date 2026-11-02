# Jitter bench (owner, 2026-10-30)

Whether Xo's reference counting (no collector, so no GC pauses) gives a
steadier far tail than Go under the same load. Twin servers, each with two
handlers:

- `GET /hello`: a 6 byte text.
- `GET /alloc`: a page of 200 items (about 1,000 fields, 400 strings, 200
  lists) built and encoded as JSON per request, 14,683 bytes, so every
  request allocates and frees a few hundred objects.

`xo/` is the Xo server (std/http, `derive Json`), `go/` the Go one
(net/http, encoding/json, Content-Length set so neither side chunks).
`loadgen/` is a raw HTTP/1.1 client: `-c` keep-alive connections send
`GET -path` back to back, each measured latency goes into a preallocated
per-connection array, and the client's own GC is off, so its allocations
add nothing to the tail it measures. It prints one JSON line: requests,
req/s, p50, p99, p99.9 and max latency (microseconds), the coefficient of
variation of the per-second request counts (`sec_cv`), and with `-pid` the
server's CPU per request (Linux /proc). With `-rate` (the open loop) latency
counts from each request's scheduled send, and the line also has `late_*`,
how late the generator sent, and `svc_*`, the send to the response's end:
at low rates the totals were mostly `late_*`, the generator's own timers
(Go's runtime sleeps in a millisecond epoll wait when every P is idle).
`-awake` (on by default in the open loop, 2026-11-02) keeps one goroutine
runnable so a send fires on time; `-awake=false` gives the old behavior
(bench/results/jitter-low-rate-p50-2026-11-02.md).
Go's `GET /alloc-strconv` is `/alloc` with strconv in place of fmt.Sprintf.

    loadgen -addr 127.0.0.1:18080 -path /alloc -c 32 -d 10s -warmup 2s -pid PID

The runs on the linux/arm64 offload host (the server on CPUs 0-5, the
load generator on 6-11, 20 interleaved rounds, each run's load average
and CPU steal ticks logged) are in
[bench/results/jitter-2026-10-30.md](../../results/jitter-2026-10-30.md).

## Comparing revisions on the offload hosts

`alloc-split.sh MODE SPECS NAME ROUNDS` builds each revision's server on the second host and measures all of
them in one exclusive lease on the first (`alloc-build-remote.sh`, `alloc-measure-remote.sh`;
knowledgebase/infrastructure/offload.md). Ratios are against v0.35.0's server built in the same job.
