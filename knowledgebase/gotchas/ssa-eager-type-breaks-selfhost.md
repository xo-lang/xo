# A type the SSA builder makes eagerly breaks TestSelfhostSSA

Status: Observed (2026-10-11, decision 0172).

## Symptom

TestSelfhostSSA (merge tier) failed on
`internal/gen/testdata/golden/fs_files` with "stage unwind: the type
lines differ" while the build stage compared equal.

## Cause

The Go builder's `env_new` intrinsic called `listsType("Proc")` and then
`listsType("Env")`, so every module that used `Env.only` also declared
`xo.Proc`; the Xo builder (selfhost/ssa) made only the type it needed.
Every named type the builder creates is printed, and the build stage
only checks that the Xo builder's type lines are a subset of the Go
builder's, while the pass stages require the same set.

## Fix

Make a lowered type only when it is used, the same way in both builders
(`name := "Proc"; if env_new { name = "Env" }; listsType(name)`). The
type line message now shows the first differing line.
