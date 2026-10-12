# Checking a cached dependency as its own run changes its content hash

Status: Observed (2026-10-12, decision 0177).

**Symptom.** A main module that requires a dependency from a fixture
module cache gets XO1304 ("content hash ... does not match xo.lock")
on some runs and not others. Nothing in the dependency's sources
changed.

**Cause.** The dependency has a `use go` line. When the dependency's
own directory is checked as a run (the differential tests check every
directory with .xo files), its module root is the cached module
itself, so the Go checker writes the binding to
`<dependency>/.xo/gobind`. mod.HashDir hashes every regular file of
the tree, dot directories included, so the cached tree now hashes
differently from the hash recorded in the main module's xo.lock. With
the runs checked in parallel, whether the main module is checked
before or after that write decides the result. A dependency's binding
normally goes to the main module's cache (check deps.go goBindRoot);
the write only happens because the dependency is a root run.

**Fix.** Keep a fixture whose cached dependency has `use go` or
`use c` out of the repository walk and check a copy of it:
internal/selfhost/testdata/hatches is skipped by moduleDirs and checked
by TestSelfhostCheckerHatches in a temporary directory, main modules
only. Next experiment if a real module cache can be written this way:
make `xo check` of a directory inside the module cache bind into a
cache outside it (`$XO_GOBIND_DIR`, or the user cache directory).
