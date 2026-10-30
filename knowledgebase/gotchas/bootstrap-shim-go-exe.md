# bootstrap-xo.sh kept Go on PATH under Git Bash

Status: Observed (2026-10-30)

**Symptom.** `scripts/bootstrap-xo.sh` on the owner's windows/amd64 box
(Git Bash) stopped at once with `FAIL: go is still on PATH
(.../shim/go)`, though the shim directory is the script's own.

**Cause.** The script hides Go by replacing each PATH directory that
holds `go` with a shim directory of links to the directory's other
programs, skipping `go`, `gofmt` and `go1.x`. On Windows the program is
`go.exe`, so the name check missed it, the shim got a copy (Git Bash's
`ln -s` copies without developer mode), and `command -v go` found it
there. The `-e "$d/go"` test still matched because MSYS tries `.exe`.

**Fix.** The skip list names `go.exe` and `gofmt.exe` too, and the
directory test checks `go.exe`. A check that a name is absent from PATH
under Git Bash must think of the `.exe` form.
