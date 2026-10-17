# HTTP server instructions per request (callgrind), 2026-10-17

Xo Lang 3. Main 0e67542f, linux/arm64 in scripts/docker-linux.sh's Debian
trixie image on the Mac (Docker Desktop), valgrind 3.24.0:
`bench/perf/http/callgrind-docker.sh <xo> <out> arm64 llvm` with N=100,
REPS=3. The server is bench/perf/http/xo (the users CRUD API) built by a
linux/arm64 xo cross built from main; the client is loadgen's `-route
cycle` (POST, GET, PATCH, GET, DELETE, a list every 8th cycle), 513
requests between the N and 2N runs. Instructions per request of the
server process, all threads: (Ir(2N) - Ir(N)) / 513.

| Scenario | arm64 (3 runs) | arm64 median | LLVM (3 runs) | LLVM median | LLVM / arm64 |
|---|---|---|---|---|---|
| plain: HTTP/1.1, one keep-alive connection | 35,364 35,156 35,126 | 35,156 | 30,869 30,867 30,770 | 30,867 | 0.878 |
| plain-newconn: a connection per request | 44,530 44,488 44,460 | 44,488 | 38,791 38,773 38,816 | 38,791 | 0.872 |
| tls: one keep-alive TLS connection | 116,149 116,147 116,138 | 116,147 | 104,476 104,449 104,462 | 104,462 | 0.899 |
| tls-newconn: a full handshake per request | 2,642,163 2,640,997 2,642,901 | 2,642,163 | 2,317,959 2,317,326 2,317,546 | 2,317,546 | 0.877 |
| h2: HTTP/2 over TLS, streams of one connection | 192,612 179,460 185,564 | 185,564 | 162,993 161,426 160,831 | 161,426 | 0.870 |

Spread (max - min over the median): HTTP/1.1 scenarios at most 0.7%
(TLS 0.01 to 0.03%), so a change of 1% or more there is real; HTTP/2 1.4%
on LLVM and 7.1% on arm64: its frames depend on when the server's reads
return (how many requests a read carries, when window updates go out),
which a serialized valgrind run does not fix, so HTTP/2 needs a larger
change, or more REPS, to judge. Rough split: TLS adds about 81k
instructions per request on arm64 (74k on LLVM) of record encryption and
decryption, a full handshake about 2.6M (P-256 and the key schedule), a
connection about 9k (accept, the connection's task and buffers).
