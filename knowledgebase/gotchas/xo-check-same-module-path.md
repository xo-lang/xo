# `xo check a b` silently skips a module whose path repeats

Status: Observed (2026-10-09)

Symptom: `xo check snap/07-cache-run1 snap/07-cache-run2` reported the
errors of the first directory only; checked alone, the second had one
error (XO0701). No message says a directory was skipped.

Cause: the checker keys modules by module path (`internal/check/load.go`,
`loadDir`). Both copies have an `xo.mod` declaring `module
example.com/cache`, so the second directory resolves to the module already
loaded from the first and is never read.

Fix: check copies of one module separately (one `xo check` per directory),
or give each copy its own module path. Measurements over snapshot copies
(bench/results/habit-diagnostics-2026-10-09.md) run one directory per
process for this reason. Next step: make `loadDir` report a usage error
when a module path is already loaded from a different directory.
