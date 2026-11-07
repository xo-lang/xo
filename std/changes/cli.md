# std/cli changes

## 2026-11-07: feature: single_dash_long, Go style -name long flags on an opt-in command

- Kind: feature
- Commit: this commit
- Found by: Xo Lang 2's port of goversioninfo (a Go tool; its users and the README call it as `goversioninfo -icon=x.ico -64`, which std cli read as the short flags -i and -6)
- Go reference: flag (go1.26.1): `-name`, `--name`, `-name=value`, `-name value`, bool `-name=false`
- Test: internal/llvm/testdata/golden/cli_single_dash (and internal/arm64's, and Go parity): the goversioninfo invocations, short clusters beside long names, `--`, a flag after a positional, `-help`, an unknown word with its suggestion, and the same arguments without the option; cli_single_dash_fault: a long name that reads as a short cluster faults at help (and parse); std/rt TestCliValidate's new cases

`cli.command(..., single_dash_long: true)` reads `-name` as `--name`
when name is a declared long flag (decision 0547), so a Go tool ported
to Xo keeps its users' command lines; short clusters apply otherwise,
and a long name that would also read as a cluster of declared shorts is
a fault at parse. Off by default: 0160's GNU reading is unchanged. No
timing row: a command line is parsed once a run.

## 2026-11-07: feature: bool_flag default

- Kind: feature
- Commit: this commit
- Found by: the goversioninfo port (its `-64` and `-arm` default to true on 64-bit and arm hosts; the port read `Parsed.is_set` to fake it)
- Go reference: flag.Bool's default value (go1.26.1)
- Test: internal/llvm/testdata/golden/cli_bool_default (and internal/arm64's): a flag defaulting to true not given, turned off with `=false` and `=0`, given as a short flag, and the help's `[default: true]`, the same text on the Go backend, arm64 and LLVM

`cli.bool_flag(..., default: Bool = false)`: `Parsed.bool` returns the
default when neither the flag nor its variable is given; the help shows
`[default: true]` for a flag that defaults to true (a false default, the
old behavior, shows nothing). No timing row: a flag is parsed once a run.

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
