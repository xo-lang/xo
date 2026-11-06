# std/native/.xo, the binding cache, matches a `*.xo` glob

Status: Observed (2026-11-06, the std/sqlite gate, decision 0534)

**Symptom.** After a std/sqlite test run, internal/repotest
TestOutsideBytesValidated failed with `read ../../std/native/.xo: is a
directory`, in a tree where no source file had changed.

**Cause.** std/sqlite's `use c` binding is cached in the module's
`.xo/cbind` directory (decision 0534), which for a std module is
`std/native/.xo/cbind`. Go's `filepath.Glob("std/native/*.xo")` matches the
directory `.xo` itself (`*` matches a leading dot), so every test that globs
std's native modules and reads each match got a directory. A walk of std
that copies its files (cmd/xo TestStdChecks) also copied the cached
declaration files as if they were std source. The directory is git ignored,
so it appears only after something built std/sqlite in that checkout.

**Fix.** The globs skip directories (internal/repotest outside_bytes_test.go,
internal/stdcoverage loadImplDecls) and the walk skips a `.xo` directory
(cmd/xo stdcheck_test.go). New code that lists std's native modules does
the same: match files, not names.
