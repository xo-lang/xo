# A repeated edit hits the back-end cache in a timing run

Status: Observed (2026-10-14, decision 0220 addendum stage A).

**Symptom**: a rebuild of selfhost/xo after a one-constant edit reported
`cache objects 43/43 reused` (nothing assembled), yet the binary differed
from the unedited build's. It looked like the cache key missed the edit.

**Cause**: the edit (`MAX_ROUNDS = 25` to 26) had been built once before,
by an earlier measurement run. The object cache is keyed by each part's
assembly text and the assembler (cc's identity, flags, and source path;
not xo's identity on the cc path, since cc's output does not depend on
it), so the edited part's object was already in <user cache>/xo/backend
and was reused correctly. A cold build directory (XO_BUILD_DIR) showed
42/43 reused and the same binary as with XO_BACKEND_CACHE=off.

**Fix**: a timing run of an edit uses an edit not built before (a
constant derived from the clock, a fresh string), or a fresh
XO_BACKEND_CACHE_DIR; compare binaries with the same `-o` base name (the
macOS linker names the ad hoc signature after the file). To prove a
cached build equals a fresh one, build the same source with
XO_BACKEND_CACHE=off and compare.
