# A heavy.sh memory stop can exit 137, not 143

Observed 2026-11-20 (tier 918, task #560).

**Symptom.** Tier 918's concurrent Mac steps (arm64 llvm cmd corpora-all
corpuscheck) were stopped at 65,644 MB over `HEAVY_MAX_GB=64` after 25
minutes, and the tier went on with no result for any of them: the
one-at-a-time rerun that tier.sh runs after a memory stop never started.
`tier918.log`: "Mac steps concurrent (...) in 1537 s, exit 137".

**Cause.** heavy.sh (heavyd) stops a job with TERM to its process tree and,
after a grace (10 s for memory), KILL to whatever still runs. A tree of
many concurrent `go test` processes outlives the grace, so the job exits
137 (KILL), not 143 (TERM). tier.sh checked `[ "$s" = 143 ]` only.

**Fix.** scripts/lead/tier-mac.sh's `mac_stopped` takes 143 or 137;
tier-mac_test.sh covers both with a stub heavy.sh. A script that reacts to a
heavy.sh stop reads either code (or the log's `over HEAVY_MAX_GB` line),
never 143 alone.
