# A build with std's store and one without it make different code

Status: Observed (xo-lang-1, 2026-11-17, task #298)

Symptom: the same xo built compiler/xo to 37.00 MB of __text one hour and 37.96 MB the next, with no
source change; `XO_BACKEND_CACHE=off` always gave the larger size. An inliner change first measured
+2.6% to +3.6% of xo's code; on like pipelines it was +0.02%.

Cause: decision 0388's separate compilation. When the backend cache holds std's store for the xo's
identity, target and variant (std/<key>.store, built by `xo dev stdcache` or a background build after a
miss), the program links std as an object: no std code is inlined into it. A miss (the store pruned,
a new xo identity, XO_SEPCOMP_BG=0) or `XO_BACKEND_CACHE=off` builds the whole program, std inlined.
`--phase-times` says which: "cache std KEY conc=... reused" against "std variant: miss, the whole
program is built". Whether a miss should build std first, so output never depends on cache state,
is with the owner (task #298, lead decision #111).

Fix: compare sizes and instruction counts on whole-program builds, what a release ships (owner,
2026-11-17, lead decision #115; both pipelines stay): `XO_SEPCOMP=0` or `--release`. The tools that
report those numbers set it themselves since 2026-11-17: `xo dev size`, scripts/identity, ab-host's
program builds (scripts/lead/ab-host-remote.sh) and bench/perf/http/callgrind-docker.sh. A hand run
sets `XO_SEPCOMP=0`; `--phase-times` then says "std variant: off (...), the whole program is built"
(or "not used (--release)"; cmd/xo TestStdVariantLineWholeProgram).
