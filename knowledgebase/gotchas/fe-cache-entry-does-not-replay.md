# A front end cache entry passed its checks and then did not replay

Status: Observed (2026-10-20, 2026-10-26, 2026-11-04 twice, 2026-11-11, 2026-11-15, 2026-11-20); each cause found is fixed, the driver still rebuilds without an entry that fails

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

Observed again 2026-11-11, cause found (Xo Lang 2, in the 0.48.1 release
verify: cmd/xo TestNoGCTests failed on test_fakenet_raw/llvm with the
same output as the Go backend plus this warning). "the front end cache
entry ... (closure tls.vconn_wrap.fn4 ...: its entry missed) did not
replay". Reproduced in a private cache with internal/gen's goldens
test_fakenet_tls then test_smtp (closure fn8), on arm64 and LLVM. The
warning now names the check that missed ("its entry missed: read type
Result[x509.Certificate, x509.CertErr]"). The closure read that type, and
then x509.CertErr, which earlier bodies made in the program that recorded
the entry and which nothing had made yet in test_smtp. Types are made at
their first use, so an entry's read of a type another body made is an
accident of the recording program's order, and a closure cannot fall
back to its source. Fix: an entry records each struct, enum, list, box,
tuple and function type it read that existed before its body as made
(events 1 and 2, as Module.named and intern would log them, components
first), with its shape; the replay makes one that is absent and keeps
one that is there. Only other kinds (an interface's handle, a sized
integer) stay reads. Test: internal/arm64 TestFrontCacheClosureReadMadeType.
A first version walked a recursive type's shape forever (33 GB in 36 s,
stopped by heavy.sh): the walk marks a type on entry.


Observed 2026-11-15 (TestWarmBuildSequence on main at 4ccafc37c, the
Mac): internal/llvm golden metrics_empty after metrics_child warned that
the closures metrics.cells_of.fn5 and series_of.fn7 and fn8 did not
replay ("its entry missed: read xo.maph.logget[Map[Str, Int]]"). Since
572a47416, std/metrics' Store keeps a Map[Str, Int], and the closures'
`st.at.get(key)` calls that Map helper. metrics_child made it in an
earlier body, so the entry listed it as a function read; metrics_empty
had none yet. This is the same accident as the types above, for
functions: a Map type's helpers (xo.map.*, xo.mlog.*, xo.maph.*[T]) are
made by thunk_func at their first use and their names determine them.
Fix: an entry records each such helper it read that existed before its
body as made whole (event 3, its body encoded), with the helpers that
body calls; their types join the types read, and the other functions
they call stay reads. The replay keeps one already there
(fe_const_fn). Test: internal/arm64 TestFrontCacheClosureReadMapHelper
(the pair: 3 non-replays with main's xo, 0 here).

Observed 2026-11-20 (TestWarmBuildSequence on main at ea1bd76a5, the
modules set, task #557; also in the runs of 2026-11-15 16:05 and
2026-11-17 19:26): "closure grpc.serve_call.fn52 ... its entry missed:
read xo.droplist[Map[Str, Str]]" (also fn37, fn62 and xo.droplist[Str])
building grpchealth_calls after grpc_health_watch. The same accident
once more, for a list type's drop function (drop_list's xo.droplist[T],
an empty function taken by its name): grpc_health_watch made it in an
earlier body, so the closures' entries listed it as a function read, and
grpchealth_calls had none yet. The output was right both times: the
build was made again without the entry, and equals the build with every
cache off. Fix: fe_named_fn (a Map helper or xo.droplist[T]) replaces
fe_map_helper where an entry records a function it read as made whole
(fidx's event 3, fe_entry); FE_CACHE_VERSION v21. Test: internal/arm64
TestFrontCacheClosureReadDropList (the pair, compared with the cold
build under one file name: a Mach-O binary holds its output file's
name, so builds to `warm` and `cold` differ in those 4 bytes). A
constant's function (xo.const[M.N]) is not of this kind: const_value
finds it in the builder's constant table (an event 6 the commit
renumbers), not by a function read.

Observed 2026-11-20 twice more (tasks #573, #574; TestWarmBuildSequence
over all 407 goldens, XO_WARMSEQ=all XO_WARMSEQ_SEED=569001, on the
0.55.0 candidate 2bd6ca2fa; the gate and modules sets do not hold these
pairs). Both are closures again, and both builds were right after the
second build:
- "closure http.Server.serve.fn16 ... read type Chan[()]" building
  websocket after websocket_stop. fe_seal's owner check (2026-11-14)
  lists the types a closure read from before its owner's body began; this
  one was made after it began, by another body. A handle type (Chan[T],
  Task[T, E]) is its name and type arguments, so name_made now takes
  Kind.Handle and the entry records it as made, as a list or function
  type. Test: internal/arm64 TestFrontCacheClosureReadHandle.
- "closure tls.server_vpeer.fn30 ... generic NNNN:prelude.list_map[
  List[Bytes], List[x509.Certificate], Never]" building tls_errors after
  tls_ekm. A body that makes a generic instance records no entry, so an
  instance a closure's entry reads was another body's; the sealed entry
  of the owner now lists its closures' generic reads and its replay
  misses on one that is not there, before any change, and the owner is
  built from source. The cost: where the instance is made between the
  owner's body and its closure's job, the owner's entry never replays
  (a body built from source each time, no warning). Test: internal/arm64
  TestFrontCacheClosureReadGeneric. FE_CACHE_VERSION v22.
The rule both follow: a closure's entry cannot miss, so everything it
reads that its owner's entry does not make is either made by the entry
(name-determined) or checked by the owner first.

