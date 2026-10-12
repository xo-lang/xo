# 0161: Richer Proc: commands, children, pipelines

Status: Accepted, implemented on the Go backend and its test fakes
(2026-10-11), and natively by decision 0172 (2026-10-11).
Spec: std.md 3.7.

## Context

`Proc.run(cmd, args) -> ProcOutput` was the only way to start a
process: no working directory, the parent's whole environment, no
input, output only after the child exits, no terminal for interactive
children, no pipes. Command line tools need all of these (a build tool
runs `git` in a checkout, a wrapper streams a child's log, an installer
runs an editor on the terminal, a script pipes `sort | uniq`).

Constraints from decision 0156: `Proc.only(cmds)` must still decide
which programs may run, whatever new path starts them; and the
environment a child gets should be visible authority, so attenuating the
`Env` (`Env.only`) narrows what a child sees.

Peers: Go's `exec.Cmd` (a struct with fields set before `Start`), Rust's
`std::process::Command` (a builder; `output`, `status`, `spawn`;
`Child` with pipes), Python's `subprocess.run` (keyword arguments).

## Decision

1. **A command value.** `p.cmd(name, args = []) -> Cmd` on `Proc`
   builds a command without running it. `Cmd` is a value; its builder
   methods return a changed copy: `dir(path)`, `env(e: Env)` (the
   child's environment is exactly the variables `e.vars()` shows, read
   at that call, `uses env`), `set_env(name, value)` (one more
   variable, on top of whichever environment), `stdin(data: Bytes)`,
   `inherit()` (standard input, output, and error are the program's own:
   the terminal), `merge_stderr()` (standard error into the standard
   output stream). Without `env`, a child inherits the program's
   environment, as `run` does.
2. **Three ways to run** (each `uses proc`, each checked against
   `Proc.only` first, failing with `ProcErr.Denied(name)`):
   - `c.output() -> ProcOutput ! ProcErr`: runs to the end, collects
     both streams (the `run` of before, with the settings).
   - `c.run() -> Int ! ProcErr`: runs on the terminal (input, output,
     and error inherited, unless `stdin` data was given) and returns the
     exit status: the `system()` of tools.
   - `c.spawn() -> Child ! ProcErr`: starts and returns at once.
3. **`Child`** (a handle): `pid()`, `write(data)` and `close_stdin()`
   (its input, a pipe unless `stdin` data or `inherit` was set),
   `read(max) -> Bytes?` and `read_err(max) -> Bytes?` (its output and
   error streams, at most `max` bytes, `None` at the end; pipes unless
   inherited or merged), `read_line() -> Str?` (one line of its
   output), `wait() -> Int` (closes its input, waits, returns the
   status), `kill()`. Reads and `wait` are cancellation points;
   cancelling the task kills the child, as for `run`.
4. **Pipelines**: `p.pipeline(cmds: List[Cmd]) -> ProcOutput !
   ProcErr` connects each command's output to the next one's input,
   runs them together, and returns the last one's output, the error
   streams of all of them in command order, and the status of the
   rightmost command that failed (bash's `pipefail`), else 0. The
   first command's `stdin` data feeds the pipeline. Every command is
   checked against `Proc.only` before any starts.
5. **Exit status**: the child's exit code; a child ended by a signal is
   128 plus the signal number (the shell's convention; Go reports -1).
6. **Attenuation**: `Proc.only` records its lists in each `Cmd` it
   builds, so a `Cmd` made from an attenuated `Proc` stays attenuated
   wherever it is passed; the check runs when the command runs.
   `Env.only` applies through `env(e)`.
7. **Test fakes**: `FakeProc.cmd` builds commands whose runs answer
   from `on_run(name, out)` (as `run` does; stdin is ignored): `output`
   returns it, `run` writes its output and error to the test's
   `FakeStdio` and returns its code, `spawn` gives a `Child` reading it,
   and `pipeline` gives the last command's output with the failing
   statuses. Unknown commands are `NotFound`.
8. **Native backends**: the methods are XO1201 (planned) from the SSA
   builder. Natively a `Proc` value carries no state yet (decision
   0156), and the C runtime's process code (`xo_proc.c`, decision 0107)
   only collects output; the plan is the same `posix_spawn` with file
   actions for the directory (`posix_spawn_file_actions_addchdir_np`),
   environment, and pipes registered with the event loop.

## Consequences

- `Proc` gains `cmd` and `pipeline`; the prelude gains `Cmd` and
  `Child`. User code implementing `Proc` (rare: capabilities come from
  `main`) must add them.
- Record and replay (core 11.3) records `output`, `run`, and
  `pipeline` like `run` (as effects `proc.output`, `proc.run_cmd`,
  `proc.pipeline`); a `Child`'s streaming reads are not recorded yet.
- A `Child` whose two pipes are both read must have them read from
  separate tasks (or use `merge_stderr`), as with any pipes: a child
  that fills one pipe while the parent waits on the other stops.

## Alternatives

- A struct of options passed to `run` (`RunOptions{dir: ..., env:
  ...}`): needs field defaults on std structs and one more type to name;
  the builder reads the same and composes with attenuation.
- Default child environment empty (least authority): safer, but breaks
  every child that needs `PATH`, and differs from `run`. Kept
  explicit: `env(os.env.only(["PATH", "HOME"]))`.
- Callbacks per output line (`on_line(fn(Str))`): convenient but hides
  the concurrency; a `Child` with `read_line` in a task is as short and
  composes with `select` and cancellation.
- Exit code -1 for a signal (Go): loses the signal; 128 + n is what
  scripts already test.
