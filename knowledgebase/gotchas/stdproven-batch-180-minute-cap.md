# A stdproven fuzz batch ran 3 hours and returned no evidence

Status: Observed (2026-11-16)

## Symptom

`stdproven-offload.sh WT NAME -- batch fuzz ... adler32 + fuzz ... crc64 +
fuzz ... idna` on host 2 passed adler32 and crc64, reached idna's seed 182
of 200, and ended `OFFLOAD-RC=124` ("the job ran past OFF_MAX_MIN=180
minutes and was stopped"). No std/evidence file came back, not even the
two modules that had passed (task #98).

## Cause

stdproven-offload.sh runs outside the lane (OFF_NO_LANE=1) and leaves
OFF_MAX_MIN to offload.sh, whose default is 180 minutes, while a fuzz run
by decision 0616 goes to 1M cases per backend or 8 hours: a 1M idna run
alone takes about 2.5 hours on host 2. The evidence files are cut from
the job's output (STDPROVEN-FILE blocks), which a batch prints after its
last run, so a stopped batch prints none.

## Fix

Run one module per stdproven-offload.sh call and give each its budget:
`OFF_MAX_MIN=540` for a fuzz, about 150 for race plus stress (three race
runs, a 10 minute stress under --race, the 30 minute stress and its
check). ~/xo-lead/notes/xw-1/98/chain.sh does this, a chain per kind, to
stay within the cap of two arm64 jobs per agent.

Since 2026-11-18 (task #331) a batch or `par N` run with OFF_MAX_MIN
unset gets SP_MOD_MIN (140) minutes a module, a fuzz group's -minutes M
counting at least M + 40, so a several-module batch no longer hits 180;
set OFF_MAX_MIN for a batch whose runs need more.
