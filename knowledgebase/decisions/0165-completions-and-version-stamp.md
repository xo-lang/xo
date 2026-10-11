# 0165: Shell completions and version stamping

Status: Accepted, implemented (2026-10-11) on every backend. Spec:
std.md 14.5, core 10.1 and 10.6 (XO1501).

## Context

Two finishing touches every command line tool needs:

- **Shell completions**: users expect `tool <TAB>` to offer subcommands
  and flags. Hand written completion scripts drift from the program.
  `std/cli` (decision 0160) already has the whole declaration.
- **`--version`**: the version belongs to the build (a tag, a commit),
  not to the source. Go uses `-ldflags -X main.version=...` on a `var`;
  Rust reads `CARGO_PKG_VERSION` at compile time. Xo had no way to put a
  value into a program at build time.

## Decision

1. **`cli.completion(cmd, shell: Str) -> Str ! CliErr`** for `"bash"`,
   `"zsh"`, and `"fish"`; another name is a `Usage` error (so a
   `completion <shell>` subcommand gets the usual exit status 2). The
   scripts are static: generated from the declaration, they complete
   subcommands per level, the flags accepted there (inherited ones
   included), `--help`, `-h`, `--version`, the values of choice flags,
   and files after other valued flags and where a command takes
   positional arguments. They do not call the program back (no
   `__complete` protocol as cobra has): no process start per keypress,
   nothing to keep compatible between the script and the binary, and
   the scripts can be reviewed as text. zsh and fish entries carry the
   help line as a description; bash has no descriptions.
2. **Command names are restricted** to letters, digits, `.`, `_`, and
   `-` (not first), so the names can appear in shell `case` patterns,
   fish conditions, and function names without quoting rules that could
   break; flag names already were.
3. **The same text on every backend**: the generator is in Go
   (`std/rt/cli_completion.go`) and in Xo (`std/native/cli.xo`); the
   golden `cli_completion` compares them. A Go test sources the bash
   script and completes words through `COMP_WORDS`, and `bash -n`,
   `zsh -n`, `fish -n` (where installed) check the syntax.
4. **`xo build --stamp NAME=VALUE`** (repeatable; also `xo run`)
   replaces the literal of `const NAME: Str = "..."` in a module named on
   the command line, after checking and before code generation, so every
   backend compiles the stamped text and constants built from it follow.
   The constant keeps a default for unstamped builds (`"dev"`), so
   `xo run` and tests need nothing. A name that is not such a constant
   (missing, another type, an interpolated or computed value) is the new
   error **XO1501**: a typo in a release script must fail the build, not
   ship "dev". Stamps are recorded in the build information (and in the
   build id), so `xo replay` rebuilds the same program.

## Consequences

- A tool gets `--version` with `version: VERSION` on its root command
  and `xo build --stamp VERSION=$(git describe)`; completions with a
  `completion` subcommand that prints `cli.completion(app(), shell)`.
- A stamped value is a constant: it can be used anywhere a `Str`
  constant can, and the checker's view of the program is unchanged (the
  stamp changes only the value, not types or effects).
- Completion scripts for programs whose flags change per run (dynamic
  values such as branch names) are out of reach of static scripts; a
  callback protocol can be added later behind the same function.

## Alternatives

- Go style `-X package.var=value` on a `var`: Xo has no package level
  variables, and naming a symbol by a mangled Go path would expose the
  Go backend.
- Environment variables read at run time for the version: needs `Env`
  in code that only prints a version, and differs per machine.
- A generated `version.xo` file written by a build script: works, but
  is one more file to keep out of version control and to regenerate.
- Completion through a hidden flag that `parse_or_exit` handles
  (`--generate-completion bash`): magic behavior the program did not
  declare; an explicit subcommand is visible in `--help`.
- Shell scripts that call back into the program (cobra's
  `__complete`): richer, but every completion starts the program, and
  the protocol becomes compatibility surface.
