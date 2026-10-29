# bench/perf/httpproxy: direct reads, 2026-10-29

The native reverse proxy (decision 0404) after a streamed body's large
pieces are read straight into their own bytes (llrt/hclient.xo
read_direct, http1.xo cbody_next: one read(2) per piece, no copy out of
the connection's 8 KiB buffer) and the proxy reads 64 KiB pieces, as Go's
bufio fast path and io.Copy's 32 KiB buffer do.

`xo bench --ab` over bench/perf/httpproxy/abrun.sh (one round: Go's
serve/, the proxy, httpstream's Go client reading 1 GiB through it;
client_ms is the client's wall time), the development Mac (arm64), measure
slot, load about 6.2 to 6.5, every round used; the one-binary control
(proxy-a against itself, 10 rounds) read wall_ms 0.998 (0.984 to 1.03):

| A | B | rounds | wall_ms B/A (95%) | client_ms A | client_ms B | client_ms B/A (95%) |
|---|---|---|---|---|---|---|
| main's xo (22e28fe5c) | this change | 12 | 0.361 (0.339 to 0.381) | 581.0 | 187.0 | 0.322 (0.301 to 0.34) |
| Go httputil.ReverseProxy | this change | 10 | 0.887 (0.816 to 0.93) | 219.0 | 191.5 | 0.874 (0.781 to 0.925) |

1 GiB moves through the native proxy in 187 ms (5.7 GB/s) against 581 ms
before and Go's 219 ms. The instructions, cycles and max_rss_kb columns of
the run measure abrun.sh's shell, not the proxy, so they are left out.
