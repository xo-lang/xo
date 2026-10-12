# Self hosted Go code generator, 2026-10-11

Decision 0167. The Go code generator in Xo (`selfhost/gen`, 13,634
lines of Xo with the driver and the two readers of generated Go)
against internal/gen (11,354 lines of Go without tests), compared
through the new hidden `xo gendump`: the generated files after
internal/gen's last step (blank uses dropped, repeated //line directives
dropped, gofmt), declaration by declaration and as whole files. The
input of both is the checked program: internal/check's for the Go
generator, selfhost/check's (not native mode) for the Xo generator.

Hybrid builds (`--hybrid`, Go with arm64 assembly) and gen.StdTypes are
not ported (decision 0167).

## The stages

The port was written whole, then compared stage by stage with the flags
of each (development sweeps: `xo gendump` against the Xo generator on
every directory, whole text compared after `xo gendump --fmt`):

| stage | flags | internal/gen goldens (131) | other repository runs (538) |
|---|---|---|---|
| 1, core | `--no-mono --no-ranges` | 118 byte identical, 13 `use go` | 520 of 531 byte identical (the five largest not run) |
| 2 and 3, monomorphization and ranges | default | 118 byte identical, 13 `use go` | 527 of 538 byte identical, 1 fault of the Xo generator before the fix below |
| test binaries | `--test` | 118 byte identical | 527 of 538 byte identical |
| release | `--no-mono --no-ranges --release --no-error-args` | 118 byte identical | the five largest byte identical |
| 4, the last step in Xo | `--prep` (default and `--test`) | 118 byte identical (after the fix below) | 527 of 538 byte identical |

The 11 runs not compared in the sweeps: 7 need modules from the
fixture cache, which the sweeps did not set (the test does), 2 have
holes (XO0900: the Go checker rejects them, the Xo checker does not
report holes), 2 bind Go packages (`use go`, which the Xo checker does
not bind). No declaration was ever skipped and no declaration differed
in the comparisons above; the two failures found were faults, not
wrong text:

- The range analysis read a stale copy of the checker's type table:
  `g.c.resolve(g.mono_type(t))` evaluates the by value receiver before
  the argument that grows the table (gotchas/receiver-read-before-args.md;
  index out of range on bench/xo/v0.4/02-worker-pool). A scan for the
  pattern found two more; all bind the argument first.
- The Go tokenizer of goparse.xo made an explicit `;` an operator
  instead of a semicolon, so the blank use resolver gave up on files
  with one line bodies (`func() bool { ...; return true }`) and kept
  every `_ = x` (19 goldens differed with `--prep`). The terminating
  statement rule had not shown it: its one line bodies are skipped as
  function literals.

## TestSelfhostGen, merge tier (2026-10-12)

The go build of the Xo generator, every repository run but the five
largest as a program, each with test blocks also as a test binary, and
every fourth also with `--no-mono --no-ranges --release
--no-error-args` (905 runs in all), the Xo side with `--prep`:

| runs (905) | count |
|---|---|
| compared | 464 |
| byte identical as a whole | 464 |
| rejected by both checkers | 362 |
| with holes (the Xo checker does not report XO0900) | 2 |
| not checked by the Xo checker (`use go`, fetching) | 22 |
| the same error from both generators (no `fn main`) | 55 |

All 23,887 declarations of the Go generator in the runs compared are
identical; none skipped. TestSelfhostGenMutations, 60 code mutations
(seed 20261011): 44 compared, 44 byte identical, 1,410 of 1,410
declarations identical, 16 rejected by both. Both tests take 30 s
(29.7 s for the package run). The first run of the test failed on
selfhost/xo only: the test cut the dump at `== file ` anywhere, and the
Xo generator's own dump code (compiled into selfhost/xo) holds that
text in a string; the test now cuts at line starts.

The arm64 build of the Xo generator (1 min 46 s to build) gives the
same text: the 118 goldens and the five largest runs byte identical
with `--prep`.

## The driver

`xo1 build --backend=go` (selfhost/xo built with the go backend)
generates, finishes, and builds with the Go toolchain; its build tree
against `xo build --backend=go` (XO_BUILD_DIR) for the same program:

| program | package files compared | identical | binaries |
|---|---|---|---|
| internal/gen/testdata/golden/closures | 2 and main.go, xo_main.go, xo_adapt.go, go.mod | all | same output |
| selfhost/check | 4 and the rest | all | same typedump of selfhost/parser (41,400 lines) |
| selfhost/xo (the driver itself) | 8 and the rest | all | |
| internal/gen/testdata/golden/time_zones (constant zone names) | 2 and the rest, xo_tzdata.go | all | same output |
| internal/gen/testdata/golden/time_dynamic_zone (the whole database) | 2 and the rest, xo_tzdata.go, xo_zoneinfo.zip | all | same output |

The fixed point: xo1 compiled by itself with `--backend=go` (B)
compiles selfhost/xo again to a byte identical build tree and a byte
identical binary (`go build -trimpath` is deterministic).

## Speed

Whole commands on selfhost/check (check and generate, one run each,
Apple M4 Pro with other agents' test suites running, so rough): `xo
gendump` 0.81 s, the Xo generator (go build) 1.66 s, with `--prep` 1.67
s, for 3.2 MB of Go.
