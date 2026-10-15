# zero-c against main: bench/perf sweep (2026-10-15)

Branch zero-c at 8c1d123d (B31 post-quantum) against main at dd33dd13
(decision 0232 accepted), each xo built from its own tip, on the
development Mac (Apple silicon, macOS 27.0). The zero-c commits after
8c1d123d (22fcd354, d8e423bf, 4048b622, 193caa6b) are not in it.

## Method

- Every bench/perf program was built by both xo binaries with
  `--backend=arm64` and `--backend=llvm`, and every workload was run 5
  times. The order was interleaved: repetition, then workload, then
  backend, then side, with main first on even repetitions and zero-c
  first on odd ones. The whole sweep ran under the machine-wide heavy
  lock.
- The metric is instructions retired (`/usr/bin/time -l`), the median
  of 5 runs. Wall time is the median in ms, and `-` means under 10 ms.
- Spread is (max - min) / median of the 5 runs, per side.
- A workload is flagged (**bold** ratio) when zero-c/main is off 1 by
  more than 5% and by more than the larger spread.
- The programs and how they were run:
  - testworld: the `xo test --no-gc` binary, with XO_TEST=0..6 (none,
    stdio, env, rand, log, gen, memfs).
  - conc: the six xo/ programs.
  - httpclient: the Go serve/ on loopback, N=3000 (h2len and h2big are
    HTTP/2 over TLS).
  - tls: the Go serve/, N=200 handshakes, or 64 MB for the bulk
    workloads. Each side uses its default TLS. On main that is Mbed (the
    C), and on zero-c it is the Xo TLS (decision 0202). This is the
    "against main's Mbed" comparison.
  - http: the task 01 server, loaded by loadgen (32 workers, 1 s warmup,
    5 s measured), plain HTTP/1.1 and HTTPS with H2=1. Its metric is the
    server's CPU us per request; requests per second are listed beside
    it.
- Profiles were taken with macOS `sample` (self time, top of stack) on
  longer runs of the same binaries.
- The scripts are not in the repo: sweep.py (the driver), analyze.py,
  and prof.py. The raw records are 3,020 runs.

## Summary

- **Faster on zero-c** (39 workloads beyond the bar):
  - Startup (`none`), 0.87 to 0.90 on arm64.
  - nested, 0.88 to 0.94 on arm64.
  - unicode lower_u, upper_u and eqlower, 0.88 to 0.94.
  - Several json and text workloads on llvm, 0.87 to 0.95.
  - testworld gen, 0.86 and 0.91.
  - conc rwmutex llvm, 0.83.
- **TLS in Xo against Mbed** (xo-fa, the TLS perf owner).
  - Handshakes:
    - x25519, rsa2048 and tls12: 2.8 to 3.0x on arm64, 1.9 to 2.0x on
      llvm.
    - p256 and p384: 1.54x on arm64, 1.03 to 1.04x on llvm.
    - Every handshake workload verifies the server's P-256 certificate,
      so all of them pay for the P-256 field multiply. On zero-c,
      `nistec.mul4` takes 34 to 48% of samples and `nistec.add` 10 to
      14% (std/native/llrt/nistec.xo), where main has fiat_p256_mul.
    - X25519 shows as `ed25519.fe_mul`, 5 to 7%
      (std/native/llrt/ed25519.xo).
    - The arm64/llvm gap on the same Xo code (1.54x against 1.04x on
      p256) says the arm64 backend's code for mul4 is the larger part.
  - Bulk:
    - aes128, aes256 and aes128tls13: 2.6 to 2.8x on arm64, 1.9 to
      2.0x on llvm.
    - chacha: 4.1x on arm64, 2.9x on llvm.
    - Hot spots: `aesgcm.hw4` 23%, `Gcm.mul` 14%, `Gcm.ctr` 10% and
      `ghash` 6% (std/native/llrt/aesgcm.xo), against
      mbedtls_aesce_crypt_ecb on main. For chacha, `chacha.xor_stream`
      52% and `absorb` (Poly1305) 24% (std/native/llrt/chacha.xo).
    - Wall time: 50 ms on main against 230 ms on zero-c for 64 MB of
      AES-GCM.
  - ed25519: main's Mbed client fails the handshake against the Go
    server ("remote error: tls: handshake failure", 5 of 5 runs, both
    backends). Zero-c's Xo client completes it (about 200 ms), so this
    row has no ratio.
  - httpclient h2big: 1.85x arm64, 1.44x llvm. h2len: 1.12x arm64. The
    same AES-GCM hot spot: `aesgcm.hw4` is 25% of samples next to 30%
    in sendto.
  - http h2 (server, HTTPS): 1.45x arm64 and 1.39x llvm CPU per
    request, with requests per second at 37k and 41k against main's
    57k and 58k. This is the server side of the same AES-GCM and
    handshake code (tlssrv over tls13srv).
- **Scheduler, channels and contexts** (Xo Lang 3):
  - sysio chan: 1.17x arm64, 1.11x llvm.
  - conc pingpong: 1.14x arm64, 1.12x llvm.
  - conc fanout arm64: 1.10x. conc spawn llvm and ctx spawn: 1.06 to
    1.08x.
    - The chan profile: `pool.steal_round`, `pool.park_wait` and an
      extra `tasks.park_wait_n` on zero-c (std/native/llrt/pool.xo,
      tasks.xo).
  - ctx mutex arm64: 1.20x. ctx budget arm64: 1.13x. ctx defer arm64:
    1.05x.
    - Zero-c calls `ctx.cur` (std/native/llrt/ctx.xo, a call around
      `rt.tlocal_get(0)`) where main's C inlined cur_get. It is 11%
      (mutex) and 17% (budget) of samples.
    - mutex also pays `mutex.held_add` and `held_del`, 7% and 5%
      (std/native/llrt/mutex.xo), against the C held_check.
  - conc mutex arm64: 1.37x, with a 30% spread on main. This is
    contended spinning, and its wall time is 80 ms on main against
    10 ms on zero-c. It is not a regression.
- **Clock** (xo-4): sysio clock arm64: 1.08x. Both sides spend 75 to
  83% in mach_absolute_time. Zero-c adds clock_gettime_nsec_np and the
  run loop's call (a few instructions per Clock.now).
- **Not flagged**:
  - http http1, within its 40% spread.
  - httpclient len, chunked, big and redirect, within 1%.
  - testworld stdio, env, rand, log and memfs, within 3%.
  - fsproc, net, digest and fault, all within the bar.

## Fixes after the sweep (xo-4, 2026-10-15)

Instructions retired, the median of 3 runs. "After" is the zero-c tip
193caa6b with the change. "Before" is the same tip without it on
arm64, and the sweep's 8c1d123d build on LLVM.

| Workload | Backend | main | before | after | after/main |
|---|---|---|---|---|---|
| ctx mutex | arm64 | 934.10M | 1121.0M | 972.3M | 1.041 |
| ctx mutex | llvm | 817.96M | 769.6M | 701.4M | 0.858 |
| sysio clock | arm64 | 93.34M | 100.4M | 92.9M | 0.995 |
| sysio clock | llvm | 87.19M | 87.5M | 83.4M | 0.957 |

