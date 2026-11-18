# A partial fuzz shard replaced a passing fuzz.md

Status: Observed and fixed 2026-11-18 (task #152).

**Symptom.** amqptest testing fell from Proven to Early (xw alert 44863)
after 08a8eca41 pushed std/evidence/amqptest/fuzz.md: the file that had
held a passing 1,000,000-case run (cbbda252f) now held a 385,000-case
`result: partial` shard.

**Cause.** `stdproven fuzz` wrote its result to fuzz.md whatever the file
held. A run capped with `-minutes N` that ends before its case count is
partial, and it overwrote a pass that still counted (decision 0616: a pass
of another source stays fresh for 90 days and until the next vX.Y.0 tag).
stdcov reads only fuzz.md, so the module lost its fuzz evidence.

**Fix.** While fuzz.md says pass, `stdproven fuzz` writes a partial to
std/evidence/M/fuzz-partial.md (stdcov ignores it) and resumes the next
shard from it; the run's own pass writes fuzz.md and removes
fuzz-partial.md. The SP_SHARD_MIN loop in stdproven-offload.sh commits
whichever file changed and stops only when fuzz.md changed to a pass;
stdproven-remote.sh and sp-par.sh no longer send a file the run removed.
Tests: TestFuzzPartialKeepsPass (scripts/stdproven) and the "beside a
pass" cases of scripts/lead/stdproven_shard_test.sh.

**Avoid.** Before pushing evidence, check `git diff origin/main --
std/evidence` for a pass turned into a partial; never push a
fuzz-partial.md as evidence (it holds progress, not a result).
