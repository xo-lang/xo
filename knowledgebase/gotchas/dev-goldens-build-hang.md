# A dev golden's build times out under load, though it takes seconds alone

Status: Observed, open (2026-10-19 and 20; the cause is not known yet).

## Symptom

TestDevGoldensArm64 (`xo dev goldens`) failed three times on a build
timeout: crypto_prims twice at 120 s (2026-10-19, load 49 to 71) and
fs_cwd_path at 240 s (2026-10-20, load 17 to 22). Each builds in 3 to 4 s
alone, and reruns passed. A small program taking 240 s at load 20 points
to a rare hang rather than to load.

## Cause

Unknown. Candidates (lead, 2026-10-20): the parallel emit's largest first
scheduler (19545764), the part cache's pack locking, the front end
cache's commit.

## Fix

Not yet; the next one will say where it waits: the build-each worker of
`xo dev goldens` stack samples a build before a timeout kills it (darwin
`sample PID 5`, Linux gdb's backtraces when gdb is installed), saves the
sample as OUT/hang-NAME.txt, and adds the top of its call graph to the
failure. Follow-up, parked: builds of the Go tests (internal/xotool) have
no timeout of their own and end in go test's -timeout panic, which kills
the children unsampled; a hook sampling the live guarded children shortly
before the test deadline would cover them.
