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
