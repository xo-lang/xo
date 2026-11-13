# Timing under heavy.sh skips the background std build and its warm step

Status: Observed (Xo Lang 2, 2026-11-13)

Symptom: release-vs-go's armcompute row showed xo's first edit after a cold build at 471 ms against
Go's 304 (fe 0/57 bodies, chk 0/9 modules, functions 0/450 reused), as if a fresh install's first
edit paid a full fill of the separate build's caches.

Cause: heavy.sh jobs run with XO_SEPCOMP_BG=0 (the lead's scripts set it so no detached child
outlives a job), so the cold build started no background std build. The script then built std's
variants by hand with `xo dev stdcache`, which builds the stores only: the background child's warm
step (XO_STD_BG_WARM_OUT, stdcache.xo), which reruns the user's build to fill the front end, module
check, function and part caches, never ran. A user's install does run it: with XO_SEPCOMP_BG unset
the fill ended 29 s after the cold build and the first edit took 233 ms (fe 49/57, chk 8/9).

Fix: a timing that stands for a user's sequence after a cold build either unsets XO_SEPCOMP_BG and
waits for the background build (its lock, `<cache>/std/.bg.lock`, free and both stores present), or
builds the variants before the cold build. Never quote an edit measured after a hand-built
`xo dev stdcache` as a fresh install's.
