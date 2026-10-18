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

## The arm64 and LLVM gap, and the TLS connection copy (2026-10-17)

Per function (callgrind_annotate of the N and 2N runs, differenced; the
arm64 helpers named from nm and the assembly's `// drop T` comments, since
they lack ELF .type and .size), arm64 over LLVM at main 0e67542f: plain
+4,538, TLS +11,777 per request. By cause: TLS record functions LLVM
inlines (flush, write, step, recv_some, ghash_hw: +7,450 TLS), reference
count helpers called out of line (drop Str alone 1,152 / 3,339; +1,103 /
+5,875), runtime fast paths behind entry functions (rl_free_n_slow,
cabi.obj_* bodies, str_eq: +1,560 / +1,985), a 2.5 KB struct passed by
value to its drop helper (7,338 against 3,564), memcpy and memmove calls
(+615 / +2,212), HTTP path helpers not inlined (about +1,000 plain), and
outlined LSE atomics (+300 plain); the emitter items went to Xo lang 4,
the inliner items to Xo Lang 2.

On both backends each TLS read and write took a copy of the connection's
state out of its Any (tlssrv.load: a dup of every buffer of the TLS
state) and saved a new Any (a drop of the old copy): about 20k of LLVM's
104k instructions per request. tlssrv.xo now moves it out
(rt.any_take) and back, and keeps the handshake state beside it.
Measured base 8084d588 against the branch, N=100, REPS=2, medians:

| Scenario | arm64 base | arm64 new | change | LLVM base | LLVM new | change |
|---|---|---|---|---|---|---|
| plain (control) | 34,784 | 35,034 | +0.7% (within the spread) | 30,819 | 30,940 | +0.4% |
| tls | 116,265 | 94,555 | -18.7% | 104,474 | 76,211 | -27.1% |
| tls-newconn | 2,642,508 | 2,610,058 | -1.2% | 2,316,521 | 2,277,409 | -1.7% |
| h2 | 183,847 | 159,750 | -13.1% | 159,896 | 129,775 | -18.8% |

## Plain HTTP/1.1, the std side (2026-10-17)

LLVM's plain profile at main 83201cc2: a quarter of the 31k instructions
is the heap (53 allocations a request), then the head parse and the
headers Map. parse_head copied each field line before copying its key and
value out of it (3 allocations a field; the line copy is only needed when
a continuation line joins it), the key and value lists grew at the fifth
field, and `req.method.to_upper()` copied "GET" on every request
(unicode.map_case always allocated). Fixed in std (http1.xo, unicode.xo):
47.6 allocations a request. Base 83201cc2 against the branch, N=100,
REPS=3, medians:

| Scenario | arm64 base | arm64 new | change | LLVM base | LLVM new | change |
|---|---|---|---|---|---|---|
| plain | 35,003 | 33,940 | -3.0% | 30,844 | 29,688 | -3.7% |
