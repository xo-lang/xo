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

## Small maps with no table (2026-10-17)

A request's headers Map was built by a put per field from `{}`: each
name hashed, the table rebuilt on the first put, each list copied off
the empty literal's. compiler/ssa map.xo now keeps a map of up to
SMALL_MAP (5) entries with no table: put appends, lookups scan the keys
(xo.map.scan; Str lengths compared first, inline), the sixth put builds
the table. (An rt.map_parts intrinsic that let std build the Map whole
was dropped: xo itself compiles std/http, and the seed's checker rejects
an intrinsic it does not know, gotchas/seed-older-than-intrinsics.md.)
Base c25c0d54 against the branch, N=100, REPS=3, medians:

| Scenario | arm64 base | arm64 new | change | LLVM base | LLVM new | change |
|---|---|---|---|---|---|---|
| plain | 33,276 | 31,783 | -4.5% | 29,401 | 27,832 | -5.3% |

Lookups on the Mac (instructions of 10^6 gets of same-length keys with
a 12 byte common prefix, base / new): 4 entries 0.88, 5 entries 0.99,
6 (a table) 1.01, 8 and 16 (tables) 1.01. A SMALL_MAP of 8 cost 1.28 at
8 entries and 1.07 at 6, so 5 it is; the scan kept out of xo.map.find,
which the arm64 inliner takes only while it stays small.

## The Date header cached per second (2026-10-17)

http1w.put_date formatted the date on every response (the clock, a
civil date conversion, 29 bytes). It now keeps the current second's text
in static words 69 to 74 (a sequence lock: a reader that sees the
sequence change, or a writer busy, formats it itself). Base 018e3969
against the branch, the same xo, N=100, REPS=3, medians: arm64 31,787 to
31,426 (-1.1%), LLVM 27,889 to 27,805 (-0.3%, within the spread).

## HTTP/2 made deterministic (2026-10-17)

The h2 scenario varied 5% to 7% between repeats on arm64
(gotchas/h2-callgrind-count-races.md). callgrind.sh now runs loadgen
with -h2det (each request's frames in one TLS record) and -gap 20ms (the
server parked before each request). Main at ca9173f4, N=100, REPS=3:

| Scenario | arm64 | LLVM |
|---|---|---|
| h2 | 167,052 / 167,325 / 167,429 (0.23%) | 131,594 / 131,673 / 131,773 (0.14%) |

These are the baseline for the HTTP/2 std hunt; earlier h2 rows are not
comparable (a different client write pattern and server path).
Measured with five repeats each (arm64): -h2det alone 153,779 to
164,030, -gap alone 186,267 to 189,048, both 167,304 to 167,616.

## HTTP/2, the std side (2026-10-18)

Deterministic h2 (above), N=100, REPS=3, medians, instructions per
request; each row against the one before, on the same xo:

| Change | arm64 | LLVM |
|---|---|---|
| main ca9173f4 | 167,325 | 131,673 |
| h2w.request_check: one pass, compared in place (it made ten strings: four pseudo field values, five `to_lower`s, a list) | 162,363 (-3.0%) | 127,618 (-3.1%) |
| tls13.cat2: an empty side is the other (record buffers) | 160,483 (-1.2%) | 126,047 (-1.2%) |
| tlssrv read and write store the connection at its last use | 156,221 (-2.7%) | 124,469 (-1.3%) |
| a stream's request parts stay Xo values (an Any of ReqParts) instead of a packed string copied out again | 150,723 (-3.5%) | 120,240 (-3.4%) |
| tlssrv tests s12 for None before matching it | 146,682 (-2.7%) | 120,928 (+0.6%, within the spread) |

In all, -12.3% (arm64) and -8.2% (LLVM). HTTP/1.1 over TLS also moved
on arm64, 90,437 to 85,247 (-5.7%).

Found and handed on (compiler side, not std):

- tlssrv.flush and recv_some took a `var c: Conn` (over 1 KB): the
  arm64 backend copied the whole record in and out around every call,
  about 18,000 instructions a request (bench/perf/varparam reproduces
  it: arm64 522 instructions a call, LLVM 21). Xo lang 4's frame homes.
