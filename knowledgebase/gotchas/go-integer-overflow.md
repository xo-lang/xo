# Go integers wrap silently

Status: Observed (2026-10-07, P1 code generation)

Xo arithmetic faults on overflow; Go wraps. The generator emits every
integer `+ - * / % << >>`, unary minus, and conversion as a call into the
runtime (`rt.AddS(a, b, "file.xo:L:C")`, `rt.Conv[int32, int64](x, at)`,
`rt.FloatToInt`), which faults with the operands and the position:
`integer overflow: 9223372036854775807 + 1 does not fit Int at main.xo:4:5`.
Unsigned types use the `U` variants. Golden tests: `arith_overflow`,
`int_ops`, `conversions`, `div_zero`.

Since 2026-10-08 Int (int64) operations use typed, inlinable helpers
(`rt.AddI` and friends, constant operand forms such as `rt.MulIK`), and
operations the range analysis in internal/gen/ranges.go proves cannot
overflow are plain Go; see `checked-arith-inline-budget.md` and
bench/results/perf-mono-2026-10-08.md. Golden test `test_range_checks`
checks that operations at and past the bound still fault.
