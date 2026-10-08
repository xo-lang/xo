# Editor shows "undefined" errors in bench Go code

Status: Observed (2026-10-07)

Symptom: gopls reports `undefined: X` and "file is within module ... not
included in your workspace" for files under `bench/go/` and `bench/planted/`.

Cause: every benchmark solution is a separate nested Go module that is not in
the editor workspace. The code builds fine with `go build` in its own folder.

Do not add a `go.work` above the bench folders to silence it: it changes how
the benchmark agents' `go` commands resolve modules.
