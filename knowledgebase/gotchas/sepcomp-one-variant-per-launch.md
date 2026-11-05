# Separate compilation stayed off for the first ~15 edit builds

Status: Observed (2026-11-05, s3 and dynamodb program, linux/arm64); fixed 2026-11-05

Symptom: after a cold `xo build` with a fresh cache, one-line edits took
about 1 s and 650 MB each (`go build`: 0.26 s), and only after about 15
edits dropped to 0.30 s and 393 MB. A 90 s pause after the cold build
did not help. Phase times showed every edit building the whole program
(`cache cln 0/898`, `std variant: miss, a background build is running`).

Cause: a build compiles separately only when both of std's concurrency
variants are in the cache (xo/sepenv.xo sepcomp_default peeks conc=0,
then conc=1). On a miss, std_variant_peek started a background build of
only the variant that missed; the other one was started by the next
build to miss it, after the first finished. So the second variant waited
for an edit, and an edit loop stayed whole program until both were done.
Releases do not ship the stores by default (XO_RELEASE_STDSTORES=0), so
every user's first session hit this.

Fix: the background child builds the variant that missed, then the
other one (stdcache.xo std_variant_peek; a cached one is reused). To
see the state: `XO_CACHE_STATS=1 xo build --phase-times` prints
`std variant: miss, ...` or `cache std KEY conc=... reused` per variant.
