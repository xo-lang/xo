# Native backends against LLVM, both architectures, 2026-11-20 (ADR 0727 baseline)

Xo Lang 1, task #480 (owner 2026-11-20, xw decision #203: performance
work is compiler wide and scored on every bench twin plus the codegen
corpus). The programs of amd64-native-vs-llvm-2026-10-22.md (the HTTP
server's five scenarios, atomic, ctx, digest, json, portable, rsa, text)
and of arm64-vs-llvm-closeouts-2026-10-24.md (fault, RSA sign,
dur_parse, trim), measured again on main: the native arm64 backend and
the native x86-64 backend, each against LLVM at the build defaults, one
xo for both sides, every binary built with XO_INLINE_STRICT=1.

Corrected the same day. The first version of this file (8f5a6531d)
gave only the "whole program" figures below as the result. Those are of
builds with a backend cache directory of their own, where the std
variant misses and xo builds the whole program: a first build on a
clean cache. The default build takes std separately compiled from its
variant ("cache std ... reused"), and its code differs: the headline is
now the default build, and the whole program column is a diagnostic.

Result (default build, instructions, native / LLVM): on arm64 native
runs fewer instructions than LLVM on ctx, digest, json and rsa
(geometric means 0.626, 0.750, 0.939, 0.948), about the same on text
(1.014), and more on fault, atomic and portable (1.159, 1.252, 1.268).
On x86-64 it runs fewer on ctx and digest (0.765, 0.927) and more on
the rest (json 1.069, text 1.125, rsa 1.175, fault 1.224, portable
1.251, atomic 1.461); the TLS and HTTP/2 server paths are below LLVM
(0.741, 0.725), plain HTTP above (1.070). The largest gaps are rows
where the default build is far behind the whole program build of the
same source: portable checked 3.036 on arm64 and 2.659 on x86-64 (1.038
and 1.001 whole program), atomic add 1.687 and 1.390 (0.719 and 0.659),
atomic contended 1.873 and 1.605 (0.501 and 0.394), text cut_prefix
1.611 on x86-64 (0.762). What a program's code calls in std is inlined
into it in a whole program build; why the default build's code is that
much longer on these rows is the first item of the list at the end.

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
- Default: `xo build --backend=B` with the host's caches, the build log
  saying "cache std ... reused" (checked for every native build). Whole
  program: the same with `XO_BACKEND_CACHE_DIR` set to an empty
  directory, the log saying "std variant: miss ... the whole program is
  built" (gotchas/private-backend-cache-builds-whole-program.md). LLVM
  builds the whole program in both; its count is the default run's.
- The HTTP server: bench/perf/http/callgrind.sh (N=100, REPS=2, the
  minimum of the two), instructions a request; the whole program
  column's server is built with XO_SEPCOMP=0.
- Build time: `xo build` of the whole program with a cold backend cache
  and the front end's caches warm (the second of two builds), wall
  seconds, the host not exclusive; the x86-64 text, rsa and fault times
  are of a run that shared the host with other jobs.
- Wall time (arm64): host 2 under an exclusive lease, default builds,
  30 interleaved rounds of each workload, the median of the per round
  ratios (decision 0726's method), the whole process timed. Host 1,
  where `xo bench --ab` runs, did not hold main; host 2 is the std
  evidence's timing host. Rows under 10 ms are mostly the start of the
  process.
- Not countable on arm64: the LLVM build of digest argon2 and the LLVM
  HTTP server's tls, tls-newconn and h2 scenarios. Under valgrind 3.24
  the LLVM argon2 run stops at an instruction valgrind does not decode
  (0xCE020400, of arm64's SHA3 extension); run directly it prints the
  same checksum as native. The LLVM server did not come up under
  callgrind for the TLS scenarios (not examined further).

## arm64 (linux/arm64 host 2, 8dd8e32c9)

| arch | bench | workloads | default: geomean | min | max | whole program: geomean | native build s (whole program) | LLVM build s |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| arm64 | atomic | 8 | 1.252 | 0.802 | 1.881 | 0.882 | 1.43 | 9.09 |
| arm64 | ctx | 5 | 0.626 | 0.436 | 0.817 | 0.607 | 2.33 | 8.32 |
| arm64 | digest | 5 | 0.750 | 0.681 | 0.894 | 0.747 | 3.44 | 11.85 |
| arm64 | json | 11 | 0.939 | 0.885 | 1.031 | 0.917 | 2.29 | 14.54 |
| arm64 | portable | 10 | 1.268 | 0.923 | 3.036 | 1.151 | 1.52 | 9.71 |
| arm64 | rsa | 4 | 0.948 | 0.934 | 0.957 | 0.948 | 6.35 | 38.38 |
| arm64 | text | 25 | 1.014 | 0.880 | 1.230 | 0.925 | 1.33 | 11.18 |
| arm64 | fault | 3 | 1.159 | 1.018 | 1.366 | 1.382 | 2.82 | 8.6 |

HTTP server, instructions a request:

| arch | scenario | native (default) | LLVM | default: native / LLVM | whole program: native / LLVM |
|---|---|---:|---:|---:|---:|
| arm64 | plain | 28374 | 29013 | 0.978 | 0.968 |
| arm64 | plain-newconn | 38341 | 39551 | 0.969 | 0.945 |
| arm64 | tls | 42538 | - | - | - |
| arm64 | tls-newconn | 1624711 | - | - | - |
| arm64 | h2 | 79965 | - | - | - |

Wall time, default builds, of rows the first version ranked (native /
LLVM):

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

| arch | bench | workloads | default: geomean | min | max | whole program: geomean | native build s (whole program) | LLVM build s |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| amd64 | atomic | 8 | 1.461 | 1.062 | 2.150 | 1.096 | 2.19 | 16.75 |
| amd64 | ctx | 5 | 0.765 | 0.536 | 0.900 | 0.742 | 2.61 | 13.83 |
| amd64 | digest | 5 | 0.927 | 0.731 | 1.043 | 0.926 | 2.23 | 14.88 |
| amd64 | json | 11 | 1.069 | 1.008 | 1.195 | 1.045 | 2.23 | 14.48 |
| amd64 | portable | 10 | 1.251 | 0.978 | 2.659 | 1.136 | 2.01 | 14.35 |
| amd64 | rsa | 4 | 1.175 | 1.141 | 1.221 | 1.183 | 7.14 | 51.69 |
| amd64 | text | 25 | 1.125 | 0.720 | 1.640 | 1.033 | 6.59 | 79.54 |
| amd64 | fault | 3 | 1.224 | 1.043 | 1.506 | 1.537 | 7.14 | 50.29 |

HTTP server, instructions a request:

| arch | scenario | native (default) | LLVM | default: native / LLVM | whole program: native / LLVM |
|---|---|---:|---:|---:|---:|
| amd64 | plain | 32778 | 30647 | 1.070 | 1.059 |
| amd64 | plain-newconn | 46217 | 45905 | 1.007 | 1.035 |
| amd64 | tls | 56072 | 75650 | 0.741 | 0.744 |
| amd64 | tls-newconn | 2928853 | 2937301 | 0.997 | 0.997 |
| amd64 | h2 | 106150 | 146512 | 0.725 | 0.739 |

atomic's callgrind ratios on x86-64 for set, cas and get were 2.16,
1.71 and 1.55 on 2026-10-22 too, where the Intel Mac's instructions
retired showed both backends equal on set and get: callgrind runs the
bench's threads one at a time. They are listed and not ranked.

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

## What trails, ranked from the default build (the compiler wide work list)

1. The default build against the whole program build of the same
   source: portable checked 3.036 arm64 and 2.659 x86-64 (1.038 and
   1.001 whole program), atomic max 1.881 and 1.230 (1.000 and 0.962),
   atomic add 1.687 and 1.390 (0.719 and 0.659), atomic contended 1.873
   and 1.605 (0.501 and 0.394), portable unix 1.590 and 1.345 (0.902
   and 0.921), text cut_prefix 1.192 and 1.611, cut_suffix 1.215 and
   1.640 (0.762 and 0.914 on x86-64). These rows call small std
   functions in their loops.
2. Faults: isolate 1.366 arm64 and 1.506 x86-64, owned 1.120 and 1.166
   (1.119 and 1.215 whole program). defer is 1.018 and 1.043 by default; it is
   1.725 and 1.854 in a whole program build, where the unwinder's uleb
   reader runs as a function of its own (24% of the run), and 1.373 on
   the wall on arm64 by default.
3. Narrow integer arithmetic, both backends (portable): arith32 1.390
   arm64 and 1.184 x86-64, arith8 1.358 x86-64, shift 1.109 and 1.243,
   divmod 1.126 and 1.142, str_hash 1.186 arm64 (1.377 wall, a wide
   spread).
4. RSA on x86-64: sign2048 1.221, sign4096 1.186, verify 1.141 to
   1.155 (arm64 is below LLVM, 0.934 to 0.957, and 0.939 and 0.946 on
   the wall).
5. Text on both: trim 1.230 arm64 and 1.364 x86-64 (1.237 arm64 wall),
   zone_offset 1.194 and 1.286, float_fmt 1.100 and 1.271.
6. Plain HTTP on x86-64: 1.070 a request (arm64 0.978).
7. JSON on x86-64: enc_floats 1.195, the bench 1.069 (arm64 0.939).

Every row at or over 1.05 by default, worst first:

| arch | bench | workload | native | LLVM | default | whole program |
|---|---|---|---:|---:|---:|---:|
| arm64 | portable | checked | 325110669 | 107083731 | 3.036 | 1.038 |
| arm64 | atomic | max | 64064793 | 34059311 | 1.881 | 1.000 |
| arm64 | atomic | contended | 90102814 | 48102918 | 1.873 | 0.501 |
| arm64 | atomic | add | 108057348 | 64053233 | 1.687 | 0.719 |
| arm64 | portable | unix | 97084312 | 61056418 | 1.590 | 0.902 |
| arm64 | portable | arith32 | 57093122 | 41080749 | 1.390 | 1.585 |
| arm64 | fault | isolate | 313193952 | 229262087 | 1.366 | 1.366 |
| arm64 | atomic | cas | 30057689 | 22053215 | 1.363 | 1.363 |
| arm64 | text | trim | 77986063 | 63399483 | 1.230 | 1.062 |
| arm64 | text | cut_suffix | 87036129 | 71638005 | 1.215 | 0.684 |
| arm64 | text | zone_offset | 71320346 | 59717219 | 1.194 | 1.067 |
| arm64 | text | cut_prefix | 87034924 | 73037621 | 1.192 | 0.638 |
| arm64 | portable | str_hash | 303838007 | 256122037 | 1.186 | 1.173 |
| arm64 | atomic | swap | 28057425 | 24053281 | 1.166 | 1.166 |
| arm64 | portable | divmod | 143103011 | 127076701 | 1.126 | 1.315 |
| arm64 | fault | owned | 575257119 | 513648008 | 1.120 | 1.119 |
| arm64 | portable | shift | 61099967 | 55077753 | 1.109 | 1.182 |
| arm64 | text | float_fmt | 103581174 | 94141062 | 1.100 | 1.071 |
| arm64 | text | cut | 168896579 | 153600713 | 1.100 | 0.770 |
| arm64 | text | dur_fmt | 114573340 | 106622274 | 1.075 | 1.067 |
| arm64 | portable | u64 | 35105672 | 33072312 | 1.061 | 1.183 |
| arm64 | portable | arith8 | 39095530 | 37067151 | 1.055 | 1.217 |
| arm64 | text | dur_parse | 99986061 | 95171428 | 1.051 | 0.992 |
| amd64 | portable | checked | 423415246 | 159222660 | 2.659 | 1.001 |
| amd64 | atomic | set | 10090578 | 4693153 | 2.150 | 2.576 |
| amd64 | atomic | cas | 26090274 | 14093156 | 1.851 | 1.993 |
| amd64 | text | cut_suffix | 132903541 | 81052239 | 1.640 | 0.914 |
| amd64 | text | cut_prefix | 131503500 | 81651466 | 1.611 | 0.762 |
| amd64 | atomic | contended | 90136255 | 56175813 | 1.605 | 0.394 |
| amd64 | atomic | get | 14090524 | 9092765 | 1.550 | 1.329 |
| amd64 | fault | isolate | 357171307 | 237107555 | 1.506 | 1.612 |
| amd64 | atomic | add | 114090213 | 82093722 | 1.390 | 0.659 |
| amd64 | text | trim | 99576762 | 73022901 | 1.364 | 1.220 |
| amd64 | portable | arith8 | 61427644 | 45223374 | 1.358 | 1.447 |
| amd64 | portable | unix | 133377776 | 99182468 | 1.345 | 0.921 |
| amd64 | text | zone_offset | 90416357 | 70316505 | 1.286 | 1.181 |
| amd64 | text | float_fmt | 132089545 | 103942833 | 1.271 | 1.244 |
| amd64 | portable | shift | 103430795 | 83226434 | 1.243 | 1.243 |
| amd64 | atomic | max | 64093881 | 52096434 | 1.230 | 0.962 |
| amd64 | rsa | sign2048 | 1051107911 | 860735171 | 1.221 | 1.219 |
| amd64 | json | enc_floats | 141451576 | 118328012 | 1.195 | 1.146 |
| amd64 | rsa | sign4096 | 6033255027 | 5087417838 | 1.186 | 1.195 |
| amd64 | portable | arith32 | 91428159 | 77214602 | 1.184 | 1.288 |
| amd64 | text | dur_fmt | 153863079 | 130799704 | 1.176 | 1.190 |
| amd64 | text | zone_civil | 122876040 | 104648541 | 1.174 | 1.087 |
| amd64 | fault | owned | 610914500 | 523834606 | 1.166 | 1.215 |
| amd64 | text | time_fmt | 170277695 | 147220898 | 1.157 | 1.167 |
| amd64 | rsa | verify2048 | 37026359 | 32045955 | 1.155 | 1.162 |
| amd64 | atomic | flag | 40090287 | 34759445 | 1.153 | 1.153 |
| amd64 | text | utf8 | 139633463 | 122265176 | 1.142 | 1.103 |
| amd64 | portable | divmod | 243421896 | 213219664 | 1.142 | 1.282 |
| amd64 | rsa | verify4096 | 124365827 | 108999717 | 1.141 | 1.157 |
| amd64 | text | cut | 221284631 | 194824909 | 1.136 | 0.936 |
| amd64 | text | dur_parse | 128762200 | 114590123 | 1.124 | 1.073 |
| amd64 | json | maps | 433463072 | 391522654 | 1.107 | 1.089 |
| amd64 | json | enc_small | 131444440 | 120143141 | 1.094 | 1.050 |
| amd64 | portable | str_hash | 363094764 | 332684910 | 1.091 | 1.128 |
| amd64 | text | parse_int | 29957992 | 27512121 | 1.089 | 1.094 |
| amd64 | json | strings | 278639029 | 256180242 | 1.088 | 1.084 |
| amd64 | text | quote | 123928575 | 113981083 | 1.087 | 1.096 |
| amd64 | text | time_parse | 49378282 | 45611310 | 1.083 | 1.072 |
| amd64 | json | small | 295808002 | 274569331 | 1.077 | 1.066 |
| amd64 | portable | conv | 161968776 | 151239215 | 1.071 | 1.111 |
| amd64 | text | hex | 107842286 | 101006913 | 1.068 | 0.980 |
| amd64 | text | base64 | 158299344 | 148272435 | 1.068 | 1.066 |
| amd64 | json | enc_strings | 236705354 | 222721362 | 1.063 | 1.009 |
| amd64 | atomic | swap | 34090751 | 32093254 | 1.062 | 1.062 |
| amd64 | text | debug | 260042815 | 245104973 | 1.061 | 1.038 |
| amd64 | text | zone_fmt | 137700503 | 129818651 | 1.061 | 1.057 |
| amd64 | json | ints | 137793911 | 130349675 | 1.057 | 1.019 |

## Every workload

| arch | bench | workload | native | LLVM | default | whole program | checksums |
|---|---|---|---:|---:|---:|---:|---|
| arm64 | atomic | none | 57302 | 52323 | 1.095 | 1.070 | agree |
| arm64 | atomic | get | 12057302 | 14052317 | 0.858 | 0.858 | agree |
| arm64 | atomic | set | 8057675 | 10052325 | 0.802 | 0.801 | agree |
| arm64 | atomic | swap | 28057425 | 24053281 | 1.166 | 1.166 | agree |
| arm64 | atomic | cas | 30057689 | 22053215 | 1.363 | 1.363 | agree |
| arm64 | atomic | add | 108057348 | 64053233 | 1.687 | 0.719 | agree |
| arm64 | atomic | max | 64064793 | 34059311 | 1.881 | 1.000 | agree |
| arm64 | atomic | flag | 26058177 | 28052704 | 0.929 | 0.929 | agree |
| arm64 | atomic | contended | 90102814 | 48102918 | 1.873 | 0.501 | agree |
| arm64 | ctx | none | 62692 | 60742 | 1.032 | 2.112 | agree |
| arm64 | ctx | defer | 316056737 | 582051647 | 0.543 | 0.467 | agree |
| arm64 | ctx | mutex | 832058118 | 1346056011 | 0.618 | 0.618 | agree |
| arm64 | ctx | spawn | 31679167 | 38760386 | 0.817 | 0.806 | agree |
| arm64 | ctx | budget | 603790024 | 1383571367 | 0.436 | 0.436 | agree |
| arm64 | ctx | await | 39892343 | 49847930 | 0.800 | 0.814 | agree |
| arm64 | digest | none | 1182630 | 4053615 | 0.292 | 0.305 | agree |
| arm64 | digest | sha256_big | 25198974 | 28182857 | 0.894 | 0.896 | agree |
| arm64 | digest | sha256_small | 134977180 | 181759015 | 0.743 | 0.733 | agree |
| arm64 | digest | hmac | 159230314 | 233706502 | 0.681 | 0.676 | agree |
| arm64 | digest | pbkdf2 | 353250733 | 478339949 | 0.738 | 0.739 | agree |
| arm64 | digest | argon2 | 168337587 | 237412369 | 0.709 | 0.709 | DIFFER |
| arm64 | json | none | 21332752 | 22837975 | 0.934 | 0.934 | agree |
| arm64 | json | small | 234985277 | 249292991 | 0.943 | 0.933 | agree |
| arm64 | json | ints | 114509722 | 120016561 | 0.954 | 0.902 | agree |
| arm64 | json | floats | 168316297 | 173313202 | 0.971 | 0.948 | agree |
| arm64 | json | strings | 215489810 | 233999002 | 0.921 | 0.917 | agree |
| arm64 | json | maps | 350276938 | 355995437 | 0.984 | 0.956 | agree |
| arm64 | json | parse | 206261791 | 224700828 | 0.918 | 0.918 | agree |
| arm64 | json | invalid | 167869923 | 189682754 | 0.885 | 0.881 | agree |
| arm64 | json | enc_small | 102537431 | 114120127 | 0.899 | 0.876 | agree |
| arm64 | json | enc_strings | 186361888 | 208363155 | 0.894 | 0.856 | agree |
| arm64 | json | enc_floats | 108968691 | 105688815 | 1.031 | 0.995 | agree |
| arm64 | json | bytes | 204858593 | 218917924 | 0.936 | 0.916 | agree |
| arm64 | portable | none | 1067990 | 1050603 | 1.017 | 1.029 | agree |
| arm64 | portable | arith32 | 57093122 | 41080749 | 1.390 | 1.585 | agree |
| arm64 | portable | arith8 | 39095530 | 37067151 | 1.055 | 1.217 | agree |
| arm64 | portable | u64 | 35105672 | 33072312 | 1.061 | 1.183 | agree |
| arm64 | portable | divmod | 143103011 | 127076701 | 1.126 | 1.315 | agree |
| arm64 | portable | shift | 61099967 | 55077753 | 1.109 | 1.182 | agree |
| arm64 | portable | conv | 99659306 | 95073652 | 1.048 | 1.132 | agree |
| arm64 | portable | str_hash | 303838007 | 256122037 | 1.186 | 1.173 | agree |
| arm64 | portable | unix | 97084312 | 61056418 | 1.590 | 0.902 | agree |
| arm64 | portable | checked | 325110669 | 107083731 | 3.036 | 1.038 | agree |
| arm64 | portable | float32 | 47169412 | 51080602 | 0.923 | 0.924 | agree |
| arm64 | rsa | sign2048 | 761314979 | 806166003 | 0.944 | 0.944 | agree |
| arm64 | rsa | verify2048 | 27518862 | 29478054 | 0.934 | 0.933 | agree |
| arm64 | rsa | sign4096 | 4478543534 | 4677875380 | 0.957 | 0.957 | agree |
| arm64 | rsa | verify4096 | 94372812 | 98595205 | 0.957 | 0.957 | agree |
| arm64 | text | none | 6197039 | 6414776 | 0.966 | 0.964 | agree |
| arm64 | text | parse_int | 22585234 | 22602463 | 0.999 | 0.981 | agree |
| arm64 | text | dur_fmt | 114573340 | 106622274 | 1.075 | 1.067 | agree |
| arm64 | text | dur_parse | 99986061 | 95171428 | 1.051 | 0.992 | agree |
| arm64 | text | time_fmt | 126542278 | 122817175 | 1.030 | 1.024 | agree |
| arm64 | text | time_parse | 39496139 | 40367071 | 0.978 | 0.960 | agree |
| arm64 | text | quote | 92124122 | 104673612 | 0.880 | 0.876 | agree |
| arm64 | text | split | 45873404 | 47996846 | 0.956 | 0.952 | agree |
| arm64 | text | lines | 55852720 | 63109243 | 0.885 | 0.885 | agree |
| arm64 | text | replace | 55981305 | 59532523 | 0.940 | 0.884 | agree |
| arm64 | text | trim | 77986063 | 63399483 | 1.230 | 1.062 | agree |
| arm64 | text | rune_count | 21820208 | 23711555 | 0.920 | 0.878 | agree |
| arm64 | text | cut | 168896579 | 153600713 | 1.100 | 0.770 | agree |
| arm64 | text | cut_prefix | 87034924 | 73037621 | 1.192 | 0.638 | agree |
| arm64 | text | cut_suffix | 87036129 | 71638005 | 1.215 | 0.684 | agree |
| arm64 | text | utf8 | 105889568 | 101923883 | 1.039 | 1.000 | agree |
| arm64 | text | int_fmt | 53901388 | 59338059 | 0.908 | 0.895 | agree |
| arm64 | text | float_fmt | 103581174 | 94141062 | 1.100 | 1.071 | agree |
| arm64 | text | float_parse | 128241715 | 125406015 | 1.023 | 0.998 | agree |
| arm64 | text | base64 | 130773070 | 128453164 | 1.018 | 1.005 | agree |
| arm64 | text | hex | 76009761 | 84097318 | 0.904 | 0.852 | agree |
| arm64 | text | debug | 202973161 | 229447311 | 0.885 | 0.878 | agree |
| arm64 | text | zone_offset | 71320346 | 59717219 | 1.194 | 1.067 | agree |
| arm64 | text | zone_fmt | 121838525 | 131134563 | 0.929 | 0.913 | agree |
| arm64 | text | zone_civil | 99843760 | 96469115 | 1.035 | 0.971 | agree |
| arm64 | text | zone_load | 32203494 | 31997947 | 1.006 | 0.999 | agree |
| arm64 | fault | none | 56782 | 52039 | 1.091 | 1.092 | agree |
| arm64 | fault | isolate | 313193952 | 229262087 | 1.366 | 1.366 | agree |
| arm64 | fault | defer | 940418005 | 923907484 | 1.018 | 1.725 | agree |
| arm64 | fault | owned | 575257119 | 513648008 | 1.120 | 1.119 | agree |
| amd64 | atomic | none | 90531 | 92769 | 0.976 | 0.957 | agree |
| amd64 | atomic | get | 14090524 | 9092765 | 1.550 | 1.329 | agree |
| amd64 | atomic | set | 10090578 | 4693153 | 2.150 | 2.576 | agree |
| amd64 | atomic | swap | 34090751 | 32093254 | 1.062 | 1.062 | agree |
| amd64 | atomic | cas | 26090274 | 14093156 | 1.851 | 1.993 | agree |
| amd64 | atomic | add | 114090213 | 82093722 | 1.390 | 0.659 | agree |
| amd64 | atomic | max | 64093881 | 52096434 | 1.230 | 0.962 | agree |
| amd64 | atomic | flag | 40090287 | 34759445 | 1.153 | 1.153 | agree |
| amd64 | atomic | contended | 90136255 | 56175813 | 1.605 | 0.394 | agree |
| amd64 | ctx | none | 96966 | 100870 | 0.961 | 1.096 | agree |
| amd64 | ctx | defer | 568089870 | 686092673 | 0.828 | 0.735 | agree |
| amd64 | ctx | mutex | 844090658 | 1148092884 | 0.735 | 0.746 | agree |
| amd64 | ctx | spawn | 36958134 | 41070319 | 0.900 | 0.888 | agree |
| amd64 | ctx | budget | 624350460 | 1163787344 | 0.536 | 0.519 | agree |
| amd64 | ctx | await | 45539323 | 51184735 | 0.890 | 0.889 | agree |
| amd64 | digest | none | 1431486 | 4107484 | 0.349 | 0.347 | agree |
| amd64 | digest | sha256_big | 737029161 | 735921551 | 1.002 | 1.001 | agree |
| amd64 | digest | sha256_small | 671234855 | 715842788 | 0.938 | 0.937 | agree |
| amd64 | digest | hmac | 935083714 | 977324867 | 0.957 | 0.956 | agree |
| amd64 | digest | pbkdf2 | 1597391321 | 1531873518 | 1.043 | 1.038 | agree |
| amd64 | digest | argon2 | 216747883 | 296675734 | 0.731 | 0.731 | agree |
| amd64 | json | none | 26636419 | 24364299 | 1.093 | 1.092 | agree |
| amd64 | json | small | 295808002 | 274569331 | 1.077 | 1.066 | agree |
| amd64 | json | ints | 137793911 | 130349675 | 1.057 | 1.019 | agree |
| amd64 | json | floats | 206114961 | 201267062 | 1.024 | 0.998 | agree |
| amd64 | json | strings | 278639029 | 256180242 | 1.088 | 1.084 | agree |
| amd64 | json | maps | 433463072 | 391522654 | 1.107 | 1.089 | agree |
| amd64 | json | parse | 253198489 | 248019347 | 1.021 | 1.021 | agree |
| amd64 | json | invalid | 200296196 | 198621458 | 1.008 | 1.004 | agree |
| amd64 | json | enc_small | 131444440 | 120143141 | 1.094 | 1.050 | agree |
| amd64 | json | enc_strings | 236705354 | 222721362 | 1.063 | 1.009 | agree |
| amd64 | json | enc_floats | 141451576 | 118328012 | 1.195 | 1.146 | agree |
| amd64 | json | bytes | 263525063 | 254131200 | 1.037 | 1.018 | agree |
| amd64 | portable | none | 1364745 | 1170422 | 1.166 | 1.148 | agree |
| amd64 | portable | arith32 | 91428159 | 77214602 | 1.184 | 1.288 | agree |
| amd64 | portable | arith8 | 61427644 | 45223374 | 1.358 | 1.447 | agree |
| amd64 | portable | u64 | 53434467 | 53220794 | 1.004 | 1.041 | agree |
| amd64 | portable | divmod | 243421896 | 213219664 | 1.142 | 1.282 | agree |
| amd64 | portable | shift | 103430795 | 83226434 | 1.243 | 1.243 | agree |
| amd64 | portable | conv | 161968776 | 151239215 | 1.071 | 1.111 | agree |
| amd64 | portable | str_hash | 363094764 | 332684910 | 1.091 | 1.128 | agree |
| amd64 | portable | unix | 133377776 | 99182468 | 1.345 | 0.921 | agree |
| amd64 | portable | checked | 423415246 | 159222660 | 2.659 | 1.001 | agree |
| amd64 | portable | float32 | 81466166 | 83276226 | 0.978 | 1.003 | agree |
| amd64 | rsa | sign2048 | 1051107911 | 860735171 | 1.221 | 1.219 | agree |
| amd64 | rsa | verify2048 | 37026359 | 32045955 | 1.155 | 1.162 | agree |
| amd64 | rsa | sign4096 | 6033255027 | 5087417838 | 1.186 | 1.195 | agree |
| amd64 | rsa | verify4096 | 124365827 | 108999717 | 1.141 | 1.157 | agree |
| amd64 | text | none | 7849711 | 7001790 | 1.121 | 1.114 | agree |
| amd64 | text | parse_int | 29957992 | 27512121 | 1.089 | 1.094 | agree |
| amd64 | text | dur_fmt | 153863079 | 130799704 | 1.176 | 1.190 | agree |
| amd64 | text | dur_parse | 128762200 | 114590123 | 1.124 | 1.073 | agree |
| amd64 | text | time_fmt | 170277695 | 147220898 | 1.157 | 1.167 | agree |
| amd64 | text | time_parse | 49378282 | 45611310 | 1.083 | 1.072 | agree |
| amd64 | text | quote | 123928575 | 113981083 | 1.087 | 1.096 | agree |
| amd64 | text | split | 58584723 | 55879996 | 1.048 | 1.040 | agree |
| amd64 | text | lines | 74313816 | 75623130 | 0.983 | 0.987 | agree |
| amd64 | text | replace | 73475582 | 70105117 | 1.048 | 1.007 | agree |
| amd64 | text | trim | 99576762 | 73022901 | 1.364 | 1.220 | agree |
| amd64 | text | rune_count | 28202424 | 39194659 | 0.720 | 0.659 | agree |
| amd64 | text | cut | 221284631 | 194824909 | 1.136 | 0.936 | agree |
| amd64 | text | cut_prefix | 131503500 | 81651466 | 1.611 | 0.762 | agree |
| amd64 | text | cut_suffix | 132903541 | 81052239 | 1.640 | 0.914 | agree |
| amd64 | text | utf8 | 139633463 | 122265176 | 1.142 | 1.103 | agree |
| amd64 | text | int_fmt | 66024410 | 63536184 | 1.039 | 1.067 | agree |
| amd64 | text | float_fmt | 132089545 | 103942833 | 1.271 | 1.244 | agree |
| amd64 | text | float_parse | 161864482 | 157465238 | 1.028 | 1.004 | agree |
| amd64 | text | base64 | 158299344 | 148272435 | 1.068 | 1.066 | agree |
| amd64 | text | hex | 107842286 | 101006913 | 1.068 | 0.980 | agree |
| amd64 | text | debug | 260042815 | 245104973 | 1.061 | 1.038 | agree |
| amd64 | text | zone_offset | 90416357 | 70316505 | 1.286 | 1.181 | agree |
| amd64 | text | zone_fmt | 137700503 | 129818651 | 1.061 | 1.057 | agree |
| amd64 | text | zone_civil | 122876040 | 104648541 | 1.174 | 1.087 | agree |
| amd64 | text | zone_load | 38044774 | 37765475 | 1.007 | 1.000 | agree |
| amd64 | fault | none | 90108 | 92140 | 0.978 | 0.946 | agree |
| amd64 | fault | isolate | 357171307 | 237107555 | 1.506 | 1.612 | agree |
| amd64 | fault | defer | 1021315847 | 978878033 | 1.043 | 1.854 | agree |
| amd64 | fault | owned | 610914500 | 523834606 | 1.166 | 1.215 | agree |
