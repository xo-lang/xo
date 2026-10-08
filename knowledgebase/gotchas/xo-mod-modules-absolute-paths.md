# Modules reached through xo.mod have absolute paths in test output

Status: Observed (2026-10-07, corpus report for spec 0.4)

Symptom: in the corpus report one diagnostic path was
`/Users/.../bench/paper/v0.3/08-notes-service/store/store.xo` while the
others were `bench/paper/v0.3/...`.

Cause: `xo check` makes a module path relative to the working directory only
when it lies below it (`relDir` in `internal/check/load.go`). The checker
tests run in `internal/check`, so modules found through `xo.mod` (`use
example.com/notes/store`) keep their absolute path, while modules named on
the command line keep the relative `../../bench/...` form.

Fix: `TestCorpusV03` strips the absolute repository root as well as `../../`
before rendering. Any new test that renders paths for modules loaded through
`xo.mod` must do the same.
