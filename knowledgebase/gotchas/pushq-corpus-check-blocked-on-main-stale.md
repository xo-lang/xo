# pushq's corpus check refused every push while main's corpora were stale

Observed 2026-11-16 (Xo Lang 2's #140, reported by the lead).

Symptom: after install.sh put #140's pushq in place, every gated push
whose rebase brought in a corpus change was refused with "stdcorpus
-check ... failed", whatever the push changed, until Lang 3's
regeneration (953150467) made main's corpora current again.

Cause: pushq ran `stdcorpus -check` over the corpora of the push's own
diff and of the commits its rebase brought in, and refused on any stale
corpus. A corpus already stale on main (another push's, before the rule)
made every later push fail the same check.

Fix (2026-11-16): pushq refuses only the stale corpora the push is to
blame for (gatelib.sh gate_corpus_blame): a module its own diff touches,
or, for a generator change, a corpus that the same check on origin/main
does not already list. Corpora stale on main are reported as a note and
the push goes on; their owner regenerates. Tests: gatesteps_test.sh
"corpus blame". Lesson for any check pushq adds: a rule that runs over
what the rebase brings in must tell a stale main from the push's own
breakage, or one red commit on main stops everyone.
