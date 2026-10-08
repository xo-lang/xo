# Nested helper generation reuses a counter that the nested call advanced

Status: Observed (2026-10-08, internal/arm64 rc.go)

Symptom: the `rc_failing` golden failed to assemble with
`error: symbol 'Lxo_drop6_v1' is already defined`.

Cause: `rcHelper` numbered its labels with `g.nhelp` at the time each
label was formatted, but generating the helper of a leaf (an enum
payload that is itself an enum with heap fields) calls `rcHelper`
recursively, which increments `g.nhelp`. Labels formatted after the
recursion used the inner helper's number and collided with its labels.

Fix: take the number once (`id := g.nhelp`) when the helper starts and
use `id` for every label; helpers added later (`eqHelper`, `boxDrop`,
`envDrop`) get each label from `helperLabel`, which takes a fresh number
per label. Any generator that recurses while building text must not
read shared counters after the recursion.
