# 0283: A persistent compile server for the edit loop

Status: Proposed (xo-fa, 2026-10-20; owner-approved direction through
the lead: a design record first, target a one-constant edit of xo from
6 to 8 s down to 1 to 2 s). Not started: the in-process caches below
come first, and the server is measured against them.

## Context

The agents' inner loop is rebuilding xo after an edit. Measured on the
development Mac on 2026-10-20 (main's xo, a one-constant edit and a
body-only edit of compiler/cli/core/fix.xo after a warm build, three
edits each, load 19 to 50, so wall times are noisy; decision 0271's
"What is left in an edit" section):

| Phase | Time of an edit |
|---|---|
| check | 0.75 to 0.94 s (0.22 to 0.27 s when unchanged modules are skipped, XO_CHK_CACHE=1) |
| ssa (the front end cache's commits, fe.build 0.5 to 0.7 s) | 0.67 to 0.98 s |
| inline (keys, apply, verify) | 0.75 to 0.85 s |
| insert_rc | 0.50 to 0.57 s |
| prune_unwind, add_landings, cleanup | about 0.5 s |
| emit | 0.4 to 0.48 s |
| assemble | 0.26 to 0.42 s |
| link | 0.36 to 0.38 s |
| fn_keys | 0.12 to 0.20 s |

Wall 4.6 to 5.8 s at moderate load, 7.2 to 8.4 s of CPU, peak memory 5.0
to 5.6 GB (about 3.1 GB when unchanged modules' checks are skipped). A
build starts from nothing each time: it parses or loads every file,
reads and decodes every cache entry it uses (fe.read, packs), commits
every front end entry (fe.build is mostly commits: 8,425 of 8,618
bodies hit on a warm build), and computes every key again. That fixed
work is most of an edit.

`xo serve` exists (decision 0178 stage 5, 0190): a Unix socket server
(compiler/cli/servers/serve.xo, `<user cache>/xo/serve.sock` or
$XO_SERVE_SOCKET) with a JSON line protocol, which runs each command as
a child (XO_NO_SERVE=1) and caches whole responses of `xo check` and
`xo query` keyed by the directories' file stamps. The CLI forwards only
check and query to it (compiler/xo main.xo forward_cli).

The Mac is shared by several agents and has 24 GB; on 2026-10-20 it ran
out of memory three times with two heavy jobs, and heavy.sh now runs one
at a time. A resident process holding gigabytes between builds competes
with every other agent's work.

## Decision (proposed)

1. Extend `xo serve` with an in-process `build` operation, not a child:
   the server holds the state of the last build of each build target
   (working directory, command line without -o, target; the key the
   packs use) and builds the next one from it. The CLI forwards `xo
   build` and `xo run` when a server for the same xo (cache_id, decision
   0275) and std is running; otherwise it builds itself, as today.
2. What the server keeps per build target, each validated like the
   on-disk caches it shadows: the parsed files (path, size, modification
   time, then the content hash), the checked modules (the module check
   cache's rule, XO_CHK_CACHE=auto: a module whose sources and imported
   interfaces are unchanged and whose bodies all replay is not checked
   again), the decoded front end entries and back-end entries and parts
   (no read, no decode), and the keys computed (fe keys, inline keys,
   fn keys) of the functions whose inputs did not change.
3. The output is the one-shot build's, byte for byte: `xo serve
   --verify` builds each request in process and as a child and compares
   the binaries; the identity scripts and the fixed point run through a
   server too before it is on by default.
4. Memory is bounded: the server keeps one build target unless asked for
   more, exits after an idle time (default 10 minutes), drops its state
   when the process's resident memory passes a cap (default 3 GB,
   XO_SERVE_MAX_GB), and never starts on its own: `xo serve` is run on
   purpose (an agent's edit loop, `xo run --watch`, decision 0267).
5. Order of work, each measured before the next: (a) the in-process
   caches that also help one-shot builds land first: closures in the
   front end cache (0271 step 1b relocations, in progress) so
   XO_CHK_CACHE=auto skips most modules, then the remaining relocations;
   (b) a server prototype that keeps only the parsed files and the
   decoded entries, measured on the edits above; (c) the checked modules
   and the computed keys; (d) default for `xo run --watch` only.

## Consequences

- An edit of xo through a warm server skips the parse, the cache reads
  and decodes, most commits and key computations: the target is the
  edited module's check, its bodies' SSA, and the per-function back end
  work of what changed, plus the link (about 0.4 s today; incremental
  linking is a separate, later item).
- A long-lived process is a new failure mode: stale state is prevented
  by validating every kept item as its on-disk cache entry is validated,
  and by keying the server on xo's cache_id and std; a server whose xo
  was rebuilt refuses requests from another xo (the client then builds
  itself).
- Memory competes with other work on a shared Mac: the cap, the idle
  exit and the opt-in start keep it from running unattended.
- The one-shot path stays the reference and keeps every cache it has:
  a build without a server is never slower than today.

## Alternatives

- Only on-disk caches (today's path, extended): every build still pays
  the fixed reading and decoding; measured, that is most of an edit.
- Forking a warm process per build (a zygote): saves start-up only, not
  the reading and decoding the parent would have to share.
- A daemon started automatically by the CLI: rejected for the shared
  Mac's memory (decision point 4).
- Per-module separate compilation (decision 0220 stage 3): a larger
  change to code generation; the server keeps whole-program compilation
  and its output.

## Where an edit's time goes, and what a server can remove (xo-fa, 2026-10-21)

Measured with the front end cache's closure relocation in (176bd7c3),
XO_CHK_CACHE=auto, a warm build of xo then one constant edit and three
body-only edits of compiler/cli/core/fix.xo (load 20 to 25, phases of
at least 100 ms, in ms):

| Phase | Edits |
|---|---|
| check | 869 to 921 (auto skipped 3 of 21 modules: generic bodies kept the rest, fixed by ModEntry.eager) |
| fe.build (front end commits) | 487 to 532 |
| ssa | 614 to 1,203 |
| inline (keys 163 to 172, apply 372 to 460, verify 372 to 419) | 748 to 838 |
| insert_rc (borrows 344 to 377, plan 473 to 520, keys 125 to 140) | 540 to 589 |
| prune_unwind, add_landings, cleanup | 109 to 113, 156 to 160, 243 to 268 |
| fn_keys | 126 to 147 |
| emit | 435 to 494 |
| write | 206 to 1,522 |
| assemble | 690 to 2,226 |
| link | 417 to 458 |

Wall 5.5 to 8.3 s, user 8.3 to 8.4 s, peak 5.9 to 6.5 GB. No phase that
only reads or decodes a cache (fe.read, inline.read, insert_rc.read,
run_cache) reached 100 ms. The time is in passes over every function of
the program: the checker's bodies, the commits, the inliner's keys and
verify, insert_rc's borrow and plan analyses, the unwind passes, the
function keys, emit of the whole module, assembling and linking. A
server that keeps parsed files and decoded entries (step (b) above)
removes the process start and the reads, well under a second.

What follows from it, ranked by the time each can remove:

1. Reuse per-function results of the whole-program passes when a
   function's inputs did not change (its key, and the keys of what the
   pass reads across functions): insert_rc's borrows and plan, prune,
   landings and cleanup, inline apply and verify, emit. Each is a cache
   keyed like the ones the build has (fn_keys), so it helps a one-shot
   build too and needs no resident process.
2. Assemble and write: the text assembly of the whole module (0.7 to 2.2
   s with write): direct machine code (plan-direct-machine-code, after
   native --race) or per-function object caching.
3. The checker: ModEntry.eager lets auto skip the modules with generic
   functions; measured next.
4. The server (this decision) for what is left: the start, the reads,
   and the commits of unchanged bodies (fe.build 0.5 s), by keeping the
   last build's module in memory.

A server's in-process build also needs the driver not to exit on an
error: compiler/xo exits from 73 `fail` calls and 42 `os.proc.exit`
calls. Options: fork a child per request from the warm server (exits
stay; needs an rt fork for darwin, linux has llrt linux.fork, and a
reset of the worker pool in the child, whose threads do not survive a
fork), or return errors from the driver (about 115 call sites).

## Stage A: incremental borrow inference (2026-10-21, xo-fa)

insert_rc's borrow inference (0.39 to 0.50 s of a warm edit of xo)
scanned all of xo's functions at least once in every build. Now each
function gets a body hash (ssa borrow_hash: the compiler's identity, the
target, the function in full after reuse_var_params with its types by
key, and whether each name it uses names a function), and the build
target's pack keeps the last build's flags by body hash (<slot>/borrow/
list). A function whose hash the record has and none of whose callees,
transitively, is new or changed keeps its recorded flags; the rest (the
new or changed functions and their callers, closed) start from
"borrowed" and run the rounds against those. That gives the full
inference's flags: a function outside the set reads only its own body
and its callees', all outside the set, so its part of the greatest
fixed point is the last build's, and the rounds give the set the
greatest fixed point against it. Starting every function from its
recorded flags would not: a callee that stops consuming a parameter
(owned to borrowed) must make its callers borrowed again, which the
rounds, only ever going from borrowed to owned, cannot do.

Verify mode runs the full inference; XO_BORROW_CHECK=1 runs both and
fails the build when they differ; --phase-times prints `cache borrows
KEPT/TOTAL kept`. Test: internal/arm64 TestBorrowIncremental (a callee
edited from consuming its list to not and back, each build checked and
compared with the cache-off build).
