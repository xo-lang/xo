# big.BigFloat against Go's math/big Float (2026-11-11)

Decision 0602's timing row (the owner's rule: every std port carries one).
bench/perf/bigfloat and its Go twin (twin.go.txt), 256 bits by
ToNearestEven: add, mul and quo chains of two square roots, sqrt of the
integers from 2, text ('g', 60 digits) and parse (a 70 digit decimal);
`none` is the setup alone. twincheck agrees on all 7 workloads.

`xo bench --ab --rounds=10`, A the Go twin (go1.26.1), B the Xo program
(native, --release, cross-built on the Mac for linux/arm64 from
d2f2d2f08 plus this change), on the linux/arm64 offload host (host 1,
exclusive), with the 0.48.0 seed's xo; Xo / Go:

| workload | Go wall | Xo wall | wall (95%) | max RSS |
|---|---|---|---|---|
| none | 0.567 ms | 0.29 ms | 0.512 | 0.205 |
| add (200,000) | 11.44 ms | 27.48 ms | 2.403 (2.333 to 2.471) | 0.121 |
| mul (200,000) | 14.36 ms | 25.26 ms | 1.758 (1.701 to 1.835) | 0.118 |
| quo (100,000) | 22.64 ms | 24.0 ms | 1.060 (1.037 to 1.084) | 0.118 |
| sqrt (20,000) | 10.72 ms | 27.24 ms | 2.541 (2.48 to 2.566) | 0.115 |
| text (20,000) | 36.79 ms | 58.52 ms | 1.591 (1.559 to 1.629) | 0.114 |
| parse (20,000) | 20.65 ms | 29.54 ms | 1.431 (1.408 to 1.486) | 0.121 |

Instructions: not reported on Linux.

Where the time goes (expected, not yet profiled): every operation decodes
its operands' BigInt keys into limbs and encodes the result (std/big's
order-preserving key, decision 0311), and add shifts both mantissas into
new lists before adding, where Go adds in place into z's words. Sqrt is
three Newton steps of four multiplications and a subtraction each, so it
pays that per-operation cost about 15 times. Next: an XO_PROF=cpu profile
of add and sqrt, then the limbs held in the value (no key round trip) or
an add that aligns by whole words.

## After the speed change (2026-11-11, std/changes/big.md "Lehmer's gcd and fewer allocations")

The same bench, A the Go twin, B the Xo program cross-built from this
change on origin/main 2651b0781, host 1 exclusive, `xo bench --ab
--rounds=10` with the 0.48.0 seed's xo; Xo / Go:

| workload | Go wall | Xo wall | wall (95%) | before | max RSS |
|---|---|---|---|---|---|
| none | 1.025 ms | 0.482 ms | 0.47 | 0.512 | 0.327 |
| add (200,000) | 11.43 ms | 22.65 ms | 1.981 (1.928 to 2.053) | 2.403 | 0.116 |
| mul (200,000) | 14.16 ms | 23.48 ms | 1.657 (1.599 to 1.711) | 1.758 | 0.117 |
| quo (100,000) | 21.74 ms | 22.98 ms | 1.057 (1.034 to 1.09) | 1.060 | 0.117 |
| sqrt (20,000) | 10.22 ms | 17.73 ms | 1.735 (1.723 to 1.747) | 2.541 | 0.121 |
| text (20,000) | 35.94 ms | 35.09 ms | 0.976 (0.97 to 0.981) | 1.591 | 0.113 |
| parse (20,000) | 19.58 ms | 15.38 ms | 0.786 (0.768 to 0.795) | 1.431 | 0.115 |

Against main (ab-host.sh, 2651b0781 against the first commits of the
change, ~/xo-lead/notes/lang3 bfr-ab and bft2-ab): add 0.831, mul 0.924,
sqrt 0.919, parse 0.531, text 0.688.

What changed (XO_PROF=cpu profiles on the Mac): raw.zero_ints stores up
to 8 zeros without a memset call (18 to 24% of add and sqrt); rounding
shifts once; add reads the aligned operand's words shifted instead of
making a shifted copy; sqrt's Newton steps run on an unpacked mantissa
(one key decode and encode per call instead of about 15); text's decimal
shift is in place and unchecked (it cannot overflow); parse gathers a
word of digits per multiply-add. What is left in add, mul and sqrt is
the key round trip of each public call (decision 0311's BigInt key) and
list allocation: the next experiment is the unpacked form for every
public operation's operands, measured the same way.

## Second speed change (2026-11-11, std/changes/big.md "BigFloat add, sub, mul and sqrt read mantissa keys in place")

The same bench and host (host 1 exclusive, `xo bench --ab --rounds=10`,
the 0.48.0 seed's xo), B cross-built from branch lang3-bfspeed2 at
bd7af0a28 (before the mul change); Xo / Go:

| workload | Go wall | Xo wall | wall (95%) | before | max RSS |
|---|---|---|---|---|---|
| none | 0.645 ms | 0.299 ms | 0.463 (0.408 to 0.54) | 0.47 | |
| add (200,000) | 11.28 ms | 11.72 ms | 1.039 (1.007 to 1.073) | 1.981 | |
| mul (200,000) | 14.23 ms | 16.89 ms | 1.187 (1.156 to 1.221) | 1.657 | |
| quo (100,000) | 22.4 ms | 19.2 ms | 0.857 (0.838 to 0.87) | 1.057 | |
| sqrt (20,000) | 10.41 ms | 13.44 ms | 1.291 (1.268 to 1.314) | 1.735 | |
| text (20,000) | 36.29 ms | 35.71 ms | 0.984 (0.973 to 0.99) | 0.976 | |
| parse (20,000) | 19.32 ms | 14.71 ms | 0.761 (0.75 to 0.769) | 0.786 | |

Against the first change's binary (A the first change, B this one, same
run): add 0.523 (0.506 to 0.538), sqrt 0.761 (0.758 to 0.764). The mul
change after it (x's limbs read from its key) ran mul 0.69 to 0.62 s on
the Mac's long version of the bench (10,000,000 steps); its host row is
the next run of this bench.

What changed (XO_PROF=cpu on a 50x longer copy of the bench, since the
bench's own runs take about 20 ms): add and sub read both mantissas'
limbs from their BigInt keys while aligning, so neither is decoded into
a list; a round up makes (m >> s) + 1 with its trailing zeros dropped
in one new list; rounding down and dropping trailing zeros shift in
place; min_prec reads the top limb from the key; sqrt's last multiply
and its SetMantExp run on the unpacked form with one pack. What is
left: the key encode of every result (make) and the allocation of each
product, about a fifth of sqrt; the next experiment is a key-direct
encode in u_pack (writing the key from the limbs without trim's pops).
