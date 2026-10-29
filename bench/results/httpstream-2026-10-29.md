# bench/perf/httpstream, 2026-10-29 (decision 0404 stage B1)

A 100 MB (104857600 byte) body over HTTP/1.1 on 127.0.0.1 from serve/
(Go, Content-Length, 64 KiB writes), read by Xo's client with
`Client.open` and `read(65536)` (stream) or `Client.send` (send), beside
the Go twin (resp.Body.Read in 64 KiB pieces, or io.ReadAll). Best of 5
under /usr/bin/time -l on the development Mac (arm64), measure slot,
`bench/perf/httpstream/measure.sh <xo> <out> arm64 go`, xo built from
the B1 change.

| client | mode | wall (s) | peak RSS (bytes) |
|---|---|---|---|
| Xo native (arm64) | stream | 0.02 | 3997696 |
| Xo native (arm64) | send | 0.02 | 281329664 |
| Xo Go backend | stream | 0.01 | 39157760 |
| Xo Go backend | send | 0.03 | 371965952 |
| Go twin | stream | 0.01 | 10813440 |
| Go twin | send | 0.02 | 265076736 |

The stream's peak RSS stays flat (4.0 MB natively, 2.7x below Go's
10.8 MB); send holds the body (281 MB natively, 265 MB in Go). Wall times
are at /usr/bin/time's 10 ms resolution, so they show no difference at
this size; HTTP/2 streams over TLS come with stage B2.
