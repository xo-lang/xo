# A fold on a program-wide fact needs that fact in the front end's cache key

Status: Observed and fixed (2026-10-29)

Symptom: internal/arm64 TestFrontCacheAcrossPrograms, "tls_coalesced_finish: the build
through the shared cache differs from the build with XO_BACKEND_CACHE=off"; the 0.34 cut's
full tier at 97a348e0b.

Cause: compiler/ssa call.xo folds rt.sym_present("__tsan_...") to false unless the build
is --race, LLVM, or the program has a `use c` module (d5503c871). fe_settings
(compiler/xo bcache.xo) keyed race and llvm but not `use c`, so a std body compiled for
use_c_add (weak check kept) was replayed into another program, whose fresh compile folds it.

Fix: fe_settings adds usec=. Any fold or lowering that reads a program-wide fact (a
module's presence, a flag, the backend) adds that fact to fe_settings in the same change,
and its gate runs TestFrontCacheAcrossPrograms.
