# A front end cache entry passed its checks and then did not replay

Status: Observed (2026-10-20), open: the root cause fix is in progress

Symptom: `xo build ./compiler/xo` (and scripts/bootstrap-xo.sh at s1)
failed with "the front end cache's entry for ssa.Builder.unbox_type does
not replay (XO_FE_CACHE=off builds without it)", against the shared
user cache, after building a branch that changed only
compiler/cli/servers. With XO_FE_CACHE=off it built.

Cause (suspected, decision 0271 step 1a's entries): an entry's commit
checks the types the body looked up by name ("t NAME" reads), but a type
the body made records its fields', variants', elements' and result's
types by name without logging them as reads, and so does a type the
body only reached structurally (a field's type, an operand's). An entry
recorded in a build where an earlier body had made such a type passes
the checks in a build where it does not exist yet, and the commit finds
the name missing after it has started changing the module. Sharing
entries more widely (std bodies across programs, 0271 step 1b; entries
across xo builds of one code generation key, 0275) makes it likelier.

Fix: the driver no longer fails the build: an entry whose commit fails
is dropped with the half committed build, the program is built again
without it (a warning names the entry), the body built fresh replaces
the entry, and the pack is rewritten without the old one (compiler/xo
bcache.xo fe_cached_build, ssa FeCache.bad). The warning names the xo
that recorded the entry (entries hold it since FE_CACHE_VERSION v8) and
the current one: a different recorder with the same cache id points at
a keying hole (decision 0275), the same one at the entry itself. The
entry is kept as $XO_FE_KEEP_DIR/KEY, else <cache>/fe-bad/KEY, for a
look after the cache moved on. The root cause fix logs
every type name an entry needs as a read, so such an entry misses
before the commit starts.

Observed again 2026-10-26, with the cause found (Xo Lang 2): "the front end
cache entry ... (closure metrics.series_of.fn3, recorded by xo X) did not
replay in xo X", the same xo recording and replaying. The kept entry
replayed alone in a private cache reproduced it, and the dropped build's
counts (XO_FE_STATS=1 now prints them as `fe dropped ...`) said
`miss.absent`: the closure had made the Map helper
`xo.maph.logget[Map[Str, metrics.Series]]` whole where it was recorded (a
program with one histogram), and in a program where another body made the
helper first (internal/llvm golden std_default_call) the entry's "absent"
check refused it. A body's entry that misses is built from source; a
closure's cannot be (its job's data is not kept), so the build was dropped
and made again without the cache. Fix: a Map type's helpers (xo.map.*,
xo.mlog.*, xo.maph.*[T]) are name-determined, as xo.const[] and
xo.droplist[] were (fe_const_fn): thunk_func takes one already there by
its name, so the commit keeps it. The replayed build's __text equals a
fresh build's. Test: internal/arm64 TestFrontCacheClosureMadeMapHelper.
The type-read hypothesis above did not apply to this case.
