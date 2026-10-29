# bench/perf/httpproxy, 2026-10-29 (decision 0404 stage D)

A 100 MB (104857600 byte) body from bench/perf/httpstream's serve/ (Go,
Content-Length, 64 KiB writes) through a reverse proxy on 127.0.0.1,
read by httpstream's Go client in 64 KiB pieces. The proxy is Xo's
`http.reverse_proxy` (native arm64, and the Go backend) or Go's
`httputil.ReverseProxy` with Rewrite (SetURL, SetXForwarded). Best of 5:
the client's wall time (/usr/bin/time -p, 10 ms resolution) and the
proxy's peak RSS (ps every 5 ms while the transfer runs) on the
development Mac (arm64), measure slot,
`bench/perf/httpproxy/measure.sh <xo> <out> arm64 go`, xo of main at
a06a91ba2 (after stage E, b2dd4d317).

| proxy | client wall (s) | proxy peak RSS (bytes) |
|---|---|---|
| Xo native (arm64) | 0.06 | 5881856 |
| Xo Go backend | 0.03 | 40714240 |
| Go httputil.ReverseProxy | 0.01 | 11681792 |

Memory: the native proxy streams in flat memory, 5.9 MB peak against
Go's 11.7 MB. Speed: the native proxy is slower, 0.06 s against 0.01 s
for 100 MB (about 1.7 GB/s against Go's 10 GB/s at this resolution); a
profile of the copy loop (32 KiB reads through ClientStream.read and
BodyWriter.write) comes next. The Go backend's proxy holds 40.7 MB.
An earlier run's "go" row was the Go backend's build overwriting the Go
twin's binary (both p-go); the twin is p-gotwin since b2dd4d317.
