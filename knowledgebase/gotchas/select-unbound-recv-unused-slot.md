# A select arm without a binding generated an unused Go variable

Status: Observed (2026-10-09, Codex r3, 4 failing cells)

Symptom: `xo check` is clean, then `xo test` or `xo build` fails with
"go build ... failed (code generator bug)" and `declared and not used:
x_rv44` at a `select` arm such as `stopping.recv() => return`.

Cause: `internal/gen/conc.go` declared a slot variable for every `recv`
arm and used it only when the arm binds the value (`v = ch.recv() =>`).
No golden had an unbound receive arm.

Fix: no slot for an unbound `recv` arm; golden
`internal/gen/testdata/golden/select_unbound`. A checker-accepted program
must always build: add new syntax forms to the golden corpus with and
without their optional parts.
