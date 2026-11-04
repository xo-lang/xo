# bcrypt: S box loads without index checks and F without overflow checks (2026-11-04)

Xo Lang 2, the lead's queue (Lang 3's bcrypt at 1.19 and 1.20 of Go,
bench/results/std-twin-rows14-2026-11-03.md lines 10 and 11). All rows on
the first linux/arm64 offload host (12 CPUs).

## Steps (ab-host.sh, bench/perf/bcrypt)

| step | workload | A wall ms | B wall ms | ratio | interval |
|---|---|---|---|---|---|
| guard proof + expand's length check + top byte mask (0dcb7d631 to 34cf4a86a) | hash | 1062 | 1042 | 0.981 | 0.98 to 0.984 better (n=6) |
| | compare | 1094 | 1074 | 0.982 | 0.962 to 0.999 better (n=10) |
| | xo self build | 32227 | 32261 | 1.001 | 0.999 to 1.004 within noise (n=5) |
| F's two adds as wrapping_add (34cf4a86a to d23443d19) | hash | 1043 | 882.9 | 0.846 | 0.826 to 0.852 better (n=10) |
| | compare | 1042 | 882.0 | 0.846 | 0.845 to 0.847 better (n=6) |

The first step removed 14 of the 18 index checks in expand (the S box
loads of F); a well predicted check costs little, so it moved the wall
2 percent. The second removed the two overflow checks of F's adds (each
operand a 32-bit word, so the sum cannot overflow): 15 percent.

## Against Go (xo bench --ab, A = Go's golang.org/x/crypto/bcrypt, B = Xo, 15 rounds)

Both built on the Mac for linux/arm64 (Xo at d23443d19 with
`--release`, Go 1.26.1 from bench/perf/bcrypt/twin.go.txt with its
vendored x/crypto), run alternated on the host:

| workload | Go wall ms | Xo wall ms | Xo / Go | interval | max RSS Xo / Go |
|---|---|---|---|---|---|
| hash | 882.9 | 881.0 | 0.998 | 0.997 to 0.999 | 0.55 |
| compare | 882.3 | 881.8 | 0.999 | 0.997 to 1.0 | 0.544 |

bcrypt was 1.44 and 1.55 of Go at its port (rows9), 1.19 and 1.20
after the ubfx and load offset codegen (rows14), and now matches Go.
