# With and without --no-mono, fault stacks list different Go frames

Status: Observed (2026-10-08, `TestGoldenMono` on test_contracts)

Symptom: the same program prints the same fault message and location with
and without `xo build --mono`, but the Go frame list under the fault
differs: a closure frame (`F_first_letters.func1`) is missing, and generic
functions appear as `F_max__int64` instead of `F_max[...]`.

Cause: `--mono` inlines closure literals passed to List higher order
methods (no Go function, so no frame) and calls specialized copies with
their own Go names (internal/gen/mono.go).

Fix: expected behavior. Messages, `file:line:col` locations, and Xo error
frames (`  at ...`) are unchanged. Since `--mono` became the default
(2026-10-08), `TestGoldenNoMono` compares golden output exactly (and
writes want.txt) and `TestGolden` compares after `dropGoFrames` removes Go
frame lists; do not compare Go frame names across the two lowerings.
