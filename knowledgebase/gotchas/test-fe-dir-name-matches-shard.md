# A test that removes every `fe` directory also removes a cache shard

Status: Observed (2026-11-12)

Symptom: TestModuleCheckAutoGeneric failed with "no fallback with the
front end cache's entries removed" on a change to compiler/check
(decision 0620's ObjFlags), and passed at the change's parent. The change
did not alter the fallback: by hand, moving only `packs/*/fe` aside, both
xo builds fell back.

Cause: internal/arm64's removeFeEntries removed every directory named
`fe` under the back-end cache. The ast and chk stores shard their entries
by the key's first two hex digits, so `ast/fe` and `chk/fe` are shards.
Any change to what the module check entry's key hashes moves the entry to
another shard; when lib's entry landed in `chk/fe`, the test deleted it,
the build had no entry to skip lib with, and nothing fell back.

Fix: removeFeEntries removes only a pack's `fe` directory
(`packs/<shard>/<key>/fe`). In general, a test that deletes cache entries
by directory name must anchor the path, since two hex digits spell
names like `fe`, `ad`, `be`.
