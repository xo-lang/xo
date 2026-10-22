# 0334: A Mutex read share does not count its reference

Status: Proposed (Xo Lang 2, 2026-10-22; for the owner's morning
summary)

## Context

The CRUD server's 6 core runs (2026-10-21) spent their extra cores on
coordination, not work: the server did about 1.8 cores of work at 3 and
at 6 cores. Readers of the store's Mutex were part of it. Since 438c34b42
a reader takes its share with one atomic add and no lock while no writer
holds or waits, but each with_read still does rt.mutex_rlock's rc.inc of
the Mutex's value list and rt.mutex_runlock's rc.dec: two more atomic
read-modify-writes, on the one list object every reader on every core
shares, so its count line moves between cores at every read.

The count buys nothing. A writer takes the value (and can replace or
free the list) only while no reader holds a share, and the compiler
treats rlock's result as an owned reference that exactly one
rt.mutex_runlock consumes (compiler/ssa conc.xo mutex_with: the share's
list feeds list.index, a borrow, and runlock or the fault path's
Cleanup.Unlock; rc.xo's call table marks runlock's list consumed). The
reader's reference is a token whose lifetime the share already bounds.

## Decision

rt.mutex_rlock returns the Mutex's list without rc.inc and
rt.mutex_runlock gives the share back without rc.dec (std/native/llrt
mutex.xo, both paths: the lock free one and the locked one, and the
grant to a waiting reader). The compiler is unchanged: it keeps
treating the share as an owned reference consumed once, which is what
makes the count unnecessary. A value the closure returns that aliases
the list's element is retained by the closure's own code (insert_rc on
the borrowed parameter), so it outlives the share as before.

Measured before landing, on the RW stress (12 tasks) and the CRUD server
at 1, 3 and 6 cores in a quiet window, against the reader fast path
alone. Landed only if the verifier (XO_RC_CHECK=1 goldens), the race
tables and TestTSanSharedMaps pass.

## Consequences

- Two fewer atomic operations on a shared line per with_read; with the
  fast path, a read share costs two atomic adds on the Mutex record
  (shared too, but one line instead of two).
- The invariant moves from "every reference counted" to "a read share's
  reference is bounded by the share": any future compiler change that
  lets the share's list outlive rt.mutex_runlock (keeping it, returning
  it, storing it) breaks it. mutex_with's comment and rc.xo's table say
  so; the RC verifier, which sees the token as owned and consumed, still
  catches a second consume or a missing one.
- The Go backend (std/rt) and wasm keep their own Mutex; unaffected.

## Alternatives

- Keep the count and move it off the shared line (a per worker count):
  more memory per Mutex and the same number of atomics.
- A compiler-side borrow for the with_read binding (no token at all):
  larger change to conc.xo and rc.xo for the same runtime effect.
