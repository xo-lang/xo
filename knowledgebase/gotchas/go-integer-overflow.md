# Go integers wrap silently

Status: Observed (2026-10-07, P1 code generation)

Xo arithmetic faults on overflow; Go wraps. The generator emits every
integer `+ - * / % << >>`, unary minus, and conversion as a call into the
runtime (`rt.AddS(a, b, "file.xo:L:C")`, `rt.Conv[int32, int64](x, at)`,
`rt.FloatToInt`), which faults with the operands and the position:
`integer overflow: 9223372036854775807 + 1 does not fit Int at main.xo:4:5`.
Unsigned types use the `U` variants. Golden tests: `arith_overflow`,
`int_ops`, `conversions`, `div_zero`.

The calls are small generic functions; their cost in hot loops has not been
benchmarked yet.
