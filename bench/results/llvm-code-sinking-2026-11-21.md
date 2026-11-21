# LLVM InstCombine code sinking: what it buys at run time, 2026-11-21 (task #590)

Xo Lang 1, for the owner's question on task #586: the cost of compiling
every optimized LLVM module with `-instcombine-code-sinking=false`.
A is xo at aaa895abc (sinking on for these programs), B the same with
the option forced for every module (scratch commit 619f3f228, not on
main). linux/arm64 host 1 (12 CPUs), clang version 23.1.3 (https://github.com/llvm/llvm-project 0d261d1ca552c95a8f007e061c787ac7132fbcbc).
Each program built by each side with `--backend=llvm`,
`XO_INLINE_STRICT=1`, a backend cache of its own (a whole program
build). The host ran nothing else during the timing.

Result: off / on wall time 1.002 (71 rows) as the geometric mean; the
LLVM-built compiler building compiler/xo is within noise; `.text` is
the same size or slightly smaller with sinking off.

## Wall time per workload

30 interleaved rounds, each side first in turn, one process per run;
the ratio is the median of the 30 per round ratios (decision 0726's
method). `xo bench --ab` (4 rounds) marked 70 of the 71 rows "too
short" (under 100 ms), so it gives no verdict on them.

| bench | workload | on ms | off ms | off / on (median of 30 ratios) | min | max |
|---|---|---:|---:|---:|---:|---:|
| atomic | get | 1.5 | 1.5 | 0.977 | 0.909 | 1.126 |
| atomic | set | 1.4 | 1.4 | 1.007 | 0.869 | 1.135 |
| atomic | swap | 6.3 | 6.4 | 1.008 | 0.966 | 1.064 |
| atomic | cas | 2.5 | 2.5 | 0.995 | 0.933 | 1.065 |
| atomic | add | 6.4 | 6.4 | 0.997 | 0.960 | 1.027 |
| atomic | max | 6.3 | 6.3 | 1.007 | 0.966 | 1.047 |
| atomic | flag | 1.9 | 1.9 | 1.002 | 0.949 | 1.067 |
| atomic | contended | 29.9 | 31.5 | 1.027 | 0.677 | 1.328 |
| ctx | defer | 29.6 | 29.7 | 1.001 | 0.984 | 1.020 |
| ctx | mutex | 59.5 | 59.4 | 1.000 | 0.962 | 1.027 |
| ctx | spawn | 3.7 | 3.7 | 0.995 | 0.886 | 1.126 |
| ctx | budget | 62.2 | 62.3 | 1.002 | 0.988 | 1.021 |
| ctx | await | 4.9 | 4.8 | 0.972 | 0.888 | 1.215 |
| digest | sha256_big | 5.3 | 5.3 | 0.995 | 0.899 | 1.100 |
| digest | sha256_small | 17.7 | 17.7 | 0.997 | 0.837 | 1.039 |
| digest | hmac | 21.0 | 21.0 | 1.003 | 0.973 | 1.243 |
| digest | pbkdf2 | 34.3 | 34.1 | 0.995 | 0.967 | 1.025 |
| digest | argon2 | 10.3 | 10.3 | 1.002 | 0.848 | 1.350 |
| json | small | 13.5 | 13.4 | 0.997 | 0.925 | 1.048 |
| json | ints | 8.6 | 8.7 | 1.007 | 0.876 | 1.126 |
| json | floats | 10.9 | 10.9 | 1.001 | 0.896 | 1.074 |
| json | strings | 13.3 | 13.2 | 0.998 | 0.872 | 1.157 |
| json | maps | 18.9 | 18.9 | 1.000 | 0.862 | 1.129 |
| json | parse | 12.8 | 12.8 | 0.996 | 0.965 | 1.200 |
| json | invalid | 11.3 | 11.4 | 0.992 | 0.935 | 1.158 |
| json | enc_small | 7.3 | 7.4 | 1.006 | 0.792 | 1.079 |
| json | enc_strings | 11.4 | 11.5 | 1.012 | 0.836 | 1.307 |
| json | enc_floats | 7.9 | 8.0 | 1.012 | 0.968 | 1.265 |
| json | bytes | 11.7 | 11.7 | 1.000 | 0.957 | 1.326 |
| portable | arith32 | 8.1 | 8.1 | 1.002 | 0.975 | 1.047 |
| portable | arith8 | 7.5 | 8.0 | 1.059 | 1.010 | 1.105 |
| portable | u64 | 9.3 | 9.2 | 0.999 | 0.941 | 1.048 |
| portable | divmod | 8.0 | 8.0 | 0.999 | 0.978 | 1.024 |
| portable | shift | 3.1 | 3.0 | 0.981 | 0.938 | 1.133 |
| portable | conv | 8.6 | 8.6 | 1.001 | 0.974 | 1.025 |
| portable | str_hash | 11.7 | 11.5 | 0.989 | 0.954 | 1.027 |
| portable | unix | 7.0 | 7.1 | 1.007 | 0.970 | 1.060 |
| portable | checked | 19.0 | 19.0 | 0.997 | 0.971 | 1.029 |
| portable | float32 | 11.5 | 11.5 | 0.993 | 0.928 | 1.057 |
| rsa | sign2048 | 41.1 | 41.2 | 1.000 | 0.946 | 1.024 |
| rsa | verify2048 | 2.1 | 2.1 | 0.996 | 0.917 | 1.115 |
| rsa | sign4096 | 159.2 | 158.9 | 0.997 | 0.926 | 1.060 |
| rsa | verify4096 | 4.6 | 4.7 | 1.017 | 0.909 | 1.114 |
| text | parse_int | 2.4 | 2.4 | 1.001 | 0.902 | 1.342 |
| text | dur_fmt | 5.4 | 5.4 | 1.007 | 0.952 | 1.066 |
| text | dur_parse | 5.5 | 5.5 | 0.998 | 0.943 | 1.189 |
| text | time_fmt | 9.2 | 9.5 | 1.035 | 0.928 | 1.058 |
| text | time_parse | 3.0 | 3.0 | 1.005 | 0.929 | 1.346 |
| text | quote | 5.4 | 5.3 | 0.989 | 0.856 | 1.069 |
| text | split | 3.5 | 3.6 | 0.996 | 0.806 | 1.067 |
| text | lines | 3.8 | 3.7 | 0.993 | 0.722 | 1.029 |
| text | replace | 3.4 | 3.3 | 0.978 | 0.899 | 1.037 |
| text | trim | 3.7 | 3.7 | 1.005 | 0.945 | 1.239 |
| text | rune_count | 2.1 | 2.0 | 0.988 | 0.795 | 1.500 |
| text | cut | 6.7 | 6.7 | 1.004 | 0.934 | 1.065 |
| text | cut_prefix | 4.8 | 4.9 | 1.012 | 0.825 | 1.100 |
| text | cut_suffix | 4.8 | 4.7 | 0.992 | 0.831 | 1.054 |
| text | utf8 | 6.3 | 6.3 | 1.005 | 0.951 | 1.079 |
| text | int_fmt | 4.0 | 4.0 | 1.010 | 0.922 | 1.403 |
| text | float_fmt | 6.2 | 6.2 | 1.012 | 0.961 | 1.194 |
| text | float_parse | 7.4 | 7.3 | 0.988 | 0.914 | 1.055 |
| text | base64 | 5.5 | 5.5 | 0.997 | 0.936 | 1.156 |
| text | hex | 4.8 | 5.0 | 1.032 | 0.988 | 1.131 |
| text | debug | 11.3 | 11.7 | 1.023 | 0.966 | 1.104 |
| text | zone_offset | 4.8 | 4.8 | 1.003 | 0.897 | 1.193 |
| text | zone_fmt | 6.6 | 6.6 | 0.999 | 0.900 | 1.149 |
| text | zone_civil | 6.8 | 6.8 | 0.998 | 0.872 | 1.042 |
| text | zone_load | 3.4 | 3.4 | 1.009 | 0.911 | 1.419 |
| fault | isolate | 13.1 | 13.1 | 1.003 | 0.906 | 1.045 |
| fault | defer | 39.7 | 40.6 | 1.022 | 0.997 | 1.038 |
| fault | owned | 26.5 | 27.0 | 1.019 | 0.985 | 1.091 |

- geomean atomic 1.002 (8 rows)
- geomean ctx 0.994 (5 rows)
- geomean digest 0.998 (5 rows)
- geomean json 1.002 (11 rows)
- geomean portable 1.002 (10 rows)
- geomean rsa 1.003 (4 rows)
- geomean text 1.003 (25 rows)
- geomean fault 1.015 (3 rows)
- geomean all 1.002 (71 rows)

## The compiler building compiler/xo

`xo bench --ab --keep-loaded --rounds=4 --warmup=1`, A and B the xo
each side built with `--backend=llvm`, both building A's compiler/xo
with the native backend, caches off:

```
4 of 4 rounds used (0 loaded), loads A/B per round: 4.93/6.21 5.66/5.4 5.97/6.36 6.65/6.95
metric                A median    B median    spread A/B        B / A   95% interval      same slot B/A   verdict
wall_ms               37033       37377       0.771% 0.801%     1.009   0.982 to 1.021    1.015 0.998     within noise (n=4)
cpu_ms                228996      228681      0.631% 0.659%     0.999   0.995 to 1.01     1.005 0.998     within noise (n=4)
max_rss_kb            4093582     4087180     1.902% 2.621%     0.998   0.958 to 1.031    1.001 0.984     within noise (n=4)
```

## Code size and clang time of the builds

| program | .text on | .text off | file on | file off | clang ms on | clang ms off | same binary |
|---|---:|---:|---:|---:|---:|---:|---|
| atomic | 415232 | 415024 | 1353480 | 1349632 | 3937.51 | 3919.72 | no |
| ctx | 435408 | 435196 | 1428480 | 1424440 | 4045.02 | 4081.93 | no |
| digest | 497388 | 497388 | 1581152 | 1581152 | 5071.08 | 5082.87 | yes |
| json | 436160 | 435872 | 1610144 | 1603664 | 4927.85 | 4925.12 | no |
| portable | 397616 | 397368 | 1389736 | 1384984 | 4159.53 | 4184.73 | no |
| rsa | 3093708 | 3093524 | 6148576 | 6133064 | 29668.11 | 29049.50 | no |
| text | 459052 | 458812 | 2215240 | 2209000 | 5428.72 | 5426.07 | no |
| fault | 406884 | 406668 | 1324408 | 1320608 | 3761.23 | 3896.09 | no |
| xo | 11157612 | 11156748 | 161421616 | 161368376 | 750075.93 | 796041.22 | no |

The clang times are one build each on a host running other jobs: no
claim. digest's two binaries are byte identical.
