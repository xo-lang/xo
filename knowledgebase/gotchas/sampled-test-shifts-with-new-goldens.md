# Adding goldens made TestModuleCheckAuto fail on a program it had skipped

Status: Observed (2026-11-05, 881677d3c then 61ba87d38: the hourly main check, bisected by the lead)

Symptom: TestModuleCheckAuto (internal/arm64, not -short) failed 3 of 3
from a commit that only added four goldens and arm64 backend fixes:
"iface_method_cross_module: the warm build fell back". The bisect named
that commit; its change could not cause a module check cache fallback.

Cause: the test checks every 8th golden directory (fcache_test.go,
`i%8 == 0`) unless XO_SELFHOST_TIER=full. New goldens sorted before it
shift which programs are sampled; iface_method_cross_module (decision
0496 (c), added 2026-11-04) entered the sample and had fallen back on
every warm build since it was added (its non-`pub` Display and Debug
methods' inferred effects are read from main; check modcache.xo).

Fix: the program's fallback was fixed (cross_reads: such methods are
eager in their module's cache entry). To tell a sample shift from a
regression: run the failing subtest at the bisect's good commit with
XO_SELFHOST_TIER=full (`-run 'TestModuleCheckAuto$/programs/NAME$'`);
if it fails there too, the program was failing before.

Since 2026-11-05 the lead's 8-hourly full tier (scripts/lead/tier.sh, step `lead-tierN-mca`) runs TestModuleCheckAuto with XO_SELFHOST_TIER=full over every golden program, so a fallback in an unsampled program shows within one tier instead of when a new golden shifts the sample.
