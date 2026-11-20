# Output derived from cache keys differs with the cache off

Status: Observed (2026-10-28, lead tier 25)

## Symptom

internal/arm64 TestBackendCacheEditIdentity and
TestWindowsWarmEditEntryShims failed on windows/amd64: "the build through
the cache differs from the build with XO_BACKEND_CACHE=off".

## Cause

177e7c654 made the PDB's GUID (and the debug directory's time stamp,
both in the .exe) from the objects' back-end cache keys, falling back to
the image's hash when there were none. cached_macho_objects returns no
keys with the cache off, so an uncached build took the fallback and wrote
another GUID. The proving tests ran with the cache on only.

## Fix

Anything in the output computed from cache keys must get the same keys
with the cache off: compiler/xo macho_object_keys computes them either way
(the windows link's call site uses it when the cache gave none). Prove
such a change with XO_BACKEND_CACHE=off exported as well as on, and with
TestBackendCacheEditIdentity.

## Again, 2026-10-29: two builds of one program, two GUIDs

The fix above kept the keys as the seed. cache_id's separate-compilation
component then changed (4c1c15e05: a facts file that is a cache entry by
its name, one given through an explicit XO_SEPCOMP_FACTS directory by
its stamp), and a default build and an explicit separate build of the
same program got different object keys and so different PDB GUIDs (Lang
2's Windows run 40820335875, with the build-id difference of
gotchas/std-record-build-id.md making 130 bytes). The seed is each
part's content now (main.xo: part_keys over the assembly texts with a
fixed prefix, no cache_id), and internal/arm64 TestPDBGUIDFromContent
builds windows/amd64 on the Mac three ways (the cache's facts entries, a
facts directory of copies, the cache off) and requires one binary. The
rule, sharpened: nothing in the output comes from a cache key, keyed or
not; hash the content the key stands for.


Observed again 2026-11-20 (task #462): the float constant labels of
3a80fdd33 (isel.xo fconst_label) took `lbl_prefix()` during isel, before
emit numbers the function, so the plain text named them `f23_fc0` while
the canonical text's token was placed as `f24_fc0`. Only windows/amd64
showed it, since its PDB GUID hashes each part's text. Fix: isel names
a label with the number emit will give (`g.nfunc + 1`), or the token.
Found by diffing the part texts the PDB seed hashes, cache on against
off.
