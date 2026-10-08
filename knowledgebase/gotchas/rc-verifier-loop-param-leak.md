# RC verifier: a loop block parameter hid a leak

Status: Observed (2026-10-08, `internal/ssa/verify_rc.go`, found by
`TestVerifyRCMutations`)

Symptom: deleting the `release` of the old accumulator in
`acc = acc + "${i}"` inside a loop still verified. The loop header's
block parameter `v352` held the accumulator, and the mutant leaked the
previous iteration's string every time around.

Cause: the verifier tracks counts per leaf key, and a block parameter is
the same key on every iteration. On each edge it set the parameter's
count to 1, overwriting the 1 left by the leaked previous value, so the
states at the header matched and the leak was invisible.

Fix: on an edge, a target parameter's leaves must have count 0 before
they are set to 1 (otherwise "the previous vN is still held ... a leak
around a loop"); the same check applies when an instruction defines a
root again. The mutation test (delete each inserted retain and release
in turn; the verifier must reject every mutant) is what found it, and
also found that a release placed before `unreachable` is not checkable
(paths that stop are exempt), so InsertRC no longer emits edge code
into blocks that end in `unreachable` or a fault.
