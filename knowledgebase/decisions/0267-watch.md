# 0267: xo run --watch and xo test --watch

Status: Accepted (owner, 2026-10-18, through the lead: the scope below);
written by Xo Lang 3 2026-10-18, not implemented yet (after decisions
0261 stage (a) and 0262)

## Context

An agent or a person editing a service runs it, edits, stops it, builds,
and starts it again, and does the same with its tests. External watchers
(entr, watchexec, air) restart on any file event: they restart a server
for an edit to a test, a comment, or a file the program does not use,
and they cannot tell which tests an edit reaches. xo already knows more
than a file watcher:

- the native run cache of `xo test` (compiler/xo/testcache.xo, decision
  0236 addendum) records every file a build read, with its hash;
- per-declaration hashes (compiler/cli/semantic/defhash.xo, decision
  0122 lineage) say which functions an edit changed;
- the backend function cache keeps a rebuild of a small edit fast;
- std's Proc has `kill(pid, "TERM")` and Child.wait, enough for a
  graceful handoff.

## Decision

Agreed scope (owner, 2026-10-18):

1. `xo run --watch [dir] [-- args]` builds and runs the program, then
   watches. On a change it rebuilds; it restarts the program only when
   the built binary changed (its bytes' SHA-256): a test-only edit, a
   comment, or an edit the build does not see leaves the program
   running. A restart is a graceful handoff: SIGTERM, wait for the exit
   up to `--grace=D` (default 5s), then SIGKILL, then start the new
   binary. A failed build keeps the old program running and prints the
   diagnostics.
2. `xo test --watch [dirs]` runs the tests, then watches. After a change
   it reruns only the tests that reach a changed function plus every
   test in a changed `_test.xo` file: a test reaches the functions of the
   program's call graph from its block (the checker's resolved calls,
   transitively; a call through an interface or a function value reaches
   every implementation the program has, so the set errs toward
   rerunning). Changed functions are those whose defhash changed. Typing
   `a` and Enter (or the line `all` on standard input) reruns every test.
3. What is watched: the files the last build read (the run cache's input
   list: the modules' .xo files, xo.mod, xo.lock, xo.work, `use c`
   sources, and the directory listings, so a new file is seen), polled
   every 200 ms by size and modification time and confirmed by hash, so
   an editor's save-by-rename and a touch without a change both behave.
   Polling, not kqueue or inotify: it is the same on every host and the
   list is small. std is watched only when it is outside the xo release
   (an XO_STD checkout).
4. `--json` prints one JSON object a cycle on standard output: `cycle`,
   `changed` (files), `diagnostics`, `build` ("rebuilt", "unchanged",
   "failed"), and for run `restarted` and `pid`, for test `passed`,
   `failed` (name, message, seed), and `unaffected` (the count skipped).
   The last cycle's object is also written to
   `<module root>/.xo/watch/last.json` (the .xo directory xo already
   uses for caches) so an agent can read the current state without
   holding the stream.
5. Both share one loop (compiler/xo/watch.xo): watch, debounce (a cycle
   starts 100 ms after the last change seen), build in-process through
   the same path as `xo build`/`xo test` (the parse cache and the
   backend function cache stay warm across cycles), act.
6. When it ships, spec/core.md 9's "Where tests live" note gains: `xo
   run --watch` compares the built program, so it needs no file-name
   exclusions for test files.

## Consequences

- No restart for an edit that does not change the program, whichever
  file it is in; a test edit never restarts a server.
- The affected-test set can be larger than needed (dynamic calls), never
  smaller, so a passing --watch cycle is as good as a full run for the
  edit; `a` runs everything anyway.
- A long-lived xo process holds caches across cycles: memory is bounded
  by the parse cache of one program plus the backend function cache.
- Native backends only at first (the run cache and the function cache
  are native); `--backend=go` with `--watch` fails with a message.

## Alternatives

- Recommend an external watcher: restarts on every save and reruns
  every test; no handoff deadline or JSON for agents.
- kqueue/inotify/ReadDirectoryChangesW: lower latency, three
  implementations and their edge cases (editors' atomic renames,
  overflowed queues); polling a small list is cheap enough.
- Restart on any change to a non-test file: the simple rule the owner's
  "restart only when the built program changed" replaces.