- `var c = match take_conn(t) { Some(x) => x ... }` shared every field of
  the payload and then dropped the Option (`drop native.tlssrv.Conn?`,
  16,677 a request on arm64). Xo Lang 2's payload move.
- any_of marks its value shareable across tasks (Op.Share): a walk of
  every heap leaf of Conn on each save, about 5,500 a request on arm64;
  parked (it needs an escape fact or a seed with a new intrinsic).

## The RC payload move, measured (2026-10-18)

19a3540d (Xo Lang 2: a single-field payload of an owned enum that dies
at the match takes the enum's ownership) on the HTTP benchmarks: xo
built from its parent (61207c14) and from 1f77f593 (only 19a3540d in
between in compiler/ and std/native), both servers built from
1f77f593's std, N=100, REPS=3, medians:

| Scenario | arm64 before | arm64 after | change | LLVM before | LLVM after | change |
|---|---|---|---|---|---|---|
| tls | 82,287 | 74,029 | -10.0% | 71,831 | 59,202 | -17.6% |
| h2 | 141,525 | 125,102 | -11.6% | 120,547 | 94,536 | -21.6% |

The take side of tlssrv (`var c = match take_conn(t) { Some(x) => x ...}`)
no longer shares and drops the connection.

## linux/amd64, the first baseline (2026-10-18)

`ARCH=amd64 bench/perf/http/callgrind-docker.sh` (an emulated
linux/amd64 container: valgrind counts the guest's instructions, the
same run to run; 8 minutes), main bfdc0a6c, N=100, REPS=2, instructions
per request, Xo's own amd64 backend (decision 0254) against LLVM:

| Scenario | amd64 | LLVM | amd64 / LLVM | arm64 / LLVM on arm64 |
|---|---|---|---|---|
| plain | 37,277 / 37,290 | 28,740 / 28,941 | 1.30 | about 1.14 |
| plain-newconn | 48,045 / 47,994 | 37,074 / 36,961 | 1.30 | |
| tls | 88,330 / 88,929 | 61,809 / 62,010 | 1.43 | 1.25 |
| tls-newconn | 5,653,593 / 5,652,832 | 4,143,682 / 4,146,802 | 1.36 | 1.13 |
| h2 | 145,600 / 145,542 | 98,447 / 98,617 | 1.48 | 1.32 |

A full TLS handshake costs both amd64 backends far more than arm64's
(LLVM 4.14M against 2.23M on arm64): the crypto that has arm64
instructions (AES, PMULL, SHA) runs as portable code or is slower here.
The biggest per function gaps (sent to Xo lang 5): tlssrv.flush (h2
12,656 against 1,514: the var-parameter record copy Xo lang 4 is fixing
on arm64), tlssrv write and recv_some (the same shape), any_of's share
walk of the TLS connection (both backends), allocation fast paths and
drops not inlined (alloc.alloc 6,134 against 4,450; `drop Str` 1,396
against 0), and the AES-GCM loops (Gcm.ctr 2,052 against 1,315).

## The full TLS handshake (tls-newconn), the std side (2026-10-18)

Per handshake request (X25519MLKEM768 key exchange, an ECDSA P-256
certificate), N=20, REPS=2; arm64 and LLVM on arm64:

| Change | arm64 | LLVM |
|---|---|---|
| main 58f5f245 | 2,531,690 | 2,228,711 |
| X25519's public key from a fixed base Edwards table (3eea92b1; one ladder a handshake instead of two) | 2,287,554 (-9.6%) | 2,018,675 (-9.4%) |
| the transcript hashed as it grows (it was hashed whole at six points) | 2,246,611 (-1.8%) | 1,986,943 (-1.6%) |
| tls13 encoders into one string, and list literals with room for their elements (SSA) | 2,180,430 (-2.9%) | 1,926,408 (-3.0%) |

In all -13.9% and -13.6%. On linux/amd64 (emulated, no SHA
instructions) the first two: Xo amd64 5,652,832 to 4,402,664 (-22%),
LLVM 4,143,682 to 3,174,248 (-23%).

Where a handshake went before (arm64): x25519 31%, P-256 signing 16%,
ML-KEM 13% and its SHAKE 7%, the runtime (allocation, counts) 10%,
SHA-256/HMAC/HKDF 6%, AES-GCM 4%.

## Allocation and free with threads running (2026-10-19, Xo lang 4)

The plain HTTP server has threads (the scheduler's workers), so static
word HEAP is 0 and llrt's allocation fast paths (alloc_fast inlined in
rs_new_n, rl_new_n, rl_grow_n; free1 in rl_free_n), which serve only
the one thread case, never ran: all 34 allocations a request went
through obj.halloc and alloc.alloc (30 instructions each of their own),
and all 26 frees through rl_free_n_slow (heap_word, a frame) and
rl_free. New out of line paths for the other threads case (obj.xo
rl_new_mt, rl_grow_mt, rs_new_mt, the head of rl_free_n_slow) take the
calling thread's heap record from its thread local and pop the class's
free list (alloc.alloc_pop: list only, so an empty list still goes to
alloc, which takes back blocks other threads freed before carving) or
free to it (alloc.free_mt). The one thread entry points keep main's
code: a first version that added the branches inside them (and a flag
argument to alloc_fast) made text/lines, split, trim and five others
0.5% to 1.0% slower on the Mac (rl_free_n stopped being inlined into
its export).

Plain, N=100, one repeat, base main 8ac3ca0b against the branch:
25,455 to 24,853 instructions a request (-2.4%). The Mac sweep (114
workloads, 3 rounds): 0 slower, 2 faster (httpparse h2_get 0.985,
h2_varied 0.979). Next: alloc.free_mt computes a class's magazine
size with a multiply on every free (about 26 instructions a free).

Then (the same day): each heap record holds its classes' magazine sizes
(alloc.xo MAG_CAP, written by heapos.xo mag_caps when the record is
made), so free_mt compares the list's length with a load instead of
the size computation and multiply, and rl_free_n_slow no longer reloads
static word HEAP (the thread local is set exactly when it becomes 0).
Plain, base main against the branch: 25,060 to 24,555 instructions a
request (-2.0%); the Mac sweep 0 slower, 2 faster (httpparse h1_url
0.987, digest/none 0.990).

## The TLS connection kept out of an Any (2026-10-19, Xo Lang 3)

A function level ranking of tls and h2 (arm64, main 065bd1c1, ANN=1,
the hashed helpers named from the server's `--asm`) put 2,938
instructions a TLS request (7.3%) and 5,882 an HTTP/2 one (8.6%) in
sharing large values: tlssrv.read and write ended with
`put(t, rt.any_of(c))`, and any_of shares its value (conc.xo one), so
every call walked the whole TLS state (obj_rl_share, then the share
helpers of the tls13srv.Server's lists of large structs, frames of 1,280
and 1,376 bytes), about 1,580 instructions a call. Only the connection's
task reads that Any.

6827ce1e: the block holds the Conn in a list of one that only its task
reads (word 0 the list object with the block's reference; take_conn
moves it out with obj_at, a decrement, and a pop of the then unique
list; read and write push the Conn at its last use, so it moves, and
keep_list takes the address through a borrowed parameter; an empty
block's release builds no `Conn?`, 2.5 KB zeroed and dropped, about 650
instructions a call), and a server's block (its SrvTls Any) has a free
of its own. No new runtime operation, so no seed bump. Full mode
callgrind (N=100, REPS=2), the change and its base interleaved, two
rounds each, the median of four:

| scenario | arm64 | LLVM |
|---|---|---|
| tls | 40,506 to 37,174 (-8.2%) | 51,460 to 49,982 (-2.9%) |
| tls-newconn | 1,925,046 to 1,916,329 (-0.5%) | 1,781,736 to 1,775,870 (-0.3%) |
| h2 | 68,188 to 61,902 (-9.2%) | 88,974 to 86,142 (-3.2%) |

On the way: a local list's last use was `rt.obj_addr(l)`, so it was
released there, before the `rc.inc` that should have kept it, and the
server crashed in its first handshake (gotchas/raw-address-outlives-bytes.md,
observed again); the first gate caught tlssrv.free freeing a server's
block as a connection's.

## HTTP/2 header constants and AES-GCM's per record work (2026-10-19, Xo Lang 3)

f8d367b7, the second and third levers of the tls and h2 ranking above:

- h2srv fragment: a kept header's common name (the pseudo headers, host,
  accept, cookie, user-agent, content-type, authorization,
  content-length, accept-encoding, accept-language) and common value
  (:method's GET, PUT, POST, HEAD, PATCH, DELETE; :scheme's https and
  http; accept-encoding's gzip) are constants (http1.h2_name_at,
  h2_value_at; an integer code per name, no Str compares), not copies:
  string allocations an h2 request 36.6 to 27.4; a value's control
  bytes are tested 8 at a time (h2w.value_ok), and a known name skips
  the name's byte check.
- AES-GCM: the CPU's rounds encrypt only the counter blocks a call needs
  (encrypt_n: a tag's one block, a record's last chunk's 1 to 3, where
  encrypt4 did four), and a record's last partial word is one load of
  the word ending at n (tail_word), not a byte loop.

Full mode callgrind, main 8a0bd1e8 and the change interleaved, two
rounds each, the median of four:

| scenario | arm64 | LLVM |
|---|---|---|
| tls | 37,132 to 36,664 (-1.3%) | 50,092 to 49,438 (-1.3%) |
| tls-newconn | 1,882,383 to 1,881,659 (0.0%) | 1,766,659 to 1,765,645 (-0.1%) |
| h2 | 61,761 to 58,932 (-4.6%) | 86,044 to 82,940 (-3.6%) |

Measured and dropped: constants for HTTP/1's common header values
(gzip, close, keep-alive, application/json): parse_head's checks cost
about what the copies saved (about -0.3%, inside short mode's noise).

With 6827ce1e the day's arm64 totals against 065bd1c1's ranking: tls
40,358 to about 36,700, h2 68,030 to about 58,900 (short mode, ANN
runs).

## TLS and h2 after the three levers: the next copies (2026-10-20, Xo Lang 3)

Ranking at 274d4929 (arm64, short mode, ANN, per request; tls 36,555,
h2 58,592): allocation and free about 5,750 (tls, 15.7%) and 9,550 (h2,
16.3%), of which string allocations (obj_rs_with_cap) 21.9 and 26.9 a
request, from raw.str_at copies (13.8 and 17), the record buffers of
tlsrec.seal, Gcm.open, and tlsrec.nonce; AES-GCM 3,524 (tls, 9.6%); the
HTTP/1 head 3,099 (tls, 8.5%); hpack Enc.encode 2,591 (h2, 4.4%). The
frees are thread local (Xo lang 4: free_mt is the threaded program's
free to its own heap, about 13 instructions each), so the lever is fewer
allocations.

- 5e0e3fcc: a TLS 1.3 write seals the caller's bytes in place
  (tlsrec.seal_at: no str_at copy, no part slice, for the usual one
  record), and a read buffer that is exactly one record is handed over,
  not sliced. Against its parent: tls 36,409 to 35,621 (-2.2%), h2
  58,278 to 57,730 (-0.9%). On the way: a call of tls13srv.send_at with
  a local's c.s as its var argument copied the whole Server in and out
  (253 instructions a write); through a var Conn parameter, as send
  does, it costs nothing.
- 3463c095: hpack's eq_at in words (the last one overlapping, every load
  inside the bytes), where the byte loop of checked loads was about
  1,000 instructions an h2 request: h2 58,009 to 57,288 (-1.2%),
  Enc.encode 2,605 to 1,385.

Each copy removed is worth 100 to 250 instructions at these request
sizes; what is left of the record path (feed's body copy, Gcm.open's
output, the nonce) is about one more percent.
