# P-384 and P-521 fixed base tables made on first use, 2026-10-16

Change: nistec.xo pt_mul_base takes P-384's and P-521's [k]G from a
table of j 16^w G (windows w of 4 bits, digits j 1 to 15, affine,
Montgomery domain) instead of pt_mul's doublings: pt_mul_g_n, as P-256's
compiled in pt_mul_g, every entry of a window read under a mask (constant
time). The table is made once per process on first use (gtab_make: the
points by additions, affine with one batched inversion; P-384 138,240
bytes, P-521 285,120 bytes of rt.heap_large memory), kept in static words
44 and 45 (nistec_gtab.xo, published with a compare and swap) for the
process's life (rt.heap_large memory, which the leak check does not
count; an exit hook was tried and dropped, see below). Compiled in, the
P-521 table cost every TLS build about 27 s and 2.5 MB (018a9e98).

Method: Mac (darwin/arm64), xo of main 1c6f81aa-era, `/usr/bin/time -l`
instructions retired. Micro: a scratch program (out of the repo) runs
std/internal's crypto_op 50 times per operation, min of 5 runs, so the
table's one time making is amortized over 50 operations; per operation.
Handshakes: bench/perf/tls/measure.sh, 100 TLS 1.3 handshakes with the
p384 and (new) p521 workloads against Go's server, min instructions of 5,
best wall of 5; both builds interleaved twice (the first round's wall
times of the new build were taken under load from other sessions).

Micro, millions of instructions per operation (before, after, ratio):

| op | llvm before | llvm after | ratio | arm64 before | arm64 after | ratio |
|---|---|---|---|---|---|---|
| p384 public | 6.889 | 2.933 | 0.43 | 8.277 | 3.495 | 0.42 |
| p384 sign | 7.227 | 3.271 | 0.45 | 8.663 | 3.879 | 0.45 |
| p384 verify | 12.899 | 8.945 | 0.69 | 15.592 | 10.810 | 0.69 |
| p521 public | 42.521 | 14.707 | 0.35 | 79.788 | 27.195 | 0.34 |
| p521 sign | 44.685 | 16.875 | 0.38 | 83.821 | 31.235 | 0.37 |
| p521 verify | 81.996 | 54.186 | 0.66 | 154.277 | 101.695 | 0.66 |

100 handshakes (instructions of the client; best wall of the second
round):

| workload | before | after | ratio | wall before | wall after |
|---|---|---|---|---|---|
| p384 llvm | 2,097,965,891 | 1,690,929,200 | 0.81 | 180 ms | 150 ms |
| p384 arm64 | 2,708,033,519 | 2,218,965,096 | 0.82 | 200 ms | 180 ms |
| p521 llvm | 9,030,902,513 | 6,208,753,997 | 0.69 | 430 ms | 340 ms |
| p521 arm64 | 16,611,801,669 | 11,290,112,531 | 0.68 | 700 ms | 530 ms |

Checks: internal/llvm TestNistecDiff (public keys, ECDH, signatures both
ways against Go's crypto/ecdsa, all four curves), goldens tls_nistec,
tls13_go (Go server traces with P-521), crypto_mlkem, crypto_prims,
tls13_server equal to the Go backend; tls_nistec and tls13_go on arm64;
XO_LEAKCHECK 0 live with the tables made.

No exit hook frees the tables: they are rt.heap_large memory, which
XO_LEAKCHECK does not count (one was written and dropped). While gating,
new darwin binaries sometimes hung at _dyld_start before their first
instruction (devgoldens' http_tls_keys, with main's std too, whatever the
linker): macOS's Gatekeeper assessments were queued behind dialogs
pending on the owner's screen (Xo Lang 3, 2026-10-16), an environment
issue, not this change.
