# 0715: --race leaves vendored third-party C uninstrumented

Status: Accepted (lead, 2026-11-18; task #152)

## Context

std/sqlite's race evidence (task #152, linux/arm64 host 3, 2026-11-18) failed on a
ThreadSanitizer report inside SQLite's amalgamation: walRestartHdr's plain write of
aReadMark (sqlite3.c, WAL index) against walTryBeginRead's AtomicLoad of it. SQLite reads a
read mark before it takes the lock and checks it again after (its WAL design), so the race is
benign and by design. The amalgamation is vendored unchanged with a checked hash (decision
0534), and Xo has no ThreadSanitizer suppression hook, so the report could be neither fixed
nor silenced. Go's race detector does not instrument cgo's C at all.

Before this decision every `use c` source in a --race build was compiled with
`-fsanitize=thread` (the link's flags, compiler/xo/main.xo build_native).

## Decision

Under --race a vendored third-party C source is compiled without `-fsanitize=thread` and its
`-mllvm -tsan-*` options; every other C source keeps them. The vendored sources are listed by
the end of their path in compiler/xo/cffi.xo `THIRD_PARTY_C` (today
`/native/sqlite/sqlite3.c`); `c_src_flags` drops the flags for them, inside `c_objects`, so
every backend and target that compiles `use c` sources does the same, and the object's cache
key holds the flags it was compiled with.

Stays instrumented: Xo code, std/llrt, std/native, Xo's own C beside a vendored library
(std/native/sqlite/xo_sqlite.c), the generated `use c` adapters, and a user's own C (also one
named sqlite3.c in the user's tree: the match is std's path).

## Consequences

- A race inside SQLite is not reported, as with Go and mattn/go-sqlite3. A race between Xo
  code and SQLite on memory Xo owns is still seen on the Xo side's accesses.
- std/sqlite's race evidence (std/evidence/sqlite/race.md) states that the C library is not
  instrumented.
- A new vendored third-party C source joins `THIRD_PARTY_C` in the change that vendors it
  (with its README Origin line and THIRD_PARTY_NOTICES entry, decision 0534's procedure).
- Test: compiler/xo/cffi_test.xo "--race compiles vendored third-party C without
  ThreadSanitizer, Xo's own C with it (decision 0715)".

## Alternatives

- A ThreadSanitizer suppressions file for sqlite3.c's functions: xo has no hook to pass one to
  its own race runtime, and suppressions still pay the instrumentation's cost.
- Patching sqlite3.c's read of aReadMark to an atomic: breaks decision 0534's "vendored
  unchanged" and its hash check, and a later SQLite update could add another.
- Leaving every user `use c` source uninstrumented too, as Go does: wider than the lead's
  scope (vendored third-party C only); a user's own C keeps race checking.