- ctx mutex: in mutex.xo, held_add got a fast path that calls
  nothing, and its fault message and growth moved out into
  held_fault and held_grow. Before, it saved nine registers on every
  Mutex.with (gotchas/arm64-call-anywhere-costs-frame.md). held_del
  searches from the end, with a fast path for one entry. The rest of
  the gap is ctx.cur, which Xo Lang 3 is taking.
- sysio clock: sysio.clock_now goes straight to clock_gettime_nsec_np
  on macOS. The first version, which tested the target with `==`, was
  8% slower on arm64 (gotchas/arm64-target-enum-eq-not-folded.md).

## Results

| Workload | Backend | main | zero-c | zero-c/main | spread main | spread zero-c | Other |
|---|---|---|---|---|---|---|---|
| conc fanout | arm64 | 302.47M | 331.57M | **1.096** | 2.4% | 6.7% | wall 30.0 / 30.0 ms |
| conc fanout | llvm | 266.22M | 275.13M | 1.033 | 4.7% | 5.4% | wall 30.0 / 20.0 ms |
| conc mutex | arm64 | 136.78M | 187.04M | **1.367** | 30.4% | 16.1% | wall 80.0 / 10.0 ms |
| conc mutex | llvm | 130.03M | 120.15M | 0.924 | 21.1% | 47.3% | wall 75.0 / 70.0 ms |
| conc pingpong | arm64 | 414.46M | 472.76M | **1.141** | 1.1% | 1.3% | wall 20.0 / 30.0 ms |
| conc pingpong | llvm | 364.50M | 407.24M | **1.117** | 5.1% | 6.0% | wall 20.0 / 30.0 ms |
| conc pmap | arm64 | 3887.97M | 3886.62M | 1.000 | 0.1% | 0.1% | wall 70.0 / 70.0 ms |
| conc pmap | llvm | 4199.68M | 4200.64M | 1.000 | 0.0% | 0.1% | wall 70.0 / 70.0 ms |
| conc rwmutex | arm64 | 1069.60M | 978.08M | 0.914 | 12.6% | 10.3% | wall 140.0 / 100.0 ms |
| conc rwmutex | llvm | 972.47M | 803.08M | **0.826** | 7.9% | 3.8% | wall 130.0 / 90.0 ms |
| conc spawn | arm64 | 595.62M | 681.62M | 1.144 | 14.9% | 2.7% | wall 50.0 / 60.0 ms |
| conc spawn | llvm | 542.23M | 583.94M | **1.077** | 7.6% | 7.5% | wall 60.0 / 60.0 ms |
| http h2 | arm64 | 29.2 | 42.2 | **1.446** | 38.2% | 10.3% | rps 56630 / 37036 |
| http h2 | llvm | 28.2 | 39.2 | **1.387** | 15.0% | 7.3% | rps 58202 / 41388 |
| http http1 | arm64 | 17.5 | 19.9 | 1.132 | 47.3% | 42.5% | rps 97497 / 96369 |
| http http1 | llvm | 19.9 | 20.1 | 1.014 | 39.6% | 44.2% | rps 99919 / 98824 |
| httpclient big | arm64 | 1009.71M | 1001.59M | 0.992 | 0.5% | 0.6% | wall 270.0 / 250.0 ms |
| httpclient big | llvm | 997.14M | 978.28M | 0.981 | 1.0% | 0.4% | wall 240.0 / 240.0 ms |
| httpclient chunked | arm64 | 888.11M | 896.32M | 1.009 | 2.3% | 3.3% | wall 240.0 / 240.0 ms |
| httpclient chunked | llvm | 869.71M | 864.08M | 0.994 | 3.3% | 0.8% | wall 210.0 / 210.0 ms |
| httpclient h2big | arm64 | 5479.17M | 10149.09M | **1.852** | 2.1% | 0.7% | wall 690.0 / 830.0 ms |
| httpclient h2big | llvm | 5454.71M | 7853.75M | **1.440** | 1.4% | 1.6% | wall 680.0 / 830.0 ms |
| httpclient h2len | arm64 | 614.18M | 686.82M | **1.118** | 1.3% | 1.1% | wall 140.0 / 150.0 ms |
| httpclient h2len | llvm | 599.50M | 621.49M | 1.037 | 1.4% | 3.1% | wall 140.0 / 150.0 ms |
| httpclient len | arm64 | 843.74M | 849.78M | 1.007 | 1.0% | 0.5% | wall 200.0 / 210.0 ms |
| httpclient len | llvm | 831.19M | 826.84M | 0.995 | 0.5% | 0.7% | wall 200.0 / 200.0 ms |
| httpclient none | arm64 | 28.00M | 27.57M | 0.985 | 4.8% | 6.3% | wall 140.0 / 150.0 ms |
| httpclient none | llvm | 25.69M | 25.04M | 0.975 | 4.1% | 4.7% | wall 130.0 / 150.0 ms |
| httpclient redirect | arm64 | 1603.54M | 1607.40M | 1.002 | 0.5% | 0.6% | wall 420.0 / 440.0 ms |
| httpclient redirect | llvm | 1587.37M | 1581.27M | 0.996 | 0.4% | 0.5% | wall 430.0 / 430.0 ms |
| alloc grow | arm64 | 67.02M | 65.07M | 0.971 | 0.3% | 0.2% | wall - / - ms |
| alloc grow | llvm | 64.44M | 63.81M | 0.990 | 0.3% | 0.2% | wall 10.0 / - ms |
| alloc maps | arm64 | 303.89M | 301.64M | 0.993 | 0.0% | 0.0% | wall 10.0 / 10.0 ms |
| alloc maps | llvm | 266.63M | 266.33M | 0.999 | 0.0% | 0.0% | wall 10.0 / 10.0 ms |
| alloc mixed | arm64 | 472.68M | 470.47M | 0.995 | 0.1% | 0.1% | wall 20.0 / 20.0 ms |
| alloc mixed | llvm | 462.38M | 462.20M | 1.000 | 0.1% | 0.1% | wall 20.0 / 20.0 ms |
| alloc none | arm64 | 17.17M | 14.89M | **0.867** | 8.5% | 8.1% | wall 130.0 / 130.0 ms |
| alloc none | llvm | 14.53M | 14.00M | 0.964 | 5.6% | 6.2% | wall 120.0 / 130.0 ms |
| alloc small | arm64 | 294.12M | 290.11M | 0.986 | 0.0% | 0.0% | wall 30.0 / 30.0 ms |
| alloc small | llvm | 247.93M | 249.34M | 1.006 | 0.3% | 0.3% | wall 40.0 / 40.0 ms |
| alloc strings | arm64 | 306.70M | 301.07M | 0.982 | 0.1% | 0.1% | wall 10.0 / 20.0 ms |
| alloc strings | llvm | 277.81M | 257.08M | **0.925** | 0.1% | 0.1% | wall 20.0 / 10.0 ms |
| alloc tasks | arm64 | 884.71M | 887.48M | 1.003 | 0.4% | 0.4% | wall 10.0 / 10.0 ms |
| alloc tasks | llvm | 789.09M | 788.31M | 0.999 | 0.4% | 0.3% | wall 10.0 / 10.0 ms |
| atomic add | arm64 | 118.03M | 116.00M | 0.983 | 0.2% | 0.1% | wall 10.0 / 10.0 ms |
| atomic add | llvm | 79.82M | 79.29M | 0.993 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| atomic cas | arm64 | 40.00M | 38.02M | 0.950 | 0.2% | 0.6% | wall - / - ms |
| atomic cas | llvm | 39.83M | 39.28M | 0.986 | 1.7% | 0.2% | wall - / - ms |
| atomic contended | arm64 | 106.42M | 105.73M | 0.993 | 5.9% | 3.8% | wall 80.0 / 90.0 ms |
| atomic contended | llvm | 76.56M | 75.63M | 0.988 | 4.6% | 3.6% | wall 80.0 / 70.0 ms |
| atomic flag | arm64 | 41.33M | 39.36M | 0.952 | 0.1% | 0.3% | wall - / - ms |
| atomic flag | llvm | 42.51M | 41.26M | 0.971 | 0.3% | 0.1% | wall - / - ms |
| atomic get | arm64 | 29.99M | 28.03M | **0.935** | 0.7% | 0.2% | wall - / - ms |
| atomic get | llvm | 27.82M | 27.28M | 0.980 | 0.6% | 0.2% | wall - / - ms |
| atomic max | arm64 | 52.02M | 50.01M | 0.961 | 0.1% | 0.1% | wall - / - ms |
| atomic max | llvm | 47.85M | 47.27M | 0.988 | 0.1% | 0.1% | wall - / - ms |
| atomic none | arm64 | 17.08M | 14.98M | **0.877** | 9.9% | 9.6% | wall 130.0 / 130.0 ms |
| atomic none | llvm | 14.52M | 13.90M | 0.957 | 6.8% | 6.4% | wall 120.0 / 130.0 ms |
| atomic set | arm64 | 26.12M | 24.05M | **0.921** | 1.0% | 0.7% | wall - / - ms |
| atomic set | llvm | 23.87M | 23.28M | 0.975 | 0.4% | 0.2% | wall - / - ms |
| atomic swap | arm64 | 40.00M | 38.04M | 0.951 | 0.4% | 0.2% | wall - / - ms |
| atomic swap | llvm | 37.82M | 37.26M | 0.985 | 0.1% | 0.1% | wall - / - ms |
| ctx await | arm64 | 99.07M | 100.54M | 1.015 | 13.9% | 16.9% | wall 10.0 / 10.0 ms |
| ctx await | llvm | 84.93M | 88.92M | 1.047 | 5.6% | 8.4% | wall - / 10.0 ms |
| ctx budget | arm64 | 1402.12M | 1580.09M | **1.127** | 0.0% | 0.0% | wall 90.0 / 90.0 ms |
| ctx budget | llvm | 1319.65M | 1359.02M | 1.030 | 0.0% | 0.0% | wall 60.0 / 60.0 ms |
| ctx defer | arm64 | 1360.11M | 1430.13M | **1.051** | 0.0% | 0.0% | wall 60.0 / 70.0 ms |
| ctx defer | llvm | 1184.04M | 1197.35M | 1.011 | 0.0% | 0.0% | wall 60.0 / 60.0 ms |
| ctx mutex | arm64 | 934.10M | 1120.15M | **1.199** | 0.1% | 0.0% | wall 40.0 / 50.0 ms |
| ctx mutex | llvm | 817.96M | 769.35M | **0.941** | 0.0% | 0.0% | wall 30.0 / 30.0 ms |
| ctx none | arm64 | 17.24M | 15.27M | 0.886 | 11.3% | 12.3% | wall 150.0 / 130.0 ms |
| ctx none | llvm | 14.70M | 14.04M | 0.955 | 8.5% | 8.1% | wall 120.0 / 130.0 ms |
| ctx spawn | arm64 | 90.59M | 96.43M | **1.064** | 4.3% | 5.1% | wall 10.0 / - ms |
| ctx spawn | llvm | 78.19M | 84.38M | **1.079** | 4.2% | 7.8% | wall - / - ms |
| digest argon2 | arm64 | 195.12M | 193.01M | 0.989 | 0.1% | 0.2% | wall 10.0 / 10.0 ms |
| digest argon2 | llvm | 204.00M | 202.56M | 0.993 | 0.2% | 0.2% | wall 10.0 / 10.0 ms |
| digest hmac | arm64 | 786.77M | 783.99M | 0.996 | 0.0% | 0.0% | wall 30.0 / 30.0 ms |
| digest hmac | llvm | 654.65M | 638.27M | 0.975 | 0.0% | 0.0% | wall 30.0 / 30.0 ms |
| digest none | arm64 | 19.87M | 17.84M | **0.898** | 9.0% | 9.3% | wall 150.0 / 140.0 ms |
| digest none | llvm | 16.60M | 15.30M | **0.922** | 6.2% | 6.4% | wall 130.0 / 130.0 ms |
| digest pbkdf2 | arm64 | 1330.66M | 1328.28M | 0.998 | 0.0% | 0.0% | wall 50.0 / 50.0 ms |
| digest pbkdf2 | llvm | 1062.85M | 1061.80M | 0.999 | 0.0% | 0.0% | wall 50.0 / 50.0 ms |
| digest sha256_big | arm64 | 575.86M | 573.86M | 0.997 | 0.0% | 0.0% | wall 20.0 / 20.0 ms |
| digest sha256_big | llvm | 472.60M | 471.21M | 0.997 | 0.0% | 0.0% | wall 20.0 / 20.0 ms |
| digest sha256_small | arm64 | 563.69M | 560.70M | 0.995 | 0.0% | 0.0% | wall 30.0 / 30.0 ms |
| digest sha256_small | llvm | 479.73M | 466.65M | 0.973 | 0.0% | 0.0% | wall 30.0 / 30.0 ms |
| fault defer | arm64 | 1590.47M | 1583.36M | 0.996 | 0.1% | 0.1% | wall 60.0 / 60.0 ms |
| fault defer | llvm | 1314.51M | 1336.27M | 1.017 | 0.1% | 0.2% | wall 60.0 / 60.0 ms |
| fault isolate | arm64 | 290.92M | 291.34M | 1.001 | 0.5% | 0.5% | wall 10.0 / 10.0 ms |
| fault isolate | llvm | 308.04M | 317.05M | 1.029 | 0.1% | 0.5% | wall 10.0 / 10.0 ms |
| fault none | arm64 | 17.00M | 14.90M | **0.877** | 8.1% | 7.5% | wall 140.0 / 140.0 ms |
| fault none | llvm | 14.26M | 13.95M | 0.978 | 5.5% | 5.2% | wall 120.0 / 120.0 ms |
| fault owned | arm64 | 931.02M | 927.87M | 0.997 | 0.0% | 0.2% | wall 40.0 / 40.0 ms |
| fault owned | llvm | 765.07M | 777.50M | 1.016 | 0.0% | 0.2% | wall 30.0 / 40.0 ms |
| fsproc cwd | arm64 | 3566.30M | 3564.90M | 1.000 | 0.2% | 0.5% | wall 210.0 / 210.0 ms |
| fsproc cwd | llvm | 3557.19M | 3556.16M | 1.000 | 0.3% | 0.2% | wall 200.0 / 210.0 ms |
| fsproc mkdir | arm64 | 1447.22M | 1446.43M | 0.999 | 0.8% | 0.7% | wall 80.0 / 80.0 ms |
| fsproc mkdir | llvm | 1448.86M | 1448.16M | 1.000 | 0.4% | 0.6% | wall 80.0 / 80.0 ms |
| fsproc none | arm64 | 19.30M | 17.32M | 0.898 | 12.3% | 12.8% | wall 190.0 / 140.0 ms |
| fsproc none | llvm | 17.10M | 16.08M | 0.940 | 8.1% | 8.2% | wall 130.0 / 130.0 ms |
| fsproc proc | arm64 | 263.45M | 259.49M | 0.985 | 0.9% | 1.5% | wall 190.0 / 190.0 ms |
| fsproc proc | llvm | 257.10M | 252.94M | 0.984 | 0.9% | 0.5% | wall 180.0 / 190.0 ms |
| fsproc proc_plain | arm64 | 234.24M | 231.76M | 0.989 | 1.0% | 1.4% | wall 180.0 / 180.0 ms |
| fsproc proc_plain | llvm | 232.10M | 230.29M | 0.992 | 0.6% | 0.8% | wall 180.0 / 190.0 ms |
| fsproc remove | arm64 | 2256.83M | 2256.42M | 1.000 | 0.6% | 0.2% | wall 150.0 / 150.0 ms |
| fsproc remove | llvm | 2251.68M | 2253.56M | 1.001 | 0.6% | 1.3% | wall 150.0 / 150.0 ms |
| fsproc temp | arm64 | 1325.19M | 1323.70M | 0.999 | 0.4% | 0.5% | wall 80.0 / 70.0 ms |
| fsproc temp | llvm | 1313.51M | 1314.24M | 1.001 | 0.7% | 0.6% | wall 80.0 / 70.0 ms |
| fsproc tree | arm64 | 1166.15M | 1164.18M | 0.998 | 0.8% | 1.1% | wall 80.0 / 70.0 ms |
| fsproc tree | llvm | 1161.57M | 1160.97M | 0.999 | 1.0% | 2.0% | wall 70.0 / 70.0 ms |
| json bytes | arm64 | 425.54M | 422.67M | 0.993 | 0.0% | 0.0% | wall 20.0 / 20.0 ms |
| json bytes | llvm | 329.87M | 312.00M | **0.946** | 0.0% | 0.0% | wall 10.0 / 10.0 ms |
| json enc_floats | arm64 | 145.72M | 144.37M | 0.991 | 0.1% | 0.1% | wall - / - ms |
| json enc_floats | llvm | 116.79M | 111.04M | 0.951 | 0.1% | 0.1% | wall - / - ms |
| json enc_small | arm64 | 150.96M | 149.63M | 0.991 | 0.2% | 0.1% | wall 10.0 / 10.0 ms |
| json enc_small | llvm | 133.79M | 123.35M | **0.922** | 0.2% | 0.1% | wall - / - ms |
| json enc_strings | arm64 | 191.98M | 190.63M | 0.993 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| json enc_strings | llvm | 182.76M | 165.18M | **0.904** | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| json floats | arm64 | 381.87M | 379.75M | 0.994 | 0.0% | 0.0% | wall 20.0 / 20.0 ms |
| json floats | llvm | 336.31M | 340.99M | 1.014 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| json ints | arm64 | 235.60M | 233.52M | 0.991 | 0.0% | 0.0% | wall 10.0 / 10.0 ms |
| json ints | llvm | 209.16M | 210.64M | 1.007 | 0.2% | 0.1% | wall 10.0 / 10.0 ms |
| json invalid | arm64 | 218.46M | 216.51M | 0.991 | 0.0% | 0.0% | wall 10.0 / 10.0 ms |
| json invalid | llvm | 197.57M | 196.64M | 0.995 | 0.1% | 0.0% | wall 10.0 / 10.0 ms |
| json maps | arm64 | 339.84M | 337.17M | 0.992 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| json maps | llvm | 303.89M | 302.05M | 0.994 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| json none | arm64 | 41.13M | 39.81M | 0.968 | 5.4% | 5.5% | wall 140.0 / 140.0 ms |
| json none | llvm | 34.96M | 34.36M | 0.983 | 3.6% | 4.6% | wall 130.0 / 130.0 ms |
| json parse | arm64 | 288.51M | 287.21M | 0.995 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| json parse | llvm | 254.07M | 253.32M | 0.997 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| json small | arm64 | 346.03M | 344.03M | 0.994 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| json small | llvm | 303.93M | 301.00M | 0.990 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| json strings | arm64 | 363.25M | 354.75M | 0.977 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| json strings | llvm | 301.97M | 289.67M | 0.959 | 0.0% | 0.1% | wall 10.0 / 10.0 ms |
| nested index2 | arm64 | 18.75M | 16.76M | **0.894** | 0.3% | 0.7% | wall - / - ms |
| nested index2 | llvm | 15.52M | 14.94M | 0.963 | 0.6% | 1.0% | wall - / - ms |
| nested mapput | arm64 | 24.89M | 22.90M | **0.920** | 0.9% | 0.9% | wall - / - ms |
| nested mapput | llvm | 20.58M | 19.88M | 0.966 | 0.7% | 1.1% | wall - / - ms |
| nested method | arm64 | 17.04M | 15.06M | **0.884** | 0.6% | 0.5% | wall - / - ms |
| nested method | llvm | 14.52M | 13.97M | 0.962 | 0.6% | 0.3% | wall - / - ms |
| nested push | arm64 | 19.05M | 17.02M | **0.893** | 7.9% | 8.2% | wall 140.0 / 130.0 ms |
| nested push | llvm | 15.72M | 15.31M | 0.974 | 4.6% | 5.0% | wall 130.0 / 130.0 ms |
| nested setdeep | arm64 | 20.13M | 18.17M | **0.903** | 0.2% | 0.4% | wall - / - ms |
| nested setdeep | llvm | 16.55M | 16.01M | 0.968 | 0.6% | 0.3% | wall - / - ms |
| nested setnested | arm64 | 17.07M | 15.08M | **0.884** | 0.3% | 0.8% | wall - / - ms |
| nested setnested | llvm | 14.70M | 14.14M | 0.962 | 0.8% | 0.6% | wall - / - ms |
| nested shared | arm64 | 23.64M | 21.65M | **0.916** | 0.7% | 0.4% | wall - / - ms |
| nested shared | llvm | 21.16M | 20.58M | 0.972 | 0.3% | 0.6% | wall - / - ms |
| nested strappend | arm64 | 18.84M | 16.85M | **0.894** | 0.2% | 0.9% | wall - / - ms |
| nested strappend | llvm | 16.06M | 15.54M | 0.967 | 0.7% | 1.0% | wall - / - ms |
| nested walk | arm64 | 31.67M | 29.64M | **0.936** | 0.1% | 0.1% | wall - / - ms |
| nested walk | llvm | 27.81M | 27.34M | 0.983 | 0.2% | 0.3% | wall - / - ms |
| net dial | arm64 | 586.79M | 590.44M | 1.006 | 2.4% | 1.8% | wall 80.0 / 80.0 ms |
| net dial | llvm | 583.80M | 580.14M | 0.994 | 2.7% | 3.1% | wall 80.0 / 80.0 ms |
| net echo | arm64 | 2708.03M | 2729.50M | 1.008 | 2.0% | 4.1% | wall 420.0 / 420.0 ms |
| net echo | llvm | 2694.80M | 2671.97M | 0.992 | 4.3% | 2.3% | wall 410.0 / 410.0 ms |
| net none | arm64 | 17.82M | 16.76M | 0.941 | 10.8% | 11.5% | wall 140.0 / 140.0 ms |
| net none | llvm | 16.60M | 15.54M | 0.936 | 7.0% | 7.5% | wall 130.0 / 120.0 ms |
| net resolve | arm64 | 984.52M | 987.30M | 1.003 | 2.3% | 2.0% | wall 320.0 / 320.0 ms |
| net resolve | llvm | 978.63M | 979.26M | 1.001 | 2.4% | 2.7% | wall 320.0 / 320.0 ms |
| stdio big | arm64 | 125.39M | 122.75M | 0.979 | 19.8% | 9.2% | wall 60.0 / 60.0 ms |
| stdio big | llvm | 121.30M | 121.14M | 0.999 | 5.0% | 18.1% | wall 60.0 / 50.0 ms |
| stdio err | arm64 | 132.14M | 130.45M | 0.987 | 1.1% | 2.7% | wall 10.0 / 10.0 ms |
| stdio err | llvm | 129.16M | 128.90M | 0.998 | 3.3% | 2.2% | wall 10.0 / 10.0 ms |
| stdio ints | arm64 | 197.98M | 193.96M | 0.980 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| stdio ints | llvm | 167.23M | 161.17M | 0.964 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| stdio lines | arm64 | 108.59M | 106.71M | 0.983 | 2.5% | 1.6% | wall - / - ms |
| stdio lines | llvm | 41.25M | 40.67M | 0.986 | 3.6% | 1.9% | wall - / 10.0 ms |
| stdio mixed | arm64 | 134.01M | 131.95M | 0.985 | 0.7% | 0.7% | wall 10.0 / 10.0 ms |
| stdio mixed | llvm | 129.99M | 129.59M | 0.997 | 1.7% | 1.0% | wall 10.0 / 10.0 ms |
| stdio none | arm64 | 17.02M | 14.95M | **0.879** | 9.1% | 11.0% | wall 140.0 / 130.0 ms |
| stdio none | llvm | 14.38M | 13.87M | 0.964 | 7.4% | 7.5% | wall 120.0 / 120.0 ms |
| stdio small | arm64 | 116.52M | 114.54M | 0.983 | 0.1% | 0.2% | wall - / - ms |
| stdio small | llvm | 44.31M | 43.76M | 0.988 | 0.2% | 0.1% | wall - / - ms |
| sysio chan | arm64 | 400.44M | 467.05M | **1.166** | 3.2% | 4.6% | wall 20.0 / 20.0 ms |
| sysio chan | llvm | 357.45M | 395.97M | **1.108** | 3.6% | 5.2% | wall 20.0 / 20.0 ms |
| sysio clock | arm64 | 93.34M | 100.39M | **1.076** | 0.1% | 0.2% | wall - / - ms |
| sysio clock | llvm | 87.19M | 86.49M | 0.992 | 0.3% | 0.1% | wall - / - ms |
| sysio copy | arm64 | 3885.86M | 3964.39M | 1.020 | 5.0% | 5.1% | wall 250.0 / 270.0 ms |
| sysio copy | llvm | 3936.72M | 3939.24M | 1.001 | 3.2% | 7.8% | wall 260.0 / 260.0 ms |
| sysio dir | arm64 | 8829.68M | 9055.52M | 1.026 | 0.1% | 0.2% | wall 500.0 / 510.0 ms |
| sysio dir | llvm | 8115.56M | 8202.23M | 1.011 | 0.6% | 0.1% | wall 460.0 / 450.0 ms |
| sysio handle | arm64 | 3957.30M | 3966.71M | 1.002 | 0.1% | 0.1% | wall 230.0 / 230.0 ms |
| sysio handle | llvm | 3920.02M | 3921.09M | 1.000 | 0.0% | 0.1% | wall 230.0 / 230.0 ms |
| sysio lines | arm64 | 3608.03M | 3757.38M | 1.041 | 0.1% | 0.0% | wall 170.0 / 190.0 ms |
| sysio lines | llvm | 2829.25M | 2672.01M | **0.944** | 0.1% | 0.1% | wall 120.0 / 120.0 ms |
| sysio lock | arm64 | 5741.78M | 5749.24M | 1.001 | 0.1% | 0.2% | wall 310.0 / 310.0 ms |
| sysio lock | llvm | 5709.12M | 5711.18M | 1.000 | 0.0% | 0.1% | wall 310.0 / 310.0 ms |
| sysio mode | arm64 | 3730.61M | 3743.99M | 1.004 | 0.2% | 0.2% | wall 210.0 / 200.0 ms |
| sysio mode | llvm | 3676.28M | 3680.57M | 1.001 | 0.3% | 0.3% | wall 200.0 / 200.0 ms |
| sysio none | arm64 | 18.44M | 16.59M | **0.900** | 7.1% | 6.7% | wall 140.0 / 130.0 ms |
| sysio none | llvm | 16.22M | 15.58M | 0.961 | 4.7% | 3.6% | wall 130.0 / 130.0 ms |
| sysio read | arm64 | 3761.27M | 3767.00M | 1.002 | 0.0% | 0.0% | wall 220.0 / 220.0 ms |
| sysio read | llvm | 3729.73M | 3730.99M | 1.000 | 0.1% | 0.0% | wall 220.0 / 210.0 ms |
| sysio read_big | arm64 | 167.24M | 165.41M | 0.989 | 0.1% | 0.2% | wall 10.0 / 10.0 ms |
| sysio read_big | llvm | 163.55M | 147.20M | **0.900** | 0.1% | 0.2% | wall 10.0 / 10.0 ms |
| sysio rename | arm64 | 15198.56M | 15191.56M | 1.000 | 0.8% | 0.6% | wall 800.0 / 820.0 ms |
| sysio rename | llvm | 15145.01M | 15158.08M | 1.001 | 0.6% | 0.6% | wall 770.0 / 760.0 ms |
| sysio sleep | arm64 | 774.27M | 783.26M | 1.012 | 1.6% | 2.2% | wall 90.0 / 90.0 ms |
| sysio sleep | llvm | 766.09M | 772.48M | 1.008 | 1.6% | 1.9% | wall 90.0 / 90.0 ms |
| sysio stat | arm64 | 1768.79M | 1822.11M | 1.030 | 0.1% | 0.0% | wall 100.0 / 100.0 ms |
| sysio stat | llvm | 1595.09M | 1615.45M | 1.013 | 0.0% | 0.0% | wall 80.0 / 80.0 ms |
| sysio tree | arm64 | 9100.33M | 9114.08M | 1.002 | 0.8% | 1.2% | wall 570.0 / 580.0 ms |
| sysio tree | llvm | 9128.79M | 9117.47M | 0.999 | 1.9% | 0.7% | wall 580.0 / 570.0 ms |
| sysio write | arm64 | 9396.58M | 9567.15M | 1.018 | 6.8% | 7.4% | wall 630.0 / 650.0 ms |
| sysio write | llvm | 9582.98M | 9665.47M | 1.009 | 5.6% | 5.4% | wall 670.0 / 650.0 ms |
| text base64 | arm64 | 168.03M | 165.24M | 0.983 | 1.2% | 0.2% | wall 10.0 / 10.0 ms |
| text base64 | llvm | 129.57M | 123.56M | 0.954 | 0.2% | 0.2% | wall 10.0 / 10.0 ms |
| text debug | arm64 | 227.25M | 225.09M | 0.991 | 0.4% | 0.4% | wall 10.0 / 10.0 ms |
| text debug | llvm | 201.21M | 187.54M | **0.932** | 0.4% | 0.1% | wall 10.0 / 10.0 ms |
| text dur_fmt | arm64 | 142.62M | 139.84M | 0.981 | 0.2% | 0.1% | wall 10.0 / 10.0 ms |
| text dur_fmt | llvm | 111.22M | 111.20M | 1.000 | 0.3% | 0.1% | wall - / 10.0 ms |
| text dur_parse | arm64 | 175.07M | 173.13M | 0.989 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| text dur_parse | llvm | 151.44M | 150.55M | 0.994 | 0.1% | 0.2% | wall 10.0 / 10.0 ms |
| text float_fmt | arm64 | 148.15M | 145.37M | 0.981 | 0.2% | 0.6% | wall 10.0 / 10.0 ms |
| text float_fmt | llvm | 108.98M | 104.02M | 0.954 | 0.1% | 2.0% | wall 10.0 / - ms |
| text float_parse | arm64 | 153.90M | 151.97M | 0.987 | 0.1% | 0.6% | wall 10.0 / 10.0 ms |
| text float_parse | llvm | 138.73M | 138.03M | 0.995 | 0.3% | 0.1% | wall 10.0 / 10.0 ms |
| text hex | arm64 | 102.35M | 99.66M | 0.974 | 0.1% | 0.2% | wall 10.0 / 10.0 ms |
| text hex | llvm | 85.25M | 83.62M | 0.981 | 0.1% | 0.3% | wall - / 10.0 ms |
| text int_fmt | arm64 | 80.66M | 77.90M | 0.966 | 0.2% | 0.3% | wall 10.0 / 10.0 ms |
| text int_fmt | llvm | 65.07M | 61.56M | **0.946** | 0.2% | 0.0% | wall 10.0 / - ms |
| text lines | arm64 | 157.77M | 154.26M | 0.978 | 0.2% | 0.1% | wall 10.0 / 10.0 ms |
| text lines | llvm | 133.30M | 138.45M | 1.039 | 0.1% | 0.2% | wall 10.0 / 10.0 ms |
| text none | arm64 | 23.90M | 22.01M | 0.921 | 12.0% | 12.6% | wall 140.0 / 140.0 ms |
| text none | llvm | 20.45M | 19.77M | 0.967 | 8.8% | 9.4% | wall 140.0 / 140.0 ms |
| text parse_int | arm64 | 46.80M | 44.81M | 0.957 | 0.5% | 0.3% | wall 10.0 / - ms |
| text parse_int | llvm | 44.29M | 43.56M | 0.983 | 0.5% | 0.1% | wall - / - ms |
| text quote | arm64 | 131.08M | 128.81M | 0.983 | 0.1% | 1.0% | wall 10.0 / 10.0 ms |
| text quote | llvm | 112.16M | 102.71M | **0.916** | 0.1% | 0.1% | wall 10.0 / - ms |
| text replace | arm64 | 83.06M | 80.55M | 0.970 | 0.4% | 0.2% | wall 10.0 / 10.0 ms |
| text replace | llvm | 65.91M | 57.19M | **0.868** | 0.2% | 0.4% | wall - / 20.0 ms |
| text rune_count | arm64 | 41.88M | 39.92M | 0.953 | 0.4% | 0.2% | wall - / - ms |
| text rune_count | llvm | 38.21M | 37.52M | 0.982 | 0.4% | 0.8% | wall - / - ms |
| text split | arm64 | 98.60M | 96.59M | 0.980 | 0.2% | 1.0% | wall - / - ms |
| text split | llvm | 81.16M | 81.34M | 1.002 | 1.7% | 0.1% | wall - / 10.0 ms |
| text time_fmt | arm64 | 178.72M | 175.94M | 0.984 | 0.1% | 0.1% | wall 10.0 / 20.0 ms |
| text time_fmt | llvm | 128.61M | 125.11M | 0.973 | 0.2% | 0.2% | wall 10.0 / 10.0 ms |
| text time_parse | arm64 | 153.80M | 151.32M | 0.984 | 0.2% | 0.1% | wall 10.0 / 10.0 ms |
| text time_parse | llvm | 99.07M | 98.34M | 0.993 | 1.3% | 0.1% | wall 10.0 / - ms |
| text trim | arm64 | 157.69M | 154.60M | 0.980 | 0.2% | 0.1% | wall 10.0 / 10.0 ms |
| text trim | llvm | 146.09M | 143.75M | 0.984 | 0.2% | 0.1% | wall 10.0 / - ms |
| text utf8 | arm64 | 118.42M | 116.41M | 0.983 | 0.1% | 0.0% | wall 10.0 / 10.0 ms |
| text utf8 | llvm | 110.12M | 109.31M | 0.993 | 0.2% | 0.5% | wall 10.0 / 10.0 ms |
| text zone_civil | arm64 | 260.58M | 257.04M | 0.986 | 0.1% | 0.0% | wall 10.0 / 10.0 ms |
| text zone_civil | llvm | 224.94M | 221.07M | 0.983 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| text zone_fmt | arm64 | 992.38M | 1016.45M | 1.024 | 0.0% | 0.0% | wall 50.0 / 50.0 ms |
| text zone_fmt | llvm | 825.88M | 862.12M | 1.044 | 0.0% | 0.0% | wall 40.0 / 40.0 ms |
| text zone_load | arm64 | 52.38M | 50.48M | 0.964 | 0.4% | 0.2% | wall - / - ms |
| text zone_load | llvm | 44.74M | 44.00M | 0.984 | 0.5% | 0.2% | wall - / - ms |
| text zone_offset | arm64 | 99.16M | 97.14M | 0.980 | 1.2% | 1.2% | wall - / - ms |
| text zone_offset | llvm | 86.33M | 85.46M | 0.990 | 1.2% | 0.9% | wall - / - ms |
| unicode classify | arm64 | 265.99M | 260.15M | 0.978 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| unicode classify | llvm | 200.35M | 198.51M | 0.991 | 0.1% | 0.1% | wall - / - ms |
| unicode eqlower | arm64 | 12192.67M | 10740.68M | **0.881** | 0.0% | 0.0% | wall 580.0 / 470.0 ms |
| unicode eqlower | llvm | 10637.59M | 9741.67M | **0.916** | 0.0% | 0.0% | wall 410.0 / 380.0 ms |
| unicode lower | arm64 | 930.63M | 925.28M | 0.994 | 0.2% | 0.3% | wall 60.0 / 60.0 ms |
| unicode lower | llvm | 648.59M | 647.43M | 0.998 | 0.2% | 0.2% | wall 30.0 / 30.0 ms |
| unicode lower_u | arm64 | 4620.61M | 4113.46M | **0.890** | 0.0% | 0.0% | wall 230.0 / 180.0 ms |
| unicode lower_u | llvm | 4014.42M | 3717.39M | **0.926** | 0.0% | 0.0% | wall 150.0 / 150.0 ms |
| unicode quote | arm64 | 146.50M | 140.77M | 0.961 | 0.2% | 0.1% | wall - / - ms |
| unicode quote | llvm | 120.21M | 119.65M | 0.995 | 0.1% | 1.0% | wall - / - ms |
| unicode runecase | arm64 | 232.66M | 226.87M | 0.975 | 0.2% | 0.1% | wall 10.0 / 10.0 ms |
| unicode runecase | llvm | 222.30M | 220.40M | 0.991 | 0.1% | 0.1% | wall 10.0 / 10.0 ms |
| unicode upper | arm64 | 1068.79M | 1062.98M | 0.995 | 0.0% | 0.0% | wall 50.0 / 50.0 ms |
| unicode upper | llvm | 729.32M | 727.81M | 0.998 | 0.1% | 0.0% | wall 30.0 / 30.0 ms |
| unicode upper_u | arm64 | 4575.76M | 4107.86M | **0.898** | 0.0% | 0.0% | wall 220.0 / 180.0 ms |
| unicode upper_u | llvm | 3960.72M | 3708.65M | **0.936** | 0.0% | 0.0% | wall 160.0 / 150.0 ms |
| testworld 0 | arm64 | 15.91M | 13.99M | **0.879** | 8.1% | 9.6% | wall 150.0 / 140.0 ms |
| testworld 0 | llvm | 13.92M | 13.38M | 0.961 | 5.7% | 8.7% | wall 130.0 / 130.0 ms |
| testworld 1 | arm64 | 115.92M | 113.20M | 0.976 | 0.4% | 0.3% | wall 10.0 / - ms |
| testworld 1 | llvm | 96.28M | 93.12M | 0.967 | 1.0% | 0.2% | wall 10.0 / - ms |
| testworld 2 | arm64 | 2913.95M | 2910.25M | 0.999 | 0.1% | 0.1% | wall 130.0 / 130.0 ms |
| testworld 2 | llvm | 2743.64M | 2741.86M | 0.999 | 0.1% | 0.1% | wall 120.0 / 120.0 ms |
| testworld 3 | arm64 | 61.91M | 59.91M | 0.968 | 0.3% | 0.3% | wall - / - ms |
| testworld 3 | llvm | 36.14M | 35.59M | 0.985 | 0.8% | 0.2% | wall - / - ms |
| testworld 4 | arm64 | 230.55M | 229.03M | 0.993 | 0.5% | 0.7% | wall 10.0 / 10.0 ms |
| testworld 4 | llvm | 203.78M | 204.47M | 1.003 | 1.0% | 0.6% | wall 10.0 / 10.0 ms |
| testworld 5 | arm64 | 70.46M | 60.49M | **0.859** | 0.1% | 0.1% | wall - / - ms |
| testworld 5 | llvm | 55.18M | 50.42M | **0.914** | 0.1% | 0.1% | wall - / - ms |
| testworld 6 | arm64 | 68.57M | 68.03M | 0.992 | 0.3% | 0.1% | wall - / - ms |
| testworld 6 | llvm | 53.55M | 54.68M | 1.021 | 0.4% | 0.4% | wall - / - ms |
| tls aes128 | arm64 | 915.69M | 2460.78M | **2.687** | 2.4% | 0.7% | wall 50.0 / 230.0 ms |
| tls aes128 | llvm | 915.82M | 1758.59M | **1.920** | 1.7% | 0.9% | wall 50.0 / 230.0 ms |
| tls aes128tls13 | arm64 | 914.22M | 2403.23M | **2.629** | 1.2% | 0.6% | wall 50.0 / 230.0 ms |
| tls aes128tls13 | llvm | 916.92M | 1717.98M | **1.874** | 3.0% | 1.0% | wall 50.0 / 220.0 ms |
| tls aes256 | arm64 | 970.46M | 2692.81M | **2.775** | 3.2% | 0.4% | wall 70.0 / 290.0 ms |
| tls aes256 | llvm | 973.57M | 1958.74M | **2.012** | 2.2% | 0.9% | wall 60.0 / 280.0 ms |
| tls chacha | arm64 | 911.81M | 3703.89M | **4.062** | 3.4% | 0.3% | wall 120.0 / 200.0 ms |
| tls chacha | llvm | 909.46M | 2659.41M | **2.924** | 2.5% | 0.9% | wall 120.0 / 150.0 ms |
| tls ed25519 | arm64 | - | - | - | - | - | wall 10.0 / 220.0 ms (failed runs: main 5, zero-c 0: error: Connect(msg: "remote error: tls: handshake failure")) |
| tls ed25519 | llvm | - | - | - | - | - | wall 10.0 / 180.0 ms (failed runs: main 5, zero-c 0: error: Connect(msg: "remote error: tls: handshake failure")) |
| tls p256 | arm64 | 2570.48M | 3959.79M | **1.540** | 1.3% | 0.7% | wall 230.0 / 260.0 ms |
| tls p256 | llvm | 2567.32M | 2665.95M | 1.038 | 1.1% | 0.8% | wall 220.0 / 230.0 ms |
| tls p384 | arm64 | 4591.20M | 7073.87M | **1.541** | 0.7% | 0.8% | wall 380.0 / 450.0 ms |
| tls p384 | llvm | 4582.04M | 4725.19M | 1.031 | 0.9% | 1.1% | wall 370.0 / 380.0 ms |
| tls rsa2048 | arm64 | 965.12M | 2765.26M | **2.865** | 2.5% | 0.9% | wall 330.0 / 500.0 ms |
| tls rsa2048 | llvm | 968.70M | 1848.85M | **1.909** | 2.8% | 1.4% | wall 880.0 / 770.0 ms |
| tls tls12 | arm64 | 1155.34M | 3237.55M | **2.802** | 1.3% | 0.4% | wall 160.0 / 230.0 ms |
| tls tls12 | llvm | 1150.39M | 2208.11M | **1.919** | 0.5% | 0.5% | wall 140.0 / 190.0 ms |
| tls x25519 | arm64 | 1058.77M | 3166.59M | **2.991** | 0.5% | 0.5% | wall 120.0 / 210.0 ms |
| tls x25519 | llvm | 1057.13M | 2155.11M | **2.039** | 1.6% | 0.9% | wall 200.0 / 340.0 ms |

