# The module cache cannot be removed with `rm -rf` or `t.TempDir`

Status: Anticipated (2026-10-09)

Symptom: `rm -rf ~/Library/Caches/xo/mod` reports "Permission denied",
and a Go test that points `XO_MODCACHE` at `t.TempDir()` fails in cleanup
with "TempDir RemoveAll cleanup: ... permission denied".

Cause: `internal/mod` makes every extracted module version read only
(files 0444, directories 0555) so nothing edits a dependency in place
(decision 0086, as Go's module cache). Removing an entry needs write
permission on its directory.

Fix: `xo mod clean` (or `mod.CleanCache`) restores write permission
first. Tests use `modtest.Setup`, which registers a cleanup that runs
`modtest.MakeWritable` on the cache before `t.TempDir` removes it
(cleanups run last registered first). By hand: `chmod -R u+w <cache>`.
