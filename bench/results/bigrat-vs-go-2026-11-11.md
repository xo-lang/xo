# big.BigRat against Go's math/big Rat (2026-11-11)

Decision 0608's timing row (the owner's rule: every std port carries one).
bench/perf/bigrat and its Go twin (twin.go.txt): add (a harmonic sum,
restarted when its denominator passes 512 bits), mul and quo of small
fractions, cmp, to_float, float_text (30 digits) and parse (a 50 digit
decimal); `none` is the setup alone. twincheck agrees on all 8 workloads.

`xo bench --ab --rounds=10`, A the Go twin (go1.26.1), B the Xo program
(native, --release, cross-built on the Mac for linux/arm64 from
2651b0781 plus this change), on the linux/arm64 offload host (host 1,
exclusive), with the 0.48.0 seed's xo; Xo / Go:

| workload | Go wall | Xo wall | wall (95%) | max RSS |
|---|---|---|---|---|
| none | 0.649 ms | 0.303 ms | 0.468 | 0.192 |
| add | 159.4 ms | 778.2 ms | 4.882 (4.825 to 5.02) | 0.108 |
| mul | 269.4 ms | 1750 ms | 6.496 (6.366 to 6.523) | 0.105 |
| quo | 69.29 ms | 146.0 ms | 2.108 (2.081 to 2.174) | 0.104 |
| cmp | 22.58 ms | 34.56 ms | 1.531 (1.478 to 1.578) | 0.109 |
| tofloat | 35.09 ms | 34.58 ms | 0.985 (0.954 to 0.998) | 0.108 |
| text | 18.71 ms | 29.92 ms | 1.599 (1.55 to 1.642) | 0.108 |
| parse | 51.12 ms | 268.9 ms | 5.259 (5.123 to 5.36) | 0.111 |

Instructions: not reported on Linux.

Where the time goes (XO_PROF=cpu on the Mac): every BigRat result is
normalized by a gcd, and std/big's gcd was plain Euclid, a multiprecision
division and a new list per step (nat_divmod_basic, 85 to 89% of add and
mul). Parse multiplied in one digit at a time. The BigFloat speed change
(branch lang3-bfspeed, after this one) ports Go's lehmerGCD and gathers
a word of digits per multiply-add; on the Mac it cuts add from 337 to 62
samples and mul from 787 to 111. Its own row replaces this table's
add, mul and parse when it lands.

## With Lehmer's gcd (2026-11-11, std/changes/big.md "Lehmer's gcd and fewer allocations")

The same bench, the Xo program cross-built with the BigFloat speed
change (std/big's gcd is Go's lehmerGCD with a fused update; parse
gathers a word of digits), host 1 exclusive, `xo bench --ab --rounds=10`;
Xo / Go:

| workload | Go wall | Xo wall | wall (95%) | before | max RSS |
|---|---|---|---|---|---|
| none | 0.941 ms | 0.471 ms | 0.501 | 0.468 | 0.189 |
| add | 147.6 ms | 129.8 ms | 0.879 (0.866 to 0.908) | 4.882 | 0.106 |
| mul | 247.2 ms | 218.4 ms | 0.884 (0.878 to 0.896) | 6.496 | 0.111 |
| quo | 63.63 ms | 58.2 ms | 0.915 (0.884 to 0.923) | 2.108 | 0.11 |
| cmp | 20.92 ms | 25.81 ms | 1.234 (1.202 to 1.288) | 1.531 | 0.105 |
| tofloat | 32.75 ms | 26.45 ms | 0.808 (0.794 to 0.83) | 0.985 | 0.104 |
| text | 17.33 ms | 30.55 ms | 1.763 (1.706 to 1.793) | 1.599 | 0.108 |
| parse | 47.97 ms | 35.41 ms | 0.738 (0.734 to 0.748) | 5.259 | 0.106 |

Left: text (float_text's decimal conversion of a quotient) and cmp
(two cross products through BigInt keys); next, profiles of both.
