# `xo mod tidy` removes requirements that no .xo file imports

Status: Observed (2026-10-09)

Symptom: a test module whose xo.mod required two modules but had no .xo
files lost both `require` lines after `xo mod tidy`, and every later step
of the test resolved nothing.

Cause: tidy keeps a requirement only when a package of the main module
imports it, or when it raises a version the rest of the graph selects
(the equivalent of Go's `// indirect` lines). A module with no imports
needs nothing.

Fix: write the imports first, then tidy (`internal/mod/mod_test.go`
gives its test modules a `main/main.xo` through `uses(...)`). To pin a
version without importing it yet, require a version higher than the
graph selects, or use `xo get` and do not run tidy until the import
exists.
