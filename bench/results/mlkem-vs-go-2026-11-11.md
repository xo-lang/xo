# crypto ML-KEM-768 against Go's crypto/mlkem (2026-11-11)

Decision 0604's timing row (the owner's rule: every std port carries one).
bench/perf/mlkem and its Go twin (twin.go.txt): keygen (a decapsulation
key from each of 3,000 seeds), parse (an encapsulation key read 3,000
times), encaps (3,000 encapsulations with fixed messages, Go's
mlkemtest.Encapsulate768) and decaps (3,000 decapsulations); `none` is
the setup alone. twincheck agrees on all 5 workloads.

`xo bench --ab --rounds=10`, A the Go twin (go1.26.1), B the Xo program
(native, --release, cross-built on the Mac for linux/arm64 from
96502f843 plus this change), on the linux/arm64 offload host (host 1,
exclusive), with the 0.48.0 seed's xo; Xo / Go:

| workload | Go wall | Xo wall | wall (95%) | max RSS |
|---|---|---|---|---|
| none | 0.73 ms | 0.433 ms | 0.593 (0.56 to 0.62) | 0.291 |
| keygen | 92.26 ms | 51.78 ms | 0.561 (0.558 to 0.564) | 0.111 |
| parse | 37.85 ms | 33.36 ms | 0.882 (0.878 to 0.885) | 0.115 |
| encaps | 99.9 ms | 29.42 ms | 0.294 (0.294 to 0.304) | 0.115 |
| decaps | 90.13 ms | 53.41 ms | 0.593 (0.583 to 0.595) | 0.206 |

Instructions: not reported on Linux.

Xo is faster on every row: the std port runs on std/native/llrt/mlkem.xo,
the ML-KEM core the TLS client already used for X25519MLKEM768 (sped up
2026-10-16, 091c80bf), now with the encapsulation key's own parse and
encaps. Why encaps gains most (0.29) is not profiled yet:
`XO_PROF=cpu` on both against Go's pprof is the next step if a
reader needs it.
