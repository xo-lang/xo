# 0162: Terminals: TTYs, width, colors, hidden input, binary stdin

Status: Accepted, implemented (2026-10-11) on every backend: the Go
runtime (`std/rt/term*.go`, `stdin.go`), the native backends
(`std/llrt/xo_stdin.c`, `std/native/prelude.xo`, `std/native/term.xo`),
and the test fakes of both. Spec: std.md 3.6, 8.4, 15.

## Context

A command line tool asks four things of its terminal that Xo could not
answer: is this stream a terminal (to decide on progress bars, colors,
prompts), how wide is it (to wrap help and tables), read a password
without echoing it, and read standard input as bytes (a tool in a pipe
may get binary data; `read_line` and `read_all` return `Str`). Colors
should follow the conventions users set: `NO_COLOR` (no-color.org),
`FORCE_COLOR`/`CLICOLOR_FORCE` for CI logs, `TERM=dumb`.

Constraints: authority stays visible (decision 0156): reading a
variable needs `Env`; the standard streams belong to `Stdio`. Tests
must be deterministic (fakes are not terminals unless told so).

## Decision

1. **`Stdio` methods** (the streams are `Stdio`'s authority, `uses
   stdio`): `stdin_is_tty()`, `stdout_is_tty()`, `stderr_is_tty()`,
   `width() -> Int?` (the first of standard output, error, and input that
   is a terminal; no `COLUMNS` fallback, a variable is `Env`'s),
   `read_secret(prompt) -> Str? ! IoErr` (prompt on standard error, echo
   off for the line, restored also on cancellation, then the newline the
   user typed), `read_bytes(max) -> Bytes? ! IoErr`, `read_all_bytes() ->
   Bytes ! IoErr`. Three explicit `is_tty` methods instead of one taking
   a stream enum: no new prelude enum, and each reads as what it asks.
2. **`std/term`** (a module, because color depends on `Env` too):
   `color_enabled(out: Stdio, env: Env)` with the rule NO_COLOR (set,
   not empty) off, else FORCE_COLOR or CLICOLOR_FORCE (set, not empty,
   not `0`) on, else TERM=dumb off, else whether standard output is a
   terminal; `painter(out, env) -> Painter`, a plain struct `{on: Bool}`
   whose methods (`bold`, `dim`, `underline`, `red`, `green`, `yellow`,
   `blue`, `magenta`, `cyan`, `gray`) return the text unchanged when
   off; `strip(s)` removes escape sequences. A painter is decided once,
   so code paints unconditionally and never repeats the check.
3. **Detection**: a terminal is a descriptor whose termios can be read
   (Unix `ioctl` TIOCGETA/TCGETS), or a console handle (Windows
   `GetConsoleMode`); width from TIOCGWINSZ or the console screen
   buffer. Go's standard library has no terminal package and
   golang.org/x/term is not a dependency (std/rt is standard library
   only), so the runtime makes the system calls itself; targets without
   them (wasip1, js, plan9, Linux mips and ppc, whose ioctl numbers
   differ) report no terminal.
4. **Native backends**: C intrinsics in `xo_stdin.c` (`isatty`,
   `ioctl`, `tcgetattr`/`tcsetattr`, a byte read on the same buffer and
   event loop wait as `read_line`), the methods in the native prelude,
   and `std/term` ported to Xo. Both SSA builders (internal/ssa and
   selfhost/ssa) route the new `Stdio` methods to the native prelude.
5. **Fakes**: `FakeStdio` is not a terminal; `set_tty(width)` makes it
   one of that width (all three streams), `set_input_bytes(data)` sets
   binary input, `read_secret` writes the prompt to the fake standard
   error and reads the fake input. Native test binaries do the same
   (`tw_set_tty`).

## Consequences

- Tools can write `if out.stdout_is_tty() { progress } else { plain }`
  and `term.painter(os.stdio, os.env).red("failed")`, and test both
  branches with `set_tty`.
- The golden runners of internal/llvm and internal/arm64 now read
  `stdin.txt`, so native parity of input handling is checked.
- `Stdio` implementations written by users (rare) must add the methods.
- Windows: the console's echo is turned off through `SetConsoleMode`;
  untested in CI (no Windows runner), built by `GOOS=windows go vet`.

## Alternatives

- `is_tty(stream: Stream)` with a prelude enum: one more predeclared
  name that can collide with user types, for no gain in clarity.
- Colors as `Stdio` methods (`out.color()`): would read `NO_COLOR`
  without `Env`, hidden authority.
- Raw mode and key reading (arrow keys, full screen interfaces): out of
  scope for this decision; a later `std/term` addition if tools need it.
- Width with a `COLUMNS` fallback inside `width()`: hidden `Env` read;
  callers that want it do `out.width() ?? Int.parse(env.get("COLUMNS")
  ?? "") ?? 80`.
