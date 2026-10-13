# MSYS2 programs split their command line at newlines

Status: Observed (2026-10-12), fixed in std/llrt/xo_proc.c (pr_w_quote).

Symptom: on Windows, `p.cmd("printf", ["a\r\nb\nrest"])` printed `a`
and nothing else, and a pipeline starting with `printf "b\na\nb\nc\n"`
gave `b` and the warning `printf: warning: ignoring excess arguments,
starting with 'a'`, while every other argument arrived intact.

Cause: Windows passes one command line string; the program splits it.
Go's syscall.EscapeArg (which the runtime followed) quotes an argument
only when it holds a space, a tab, a quote, or a backslash, because the
MSVC C runtime and CommandLineToArgvW split at spaces and tabs only.
Git for Windows' sh, printf, and the other MSYS2 and Cygwin programs
split at every space character, line ends included.

Fix: quote an argument that holds `\n`, `\r`, `\v`, or `\f` too; the
MSVC rules remove the quotes, so other programs see the same argument.
The Go backend (os/exec) still splits there for MSYS2 programs.
