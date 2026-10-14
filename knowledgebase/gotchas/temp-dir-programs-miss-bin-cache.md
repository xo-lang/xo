# Programs built from a fresh temporary directory never hit XO_BIN_CACHE

Status: Observed (2026-10-14, bench/results/short-tests-2026-10-14.md)

Symptom: a test that writes the same program into `t.TempDir()` on
every run rebuilds it every run (a Go backend build is a `go build`, a
native one a clang link), although XO_BIN_CACHE is on; internal/gen's
`use go` goldens took 5 to 7 s each under load, its compile property
programs 21 CPU seconds per run.

Cause: the binary cache key covers the generated tree, and the
generated tree holds the build information, which records the
program's absolute directory (for `xo replay`). A new directory is a new
key. internal/gen's TestMain also set XO_GOBIND_DIR to a fresh directory
per run, so every `use go` program was bound again first (about 0.8 s)
and then missed the cache for the same reason.

Fix: put programs a test generates in `xotool.TestProgramDir(kind,
files)` (a directory named by the files' contents under
<user cache>/xo/testprog, the same across runs and worktrees) and write
outputs to `t.TempDir()`; keep binding caches with
`xotool.TestGobindDir()` (keyed by the xo binary and the go version).
Compile property CPU fell from 21 to 12.7 s on a warm run.
