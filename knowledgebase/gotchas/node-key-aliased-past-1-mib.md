# A node past 1 MiB of a file had another file's checker records

Status: Observed (2026-11-04)

Symptom: aws's generated S3 client (service/s3/client.xo, 1,268,329
bytes) failed to build on the native backends once it imported one more
package, a package with a single function: XO1201 "the method
`List.push` ... not available on the native backends" and "assigning
to this expression" in functions the change never touched
(serialize_put_object, deserialize_put_object; aws issue 8). Without
the import, or without the new code near the end of the file, it built.
A two file program built to hit the same keys built with no error and
called the wrong function: `alpha()` ran `beta()` (internal/arm64
TestNodeKeysPast1MiB printed `2 2 1 2` for `1 2 1 2`).

Cause: the checker keys what it records about a node (its type, the
function or method a call resolves to, constructors, patterns) by the
node's file, start and end packed into one Int, 20 bits each:
`((file * 2^20 + start) * 2^20 + end) * 64 + kind` (compiler/check
info.xo key_in, expr.xo key_of). An offset of 2^20 or more carries into
the next field, so the node [s, e) of file f past 1 MiB has the key of
the node [s - 2^20 + 1, e - 2^20) of the same kind in file f + 1. When
both exist, the records of one are read for the other. Adding an import
renumbers the files, which moved the client's late nodes onto real
nodes of another file. A stolen record that does not fit is reported
(XO1201 here); one that fits builds the other node's meaning silently.
The packing dates from the self-hosted checker's port, so every release
before the fix had it; it needs a .xo file over 1 MiB (several std and
stdtest corpus `_test.xo` files are, up to 8.2 MB, so their tests ran
with it too).

Fix: key_in keys a node by its start in the run's sources laid one
after another (each file's base, given by give_base when its source is
known), its length, and its kind: 29, 28 and 6 bits, so no two nodes
share a key. A run whose sources pass 512 MiB, or a key asked for a
file with no source yet, is an internal compiler error (a fault naming
the file), never an alias. Regression test: internal/arm64
TestNodeKeysPast1MiB.

Avoid: never pack positions into a key with a field narrower than what
the input can reach; when a bound is unavoidable, check it and fail
loudly.
