# A front end cache entry passed its checks and then did not replay

Status: Observed (2026-10-20, 2026-10-26, 2026-11-04 twice); each cause found is fixed, the driver still rebuilds without an entry that fails

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

Observed again 2026-11-04, cause found (Xo Lang 2, reported by aws-19):
during a gate in the aws repository, xo b29cad7a0 printed "the front end
cache entry ... (sigv4.sign_header, transport.attempt_once, ...) recorded
by xo b29cad7a0 ... did not replay in xo b29cad7a0", then rebuilt
without the entries. It reproduced in a fresh private cache with
`xo test ./config` then `xo test ./credentials/stscreds` (5 entries).
The warning now says which part of the commit failed; here it was "its
body did not decode: no github.com/xo-lang/aws/sigv4.Options here". A
user type two loaded modules both declare is interned under its module
path (ssa ambiguous_type_names): the config program loads
config.Options beside sigv4.Options, so sigv4's bodies named the type
by its path, and the stscreds program, which loads only sigv4, names it
`Options`. The entry key covered the bodies' modules but not the
program's type naming. Fix: the own flags in every entry's key
(fe_own_flags) carry a digest of the program's shared type names
(names_key, "" when there are none), so the two programs read different
entries. Test: internal/arm64 TestFrontCacheTypeNaming. The read-logging
idea above would only have turned these into misses: a type a replayed
`declare` makes is named the same way and is not a read.

Observed a second time 2026-11-04, in another form (Xo Lang 1, reported
by aws-19). The entry replayed, and the backend then failed:
"protocol/restjson/restjson.xo:31:7: in restjson.error_code (inlined into
deserialize_greeting_with_errors_error): arm64 backend: runtime call
restjson.sanitize_code". From an empty cache it is deterministic:
`xo test ./internal/protocoltest/jsonrpc10`, then
`xo test ./internal/protocoltest/restjson`. XO_FE_CACHE=off passes, and
XO_BACKEND_CACHE=verify reports "5 function bodies' SSA differs, the
first ... restjson.error_code".

Cause: the restjson test program loads two modules named restjson (the
test package and protocol/restjson). A program module whose default name
another module has gets its functions named by its path
(path_named_mods, mod_prefix). So in that program, error_code's call is
to `github.com/xo-lang/aws/protocol/restjson.sanitize_code`, while the
entry recorded in jsonrpc10's program calls `restjson.sanitize_code`.
names_key covered shared type names, not shared module names.

Fix: names_key also digests the paths of the path-named modules, and
FE_CACHE_VERSION is v15. Test: internal/arm64
TestFrontCacheRootAndLibrary (a module t/a beside the library a). It
fails on the xo before the fix, with "runtime call
ferootlib/a.sanitize_code". This one was loud, a compile error. A body
whose stale name happened to exist in the program would have called the
wrong function.
