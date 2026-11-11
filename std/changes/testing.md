# std/testing changes

## 2026-11-11: fix: FakeNet patterns match natively as on the Go backend (a trailing slash, `{$}`, `{rest...}` on `/`)

- Kind: fix
- Commit: this commit
- Found by: data/aztables' tracing test (Xo Azure) failed natively since v0.46.0: `os.net.handle("GET https://acct.table.example.test/{rest...}", ...)` answered the service's `GET /?restype=service&comp=properties` with "no fake handler", where the Go backend matched it (aws-43, 2026-11-11)
- Go reference: net/http ServeMux's patterns (Go 1.22), less its redirects: a fake answers or fails, it never redirects
- Test: cmd/xo TestFakeNetRestRoot on Go, arm64 and LLVM; natively before this change `https://rest.example.test/` gets "no fake handler"
- Timing: none (test only)
- Shipped in: not yet released

Since aaef8a248 (v0.46.0) the native FakeNet split a request's path with
the slashes at both ends dropped and turned a pattern's trailing slash
into an exact path, where the Router and the Go backend's FakeNet keep
the empty last piece of a trailing slash. Natively a `{rest...}` pattern
did not match `/` or a path ending in a slash before it, a pattern ending
in `/` (`POST https://sts.test/`) answered only that path, `{$}` was
dropped, and `/users/{id}` also answered `/users/7/`. A FakeNet pattern
now matches on every backend as the Router's does: a trailing slash is a
subtree, `{$}` the exact path with a trailing slash, a rest takes the
empty piece after a trailing slash (`""` on `/`), and a pattern without
a trailing slash is an exact path. A test that registered `https://h/`
for one call now answers every path on `h` there, as it already did on
the Go backend.

## 2026-11-11: docs: the header says what the module is for

- Kind: docs
- Commit: this commit
- Found by: pkg.xolang.org and xolang.dev showed `xo doc std`'s summary of std/testing as a spec reference or a note about the file (the Xo Lead, 2026-11-11)
- Go reference: none
- Test: cmd/xo TestDocStdSummaries
- Shipped in: not yet released

The header comment of std/testing.xo opens with a sentence that says what the module is for, which `xo doc std` lists as its summary. No declaration changed.

## 2026-11-10: feature: TestOs.secrets, a test keyring

- Kind: feature
- Commit: this push
- Found by: decision 0584 stage C (Azure 1's token cache needs tests that do not touch the user's keyring)
- Go reference: none
- Test: cmd/xo TestFakeSecrets (testdata/secrets) on Go, arm64 and LLVM
- Timing: none (test only)
- Shipped in: not yet released

`os.secrets.keyring()` in a test is an in-memory keyring on every OS,
empty when each test starts, shared with `testing.run_main`'s Os.

## 2026-11-09: feature: FakeProc.runs() lists the commands the code asked to run

- Kind: feature
- Commit: this commit
- Found by: Xo Azure 1 (azidentity's developer-tool credentials need to see what a program would have run, with its arguments)
- Go reference: none (a test double's recorder; the quoting is strconv.Quote)
- Test: golden test_proc_cmd, "runs lists every command asked to run" (go and native `xo test`)
- Shipped in: not yet released

`os.proc.runs()` returns one entry per command asked to run (`Proc.run`, `Cmd.output`, `Cmd.run`, `Cmd.spawn`, each command of a
pipeline), in order, found by `on_run` or not (an unanswered one is listed and fails with NotFound). An entry is the program and
each argument after a space; an empty program or argument, one with a space, or one `strconv.Quote` writes differently, is written
with `strconv.Quote` (`az account show --name "my sub"`). Real processes (`--real`) record nothing. Both backends: std/rt's
fakeProc and the native test world (std/native/llrt/testworld.xo, `proc_note`).

## 2026-11-07: feature: FakeNet.handle_tls runs the client's TLS checks, and a FakeNet handler's host() is the URL's authority

- Kind: feature
- Commit: this commit
- Found by: Xo Azure 1 (azidentity's Service Fabric wrong-pin case needs the client's pin to run against a FakeNet server), decision 0544
- Go reference: httptest.NewTLSServer with tls.Config.ServerName, VerifyConnection, InsecureSkipVerify; http.Request.Host
- Test: golden test_fakenet_tls (go and native `xo test`)
- Shipped in: not yet released

`os.net.handle_tls(pattern, cert, h)` registers an https handler with the certificate its server presents. A request first
runs the client's roots, server name, `with_verify` and `with_verify_conn` checks against `cert` with the handshake's error
texts and fails before the handler when one refuses; the handler's `tls_state()` shows the server name sent. A FakeNet
handler's `host()` was "" unless the client called `with_host`; it is now the URL's host and port as written.

## 2026-10-26: fix: timers due at the same instant fire in scheduled order on the Go backend (FakeClock.advance, FakeClock.sleep)

- Kind: fix
- Commit: this commit
- Found by: three tasks sleeping 20s, 10s and 10s then `advance(10s)` woke the two 10s tasks in the reverse of the order they called `sleep` on the Go backend, where arm64 and LLVM woke them in the scheduled order std.md 8.2 names (reported by DS-2, 2026-10-26)
- Go reference: n/a (FakeClock is Xo's; the Go world's timers only)
- Test: the third test of the fakeclock corpus (internal/stdtest/testdata/corpus/fakeclock, generated by scripts/stdcorpus), which now checks the order, on the Go, arm64 and LLVM backends; it fails on the Go backend before this change with `left: "c,b"`

`std/rt/world.go`'s `fireNextLocked` fired every timer of the earliest
instant in one call, so both woken threads were ready together and the
deterministic scheduler's seeded pick could run the later timer's task
first. It fires one timer per call now: the callers loop, so the woken
task runs and blocks again before the next same-instant timer fires.

## 2026-10-20: feature: FakeNet raw connections

- Kind: feature
- Commit: 5a6d15fa
- Found by: survey (raw protocol drivers could not be unit tested: std/sql's Postgres driver, std/smtp)
- Go reference: net.Pipe and net/http/httptest's in process listeners (the idea; the buffering is Xo's)
- Test: internal/gen/testdata/golden/test_fakenet_raw (Go, arm64, LLVM; cmd/xo TestNoGCTests), std/rt TestMemConn

Decision 0292: a test's `os.net.dial(addr)` reaches the in memory
listener at `addr` (Refused without one) and `accept()` returns the other
end; `listen` takes host names and refuses a busy address; 64 KiB per
direction with task waits; close ends the peer's stream and fails its
writes; closing the listener resets its backlog; `tls.client` passes the
bytes through. Before, a dial in a test always failed with Refused.
