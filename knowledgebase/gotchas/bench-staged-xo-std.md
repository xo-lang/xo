# A staged xo under `xo bench --ab` ignored the caller's XO_STD

Status: Observed 2026-11-21 (task #645), fixed in compiler/cli/bench.

## Symptom

release-vs-go's std test "edit" rows on linux/arm64 host 1 for 0.55.0 read
12.86 ms (base64) and 13.36 ms (json) for `xo test --std-module` after
appending a function, with B's max RSS equal to A's (116,698 against
116,700 KB). Run directly, or under bench on the Mac, the same edit
rebuilds in 150 to 250 ms.

## Cause

On linux, `xo bench --ab` copies both programs to a tmpfs stage (task
#347). So that a staged xo still finds its std, `with_std` added
`XO_STD=<std beside the original binary>` to that side's environment
unless `--env-a`/`--env-b` set XO_STD. An XO_STD inherited from bench's own
environment did not count, so the side's XO_STD (the edited copy) was
replaced by the release std: every round tested the unedited std, a run
cache hit. The Mac does not stage, so its rows were right. The probe that
showed it: B as a wrapper script (not an xo, so nothing added) rebuilt in
139 ms at 170 MB.

## Fix

`with_std` leaves the environment alone when bench's own XO_STD is set.
scripts/release-vs-go.sh also passes `--env-b=XO_STD=...` so a run with an
older xo measures the edited copy. Check a bench row that is far faster
than a direct run of the same command by running B through a wrapper.
