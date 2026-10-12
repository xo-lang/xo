# 0167: The Go code generator in Xo, compared as generated Go text, and `--backend=go` in the Xo driver

Status: Accepted (2026-10-11), implemented in `selfhost/gen` (gen.xo,
decl.xo, fn.xo, expr.xo, call.xo, pattern.xo, support.xo, own.xo,
conc.xo, json.xo, fuzz.xo, adapt.xo, arena.xo, mono.xo, ranges.xo,
goparse.xo, goblank.xo, main.xo), the hidden `xo gendump`
(cmd/xo/gendump.go, build.GoGenDump, gen.FormatFile), and
`internal/selfhost/gen_test.go` (TestSelfhostGen,
TestSelfhostGenMutations): every declaration of every repository run
the Xo checker checks is identical to internal/gen's, and every such run
is byte identical as a whole, in every mode compared. `selfhost/xo`
gains `xo1 build --backend=go` (gobackend.xo): it compiles
selfhost/check to the Go internal/build writes, byte for byte, and
compiles itself to a fixed point. Third stage of decision 0150's
amended order (after the arm64 emitter, 0158, and the LLVM emitter,
0166). Evidence: `bench/results/selfhost-gen-2026-10-11.md`.

## Context

Decision 0150 (amended 2026-10-11) orders the self hosting stages: the
arm64 emitter and the switch-over, the LLVM emitter, the Go code
generator (internal/gen, 11,354 lines of Go without tests), then the
retirement of the Go implementation. Unlike the native emitters, the
Go generator works from the checked tree, not the SSA: its input is
what selfhost/check records (types, objects, calls, constructors,
patterns, bindings), and its output is Go text, written in pieces
(declarations, captured statement text spliced into enclosing code)
and finished by a step on the whole file (internal/gen pkgGen.file:
blank uses dropped with go/parser's identifier resolution, repeated
//line directives dropped, gofmt). Two parts of the generator read
generated Go back: `terminates` (does a body end in a terminating
statement, go/parser) and the blank use step.

## Decision

**The dump.** `xo gendump [--test] [--no-mono] [--no-ranges]
[--no-arenas] [--release] [--no-error-args] [--each] <dirs...>`
(hidden) checks dirs as `xo build` does for the Go backend and prints
the files gen.Generate makes (`== file <path>` before each, sorted by
path; not the build information or zone data internal/build adds), or
`== rejected`, or `== error <why>`; `--each` prints `=== gen <dir>`
before each run. `xo gendump --fmt` reads the output of the Xo
generator and puts its Go files through internal/gen's last step
(gen.FormatFile), or only gofmt after a line `== prepped`.

The Xo generator (`xo run selfhost/gen -- [--std=DIR] [--wd=DIR]
[--each] [--test] [--no-mono] [--no-ranges] [--no-arenas] [--release]
[--no-error-args] [--prep] <dirs...>`) checks with selfhost/check (not
native mode) and prints the same files with the text of each package
before the last step; with `--prep` it applies that step itself but
gofmt (selfhost/gen `format_prep`: goblank.xo, a resolver of the
locals of Go function bodies over the tokens of goparse.xo), then `==
skipped` and one line per declaration it left out (`<file>
<declaration>: <why>`).

**The comparison** (TestSelfhostGen): every repository run with .xo
files, as a program and, when it has test blocks, as a test binary,
with the default lowering; every fourth run also with
`--no-mono --no-ranges --release --no-error-args` (every run in the full
tier). The Xo side runs with `--prep` and its files go through gofmt;
each file is cut into top level declarations (go/parser), keyed by name
(`Recv.Name` for a method), with the temporaries `x_<prefix><N>` (a
package wide counter) renumbered within each declaration, so a
declaration skipped earlier shifts no other. Every declaration the Xo
generator writes must be identical to the Go generator's; one it skips
is counted, never compared; a declaration missing after a skip in the
same file is counted apart; a run with nothing skipped must be byte
identical as a whole. Runs the Xo checker does not check (`use go`,
fetching), runs with holes (XO0900, which the Xo checker does not
report), and errors of both generators are counted, not compared.
TestSelfhostGenMutations applies decision 0150's code mutations (60 in
the merge tier, 300 in the full tier). The merge tier leaves the five
largest runs (selfhost/check, ssa, parser, arm64, gen) to the full
tier; the full tier builds the Xo generator with the go, arm64, and
LLVM backends.

