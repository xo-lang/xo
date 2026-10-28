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
