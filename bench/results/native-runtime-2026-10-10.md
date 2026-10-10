# Native runtime performance and gaps (2026-10-10)

M4 Pro (12 cores), macOS 27, Apple clang 21, Go 1.26.1, darwin/arm64. The
machine was shared with other agents' test suites: load average 4 to 17
during these runs, so only interleaved runs are compared, and the
handshake rates drift by about 5 percent between sequences.

## 1. TLS handshakes (decision 0117)

Harness: `bench/perf/http/run.sh` (users CRUD, 32 workers, 5 s measured
after 1 s warmup, medians of 3, configs interleaved), `SCHEME=https
NEWCONN=1` (a new connection, so a full handshake, per request; Go's
client offers X25519MLKEM768 and X25519, the native server takes X25519,
the Go backend X25519MLKEM768), ECDSA P-256 certificate from
`loadgen -gencert`. "xo" is the Go backend (net/http, crypto/tls).

| Step | xo req/s, CPU us/req | xo-llvm | xo-arm64 |
|---|---|---|---|
| Before (main at 6b7e00c) | 16993, 230 | 9743, 763 | 9687, 767 |
| X25519 public key through HACL* (not the bignum ladder), SHA-256/512 instructions | 16864, 221 | 13412, 396 | 13135, 412 |
| + spinning lock for the PSA key slot mutex | 17400, 219 | 14392, 420 | 14052, 434 |
| + P-256 inverses by Fermat (Montgomery exponentiation) | 17574, 224 | 15067, 377 | 14931, 388 |
| + session tickets on (each full handshake also writes one) | 17078, 234 | 14469, 389 | 14383, 390 |

Interleaved A/B of the lock alone (llvm, 4 pairs, same binary, the plain
lock selected by a temporary switch): spinning 13808 to 14422 req/s at
413 to 433 us, plain 13185 to 13891 at 429 to 459 us; spinning won every
pair.

With `RESUME=1` (loadgen keeps a TLS session cache, so new connections
resume with the server's tickets; Go's client resumes TLS 1.3 with
psk_dhe_ke):

| Config | Req/s | CPU us/req | p99 us |
|---|---|---|---|
| xo | 18843 | 210 | 3956 |
| xo-llvm | 18346 | 244 | 3410 |
| xo-arm64 | 18088 | 249 | 3484 |

Kept alive HTTPS (no `NEWCONN`), after: xo 132386 req/s at 39 us,
xo-llvm 104916 at 35 us, xo-arm64 105090 at 35 us.

