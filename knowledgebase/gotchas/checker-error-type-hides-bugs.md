# The `<error>` type hides checker bugs

Status: Observed (2026-10-07, building the P1 checker)

Symptom: the checker reported almost nothing on the v0.3 corpus, and a
mutation test (delete one match arm from each corpus match) found a missed
non exhaustive match in task 03 that the same code in a small test caught.

Cause: to avoid cascades (core 10.2), a bad expression gets the `<error>`
type, which unifies with everything and suppresses dependent diagnostics. A
checker bug that produces `<error>` without reporting anything therefore
turns whole regions of code unchecked, silently. Here, variant pattern
fields of a non generic enum were typed `<error>` because the type argument
substitution of an enum without type parameters is nil, and nil was taken to
mean "the enum did not match".

Fix: the substitution and the "matched" flag are separate now. To catch
the next one:

- A temporary debug test that lists every binding and expression typed
  `<error>` or still holding an open inference variable on a clean corpus
  should list only the expressions that have diagnostics.
- Mutation testing on the corpus (remove a `?`, a match arm, a `var`, a
  derive, a conversion, a `uses` clause) and comparing diagnostics with the
  unmodified run finds silent spots quickly:
  `go test ./internal/check -run TestCorpusMutations -mutations -v`
  (results in `bench/results/check-corpus-v0.3.md`).
