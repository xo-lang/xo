# A std `use c` binding's key held the checkout's absolute path

Status: Observed (2026-11-07)

## Symptom

A std/sqlite object built ahead of time (scripts/sqlite-prebuilt.sh) was
never used: every other checkout, and every installed toolchain, compiled
the 260k line amalgamation again, and the windows/amd64 cross build
needed clang and a Windows sysroot it was meant to avoid.

## Cause

The binder named a binding's symbols `rt.c_<module>_<hash>_<function>`
with a hash of a key that held the header's absolute path, so the prefix
(and the adapter's file name) differed per checkout and per install. A
prebuilt object compiled against one prefix had undefined symbols
everywhere else, so the build fell back to the C sources.

## Fix

A binding of a header under std/ is pinned (bind.std_key, decision 0534
Implementation): its key holds the header's path relative to std, the
link files and libraries relative to std, and the header's sha256, and
the generated `// c-header:` line and the adapter's `#include` are
relative too. The checker (check/cimport.xo) computes the same key. A
new std binding gets the same prefix on every checkout as long as its
header bytes match.
