# A cache key hashed std's modules in load order

Status: Observed (2026-11-05, Xo Lang 1, the s3 + dynamodb edit loop)

Symptom: after a cold build, the second edit still missed every front end
and module check cache entry (`cache fe 0/198 bodies`, `cache chk 0/20
modules`, 0.58 s), and only the third edit hit (0.30 s). Every program
module's own interface hash was the same in both builds.

Cause: `Checker.iface_closures` (compiler/check/modcache.xo) folded std's
modules into `std_sum` in `c.mods` order, the order the build loaded
them. A build that loads std from its record (stdx, the first edit after
the background store writes it) loads them in another order than a build
that parses them, so the sum and every closure built on it changed.

Fix: the lines are sorted by module path before hashing. Rule for any
cache key: hash a sorted list, never a list in load, map or index order.
TestSepcompStdxKeepsFrontCache (internal/arm64) checks it.
