# `xo typedump` paths depend on the working directory

Status: Observed (2026-10-10, writing TestSelfhostCheckerNames).

Symptom: the Go checker run from `internal/selfhost` on `../../bench/...`
printed some file paths absolute and others relative, and the Xo
checker's dump of the same run differed in every path.

Cause: the Go checker makes a module directory relative to the working
directory when it lies below it and leaves it absolute otherwise
(`relDir`); modules reached through an xo.mod are found by absolute path,
so from `internal/selfhost` they print absolute. The Xo checker keeps
paths as given and does not look for an xo.mod above the working
directory.

Fix: run both checkers from the repository root with relative
directories: the test calls `t.Chdir(root)` before the Go checks (safe
because no test of the package runs in parallel at top level) and runs
the Xo binary with `cmd.Dir = root`. The mutation test does the same in
its temporary directory.
