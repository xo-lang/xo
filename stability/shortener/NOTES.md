# shortener: notes from writing it blind (decision 0520 phase 2)

Written 2026-11-06 from spec/core.md, spec/std.md and stability/README.md
only. Each bullet is a place where the spec was silent or ambiguous and I
guessed or experimented, with what I found.

## Spec gaps and ambiguities

- core 1.1, 4.2: the spec does not say whether a directory with no xo.mod
  inside a repository is a module of its own, or how `xo test DIR`
  resolves it. I wrote no xo.mod; `xo check`, `test` and `build` of
  `stability/shortener` all worked.
- std 4.1: route precedence is not stated. `GET /health` and `GET /{code}`
  both match `/health`, and the spec does not say whether the literal
  segment wins (Go 1.22's most specific rule) or registration order
  decides. I registered `/health` first and it answered. I also reserved
  the codes `links`, `health` and `debug` so the question cannot change
  behavior.
- std 4.1: when a path matches a literal route of another method
  (`POST /links`) and a `{name}` route of this method (`GET /{code}`), the
  spec does not say whether `GET /links` is a 405 (the literal route) or
  goes to `GET /{code}`. Observed: 404 with `{"error":"not found"}`, which
  the router's own 404 and my handler's 404 both write, so I could not
  tell which ran.
- std 4.2: a handler error's response is only `status()` plus
  `{"error": display()}`; there is no way to add a header through
  StatusError (a 401 cannot carry `WWW-Authenticate`). I left it out.
- std 4.1, 4.3: `Router.call` is "in process dispatch, for tests"; the
  spec does not say that it applies 4.2's error mapping and fault
  isolation, as `serve` does. Experiment: it does (my 400, 401, 404 and
  409 tests go through it).
- std 4.3: the Content-Type values are not stated: what `http.json`
  sets (with or without `; charset=utf-8`), and what `http.empty(302)`
  and a 204 carry. The harness compares Content-Type, so this matters
  for the Go twin. The twin writes `application/json` and none for empty
  bodies.
- std 4.3: `http.redirect(req, url, status)` writes "an HTML body on
  GET" and resolves Location "against the path"; it is not said what it
  does with an absolute URL. I used `http.empty(302).with_header(...)` to
  keep the body empty on the hot path.
- std 4.4, core 7.5: what `serve()` returns after a stop request is not
  stated. 4.4 says it "runs until its scope is cancelled ... returns
  Ok(())"; 7.5 says servers stop accepting in phase 1 but not that
  `serve` then returns, or what it returns when the grace period cancels
  it mid-drain. Experiment (arm64 and Go backend binaries): SIGTERM made
  `serve` return Ok and the program exit 0. I match `Err(Canceled)` as a
  guess for the grace case; not tested.
- core 7.5, std 3.7: `set_grace(5s)` is the only way I found to bound the
  drain to 5 s, but the grace period covers all of `main`, not only the
  server's in-flight requests. There is no `serve` option for a drain
  timeout (Go's `Shutdown(ctx)`).
- std 3.1: `write_atomic(fs, path, data)` is listed among Fs's methods as
  a free function, with no module named. Experiment: it is in the
  prelude (unqualified call compiles).
- std 3.1: it is not said whether `fs.write(path, b"")` on a file that an
  `open_append` handle still holds is safe, or whether the handle keeps
  writing at the old offset. I close the handle, truncate, then reopen.
- std 3.1: "each write reaches the OS" leaves open whether an acknowledged
  write survives a power loss; it survives SIGKILL. I do not fsync each
  record (the task says "written", and the harness uses SIGKILL); `File.sync`
  exists if the owner wants durability across a crash of the machine.
- std 3.1: `create` and `open_append` do not make parent directories on
  the real filesystem but do on MemFs, so a test passes where a real run
  fails. I call `create_dir_all` first.
- std 2.6, std 5: "Time displays as RFC 3339 in UTC" does not say whether
  fractional seconds are written (Go's RFC3339 versus RFC3339Nano). I
  truncate `created` to whole seconds (`Time.from_unix(now.unix())`) so
  the text is fixed.
- std 5: the text of a missing field's `DecodeErr` is not given. Observed:
  `at $: missing field url`. The twin copies it.
- core 5.1, 5.4: the copy rule inside `Mutex.with` ("a write followed by
  a point that can fail or wait copies the collection it writes, once
  per with") does not say whether a write to a scalar field (`st.seq`,
  `st.log_size`) before a failing call makes the Map in the same struct
  be copied. To be safe every `with` does its fallible journal write
  first and the Map write last; I could not observe which is true.
- core 2.9, std 2.7: copying a struct that holds an `Atomic` shares the
  atomic (std 0 says so for capabilities, 5.1 lists Atomic as a handle);
  the spec never says it in so many words for `Atomic` taken out of a
  Map with `get`. The redirect path depends on it (`get` then
  `e.hits.add(1)`); the tests show it works.
- core 3.3, 6.5: whether a closure passed to `Mutex.with_read` and not
  kept allocates is not stated (6.5 lists "closure" among allocations in
  real-time code). The redirect path makes one per request.
- std 36, the task's "measure": `/debug/sched` gives `heap_live` (blocks
  live) but no allocation count, and no std or runtime counter of
  allocations per request is documented outside 6.5's real-time
  sections. I measured only that the heap does not grow: heap_live 119
  before and 121 after 2000 redirects (arm64, XO_RTSTATS=1). Allocations
  per request are not measured; `XO_PROF=heap` is the next step.
- std 8.1: a request from `http.client(os.net)` made before the program
  under `testing.run_main` has called `listen` fails at once with `no
  fake handler for GET ...`; the spec only says a request to a listener
  whose `serve` has not started waits. My end to end test polls
  `os.stdio.output()` for the ready line first.
- std 8.4, core 7.5: `signal_after(d)` is "virtual time", and the virtual
  clock jumps when every task is blocked. It is not said whether time
  can jump while a test's client waits on the fake network for a
  response, so a short `d` might stop the server mid test. I used 10s.
- std 3.6: whether `Stdio.println` reaches a pipe at once (the harness
  waits for `listening on ADDR` on stdout) is not stated. It did,
  redirected to a file.
- std 11 (not read in full): I did not find a constant time compare in
  the prelude, so the token check is a hand written loop over the bytes.
- stability/README.md: app.json's key names are not given (only what it
  holds). I guessed `env`, `ready`, `tokens`, `scenario`, `redact`,
  `stats`, `stats_token` and `twin`. Also unstated: how a named token is
  sent (I assume `Authorization: Bearer TOKEN`), whether a step's `body`
  is a JSON value or a string (I wrote objects), and whether `redact`
  masks a key at any depth. `code` and `created` are redacted because a
  generated code and the time differ between runs.

## Diagnostics worth improving

- core 9.2: `expect(s).contains("x")` (my mistake: expect takes only a
  comparison) gives `XO0401 expected Bool, found Str` and `XO0601 ()
  has no field or method contains`, neither of which says that `expect`
  needs a comparison or suggests `expect(s.contains("x"))`.

## Design choices (not spec gaps)

- Journal records carry a sequence number and the snapshot stores the
  last one it covers, so a crash between the snapshot rename and the
  journal truncation replays nothing twice. Hits records hold totals,
  not deltas, so replaying one is idempotent.
- On start a non-empty journal is compacted into a new snapshot, so a
  line cut short by a crash is always the last line when read, and is
  dropped.
- Redirects take a shared read of the map and bump an `Atomic` outside
  the lock; a flusher task writes one hits record a second with the
  totals that changed, and the shutdown path flushes in a `defer`.

## Harness fixes after the blind write (Xo Lang 2, 2026-11-06)

- The scenario's `body` was only a JSON string; the writer used objects, which README did not rule out. The harness now takes either.
- The harness client followed the 302 to the link's URL on the internet; it now records the redirect and its Location.
- A step's result held the filled in path, so a generated code differed between backends; results now hold the path as written.
- README did not name app.json's `name` key; it does now.
