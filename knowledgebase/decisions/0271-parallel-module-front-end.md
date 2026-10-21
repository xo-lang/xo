# 0271: Whole-program compilation with a parallel module front end

Status: Proposed (xo-fa, 2026-10-19; owner-approved direction, lead
2026-10-19). Supersedes stage 3 of decision 0220 (per-module code
generation with a link-time merge); stages 1 and 2 of 0220 stand.

## Context

Xo compiles the whole program at once: the checker sees every module,
the SSA builder starts from main and builds what it reaches, and the
passes after it (borrow inference, decision 0186; inlining, 0184;
monomorphization; dead code elimination) read every function. Decision
0220 made the back end parallel (stage 1) and the front end cached per
body and per module (stage 2, its addendum). What is left serial is the
front end itself: the checker's bodies and the SSA build.

Measured on xo (cold build: empty back-end cache, arm64, the Mac's 12
cores; three runs at main 7311db65, 2026-10-19, load average 39 to 64,
so the wall times are inflated and the CPU times are the firm figures):

| Phase | CPU s | Cores used | Parallel today |
|---|---|---|---|
| check | 1.1 to 1.4 | 0.6 to 0.9 | no |
| SSA build (fe.build) | 1.6 to 1.8 | 0.9 to 1.1 | no |
| SSA passes (prune_unwind, landings, cleanup) | 0.6 to 0.8 | about 1 | no |
| insert_rc: borrows, keys, plan | 4.8 to 5.1 | 2 to 5 | yes |
| insert_rc.apply | 57.6 to 61.6 | 2.2 to 5.2 | yes |
| insert_rc.sums | 1.4 to 1.7 | 0.4 to 1.0 | partly |
| inline.apply | 4.3 to 6.1 | 0.2 to 1.2 | no |
| inline.verify | 10.3 to 11.2 | 3.2 to 3.9 | yes |
| emit | 52.2 to 57.0 | 3.0 to 3.5 | yes |
| assemble | 6.2 to 6.4 | 2.4 to 4.4 | yes |
| total | 140 to 142 user, 12 to 16 sys | | |

