# Native backends against LLVM, both architectures, 2026-11-20 (ADR 0727 baseline)

Xo Lang 1, task #480 (owner 2026-11-20, xw decision #203: performance
work is compiler wide and scored on every bench twin plus the codegen
corpus). The programs of amd64-native-vs-llvm-2026-10-22.md (the HTTP
server's five scenarios, atomic, ctx, digest, json, portable, rsa, text)
and of arm64-vs-llvm-closeouts-2026-10-24.md (fault, RSA sign,
dur_parse, trim), measured again on main: the native arm64 backend and
the native x86-64 backend, each against LLVM at the build defaults, one
xo for both sides, every binary built with XO_INLINE_STRICT=1.

Result: on arm64 native runs fewer instructions than LLVM on six of the
eight benches (geometric means 0.607 to 0.948) and more on portable
(1.151) and fault (1.382). On x86-64 it runs fewer on ctx and digest
(0.742, 0.926) and more on the rest (1.033 to 1.537); the TLS and
HTTP/2 server paths are below LLVM (0.744, 0.739), plain HTTP above
(1.059). Native builds a bench in 1.3 to 6.4 s on arm64 and 2.0 to 5.9 s
on x86-64, LLVM in 8.3 to 38.4 s and 13.2 to 50.9 s. The ranked list of
what trails is at the end.

## How

- Instructions: callgrind's "Collected" total of one run of each
  workload (each bench's measure.sh list), natively on each host, no
  Docker: linux/arm64 host 2 (18 CPUs, valgrind 3.24.0, clang 23.1.3) at
  8dd8e32c9, and the linux/amd64 host (28 CPUs, valgrind 3.22.0, clang
  18.1.3) at d2dbc9517, which has the x86-64 loop_split of decision 0724
  (7a308f44a). Neither host has valgrind installed: its .deb is unpacked
  in a scratch directory and run from there (`apt-get download valgrind
  libc6-dbg`, `dpkg -x`, `VALGRIND_LIB`). The 2026-10-22 file counted in
  Docker's emulated linux/amd64 with a cross built xo; these counts are
  of binaries the host's own xo built.
- The HTTP server: bench/perf/http/callgrind.sh (N=100, REPS=2, the
  minimum of the two), instructions a request.
- Build time: `xo build` of the bench with a cold backend cache and the
  front end's caches warm (the second of two builds), wall seconds, the
  host not exclusive. The x86-64 times are of the run at 8dd8e32c9; the
  run at d2dbc9517 shared the host with other jobs.
- Wall time (arm64): host 2 under an exclusive lease, 30 interleaved
  rounds of each workload, the median of the per round ratios (decision
  0726's method), the whole process timed. Host 1, where `xo bench --ab`
  runs, did not hold main; host 2 is the std evidence's timing host.
  Rows under 10 ms are mostly the start of the process.
- Not countable on arm64: the LLVM build of digest argon2 and the LLVM
  HTTP server's tls, tls-newconn and h2 scenarios. Under valgrind 3.24
  the LLVM argon2 run stops at an instruction valgrind does not decode
  (0xCE020400, of arm64's SHA3 extension); run directly it prints the
  same checksum as native. The LLVM server did not come up under
  callgrind for the TLS scenarios (not examined further). Those arm64
  rows have a wall time or no LLVM figure.

## arm64 (linux/arm64 host 2, 8dd8e32c9)

| arch | bench | workloads | geomean | min | max | native build s | LLVM build s |
|---|---|---:|---:|---:|---:|---:|---:|
| arm64 | atomic | 8 | 0.882 | 0.501 | 1.363 | 1.43 | 9.09 |
| arm64 | ctx | 5 | 0.607 | 0.436 | 0.814 | 2.33 | 8.32 |
| arm64 | digest | 5 | 0.747 | 0.676 | 0.896 | 3.44 | 11.85 |
| arm64 | json | 11 | 0.917 | 0.856 | 0.995 | 2.29 | 14.54 |
| arm64 | portable | 10 | 1.151 | 0.902 | 1.585 | 1.52 | 9.71 |
| arm64 | rsa | 4 | 0.948 | 0.933 | 0.957 | 6.35 | 38.38 |
| arm64 | text | 25 | 0.925 | 0.638 | 1.071 | 1.33 | 11.18 |
| arm64 | fault | 3 | 1.382 | 1.119 | 1.725 | 2.82 | 8.6 |

HTTP server, instructions a request:

| arch | scenario | native | LLVM | native / LLVM |
|---|---|---:|---:|---:|
| arm64 | plain | 28106 | 29041 | 0.968 |
| arm64 | plain-newconn | 37972 | 40195 | 0.945 |
| arm64 | tls | 42417 | - | - |
| arm64 | tls-newconn | 1625352 | - | - |
| arm64 | h2 | 80082 | - | - |

Wall time of the rows that trail most (native / LLVM):

| bench | workload | LLVM ms | native ms | native / LLVM (median of 30 ratios) | min | max |
| fault | defer | 44.3 | 60.5 | 1.373 | 1.305 | 1.401 |
| fault | isolate | 12.4 | 13.8 | 1.108 | 1.053 | 1.236 |
| fault | owned | 25.2 | 23.8 | 0.942 | 0.914 | 1.042 |
| portable | arith32 | 7.9 | 8.7 | 1.094 | 1.021 | 1.190 |
| portable | divmod | 7.8 | 7.7 | 0.996 | 0.926 | 1.043 |
| portable | arith8 | 7.3 | 7.3 | 0.987 | 0.756 | 1.121 |
| portable | u64 | 9.2 | 9.6 | 1.045 | 0.965 | 1.124 |
| portable | shift | 2.9 | 3.3 | 1.120 | 0.925 | 1.215 |
| portable | str_hash | 10.0 | 13.8 | 1.377 | 0.887 | 1.513 |
| portable | conv | 8.3 | 9.5 | 1.125 | 1.080 | 1.185 |
| atomic | cas | 8.5 | 8.6 | 0.998 | 0.934 | 1.110 |
| atomic | swap | 6.3 | 6.6 | 1.062 | 0.965 | 1.137 |
| rsa | sign2048 | 39.6 | 37.1 | 0.939 | 0.921 | 0.980 |
| rsa | sign4096 | 151.5 | 143.2 | 0.946 | 0.863 | 0.964 |
| text | dur_parse | 5.1 | 5.2 | 1.027 | 0.900 | 1.129 |
| text | trim | 3.4 | 4.2 | 1.237 | 1.050 | 1.437 |
| text | float_fmt | 5.5 | 5.3 | 0.981 | 0.920 | 1.065 |
| text | dur_fmt | 4.7 | 5.3 | 1.118 | 1.040 | 1.215 |
| digest | argon2 | 9.9 | 10.6 | 1.067 | 0.916 | 1.267 |

## x86-64 (linux/amd64 host, d2dbc9517)

| arch | bench | workloads | geomean | min | max | native build s | LLVM build s |
|---|---|---:|---:|---:|---:|---:|---:|
| amd64 | atomic | 8 | 1.096 | 0.394 | 2.576 | 2.14 | 16.73 |
| amd64 | ctx | 5 | 0.742 | 0.519 | 0.889 | 2.42 | 13.23 |
| amd64 | digest | 5 | 0.926 | 0.731 | 1.038 | 2.22 | 14.49 |
| amd64 | json | 11 | 1.045 | 0.998 | 1.146 | 2.17 | 14.49 |
| amd64 | portable | 10 | 1.136 | 0.921 | 1.447 | 2.01 | 14.32 |
| amd64 | rsa | 4 | 1.183 | 1.157 | 1.219 | 5.94 | 50.85 |
| amd64 | text | 25 | 1.033 | 0.659 | 1.244 | 2.01 | 16.58 |
| amd64 | fault | 3 | 1.537 | 1.215 | 1.854 | 2.01 | 13.67 |

HTTP server, instructions a request:

| arch | scenario | native | LLVM | native / LLVM |
|---|---|---:|---:|---:|
| amd64 | plain | 32813 | 30987 | 1.059 |
| amd64 | plain-newconn | 47735 | 46131 | 1.035 |
| amd64 | tls | 56439 | 75874 | 0.744 |
| amd64 | tls-newconn | 2930015 | 2939531 | 0.997 |
| amd64 | h2 | 101636 | 137590 | 0.739 |

atomic's callgrind ratios on x86-64 (set 2.576, cas 1.993, get 1.329)
are as on 2026-10-22 (2.16, 1.71, 1.55), where the Intel Mac's
instructions retired showed both backends equal on set and get:
callgrind runs the bench's threads one at a time. They are not ranked
below.

## std/go on x86-64 (not evidence)

bench/perf/gobridge on the linux/amd64 host at 851817d39, `stdproven
perf -rounds 30`, after the twin change of xw decision #207 (std/go's
evidence is linux/arm64 host 2: std/evidence/go/perf.md). Before it the
host measured bytes 1.151 and none 1.126 (task #479).

| workload | Go ms | Xo ms | Xo / Go |
|---|---|---|---|
| bench/perf/gobridge/bytes | 110.3 | 111.4 | 1.010 |
| bench/perf/gobridge/diag_bytes_checked (diagnostic, not judged) | 108.7 | 120.8 | 1.109 |
| bench/perf/gobridge/diag_none_checked (diagnostic, not judged) | 163.5 | 204.9 | 1.253 |
| bench/perf/gobridge/diag_startup (diagnostic, not judged) | 1.5 | 2.5 | 1.663 |
| bench/perf/gobridge/diag_str_checked (diagnostic, not judged) | 143.5 | 160.8 | 1.124 |
| bench/perf/gobridge/int | 132.7 | 125.8 | 0.953 |
| bench/perf/gobridge/none | 144.0 | 138.5 | 0.961 |
| bench/perf/gobridge/str | 145.8 | 152.9 | 1.040 |

## What trails, ranked (the compiler wide work list)

1. Faults and defers, both backends: fault defer 1.725 instructions on
   arm64 (1.373 wall) and 1.854 on x86-64, isolate 1.366 and 1.612,
   owned 1.119 and 1.215. The 2026-10-24 closeout had defer at 1.056
   instructions and 0.915 wall on macOS; on Linux the gap is in the
   open again.
2. Narrow and checked integer arithmetic, both backends (portable):
   arith32 1.585 arm64 and 1.288 x86-64, divmod 1.315 and 1.282, arith8
   1.217 and 1.447, shift 1.182 and 1.243, u64 1.183, str_hash 1.173
   (1.377 wall, a wide spread), conv 1.132 (1.125 wall). The 2026-10-22
   file's cause stands: LLVM proves ranges and narrows; native keeps
   the 64 bit sequences and tests.
3. RSA on x86-64: sign2048 1.219, sign4096 1.195, verify 1.157 to
   1.162 (arm64 is below LLVM, 0.933 to 0.957, and faster on the wall,
   0.939 and 0.946).
4. Text formatting and scanning on x86-64: float_fmt 1.244, trim 1.220,
   dur_fmt 1.190, zone_offset 1.181, time_fmt 1.167 (arm64: 1.071,
   1.062, 1.067, 1.067; trim 1.237 on the wall at 3 to 4 ms a run).
5. Plain HTTP on x86-64: 1.059 a request, a new connection 1.035
   (arm64 0.968 and 0.945).
6. JSON on x86-64: enc_floats 1.146, the bench 1.045 (arm64 0.917).
7. atomic cas and swap on arm64: 1.363 and 1.166 instructions, 0.998
   and 1.062 on the wall: counted, not felt.

Every row at or over 1.05, worst first:

| arch | bench | workload | native | LLVM | ratio |
|---|---|---|---:|---:|---:|
| arm64 | fault | defer | 1593959663 | 923910388 | 1.725 |
| arm64 | portable | arith32 | 65101339 | 41081836 | 1.585 |
| arm64 | fault | isolate | 313136297 | 229265356 | 1.366 |
| arm64 | atomic | cas | 30059752 | 22056121 | 1.363 |
| arm64 | portable | divmod | 167100936 | 127082667 | 1.315 |
| arm64 | portable | arith8 | 45105110 | 37071942 | 1.217 |
| arm64 | portable | u64 | 39115107 | 33073502 | 1.183 |
| arm64 | portable | shift | 65105970 | 55083667 | 1.182 |
| arm64 | portable | str_hash | 300945854 | 256545809 | 1.173 |
| arm64 | atomic | swap | 28060304 | 24056189 | 1.166 |
| arm64 | portable | conv | 107662119 | 95077837 | 1.132 |
| arm64 | fault | owned | 574998536 | 513650142 | 1.119 |
| arm64 | text | float_fmt | 100862977 | 94144456 | 1.071 |
| arm64 | text | dur_fmt | 113763853 | 106624785 | 1.067 |
| arm64 | text | zone_offset | 63699445 | 59720609 | 1.067 |
| arm64 | text | trim | 67340488 | 63400257 | 1.062 |

| amd64 | atomic | set | 12089797 | 4693153 | 2.576 |
| amd64 | atomic | cas | 28088932 | 14093156 | 1.993 |
| amd64 | fault | defer | 1815291728 | 978880561 | 1.854 |
| amd64 | fault | isolate | 382119782 | 237111028 | 1.612 |
| amd64 | portable | arith8 | 65424635 | 45217179 | 1.447 |
| amd64 | atomic | get | 12088829 | 9093149 | 1.329 |
| amd64 | portable | arith32 | 99424424 | 77209218 | 1.288 |
| amd64 | portable | divmod | 273425758 | 213213217 | 1.282 |
| amd64 | text | float_fmt | 129353644 | 103946064 | 1.244 |
| amd64 | portable | shift | 103429633 | 83221611 | 1.243 |
| amd64 | text | trim | 89066087 | 73026374 | 1.220 |
| amd64 | rsa | sign2048 | 1049127525 | 860735171 | 1.219 |
| amd64 | fault | owned | 636583113 | 523836893 | 1.215 |
| amd64 | rsa | sign4096 | 6078275441 | 5087421454 | 1.195 |
| amd64 | text | dur_fmt | 155610137 | 130802793 | 1.190 |
| amd64 | text | zone_offset | 83056869 | 70318471 | 1.181 |
| amd64 | text | time_fmt | 171861346 | 147223986 | 1.167 |
| amd64 | rsa | verify2048 | 37245127 | 32046098 | 1.162 |
| amd64 | rsa | verify4096 | 126124840 | 109002245 | 1.157 |
| amd64 | atomic | flag | 40089307 | 34760390 | 1.153 |
| amd64 | json | enc_floats | 135601690 | 118328957 | 1.146 |
| amd64 | portable | str_hash | 372530970 | 330133803 | 1.128 |
| amd64 | portable | conv | 167960512 | 151242005 | 1.111 |
| amd64 | text | utf8 | 134855387 | 122268264 | 1.103 |
| amd64 | text | quote | 124952194 | 113984171 | 1.096 |
| amd64 | text | parse_int | 30113508 | 27515210 | 1.094 |
| amd64 | json | maps | 431319016 | 395909886 | 1.089 |
| amd64 | text | zone_civil | 113774532 | 104651629 | 1.087 |
| amd64 | json | strings | 277655070 | 256183296 | 1.084 |
| amd64 | text | dur_parse | 122962352 | 114593211 | 1.073 |
| amd64 | text | time_parse | 48879846 | 45614399 | 1.072 |
| amd64 | text | int_fmt | 67824274 | 63539657 | 1.067 |
| amd64 | json | small | 292692457 | 274572385 | 1.066 |
| amd64 | text | base64 | 158052173 | 148275528 | 1.066 |
| amd64 | atomic | swap | 34089411 | 32092870 | 1.062 |
| amd64 | text | zone_fmt | 137186935 | 129821882 | 1.057 |
| amd64 | json | enc_small | 126187664 | 120142998 | 1.050 |

## Every workload

| arch | bench | workload | native | LLVM | ratio | checksums |
|---|---|---|---:|---:|---:|---|
| arm64 | atomic | none | 57745 | 53962 | 1.070 | agree |
| arm64 | atomic | get | 12058367 | 14056092 | 0.858 | agree |
| arm64 | atomic | set | 8058131 | 10053815 | 0.801 | agree |
| arm64 | atomic | swap | 28060304 | 24056189 | 1.166 | agree |
| arm64 | atomic | cas | 30059752 | 22056121 | 1.363 | agree |
| arm64 | atomic | add | 46060243 | 64056129 | 0.719 | agree |
| arm64 | atomic | max | 34067336 | 34062213 | 1.000 | agree |
| arm64 | atomic | flag | 26060238 | 28056102 | 0.929 | agree |
| arm64 | atomic | contended | 24111126 | 48103278 | 0.501 | agree |
| arm64 | ctx | none | 128314 | 60760 | 2.112 | agree |
| arm64 | ctx | defer | 272059639 | 582055430 | 0.467 | agree |
| arm64 | ctx | mutex | 832059824 | 1346077114 | 0.618 | agree |
| arm64 | ctx | spawn | 31240496 | 38768182 | 0.806 | agree |
| arm64 | ctx | budget | 603790477 | 1383574509 | 0.436 | agree |
| arm64 | ctx | await | 40676233 | 49950460 | 0.814 | agree |
| arm64 | digest | none | 1237020 | 4055262 | 0.305 | agree |
| arm64 | digest | sha256_big | 25249399 | 28184288 | 0.896 | agree |
| arm64 | digest | sha256_small | 133245740 | 181762810 | 0.733 | agree |
| arm64 | digest | hmac | 157992320 | 233710304 | 0.676 | agree |
| arm64 | digest | pbkdf2 | 353306124 | 478343363 | 0.739 | agree |
| arm64 | digest | argon2 | 168390587 | 237412877 | 0.709 | DIFFER |
| arm64 | json | none | 21333576 | 22841242 | 0.934 | agree |
| arm64 | json | small | 232685268 | 249296765 | 0.933 | agree |
| arm64 | json | ints | 108267582 | 120019832 | 0.902 | agree |
| arm64 | json | floats | 164282375 | 173316981 | 0.948 | agree |
| arm64 | json | strings | 214657954 | 234002762 | 0.917 | agree |
| arm64 | json | maps | 344193400 | 360165577 | 0.956 | agree |
| arm64 | json | parse | 206261564 | 224700457 | 0.918 | agree |
| arm64 | json | invalid | 167069028 | 189683024 | 0.881 | agree |
| arm64 | json | enc_small | 99914699 | 114122034 | 0.876 | agree |
| arm64 | json | enc_strings | 178349848 | 208365779 | 0.856 | agree |
| arm64 | json | enc_floats | 105134442 | 105689543 | 0.995 | agree |
| arm64 | json | bytes | 200622035 | 218920730 | 0.916 | agree |
| arm64 | portable | none | 1072534 | 1042012 | 1.029 | agree |
| arm64 | portable | arith32 | 65101339 | 41081836 | 1.585 | agree |
| arm64 | portable | arith8 | 45105110 | 37071942 | 1.217 | agree |
| arm64 | portable | u64 | 39115107 | 33073502 | 1.183 | agree |
| arm64 | portable | divmod | 167100936 | 127082667 | 1.315 | agree |
| arm64 | portable | shift | 65105970 | 55083667 | 1.182 | agree |
| arm64 | portable | conv | 107662119 | 95077837 | 1.132 | agree |
| arm64 | portable | str_hash | 300945854 | 256545809 | 1.173 | agree |
| arm64 | portable | unix | 55102974 | 61057870 | 0.902 | agree |
| arm64 | portable | checked | 111115288 | 107078758 | 1.038 | agree |
| arm64 | portable | float32 | 47173322 | 51080739 | 0.924 | agree |
| arm64 | rsa | sign2048 | 761317825 | 806168915 | 0.944 | agree |
| arm64 | rsa | verify2048 | 27518313 | 29479322 | 0.933 | agree |
| arm64 | rsa | sign4096 | 4478546012 | 4677876762 | 0.957 | agree |
| arm64 | rsa | verify4096 | 94371730 | 98595219 | 0.957 | agree |
| arm64 | text | none | 6186417 | 6415165 | 0.964 | agree |
| arm64 | text | parse_int | 22176697 | 22605345 | 0.981 | agree |
| arm64 | text | dur_fmt | 113763853 | 106624785 | 1.067 | agree |
| arm64 | text | dur_parse | 94377515 | 95175207 | 0.992 | agree |
| arm64 | text | time_fmt | 125734079 | 122820579 | 1.024 | agree |
| arm64 | text | time_parse | 38737048 | 40368710 | 0.960 | agree |
| arm64 | text | quote | 91664747 | 104676512 | 0.876 | agree |
| arm64 | text | split | 45673863 | 48000244 | 0.952 | agree |
| arm64 | text | lines | 55838317 | 63109630 | 0.885 | agree |
| arm64 | text | replace | 52606112 | 59533416 | 0.884 | agree |
| arm64 | text | trim | 67340488 | 63400257 | 1.062 | agree |
| arm64 | text | rune_count | 20808597 | 23713079 | 0.878 | agree |
| arm64 | text | cut | 118245057 | 153604474 | 0.770 | agree |
| arm64 | text | cut_prefix | 46627130 | 73041386 | 0.638 | agree |
| arm64 | text | cut_suffix | 49027150 | 71641417 | 0.684 | agree |
| arm64 | text | utf8 | 101879830 | 101927273 | 1.000 | agree |
| arm64 | text | int_fmt | 53093201 | 59341467 | 0.895 | agree |
| arm64 | text | float_fmt | 100862977 | 94144456 | 1.071 | agree |
| arm64 | text | float_parse | 125127767 | 125409786 | 0.998 | agree |
| arm64 | text | base64 | 129064057 | 128456929 | 1.005 | agree |
| arm64 | text | hex | 71664886 | 84100200 | 0.852 | agree |
| arm64 | text | debug | 201463738 | 229451075 | 0.878 | agree |
| arm64 | text | zone_offset | 63699445 | 59720609 | 1.067 | agree |
| arm64 | text | zone_fmt | 119718722 | 131137965 | 0.913 | agree |
| arm64 | text | zone_civil | 93658410 | 96472922 | 0.971 | agree |
| arm64 | text | zone_load | 31970093 | 32001357 | 0.999 | agree |
| arm64 | fault | none | 56418 | 51659 | 1.092 | agree |
| arm64 | fault | isolate | 313136297 | 229265356 | 1.366 | agree |
| arm64 | fault | defer | 1593959663 | 923910388 | 1.725 | agree |
| arm64 | fault | owned | 574998536 | 513650142 | 1.119 | agree |
| amd64 | atomic | none | 89191 | 93153 | 0.957 | agree |
| amd64 | atomic | get | 12088829 | 9093149 | 1.329 | agree |
| amd64 | atomic | set | 12089797 | 4693153 | 2.576 | agree |
| amd64 | atomic | swap | 34089411 | 32092870 | 1.062 | agree |
| amd64 | atomic | cas | 28088932 | 14093156 | 1.993 | agree |
| amd64 | atomic | add | 54089348 | 82093545 | 0.659 | agree |
| amd64 | atomic | max | 50091545 | 52095873 | 0.962 | agree |
| amd64 | atomic | flag | 40089307 | 34760390 | 1.153 | agree |
| amd64 | atomic | contended | 22141822 | 56136797 | 0.394 | agree |
| amd64 | ctx | none | 110095 | 100486 | 1.096 | agree |
| amd64 | ctx | defer | 504090735 | 686095585 | 0.735 | agree |
| amd64 | ctx | mutex | 856089637 | 1148093268 | 0.746 | agree |
| amd64 | ctx | spawn | 36464882 | 41070556 | 0.888 | agree |
| amd64 | ctx | budget | 604329641 | 1163788112 | 0.519 | agree |
| amd64 | ctx | await | 45538120 | 51231408 | 0.889 | agree |
| amd64 | digest | none | 1424944 | 4107307 | 0.347 | agree |
| amd64 | digest | sha256_big | 736816461 | 735921792 | 1.001 | agree |
| amd64 | digest | sha256_small | 670986260 | 715846260 | 0.937 | agree |
| amd64 | digest | hmac | 934307286 | 977325393 | 0.956 | agree |
| amd64 | digest | pbkdf2 | 1590786514 | 1531873518 | 1.038 | agree |
| amd64 | digest | argon2 | 216760151 | 296676261 | 0.731 | agree |
| amd64 | json | none | 26618808 | 24365451 | 1.092 | agree |
| amd64 | json | small | 292692457 | 274572385 | 1.066 | agree |
| amd64 | json | ints | 132821468 | 130352764 | 1.019 | agree |
| amd64 | json | floats | 200856391 | 201270117 | 0.998 | agree |
| amd64 | json | strings | 277655070 | 256183296 | 1.084 | agree |
| amd64 | json | maps | 431319016 | 395909886 | 1.089 | agree |
| amd64 | json | parse | 253180322 | 248018963 | 1.021 | agree |
| amd64 | json | invalid | 199439034 | 198621985 | 1.004 | agree |
| amd64 | json | enc_small | 126187664 | 120142998 | 1.050 | agree |
| amd64 | json | enc_strings | 224744883 | 222721219 | 1.009 | agree |
| amd64 | json | enc_floats | 135601690 | 118328957 | 1.146 | agree |
| amd64 | json | bytes | 258626110 | 254131968 | 1.018 | agree |
| amd64 | portable | none | 1367009 | 1191186 | 1.148 | agree |
| amd64 | portable | arith32 | 99424424 | 77209218 | 1.288 | agree |
| amd64 | portable | arith8 | 65424635 | 45217179 | 1.447 | agree |
| amd64 | portable | u64 | 55430372 | 53223413 | 1.041 | agree |
| amd64 | portable | divmod | 273425758 | 213213217 | 1.282 | agree |
| amd64 | portable | shift | 103429633 | 83221611 | 1.243 | agree |
| amd64 | portable | conv | 167960512 | 151242005 | 1.111 | agree |
| amd64 | portable | str_hash | 372530970 | 330133803 | 1.128 | agree |
| amd64 | portable | unix | 91386521 | 99178863 | 0.921 | agree |
| amd64 | portable | checked | 159420929 | 159219258 | 1.001 | agree |
| amd64 | portable | float32 | 83491261 | 83271586 | 1.003 | agree |
| amd64 | rsa | sign2048 | 1049127525 | 860735171 | 1.219 | agree |
| amd64 | rsa | verify2048 | 37245127 | 32046098 | 1.162 | agree |
| amd64 | rsa | sign4096 | 6078275441 | 5087421454 | 1.195 | agree |
| amd64 | rsa | verify4096 | 126124840 | 109002245 | 1.157 | agree |
| amd64 | text | none | 7802721 | 7003887 | 1.114 | agree |
| amd64 | text | parse_int | 30113508 | 27515210 | 1.094 | agree |
| amd64 | text | dur_fmt | 155610137 | 130802793 | 1.190 | agree |
| amd64 | text | dur_parse | 122962352 | 114593211 | 1.073 | agree |
| amd64 | text | time_fmt | 171861346 | 147223986 | 1.167 | agree |
| amd64 | text | time_parse | 48879846 | 45614399 | 1.072 | agree |
| amd64 | text | quote | 124952194 | 113984171 | 1.096 | agree |
| amd64 | text | split | 58139890 | 55882489 | 1.040 | agree |
| amd64 | text | lines | 74660187 | 75626219 | 0.987 | agree |
| amd64 | text | replace | 70571854 | 70107261 | 1.007 | agree |
| amd64 | text | trim | 89066087 | 73026374 | 1.220 | agree |
| amd64 | text | rune_count | 25829642 | 39197748 | 0.659 | agree |
| amd64 | text | cut | 182437206 | 194827613 | 0.936 | agree |
| amd64 | text | cut_prefix | 62253140 | 81655323 | 0.762 | agree |
| amd64 | text | cut_suffix | 74053179 | 81055327 | 0.914 | agree |
| amd64 | text | utf8 | 134855387 | 122268264 | 1.103 | agree |
| amd64 | text | int_fmt | 67824274 | 63539657 | 1.067 | agree |
| amd64 | text | float_fmt | 129353644 | 103946064 | 1.244 | agree |
| amd64 | text | float_parse | 158121800 | 157468853 | 1.004 | agree |
| amd64 | text | base64 | 158052173 | 148275528 | 1.066 | agree |
| amd64 | text | hex | 98963899 | 101010385 | 0.980 | agree |
| amd64 | text | debug | 254379325 | 245108061 | 1.038 | agree |
| amd64 | text | zone_offset | 83056869 | 70318471 | 1.181 | agree |
| amd64 | text | zone_fmt | 137186935 | 129821882 | 1.057 | agree |
| amd64 | text | zone_civil | 113774532 | 104651629 | 1.087 | agree |
| amd64 | text | zone_load | 37755117 | 37768948 | 1.000 | agree |
| amd64 | fault | none | 89889 | 95005 | 0.946 | agree |
| amd64 | fault | isolate | 382119782 | 237111028 | 1.612 | agree |
| amd64 | fault | defer | 1815291728 | 978880561 | 1.854 | agree |
| amd64 | fault | owned | 636583113 | 523836893 | 1.215 | agree |
