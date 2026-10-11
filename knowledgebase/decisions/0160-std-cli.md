# 0160: std/cli: argument parsing

Status: Accepted, implemented (2026-10-11) on the Go backend
(`std/rt/cli.go`) and the native backends (`std/native/cli.xo`), with
byte for byte the same help, usage errors, and faults. Spec: std.md 14.

## Context

Xo targets backends and the tools around them. A command line tool
needed `os.env.args()` and hand written parsing: every program
reinvented flags, help text, and exit codes, and agents writing Xo had
no idiom to reach for. The std gaps for command line tools (this
decision and 0161 to 0165) start here.

What the parser must give, set by the task: bool, int, string,
duration, and repeated flags; positional arguments; subcommands; a
generated `--help`; usage errors with exit status 2; the `--`
terminator; variables as fallbacks. And it must be easy for an AI agent
to write correctly: declarative, typed results, no reflection.

Peers. Go's `flag` (a pointer per flag, no subcommands, no short
names, stops at the first positional argument); cobra (commands as
structs plus flag registration calls, persistent flags); Rust's clap
(derive on a struct, or a builder; the derive needs macros Xo does not
have); Python's argparse (a builder returning a namespace read by
string); kong and urfave/cli (struct tags or literals). Xo has no
reflection and no macros, so the choice is between a builder with typed
handles and a declaration read by name.

## Decision

1. **A command is a value made by functions with named arguments.**
   `cli.command(name, help, flags: [...], args: [...], commands: [...],
   version: "")`, `cli.bool_flag`, `str_flag`, `int_flag`,
   `duration_flag`, `choice_flag`, `strs_flag` (repeated), `cli.arg`
   (optional with `optional: true`), `cli.rest` (with `min`). Every
   option is a named argument with a default, so a declaration reads as
   data, is order free, and has no builder state (no "call parse before
   adding flags", no `var` receivers). Typed defaults: `int_flag` takes
   `default: Int`, `duration_flag` a `Duration`.
2. **Results are read by name with typed accessors.**
   `p.bool("verbose")`, `p.int("count")`, `p.str("config")`,
   `p.duration`, `p.strs`, the `_opt` forms for "was it given",
   `p.arg`, `p.arg_opt`, `p.rest`, `p.is_set`, and `p.command()` (the
   subcommand path as one string, so a `match` on string literals
   dispatches). A name not declared anywhere, or read with the
   accessor of another type, is a fault whose message names the right
   accessor. A flag of a subcommand that was not selected reads as not
   given (default, `None`, `[]`), so programs may read flags up front.
3. **Mistakes in the declaration are faults** at `parse` or `help`
   (bad or duplicate names, a short name used twice along a path, `-h`
   or `--help` or `--version` taken, required after optional
   arguments, `rest` not last, arguments and subcommands on one
   command): they are bugs in the program, found by the first run or
   test, not input errors.
4. **Errors in the arguments are values.** `parse(cmd, args, env) ->
   Parsed ! CliErr uses env` with `CliErr { Help(text), Version(text),
   Usage(msg, usage) }`, `exit_code()` 0 or 2. `parse_or_exit(cmd, env,
   stdio, proc)` is the `main` form: help and version to standard
   output with status 0, `error: <msg>`, the usage line, and a pointer
   to `--help` on standard error with status 2. It takes the three
   capabilities it uses instead of `Os`, so the call shows its
   authority (decision 0156's direction).
5. **Grammar**: GNU style. `--name value`, `--name=value`, `-n value`,
   `-nvalue`, clustered short bools (`-vq`, a valued short flag ends the
   cluster), `--flag=true|false|1|0` for bools, `--` ends flags, `-`
   is positional, flags anywhere (before or after positional
   arguments). Subcommands are chosen by the first positional argument;
   the flags of every command on the path stay accepted after a
   subcommand (cobra's persistent flags, by default). Last value wins;
   repeated flags collect.
6. **Variables as fallbacks**: `env: "NAME"` on a flag reads the
   variable through the `Env` passed to `parse` when the flag is not on
   the command line (empty counts as unset; a repeated flag's variable
   is split on `,`). The read goes through the capability, so `Env.only`
   attenuation applies and tests use `FakeEnv`. An invalid value names
   its variable: `invalid value "x" for --count (from $COUNT): ...`.
7. **Help text is fixed by the spec** (std.md 14.4): sections, column
   alignment, annotations `[default: x] [env: X] [required]
   [repeatable]`, placeholders from `value:` or the type or the
   choices. Fixed text lets tests compare it and lets both backends be
   checked against each other byte for byte.
8. **Did you mean**: the closest name within two edits for unknown
   flags and commands.
9. **Both backends**: Go code in `std/rt/cli.go`, an Xo port in
   `std/native/cli.xo` (pure Xo over the prelude, no new C). The golden
   programs `internal/llvm/testdata/golden/cli_*` run on the Go, LLVM,
   and arm64 backends and must agree; `internal/gen` golden
   `test_cli` runs `parse_or_exit` under `testing.run_main` with
   `FakeEnv` (also under `xo test --no-gc`).

## Consequences

- A tool's `main` is `let p = cli.parse_or_exit(app(), os.env, os.stdio,
  os.proc)` and a `match p.command()`. Tests drive the same function
  through `testing.run_main(main, os, args: [...])` and check exit
  codes, output, and errors, or call `cli.parse` directly.
- Typos in flag names inside the program are faults at run time, not
  compile errors: the price of reading by name without macros. The
  first run or test that reaches the accessor finds them, and the
  message names the declared kind. A checker rule that resolves
  constant names against a constant declaration is possible later.
- Golden test harnesses gained `env.txt` (`NAME=VALUE` per line, extra
  variables for the run) in internal/gen, internal/llvm, and
  internal/arm64; the arm64 golden runner now also passes `args.txt`.
- Shell completions (decision 0165) are generated from the same
  `Command` values.

## Alternatives

- **Typed handles** (`let n = cli.int_flag(...)` returning `Flag[Int]`,
  `p.get(n)`): typos become compile errors, but a command's flag list
  then holds flags of different types (needs an erasing conversion per
  flag), declarations split between bindings and the command, and
  subcommand flags need handles threaded to where they are read. More
  for an agent to get right than a name.
- **A struct literal tree** (`cli.Command{name: ..., flags: [...]}`):
  equally declarative, but needs field defaults on std structs and
  exposes fields that become compatibility surface.
- **Derive on a user struct** (clap derive, kong): needs a derive that
  knows flag semantics (names, shorts, help) on fields, that is,
  attributes Xo does not have.
- **Go's `flag` semantics** (stop at the first positional argument, no
  short names, `-flag` long names): familiar to Go users, surprising to
  everyone else; GNU style is what users of most tools expect.
- **Exit status 1 for usage errors**: 2 is the long standing convention
  (`grep`, `ls`, BSD `sysexits` EX_USAGE is 64 but rarely used), and
  scripts tell a usage error from a failure by it.