Wall: 57.1 to 76.9 s. The front end (check plus SSA build) is 2.9 to
3.2 s of CPU, about 2% of a cold build, all of it on one core: the
ceiling of making it parallel is about 2.7 s of a cold build's wall on
12 cores. After an edit (decision 0220's addendum, about 8.5 s) the
check and the SSA build are about 2.5 s, most of what a warm build does
before the cached passes.

The state the two phases keep (compiler/, 2026-10-19):

- The checker runs whole-program phases over the modules in import
  order (check/dump.xo run): declare, export names, types, signatures,
  derives, then the bodies module by module, then the whole-program
  checks (solve_effects, a fixed point over every function's effects;
  report_discards; check_realtime; flow_bodies; unused; never results;
  XO0402). Only the bodies phase is per module.
- The bodies phase appends to tables shared by all modules and numbered
  in order: objects (locals), types (every fresh variable and
  substitution), effect sets, the names and types lists, diagnostics,
  and ordered lists the later passes walk (bodies, obligations,
  discards, never results). Keyed tables (by file and offset) hold those
  numbers as values. Two writes cross modules: a constant's type is
  checked lazily on first use (const_type), and named types are interned
  on first use (named0).
- The SSA builder makes functions in one breadth-first order from main
  (build.xo run_queue): a function's index, an instance's name suffix
  and the module's state fingerprint (Module.fe_h) depend on that
  order. The front end cache replays a body only in the exact state it
  was recorded in.

## Decision

The compilation model is whole-program optimization, fine-grained
caches, and a parallel front end. Not separate compilation in the style
of Go.

Why not separate compilation (0220 stage 3): every pass that makes Xo's
code fast reads across modules. Borrow inference decides per callee
whether a parameter is borrowed; at a module boundary separate
compilation needs summaries and a conservative default (owned), which
costs reference count traffic on every cross-module call. Inlining
crosses modules (std's small functions into the program); export data
can carry bodies, but then an edit to one invalidates its importers
anyway. Monomorphization makes instances where they are used, so a
separate unit emits duplicates for the linker to fold. Dead code
elimination needs the whole call graph. And the caches of 0220 already
give the incremental cost separate compilation is for: an edit rebuilds
the bodies it changed and the parts whose code changed.

What is parallel:

1. Checker bodies. The signature phases stay serial and global; once
   they are done, the tables they made are read only. Each module's
   bodies are checked on a task against a local overlay: its own
   objects, types, effect sets, names, diagnostics and ordered lists,
   numbered from the global sizes. Constants' types are resolved in the
   serial phase (no lazy check across modules during bodies). Named
   types interned by a body go to the overlay. The overlays are merged
   in the serial module order: each local number is renumbered to what
   a serial check would have given it (a named type already interned by
   an earlier module maps to that one, the rest follow in order), and
   the keyed tables' values are rewritten. Bodies do not depend on
   other modules' bodies, so the dependency order is not needed for
   them; the scheduler starts the largest modules first.
2. SSA bodies. A body is built into a relocatable entry: the front end
   cache's entry, with every type and function it references or makes
   recorded by name (a type's interned name; a function's instance key,
   module path, receiver, name and type arguments), with no indexes and
   no state fingerprint. Workers build the bodies of the functions the
   build will reach, in parallel (the set is the previous build's list,
   as the parse prefetch does, or the checker's call graph cold). A
   serial commit then walks the breadth-first order as today and, for
   each body, interns its names into the module, which gives the
   indexes a serial build gives. A body with no entry (not prebuilt, or
   one the entry cannot express: closures, constants, thunks today) is
   built at its turn, serially. The same entry is the cache's: a hit
   from an earlier build, or from another program (a shipped std
   cache, Xo Lang 3's item), is a commit.
3. The whole-program passes stay whole-program: solve_effects, flow,
   borrow inference (already parallel in rounds, 0220 2f), inlining,
   dead code elimination and emission (parallel parts) see every
   function.

What is per module and what is global:

| State | Owner |
|---|---|
| modules, files, imports, signatures, declared types | global, serial, read only during bodies |
| a body's locals, fresh types, effect sets, diagnostics, ordered lists | per module overlay, merged in module order |
| constants' types | global, resolved before bodies |
| effects solution, flow, unused, never results | global pass after the merge |
| SSA function and type indexes, instance names, fe state | global, assigned by the serial commit |
| a body's SSA, the names it references | per body entry (relocatable) |
| borrow, inline and code keys | global passes; keys by name (Xo Lang 3's inline key) |

Determinism: a parallel build's binary is byte identical to the serial
one's (XO_FE_JOBS=1), checked by the arm64 and amd64 identity scripts
over the goldens and by the fixed point, which compiles xo with a
parallel xo. XO_BACKEND_CACHE=verify builds every body serially as well
and fails on an entry or overlay merge that differs. The number of
tasks never changes the output.

## Consequences

- Cold builds gain at most about 2.7 s (the front end's 3 s of serial
  CPU over 12 cores); edit builds gain where the edit invalidates many
  bodies (an interface edit), and every build gains the relocatable
  entries' reuse across programs. The cold build's cost is elsewhere:
  insert_rc.apply and emit are 110 to 119 s of its 141 s of CPU.
- The relocatable entry replaces the fe state fingerprint, so a body
  built in one program can replay in another; the std cache and the
  inline key by name build on it.
- The checker's overlay and merge are new code on the path every build
  takes; the serial mode stays for verify and for bisecting.

## Alternatives

- Separate compilation (0220 stage 3): above; rejected.
- Parallel checking with shared tables under locks: the numbers would
  depend on the schedule, so binaries would differ run to run; rejected.
- Parallel SSA build per module (each module's bodies built into its
  own module, linked by name): changes every index and the inline and
  borrow passes' order, so the output differs from a serial build;
  rejected for the commit step that keeps it.

## Order of work (lead, 2026-10-19)

Relocatable entries first (the base of the rest, and of Xo Lang 3's
std cache shipped across programs), then SSA bodies built in parallel
through them, then the checker overlay.

### Step 1a: entries by name, a serial commit (implemented 2026-10-20)

compiler/ssa fecache.xo, ir.xo, build.xo. An entry of the front end
cache (decision 0220's addendum) no longer holds the builder's state
(the fingerprint of every name so far and the sizes of about 20
tables) or any index:

- What the body found that was there before it: each type found by
  name (Module.fe_seen in the type constructors) and each function found
  by name (Builder.fidx); each generic instance it calls, by its
  instance key and the name the body calls it by.
- What it made, in order (Module.fe_ev): a type (kind, name, then its
  fields, variants, elements and result by name), an order entry, a
  whole function (a helper such as xo.hash[T], in the codec), a declare
  (the function's module, receiver and name). A declare pauses the
  recording: the commit runs it as the build does, so what it makes is
  its own.
- The body itself, in the codec (types and callees by name, as before).

The commit (fe_replay) checks first: every type and function found must
be in the module (else a build would make it where the body ran: a
miss, built instead), and every generic instance is found by its key;
a different name (`#n`) is patched into the body's calls. Then it
replays the events in order: a type made unless one of its name is
there, an order entry for a type made now, a helper made unless there,
a declare run; then the types made get their structure by name and the
helpers and the body are decoded. The module grows as a serial build at
that point grows it.

Not cached yet (each a skip reason of XO_FE_STATS, step 1b's
relocations): a body that reads a type id (iface.xo type_id), a spawn
site (conc.xo), a constant slot (show.xo const_once), or a thunk; one
that makes a closure, a new generic instance, or a contract wrapper.

Results on xo (main 8a48ac13 against this, interleaved, load 26 to 45):
binaries identical (identity 402 of 402, the fixed point, TestBackend
CacheEditIdentity and TestFrontCacheInterfaceEdits, whose new step edits
the first body built after main to make a type no other body has and
requires the bodies after it to hit, verify clean); hits on a warm build
8,233 of 8,483 (8,241 of 8,477 before: the new skip reasons consts 31,
sites 18, thunks 6), the same after a constant or statement edit within
8; cold SSA 3.07 s (2.9 s before; recording reads by index, not by
name, which first took 7.0 s); edit builds the same within noise. The
gain is the entries' independence: step 2 builds them in other
builders, and they can be reused across programs.

Fix (2026-10-20, step 2's gate found it): the builder also branches on
a function name being absent (map_remove gives its helper a result
suffix when another helper of the name exists; helper_name tries `#2`,
`#3`), and a function's name, unlike a type's, does not say what it
is. A body recorded where xo.map.remove[Map[Str, Str]] was free made
it for a Bytes? result; committed after another body had made the name
for a Str? result, it called that one (1a, 2e0f5dce to the fix). An
entry now holds the function names the body looked for and did not
find, and the helpers it made; the commit requires them absent (else a
miss). Verify compares an entry's leading part only: the body and what
it does to the builder itself (checks, variable counter, modules,
target), not what it found or made, which depend on the builder that
recorded it (FE_CACHE_VERSION v3). Test: TestFrontCacheAbsentHelper.

### Step 2: SSA bodies built in parallel (measured 2026-10-20, not landed)

Built as designed (branch fa-fe-par): before the build, tasks with fresh
builders record entries for the bodies the cache lacks (the target's
last build list, or cold, the checker's calls from main), and the serial
build commits them in its turn. Correct within a build once two more
leaks of builder state were closed (a cold xo with 8 jobs byte identical
to 1 job, verify clean, 5,054 of 8,495 bodies committed from tasks), but
the gain is about nothing: cold SSA 2.44 s to 2.36 s on the Mac at load
4, since the prebuild takes about 0.4 s and committing a body costs
nearly as much as building it; warm builds were 0.3 s slower (the
bodies that can never be cached were prebuilt each time). Not landed.
The ceiling is the commit, not the building: a faster commit (no codec
decode of a body until it is needed, as rc_cached's lazy shells) would
be the next lever, not more tasks. The leaks it found: function names
looked for and not found (fixed, d4d4b244), one-way flags (need_conc,
uses_net; fixed in the entry format separately), and the declared-from
module list (a delta relative to the recording builder, which only
differed between builders). TestFrontCacheAcrossPrograms builds the
goldens into one shared cache in several orders and compares each with a
cache-off build: the guard for entries that cross programs.
Fix 2 (2026-10-20): need_conc and uses_net are one-way flags a body
sets; an entry recorded by a builder that had one set already carried
nothing. A serial build is covered in practice (the commit's declares
mark the network's modules; the task scan sets need_conc first), but
bodies recorded elsewhere (step 2) lost the network's exports. Bodies
set them through set_flag, the entry carries the bits and the commit
sets them again (FE_CACHE_VERSION v4). Test, a guard of the format:
TestFrontCacheOneWayFlags.

### Step 1b, part 1: keys a std body shares across programs (2026-10-20)

For Xo Lang 3's shared std cache (decision 0272). A body of std, the
prelude or the native runtime is keyed by a hash of every std source
(std_tree_sum, compiler/xo bcache.xo: 185 files, 3.4 MB on 2026-10-20)
and whether the build has a root (its positions name std files the same
either way), not by the std modules this program loads (std_sum) or the
root; a program module's body keeps its interface closure and the root,
and so does a std module that is the build's root (`xo test
--std-module`: its functions are named and built as the main module's),
and every std body keys on the std modules that are the build's root
("" for a program), since a std body names that module's functions
unprefixed there.
An entry is stored under its key and the builder's own flags (a body of
a program with tasks differs from one without: fe_variant), and a build
reads the two common variants (no tasks, tasks). FE_CACHE_VERSION v7.

Measured (one fresh cache, built in order; main 0 everywhere):
http_server 0 of 2,359, json_text 0 of 1,132 (the first without tasks),
tls_nistec 1,039 of 2,018, http_tls 2,287 of 2,702. Cost: fe.keys 5 to
10 ms to 11 to 18 ms and fe.read 7 to 10 ms to 16 to 48 ms a build; a
hit saves about 100 ms of fe.build on tls_nistec. TestFrontCacheAcross
Programs passes (every golden and run program, 1,286 s).

### Step 1b, part 2: the variant a build reads, and the std hash memoized (2026-10-20)

A build reads one variant when it knows which: after each build the
cache stores a hint, the own flags the build ended with (fe_final_own),
keyed by the cache id, the build's flags and the root modules' paths
and absolute directories (bcache.xo fe_program_id, `<cache>/fe-hint/`).
With a hint the build reads that variant only, and when the hint has
unwinding checks (conc=true) it starts with them: a program that needs
concurrency only in std bodies (an HTTP server, a TLS client) no longer
builds every body once without them first. A wrong hint costs a build
again with the other setting (build_program_fe: need_conc against
conc), as the first build of such a program did before; the output is
the same either way (byte identical to the cache-off build, first and
hinted). With no hint (a program's first build) both common variants
are read.

A first try predicted the variant from the scans (tasks in the user's
modules): wrong for http_tls, whose need for concurrency comes from std
bodies, so its second build read the other variant and hit 0 of 2,702.

The std tree hash is memoized in `<cache>/stdsum` by the std files'
paths, sizes and modification times (recomputed when a file changed in
the last 2 s).

Measured (one fresh cache, json_text, tls_nistec, http_server, http_tls
in order, three rounds; part 1 then part 2, ssa phase in ms):

| program | round 1 hits | ssa r1 | ssa r2 | ssa r3 |
|---|---|---|---|---|
| json_text | 0 of 1,132 | 279, 277 | 107, 65 | 53, 42 |
| tls_nistec | 1,039 of 2,018 | 300, 301 | 121, 84 | 77, 64 |
| http_server | 0 of 2,359 | 602, 607 | 173, 101 | 124, 74 |
| http_tls | 2,287 of 2,702 | 290, 288 | 217, 119 | 166, 82 |

fe.keys 10 ms to 6 ms; fe.read on a warm build 14 ms to 6 ms; fe.build
of http_server 90 ms to 54 ms and http_tls 135 ms to 63 ms (no wasted
first build).

### What is left in an edit, and the relocations (2026-10-20)

Skips building xo (main's xo, cold): 250 of 8,534 bodies have no entry:
generic instances 136, closures 59, constants 31, spawn sites 18,
thunks 6, interface type numbers 0. The relocations (tids, sites,
constants, closures) would add 114 bodies (1.3%), at most about 0.1 s a
cold build; a warm rebuild already hits 8,284 and spends its 1.09 s of
fe.build mostly committing entries. http_server skips 28 of 2,342,
tls_nistec 12 of 2,000.

A one-constant edit of xo after a warm build (three edits, load 28 to
32): 5.5 to 5.8 s wall, 7.8 s user, 4.2 to 4.8 GB peak; check 0.86 to
0.93 s, ssa 0.93 to 0.98 s (fe.build 0.71 to 0.73), inline 0.75 to
0.85 s, insert_rc 0.52 to 0.57 s, prune_unwind, add_landings and cleanup
0.53 s, emit 0.40 to 0.48 s, assemble 0.26 to 0.42 s, link 0.36 to
0.38 s, fn_keys 0.15 to 0.20 s. The checker's bodies (0.49 s of a 1.1 s
`xo check` of xo, flow 0.18 s, parsing 0.23 s, CPU profile) come first
(the checker overlay, step 3), then the relocations; linking is 6.6% of
an edit.

### Step 1b, the closure relocation (2026-10-20)

A body that makes closures is cached: each closure it makes is an event
of its entry (its name as made, its parameters, results and captures by
type name, whether it catches), and each closure's body, built by its
job later in the queue, gets an entry of its own that is sealed into the
owner's entry once the build is done (nested closures in turn). A commit
numbers each closure as the build numbers closures (Builder.nclos is
builder-wide, so a closure's name depends on every earlier body),
renames the body's references, declares the function, and queues a
replay of the closure's entry at its job's turn, so the queue order and
the output are a serial build's. A closure entry that does not replay
drops the build as a failed commit does. Measured on xo: the 59 bodies
that made closures no longer skip; a warm rebuild hits 8,425 of 8,618
bodies (8,284 of 8,534 before) and fe.build 575 ms (1,092 ms); identity
446 of 446. With them cached, XO_CHK_CACHE=auto can skip the modules
whose only uncached bodies made closures (3 of 21 skipped before).
Remaining skips on xo: generic instances 139 (always built), constants
32, spawn sites 19, thunks 6.

### Step 1b, the constants, spawn sites and thunks relocations (2026-10-20)

Three more tables a body changed and a commit did not replay:

- Constants of collection types (Builder.consts, keyed by the name of
  the function that builds each, `xo.const[mod.NAME]`): each use is an
  event 6 of the entry (that name, and whether the body made it, its
  function then one of the functions made whole). A commit numbers a
  constant as the build there numbers constants and writes the number
  into the body's `rt.const_once` calls (and those of the functions it
  made whole); a constant it read must be there. One it made may be
  there already, made by another body the build reached first: the
  commit keeps that one and does not decode the entry's copy of its
  function, nor of its list type's drop (`xo.droplist[T]`), both fixed
  by their names, which are exempt from the absent check (fe_const_fn).
  Requiring it new (the first version) made the entry depend on which
  reader a build reaches first: under XO_CHK_CACHE=auto every warm edit
  of xo missed three such bodies and fell back, lowering the program
  twice (12 s an edit; 5.0 s after, 2026-10-21).
- Spawn sites (Builder.sites, the spawns whose task can fail): each site
  a body numbers is an event 8; a commit numbers it next and writes the
  number into its `rt.task_spawn_fn` (or `rt.task_spawn`) call. A body
  that reads a site numbered before it is still not cached.
- Thunks (a function used as a value, `NAME.value`): each is an event 7
  (the function's identity, the thunk's name, whether the body made it);
  a commit finds the thunk, or registers the one it made whole. A thunk
  of a generic instance (named by the instance's name here) still keeps
  its body out.

FE_CACHE_VERSION v11 (v10 had the constants alone). The skip counts on xo and identity are measured in the end-of-night run (owner, 2026-10-21: testing at the end).

## Addendum: std bodies keyed by std's interfaces (2026-10-21, xo-fa)

A std body's front end cache key held std_tree_sum, the hash of every
std source in full, so any edit of any std file, a comment too, missed
every std body of every program (`cache fe 0/1188` after a comment was
added to json.xo; bench/results/test-speed-vs-go-2026-10-21.md). It now
hashes each std file's interface (compiler/parser iface_text: the tokens
outside function bodies, without comments or layout; std sum v2). A
comment, layout or body edit keeps every other std body's key; the
edited body misses by its own text, as a program module's body does.
Program modules' interface closures still hash comments (a comment edit
misses that module's bodies): the next step. Test: internal/arm64
TestFrontCacheStdCommentEdit, compiler/parser iface_test.xo.
