# HTTP client: pooling after a request with a body source (2026-11-03)

The change: a connection that served a request whose body came from a
source (Request.with_body_reader) goes back to the pool once the response
was read to its end and the body was sent in full, as net/http's
Transport does (persistConn's wroteRequest). Before, Client.open's full
duplex path (decision 0433) closed it every time.

Program: bench/perf/httppost, 2000 keep-alive POSTs of a 34-byte body
source against a std/http server in the same process. `xo bench --ab`
on the first linux/arm64 offload host (12 CPUs, Linux aarch64, xo 0.37.0),
3 rounds, A the parent 2daf4f48c and B the change 2b761fc75 (branch
revisions; the pushed commits carry the same code).

`open` (Client.open, the full duplex path):

| metric | A median | B median | B / A | 95% interval | verdict |
|---|---|---|---|---|---|
| wall_ms | 113.4 | 28.69 | 0.253 | 0.231 to 0.314 | better |
| cpu_ms | 189.3 | 30.28 | 0.16 | 0.142 to 0.207 | better |
| max_rss_kb | 3884 | 2988 | 0.769 | 0.722 to 0.83 | better |

`send` (Client.send, which already pooled such a connection):

| metric | A median | B median | B / A | 95% interval | verdict |
|---|---|---|---|---|---|
| wall_ms | 22.42 | 23.31 | 1.04 | 0.953 to 1.1 | within noise |

Load 0.95 to 0.98 in every round, none loaded. Connections counted by
golden http_client_pool_body: 6 POSTs with body sources and 2 GETs on 1
connection, where the native client dialed 4 (main's std at 98f71e696);
the Go backend gives 1.
