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
