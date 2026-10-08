# Older xo binaries break on build directories written by newer ones

Status: Observed (2026-10-08, adding package main's xo_adapt.go)

Symptom: after a new xo wrote `xo_adapt.go` (package main) into a build
directory, an older xo binary building the same program failed with
`undefined: rt.RegAdapter` reported as a code generator bug.

Cause: the build directory is keyed by the program's paths and the mode
(`<user cache>/xo/build/<hash>`), not by the compiler version, and the
older compiler only removes stale files under `m/` and `rt/`, so the new
top level file stayed and referenced runtime code the older runtime lacks.

Fix: the hash includes a layout tag (`layout=2`, internal/build/build.go),
so builds by compilers with a different layout use different directories,
and `writeTree` now removes generated `.go` files at the top level that the
program no longer has. Bump the tag when the generated layout changes in a
way older binaries cannot clean up (`.tools/bin/xo-0.4` and `xo-0.6` share
the cache).