**Skipping.** A construct of a stage not ported marks the declaration
being generated as skipped (`Gen.unsupported`): its text is dropped at
the end of the declaration (`begin_decl`, `end_decl`, keeping the
interface adapters written meanwhile, which are declarations of their
own) and listed, so nothing is ever written wrong. No declaration is
skipped at the end of the port.

**The stages**, in the order they were compared:

1. **Core** (everything but monomorphization and the range analysis):
   declarations and derived methods, functions, contracts and
   invariants, statements, loops, places, expressions, calls, patterns,
   closures, list ownership marks, interface adapters and the adapter
   registry, concurrency, `derive Json` and `SqlRow`, property test
   generators, contract fuzzing, arenas, zone use, test binaries, and
   the terminating statement rule (goparse.xo); compared with
   `--no-mono --no-ranges`.
2. **Monomorphization** (mono.xo): copies, typed comparisons,
   devirtualized constraint methods, lambda specialization, inlined
   higher order List methods.
3. **The range analysis** (ranges.xo), with the default flags.
4. **The last step** (goblank.xo: blank uses, repeated directives),
   compared with `--prep`.
5. **The driver**: `xo1 build --backend=go` writes the generated
   packages after `format_prep` and gofmt (the `gofmt` of `go env
   GOROOT`), the runtime package copied from DIR/rt (`*.go`, `*.s`, not
   tests), the build information (sources, ID, as internal/build
   computes them), the zone data of std/time (internal/build tzFiles
   with the default `--tzdata=embedded`: the TZif data of constant zone
   names read from the stored entries of the toolchain's zoneinfo.zip,
   or the whole archive for a name known at run time), then runs `go
   build -trimpath` with internal/build's flags and tags.

Hybrid builds (`xo build --hybrid`, hybrid.go: Go with arm64 assembly)
and `gen.StdTypes` (the generator of std/rt/std_gen.go) are not
ported, as the arm64 emitter's Go mode was not (decision 0158).

## Consequences

- A change to internal/gen must be mirrored in selfhost/gen in the same
  change once TestSelfhostGen runs in the merge tier (`go test -short`
  compares every 25th run).
- Decision 0173's changes to internal/gen (resource budget polls,
  budget.go; log attribution, logfrom.go) landed on main during the port
  and are mirrored in selfhost/gen budget.xo (2026-10-12).
- selfhost/check exports `implements`, `find_method`, and `const_str`
  (Info.Implements, FindMethod, ConstString); its records were enough
  otherwise.
- With the Go generator ported, the whole compiler exists in Xo; what
  remains of decision 0150's order is the retirement of the Go
  implementation (with `--tzdata=system`, the other build flags, and hybrid
  builds in the Xo driver, or decided out), the WebAssembly backend, and the runtime port.

## Alternatives

- **Compare the text before the last step** (raw): the Go generator
  writes indentation and blank uses that gofmt and dropBlankUses
  normalize, so raw text differences would not all be bugs; and the
  last step is part of what the driver must do.
- **Run gofmt and dropBlankUses in Go for the Xo driver** (a helper
  program): the Go backend needs the Go toolchain anyway, but the
  point of the stage is a compiler with no Go implementation; gofmt is
  part of the toolchain, dropBlankUses is the generator's own.
- **Port go/parser's resolution whole**: generated Go is a small subset;
  the resolver of goblank.xo handles that subset and is checked against
  dropBlankUses on every generated file of the repository.
