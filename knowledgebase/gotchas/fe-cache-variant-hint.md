# The front end cache read the wrong variant (predicted, then shared hints)

Status: Observed (2026-10-20)

Symptom: with decision 0271 step 1b part 2, http_tls hit 0 of 2,702
bodies where reading both variants hit 2,287. After the fix, a second
round of builds in one cache hit 0 on json_text and http_server while
the other two programs hit.

Cause: entries are stored per builder own flags (conc, budgets, scan,
rt). The first try predicted conc from the scans of the user's modules
(tasks), but a program needs concurrency also when std bodies do (net
waits, scope blocks: set_flag(1) while building), and then the build is
made again with unwinding checks and reads the other variant. The hint
that replaced the prediction (the own flags the last build ended with)
was keyed by the root modules' path and dir, and a root's dir is as
named on the command line, "." for every program built in its own
directory: all programs shared one hint, each overwriting the others'.

Fix: the hint is keyed by the root modules' absolute directories
(bcache.xo fe_program_id); a build with no hint reads both common
variants. A wrong hint only costs misses and a second build: the output
is the cache-off build's.