Profile (`sample` of the llvm server under the full handshake load,
inclusive samples of the worker threads' handshake), before:
`psa_export_public_key` (the X25519 key share through
`ecp_mul`'s Montgomery ladder on bignums) 1386 of 3365, ECDSA P-256
signing 824, `__psynch_mutexwait` 804 (the PSA key slot mutex), HACL*'s
X25519 agreement 153. After the X25519 change: ECDSA signing 1079 of
2678 (40 percent), of which the binary GCD inverses 400. Microbenchmark
(one core, `psa_sign_hash`, deterministic ECDSA): 130 us per signature,
112 us with the Fermat inverses (inverse alone: 26.7 us binary GCD, 14.0
us exponentiation); p256-m (the vendored alternative driver) 635 us;
X25519 key generation and export 29 us.

Open: P-256 signing is now about 45 percent of a native full handshake
(112 us against roughly 10 to 15 us for Go's assembly). A fast constant
time P-256 (fiat-crypto's formally verified field arithmetic with Go's
complete formulas and a precomputed generator table) behind the same
interposition would bring the full handshake near Go's; vendoring it
needs the owner's approval (the session's permission check refused
adding downloaded third party code). Peak RSS grows with the number of
connections served (about 1 KB per connection on both the old and new
builds: 199 MB after 200k handshakes before, 293 MB after 315k now);
`leaks` finds none and `heap` shows 12 runtime allocator chunks of 4 MB
(`xo_refill`), so it is retention in the runtime, not TLS: the server's
scope kept every finished connection task (fixed in section 2) and the
allocator keeps blocks on the thread that freed them (section 2).

## 2. Contention: Mutex wake ups, handle counts, finished tasks (decision 0118)

Harness: `bench/perf/conc/run.sh` (medians of 7, interleaved), with the
new `rwmutex` workload (64 tasks x 4,000 accesses, a quarter `with`
holding the lock for a Collatz count of about 111 steps, the rest
`with_read`; Go: `sync.RWMutex`).

| Workload, wall / CPU ms | go | xo-go | xo-llvm before | xo-llvm after | xo-arm64 before | xo-arm64 after |
|---|---|---|---|---|---|---|
| rwmutex | 11.2 / 16.4 | 375 / 1682 | 966.7 / 6262 | 131.0 / 253 | 970.7 / 6337 | 135.2 / 262 |
| mutex | 24.5 / 98.5 | 125 / 440 | 9.7 / 11.8 | 8.9 / 9.2 | 9.2 / 11.0 | 8.8 / 9.8 |
| spawn | 51.5 / 170 | 133 / 330 | 57.1 / 258 | 59.3 / 232 | 52.2 / 231 | 55.5 / 213 |

`XO_RTSTATS=1` on rwmutex (llvm): before, fiber_park 912,022 and
lock_slow 531,898; after the reader handoff, 230,468 and 20,440. Spinning
before parking (`XO_MUTEX_SPIN`, a temporary switch; medians of 7):
rwmutex CPU 264 ms with 0 rounds, 274 (50), 336 (200), 623 (800), 1165
(3000); mutex 8 to 10 ms throughout. Not kept.

bench/perf/http, kept alive (2 interleaved rounds, base then new):
xo-llvm 112393 and 111705 req/s at 31.3 us, after 112973 and 113770 at
30.2 us; p50 about 240 to 195 us, p99 about 880 to 955 us. arm64 alike.

Handle reference counts (`spawn`, profile of a 15x longer run, llvm): of
the active samples about 13 percent are the release of the captured
`Atomic` in the closure's environment on the workers
(`xo.envdrop.main.fn1`) and part of `xo_entry` its retain on the
spawning thread: one cache line written from every core. arm64, which
uses LSE atomics, spends 10 percent less CPU on this workload than llvm
(section 3). Not changed here: the fix that removes the traffic is a
borrowed capture for closures that cannot outlive their scope (no
retain at spawn, no release at the end, the owner kept alive to the
join), a change of internal/ssa's RC insertion and of both backends'
environment drops.

Finished tasks: peak RSS of the TLS server after 20 s of a handshake
per request (section 1 harness, one run each): 295 MB without pruning,
223 MB with it.

Allocator magazines (decision 0118 point 4). Interleaved A/B, 9 rounds,
llvm, HEAD (pruning, no magazines) against magazines, wall / CPU ms:
spawn 60.5 / 241.8 against 69.6 / 268.7, fanout 23.6 / 84.2 against
29.3 / 105.9, pmap 60.2 / 611.4 against 60.0 / 618.4. Peak RSS: spawn 45
to 5 MB, fanout 24 to 5 MB, pmap 2 to 2 MB. The first version (moving
half a list to the central list when it passed two batches, walking it)
cost spawn 30 and fanout 43 percent. TLS server, 20 s of a handshake per
request: 220 MB without magazines, 28 MB with, 15019 and 15056 req/s.
bench/perf/http kept alive (2 interleaved rounds): 112804 and 113347
req/s at 30.3 us before, 113013 and 113072 at 30.3 us after; RSS 30 to
21 MB.

## 3. LSE atomics in LLVM backend code (decision 0119)

The generated functions now carry clang's default target (darwin/arm64:
`apple-m1`, LSE). `otool -tv` of the llvm `spawn` binary: exclusive
loads (`ldxr`/`ldaxr`) 20 before, 0 after; LSE instructions 55 (the C
runtime's) before, 72 after. Interleaved A/B (llvm, before = the commit
with magazines, wall / CPU ms):

| Workload | before | after |
|---|---|---|
| spawn (9 rounds) | 67.9 / 267.3 | 67.0 / 261.8 |
| fanout | 30.8 / 111.6 | 31.2 / 110.5 |
| pingpong | 27.4 / 47.0 | 28.4 / 48.8 |
| mutex | 6.7 / 8.1 | 6.5 / 7.9 |
| rwmutex | 131.2 / 255.2 | 131.1 / 254.7 |
| pmap | 60.2 / 624.8 | 58.8 / 608.0 |
| fib (15 rounds) | 216.6 | 218.3 |
| sieve (15 rounds) | 97.5 | 99.1 |
| nbody (7) | 181.6 | 181.2 |
| sort | 210.5 | 211.0 |
| interp | 64.6 | 64.9 |
| collatz | 468.4 | 459.8 |
| strings | 35.0 | 35.1 |

bench/perf/http kept alive (xo-llvm, 3 interleaved rounds): before
112716, 112909, 112988 req/s (p99 959, 969, 957 us), after 117012,
116386, 116824 (p99 924, 932, 914 us); CPU per request 30.1 to 29.6 us.

Linux containers: Debian clang 19 reports `generic` with
`+outline-atomics` on arm64 (the helpers pick LSE at run time, exclusive
loops on ARMv8.0) and `x86-64` on amd64; TestLLVMTargetAttrs and the
LLVM goldens (plus TLS, signals, runtime threads on arm64) pass in both.

## 4. Faults in deferred code (decision 0085 amendment)

Not a performance item. Golden `fault_defer_chain` (nine cases): before,
the LLVM and arm64 backends skipped 6 of the Go backend's output lines
in cases 1 to 4 and 7 (the earlier and outer deferred code), reported
the first of two deferred faults instead of the last, and case 8
deadlocked (XO-F001: the caught fault left the task shielded); after,
all three backends print the same, leak check 0. bench/perf/http kept
alive (2 interleaved rounds, before / after): llvm 116261, 116262 /
115880, 116083 req/s; arm64 113231, 113256 / 112515, 113226.
