# Tokens in a canonical function text hid its labels from pair_mem

Status: Observed (2026-10-14, decision 0220 addendum stage B).

**Symptom**: compiler/xo built with the per-function cache on (cold or
warm) differed from the build with XO_BACKEND_CACHE=off, though verify
found every cached text equal to a fresh one. The part assembly showed
`ldr x11, [sp, #32]`, a block label, `ldr x15, [sp, #40]` where the
plain generation wrote one `ldp x11, x15, [sp, #32]`.

**Cause**: the emitter's post passes run on the canonical text, whose
labels carry a token (`L\x01F\x01_b21` for `Lp3ab_f51_b21`). pair_mem
pairs two loads across a label nothing refers to, and finds labels and
references by scanning `L` and then letters, digits, and `_`; the token's
byte 1 ended the scan, so no label in a canonical text was recognized
and none was paired across. Verify compares canonical with canonical,
so it cannot see a difference between the canonical and the plain path.

**Fix**: lsr.xo is_label_byte accepts byte 1 (the token delimiter). Any
pass that reads label names out of a function's text must treat a token
as label characters. The test of the canonical path is the binary with
the cache off against on (stageb.sh in the results: off, cold, warm,
verify must be one file), not verify alone.