## Follow up: the scheduler paths (zc-b19, 2026-10-15)

Three fixes on zero-c after the sweep:

- tasks.xo park_wait_n (scheduler S1) made two C calls the C version
  never made on every wait: rt.sched_rt for the test world's record
  (now twflags.virt, two static words) and for ThreadSanitizer's
  acquire (now only when the services record's tsan word is set, as the
  C's TSAN_ACQUIRE compiled to nothing elsewhere).
- ctx.cur was not inlined: root() counted inside it put it over the
  arm64 inliner's budget. root is now behind root_out, kept out of line
  by a call cycle on a path never taken, so cur inlines (28 call sites
  in bench/perf/ctx).
- stdio.flush, called on every wait, was a real call to find an empty
  buffer: its work is now flush_out, out of line the same way, so the
  check inlines.

Instructions retired against main (dd33dd13, xo built from that tip),
the median of 9 interleaved runs per side (scratch script, the same
`/usr/bin/time -l` metric), zero-c at e6e502f0 plus the fixes:

| Workload | Backend | Sweep zero-c/main | Now zero-c/main |
|---|---|---|---|
| sysio chan | arm64 | 1.17 | 1.090 |
| sysio chan | llvm | 1.11 | 1.028 |
| ctx spawn | arm64 | 1.06 | 1.076 |
| ctx mutex | arm64 | 1.20 | 1.122 |
| ctx budget | arm64 | 1.13 | 0.999 |
| ctx defer | arm64 | 1.05 | 0.999 |
| ctx spawn | llvm | 1.08 | 1.072 |
| ctx mutex | llvm | - | 0.943 |
| ctx budget | llvm | - | 1.030 |
| ctx defer | llvm | - | 1.011 |
| conc pingpong | arm64 | 1.14 | 1.210 |
| conc pingpong | llvm | 1.12 | 1.007 |
| conc fanout | arm64 | 1.10 | 1.038 |
| conc fanout | llvm | - | 1.018 |
| conc spawn | arm64 | - | 1.115 |
| conc spawn | llvm | 1.08 | 1.081 |

ctx budget and defer are back to main's count. ctx mutex on arm64 is
xo-2b's (mutex.xo held_add and held_del). What remains above 5% is
spawning (ctx spawn, conc spawn: tasks.xo's spawn and run_claimed
against the C's) and chan on arm64 (chan.xo's send and receive against
the C's: a profile of 20,000,000 messages before these fixes, 1.108
times main's instructions, had chan.send and recv at 63 and 47 samples
against the C's 80 and 52, and tasks.park_wait_n at 78 with no
counterpart in main; not profiled again after them); conc pingpong on arm64 moves by 10% between runs of
the same binaries (threads), so its row is not conclusive.

## Follow up: HTTP/2 against TLS (xo-fa, 2026-10-15)

The lead asked whether the h2 rows are the HTTP/2 code (h2srv.xo,
hclient.xo) or TLS. Same method as above (5 interleaved repetitions,
medians), three sides: main at dd33dd13, zero-c at 879abb5c (after the
TLS work of decisions 0231 and steps 5 to 9 of
bench/results/tls-perf-2026-10-15.md), and the same zero-c built with
XO_LLRT_C=tlsio,tlssrv (Mbed TLS under the Xo HTTP/2 code).

| Workload | Backend | main | zero-c | zero-c/main | zero-c + Mbed/main |
|---|---|---|---|---|---|
| http h2 (server CPU us/req) | arm64 | 47.5 | 50.3 | 1.059 | 1.038 |
| http h2 (server CPU us/req) | llvm | 47.1 | 46.7 | 0.992 | 1.000 |
| http h2 rps | arm64 | 66087 | 62957 | 0.953 | 0.986 |
| http h2 rps | llvm | 64440 | 64287 | 0.998 | 1.009 |
| http http1 (control) | arm64 | 30.5 | 32.4 | 1.062 | 1.052 |
| httpclient h2big (M instr) | arm64 | 5253 | 7699 | 1.466 | 1.021 |
| httpclient h2big (M instr) | llvm | 5233 | 5946 | 1.136 | 1.015 |
| httpclient h2len (M instr) | arm64 | 623 | 692 | 1.112 | 1.037 |
| httpclient h2len (M instr) | llvm | 605 | 622 | 1.028 | 1.011 |
| httpclient big, HTTP/1.1 (control) | arm64 | 1053 | 1046 | 0.994 | 0.992 |

- The h2 server's 1.45x and 1.39x of the sweep are gone: it is within
  6% of main on arm64 and at main on llvm, with or without Mbed. That
  was the TLS (the handshakes and AES-GCM fixed since).
- With Mbed under it, the Xo HTTP/2 code is within 1.5 to 4% of main on
  every row, the same as the HTTP/1.1 controls: the h2 code itself is
  not the regression.
- h2big is still 1.47x on arm64 and 1.14x on llvm with the Xo TLS: the
  client decrypting the big body, AES-GCM's record path on arm64 (26M
  instructions a MiB against llvm's 18M and Mbed's 14M, results file
  step 8). That is the next step.
