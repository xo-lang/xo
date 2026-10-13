# One golden line per function repeats std in every unit

Status: Observed (2026-10-12, decision 0178 stage 7)

Symptom: the first committed layout of the recorded Go outputs of the
TestSelfhost* differentials (one line `<kind> <unit> <function> <hash>`
per function, helper, type line, and literal of every unit) took
1,147,112 bytes for 23 SSA, LLVM, and arm64 units of a sample (9,199
lines for 9 ssadump units): about 85 MB for the 1,221 ssadump units
alone, far from the few megabytes a git diff can carry. gzip saves only
a factor of five, since the hashes are random.

Cause: every unit of the native kinds is a whole program: even a small
example prints 543 to 1,950 pieces, most of them the std and runtime
functions every program reaches (one line each, per stage after the SSA
passes too), and their hashes repeat from unit to unit.

Fix: per unit, one line per family of pieces (the functions of a stage,
the definitions, the helpers, the declarations of a file, a set of type
lines or literals): its member count and the hash of its member hashes;
the member hashes themselves once per kind in a pool (pool-<x>.txt).
Replay resolves a family against the Xo side's members
(goldPieces.resolved, internal/selfhost/gogoldens_test.go): when they
hash to the recorded root they are the Go side's (exact verdicts and
counts); else each Xo member is checked against the pool, which names
the one that differs. The sample took 141,896 bytes this way. A test
that reads a family must resolve it first; names panics otherwise.
