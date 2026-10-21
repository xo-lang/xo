# A reuse value is the uniqueness mark: mark it, do not remove it

Status: Observed (2026-10-21, decision 0322)

## Symptom

Removing the entry `reuse` of a `var` List parameter proved unique (the
first version of decision 0322) made bench/perf/llvm nbody 10.7% slower
on arm64 (2,921 to 3,235 M instructions), and the build printed "the
inlined arm64 module does not verify (... offset: b5: v44 updates v0,
which is neither a reuse result nor provably unique), so it is built
without inlining".

## Cause

Several passes read a `reuse` result as the proof that a list is held
only here: the SSA verifier (an update must be of a reuse result or a
provably unique list), insert_rc's uniq (the reuse step skips updates of
reuse results), and the arm64 field LICM and in-place passes, which walk
through a reuse to the list's root. A bare parameter is none of these,
so the verifier failed the inlined module and the build fell back to no
inlining.

## Fix

Keep the `reuse` and change what it costs: decision 0322 marks it proved
(aux 2) and the backends lower a proved reuse to its argument. Every
pass still sees a reuse result. The same applies to any pass that wants
to drop a copy-on-write check: change its lowering, not its presence.
