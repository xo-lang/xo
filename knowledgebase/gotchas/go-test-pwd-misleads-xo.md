# A Go test that runs xo from the repository root must also set PWD

Status: Observed (2026-10-14, decision 0209).

Symptom: `xo dev goldens --backend=go internal/gen/testdata/golden` passed
136 of 136 from a shell, but the same command run by
internal/devgoldens' TestDevGoldensGo failed one program every time:
`gc_config: differs: line 3: want "percent 1200 limit 1073741824", got
"percent 600 limit 9223372036854775807"`. The xo.mod `gc` lines of the
program were not applied.

Cause: xo takes its working directory from $PWD (as cmd/xo did, so
paths print as the shell shows them). `go test` runs a test with $PWD
set to the package directory, and a Go `exec.Cmd` with `Dir` set and an
explicit `Env` passes that $PWD through unchanged. xo then resolved the
program's module root (gobackend.xo gc_line reads `<root>/xo.mod`)
against internal/devgoldens instead of the repository root and found no
gc lines.

Fix: a Go test that runs xo with `cmd.Dir` set also sets `PWD` to the
same directory in `cmd.Env` (internal/devgoldens does). Tests that use
goVetEnv() or internal/xotool's helpers should check the same.