- After that step (aa859eb0: the keystream written in place, the body
  read in place, GHASH as a loop over H's words; tls-perf step 10), the
  same method again:

| Workload | Backend | main | zero-c | zero-c/main | zero-c + Mbed/main |
|---|---|---|---|---|---|
| httpclient h2big (M instr) | arm64 | 5220 | 6067 | 1.162 | 1.025 |
| httpclient h2big (M instr) | llvm | 5231 | 5207 | 0.995 | 1.011 |
| httpclient h2len (M instr) | arm64 | 617 | 670 | 1.086 | 1.045 |
| httpclient h2len (M instr) | llvm | 603 | 608 | 1.007 | 1.012 |

  h2big is at main on llvm; arm64's 1.16x is the arm64 backend's
  AES-GCM code (17.7M instructions a MiB against llvm's 14.1M).

### Spawning: a spare context per thread

tasks.xo run_claimed allocated, zeroed, and freed a 320 byte context from
the Xo heap for every task run (the C's was on its stack). It now keeps
one spare per thread (thread local word 13, from rt.heap_large like a
root context, so the leak check does not count it). Against zero-c
before this step (median of 9 interleaved runs, measured with the spare
in word 12, which only changes where the pointer lives):

| Workload | Backend | After/before |
|---|---|---|
| ctx spawn | arm64 | 0.979 |
| ctx spawn | llvm | 0.956 |
| concspawn  | arm64 | 0.965 |
| concspawn  | llvm | 0.987 |

With the earlier ratios against main, ctx spawn is about 1.05 (arm64)
and 1.03 (llvm) of main, conc spawn about 1.08 and 1.07: still over the
bar on arm64. The rest is spread over run_claimed's other work, the
run_entry service call (rt.sched_rt 29, a C frame by design), and the
closures' environment release.

The first try used word 12, sysio.xo's file operation message: xo itself
then exited 137 on every build (gotchas/thread-local-word-collision.md;
internal/repotest TestThreadLocalWordOwners now checks the words).

