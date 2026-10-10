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
(`xo_refill`), so it is retention in the runtime's allocator, not TLS.
