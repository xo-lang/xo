# A bisect from a GOOD that already fails names the commit after GOOD

Status: Observed (2026-10-22)

Symptom: Xo lang 4's first constant-time bisect with scripts/bisect.sh
tested bad at every step and named the commit right after GOOD as the
first bad commit. Separately, a bisect with GOOD and BAD adjacent
reported the BAD commit as good, then failed with "was both good and
bad".

Cause: `git bisect run` trusts the endpoints it is given: when the test
already fails at GOOD, every step is bad and the search converges on
GOOD's child. bisect.sh's work tree is a `git clone --no-checkout`; with
nothing left to bisect between adjacent GOOD and BAD, `git bisect start`
checks nothing out and `git bisect run` ran the step in an empty tree.

Fix: bisect.sh now runs the step at GOOD and BAD before bisecting and
stops with exit 3 when GOOD does not pass or BAD does not fail
(`--skip-verify` turns this off), and checks BAD out in its clone before
`git bisect start`. scripts/bisect_test.sh covers both. When bisecting by
hand, run the test at both endpoints first.
