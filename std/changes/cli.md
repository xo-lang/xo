# std/cli changes

## 2026-10-29: feature: float_flag

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey, gap 16 (a CLI needed a floating point flag; knowledgebase/infrastructure/std-coverage.md)
- Go reference: flag.Float64 (strconv.ParseFloat) (go1.26.1)
- Test: internal/llvm/testdata/golden/cli_float (and internal/arm64's): a value, the default, the variable, exponents, inf, NaN, an underscore separated value, a value that is not a number from the command line and from the variable, a missing value, and the help line, the same text on the Go backend, arm64 and LLVM

`cli.float_flag(name, help, short, default, env, required)` and
`Parsed.float` and `float_opt`, as `int_flag` and its readers: the value
is `Float.parse`'s syntax (Go's strconv.ParseFloat: `1.5`, `2e-3`,
`1_000.5`, `inf`, `NaN`), a bad value is the usage error `expected a
number`, and the help shows `<float>` and a nonzero default. No timing
row: a flag is parsed once a run.
