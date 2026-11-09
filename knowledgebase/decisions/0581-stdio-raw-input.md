# 0581: Stdio.raw_input, raw terminal input

Status: Accepted (owner, 2026-11-09, option A of three, through the lead). Written by Xo Lang 2.

## Context

The REPL's line editing (decision 0572 stage 2: history, multi-line editing) needs each key as it is typed (arrows,
ctrl keys, backspace) with no echo and no line buffering by the terminal. std had only echo off, for
`Stdio.read_secret` (llrt stdin.xo `echo`: it saves the termios, or the Windows console mode, in stdin's record,
clears ECHO and restores it). Go's standard library has no raw mode either; golang.org/x/term `MakeRaw` is the usual
reference.

## Decision

- `Stdio.raw_input(on: Bool) -> Bool uses stdio` in the prelude (spec/std.md, Stdio):
  - true sets raw input, false restores the settings;
  - it returns true when standard input is a terminal it set;
  - a pipe, a file, a test's FakeStdio and WASI return false;
  - FreeBSD also returns false: llrt's standard input has no terminal support there yet (isatty, width and echo are
    false or no-ops as well), which is a follow-up.
- Raw input means x/term MakeRaw's input flags:
  - BRKINT, INPCK, ISTRIP, ICRNL and IXON off;
  - ECHO, ICANON, ISIG and IEXTEN off;
  - VMIN 1, VTIME 0.
  - On Windows, the console's PROCESSED_INPUT, LINE_INPUT and ECHO_INPUT off and VIRTUAL_TERMINAL_INPUT on, so keys
    arrive as the same VT escape sequences as on Unix.
- Output processing (OPOST) stays on, unlike MakeRaw: `println` still starts the next line at column 1, so a program
  writes as it does cooked.
- Native: llrt stdin.xo `raw(on)` shares echo's saved settings. The record's mode word is 0 cooked, 1 echo off,
  2 raw, and the first of echo and raw to change the terminal saves the cooked settings, so restoring always returns
  to them. Layouts:
  - Darwin: tcflag_t 64 bit, c_lflag at 24, c_cc at 32;
  - Linux: TCGETS and TCSETS, 32 bit flags, c_lflag at 12, c_cc at 17.
- Go backend: std/rt term_unix.go and term_windows.go `rawOn`, through the syscall package's names.
- **Restore on every way out.** llrt stdin.xo `restore_tty` allocates nothing: one tcsetattr, ioctl or
  SetConsoleMode from the saved record, and nothing when the mode is 0. It runs from:
  - process.exit, main's return (entry.xo finish) and an uncaught fault (fault.xo exit), through cabi.xo
    `tty_restore` and `rt.fn_addr`, so llrt gains no import cycle;
  - the crash handlers (cabi.xo on_crash, on_crash_win).
  - The Go backend restores in fxFinish, which every exit, fault and interrupt passes.
  - read_secret's echo off gets the same protection; a fault during it used to leave echo off.
  - A signal sent from outside with no handler (SIGKILL) is not covered; `reset` repairs the terminal, as after any
    such program.
- Key decoding (escape sequences into keys) is not in std. The REPL decodes them itself
  (compiler/cli/repl, 0572 stage 2).

## Verification (2026-11-09)

- cmd/xo TestStdioRawInput runs under script(1), a pseudo terminal, on darwin and linux, with the native and Go
  backends:
  - a key arrives with no Enter (`set=true byte=1`);
  - a pipe gives false and still reads its byte;
  - after a program that turned raw input on faults, `stty -a` shows `icanon` and `echo` again.
- Before the Go backend's restore was added, its fault case left `-icanon -echo`. The test caught it.
- The goldens call it where no terminal exists: internal/llvm term_stdio prints `raw: false false`, and internal/gen
  test_term expects false from a FakeStdio after set_tty (TestStdAPICovered).
- Windows is not verified on a console yet: the amd64 and Windows host is down (~/xo-lead/notes/deferred.md).

## Consequences

- A new prelude method: the native, Go and Tee (compiler/xo wholebuild.xo) Stdio implementations, FakeStdio,
  compiler/ssa's Stdio method lists and waits.xo's NO_WAIT list.
- **Seeds:** a seed made before this decision does not know `Stdio.raw_input`, and the seed builds compiler/xo.
  Compiler code cannot call the method until a seed that knows it is cut.
  - wholebuild.xo's Tee therefore returns false instead of calling through.
  - The REPL's line editing (compiler/cli/repl) needs the next seed (0.45 or the next minor) before it can call
    `raw_input`. Options for the lead: a seed cut after this lands, or the REPL's line editing landing with the next
    release's seed.
- std/changes/prelude.md has the entry (decision 0276).
- Programs other than the REPL (prompts, menus, TUIs) can read keys.

## Alternatives

- **Private to the REPL** (option B): smaller, but compiler code would reach into llrt, and no program could reuse it.
- **No line editing** (option C): misses the owner's "editor-style" requirement for the REPL.
- **x/term's full MakeRaw, OPOST off too:** every writer would need "\r\n" while raw, so println would break.
