# 0083: `--no-gc` builds and the XO1201 check

Status: Accepted (2026-10-09). Implemented in `cmd/xo` (`--no-gc` on
build, run, test, check), `internal/build/nogc.go` (`NativeBackend`,
`CheckNoGC`), `internal/ssa/nogc.go` (`CheckNoGC`, the diagnostics),
`internal/ssa/build.go` (`unsupported`, `impossible`). Tests:
`internal/build/nogc_test.go` (TestNoGCCheckAgrees), `cmd/xo`
(TestNoGC), `internal/llvm/testdata/unsupported/*`.

## Context
The native backends (decisions 0046, 0049, 0068 to 0082) build a growing
subset of Xo without a garbage collector, but a program outside the
subset failed late, inside the SSA builder, with plain text errors
("the SSA builder does not support ...") and no way to tell whether the
gap will close. The backend had to be named (`--backend=llvm` or
`arm64`) and the host decided which one works. Agents need the answer
as stable diagnostics before anything is generated.

## Decision
1. **`--no-gc`** on `xo build`, `xo run`, `xo test`, and `xo check`
   selects a native backend: arm64 on darwin/arm64 and linux/arm64
   hosts, LLVM otherwise. `--backend=llvm` or `--backend=arm64` refines
   it; `--backend=go` with `--no-gc` is a usage error.
2. **XO1201**, a new code in a new `12xx` group (native builds without a
   garbage collector): "not available without a garbage collector
   (`--no-gc`)", at the construct's site, naming the feature, with the
   support class in the message and in a note: *planned* (the native
   backends will grow it) or *impossible* (a call of a `use go` package
   or a value of its types: Go code needs Go's runtime).
3. **The check is the builder.** Every native build (with `--no-gc` or a
   named native backend) runs internal/ssa's builder as a check after
   type checking and before code generation; each place the builder
   cannot lower something records it (`unsupported`: planned,
   `impossible`: never) with its span, and those records are the
   diagnostics. No separate list of supported features exists to drift.
   An error recorded inside the span of an earlier one is dropped
   (core 10.2: never cascade). TestNoGCCheckAgrees requires, for every
   native golden, unsupported, benchmark, selfhost, and example program,
   that the check reports nothing exactly when both the LLVM and the
   arm64 code generators succeed.
4. **What the check adds beyond the builder's gaps:** an `https` URL
   written as a literal in a std/http client call (`get`, `post_json`,
   `new_request`) is XO1201 (TLS, planned; HTTP/2 needs TLS too). A URL
   computed at run time still fails with `ClientErr.Protocol` (std.md
   4.5). In a test build the check is off: a test's requests go to its
   fake network in process. A test build is checked as the test binary
   (point 7): every test block and what it reaches.
5. `xo check --no-gc` checks programs only (the builder starts at
   `main`); a library is not checked by it. The MCP server's `check`,
   `test`, and `build` tools take `no_gc`.
6. **Native std parity**, one feature at a time, each with a golden
   program whose output is identical on Go, LLVM, and arm64 and that
   frees every allocation:
   - **Rand and std/crypto** (`std/native/crypto.xo`, `rtcrypto.xo`,
     `std/llrt/xo_crypto.c`; golden `native_crypto`): the Rand
     capability draws from `getentropy` (macOS) or `getrandom` (Linux)
     (`int`, `float`, `bool`, `bytes`; `shuffle` stays XO1201). SHA-256,
     HMAC, PBKDF2, BLAKE2b, and Argon2 are written in C from their
     specifications and x/crypto's portable code (lanes of a slice run
     one after another, same result); verify_password ports std/rt's
     parsing (strings.Split, strconv.ParseInt, RawStdEncoding with its
     skipped newlines and ignored trailing bits). TestCryptoDiff checks
     them against Go on random inputs, the RFC 9106 vectors, the x/crypto
     outputs of std/rt/testdata, and mutated hashes, under
     AddressSanitizer and UBSan. `Bytes.from_hex` and `Str.runes` were
     added to the native prelude on the way.
   - **std/sql with the in memory driver** (`std/native/sql.xo`,
     `rtsql.xo`, `internal/ssa/build_sql.go`; golden `native_sql`): a
     port of std/rt/sqlmem.go and sql.go to Xo, statement for statement,
     with the same results and error texts (Go's `%T` names included).
     Ported to Xo rather than C: tables, rows, and values are Xo values,
     so reference counting manages them and a statement's private copy
     of the tables is shared until written (the Go driver's copy on
     write by hand); C would have needed a value bridge for every row.
     The database is a `Mutex`, the single writer permit of
     transactions a channel holding one token. `derive SqlRow` decoding
     is the generic intrinsic `sql_decode[T, R]`, lowered to a function
     per struct that calls the Row accessor of each field's type. Not
     ported: the `max_open` pool bound (the in memory driver runs one
     statement at a time, so nothing waits on it), drivers registered
     from Go (`sql.open` reports an unknown driver, as std/rt does with
     none registered), and the Debug text of a `Row`. On the way:
     wrapping, checked, and saturating Int methods in the native
     prelude; lowered types named after the implementation types and
     generic instances keyed by checker types
     (`gotchas/ssa-types-that-lower-alike.md`); LLVM accepts an enum
     holding a List of itself (`gotchas/llvm-recursion-through-list.md`).
   - **http.stream** (`std/native/http.xo`, sinks in `std/llrt/xo_http.c`;
     golden `http_stream`): the implementation's Response has a hidden
     `stream` field, the number of the stream's function in a table of
     the C runtime (an Int, so `==` on responses still lowers; a function
     field would not). Serving sends the body as std/rt over net/http
     does: writes go through a 2048 byte buffer as bufio's WriteString
     fills it (no write goes around it), the head leaves with the first
     data out of the buffer or the first flush while the handler is not
     done (chunked on HTTP/1.1, close delimited on HTTP/1.0, none for
     HEAD), each flush or full buffer is a chunk, and a stream that never
     leaves the buffer is an ordinary response with Content-Length.
     `Router.call`, `text`, and `decode_json` collect it into a buffer; a
     copy can be collected again, so the table keeps a function until
     its response is sent (or the program exits; a streamed response that
     is only collected keeps its entry). The builder gained the generic
     intrinsic `cast[A, B]` (a std type and its implementation type are
     one lowered type) and defaults for the hidden fields of an
     implementation in a std struct literal.
   - **A fault in a handler still exits** (not a 500): native faults do
     not unwind (decision 0070). (Superseded 2026-10-09 by decision 0085:
     faults unwind and the handler answers 500.) Answering 500 needs the handler's task
     to unwind from a fault: its deferred code run, what its frames hold
     released, its Mutex values (moved into `with` closures, decision
     0082) handed back and its locks released, its open scopes closed.
     Planned; the next experiment is a protected call at the connection
     task (a setjmp point the fault path jumps to on a fiber that has
     one), measuring what a fault then leaks and leaves locked before
     making it unwind properly.
7. **Native test binaries** (`xo test --no-gc`; `internal/ssa/
   build_testmode.go`, `std/llrt/xo_testing.c`, `std/native/testing.xo`,
   `rttest.xo`, `cmd/xo/nativetest.go`; tests `cmd/xo` TestNoGCTests and
   TestNoGCTestLeaks):
   - **One process per test.** The builder makes a function per test
     block and a main that runs the one XO_TEST names; the harness runs
     the binary once per test with XO_TEST and XO_TEST_SEED and prints
     the report std/rt's RunTests prints (`-v`, `-json`, `-run`,
     `--seed`). A fault fails its test only, with no unwinding: natively
     faults exit (decision 0070; since decision 0085 the test's deferred
     code runs before the process exits). A failed `expect` writes XO-TEST-FAIL
     and its message (left and right as Debug text) and exits 1, which
     works anywhere (helpers, closures, tasks).
   - **The fakes are the capabilities.** TestOs, FakeNet, MemFs,
     FakeClock, FakeEnv, FakeStdio, and FakeProc lower to Os, Net, Fs,
     Clock, Env, Stdio, and Proc; the C runtime runs a test binary in
     its test world: a virtual clock from 2026-01-01T00:00:00Z whose
     timers sit on the event loop's heap with no poller, every wait
     counted, so the deadlock check moves the clock to the next timer
     when every context waits (a deadlock when none is left); Rand
     seeded with Go's PCG and std/rt's seeds (the values match the Go
     backend's); an empty environment and file system; captured standard
     streams and log records (echoed to the harness as XO-TEST-LOG);
     FakeNet.listen gives in memory listeners that http.server serves by
     registering its router, reached in process by the client, and
     FakeNet.handle, fail, fail_next, requests keep their values as
     `rt.Any`. testing.run_main runs main below a node of its own that
     the (reset) stop state cancels, with FakeProc.signal_after as
     virtual timers.
   - **Not available yet** (XO1201 or a skip, planned): property tests,
     `--real` capabilities (`uses` tests are skipped as without
     `--real`), `--explore`, MemFs writes, stdin, `os.proc.exit` inside
     run_main (outside it ends the test, as on the Go backend), `os.tasks`.
     Error traces, fault frames, and the deadlock's list of blocked tasks
     are not recorded natively, so those reports are shorter.
   - examples/notes-service passes its 16 tests on arm64 and LLVM with a
     report identical to the Go backend's (logs and virtual times
     included), and no allocation is live at exit. Built with `--no-gc`
     and served on a socket, it answers a scripted session (health,
     notes, login, 401 and 400 cases) like the Go backend's build and
     stops on SIGTERM with status 0 after logging "stopped" (cmd/xo
     TestNoGCNotesServer); a manual smoke run on 2026-10-09 with curl
     added 20 concurrent creates (21 notes listed after) on both native
     backends.

## Consequences
- `xo build --backend=llvm` of an unsupported program now fails with
  XO1201 diagnostics (exit 1, JSON with `--json`) instead of a plain
  error.
- The check builds the SSA once more than a plain native build; the cost
  is a few milliseconds per program.
- Adding native support for a construct removes its XO1201 with no
  other change; a backend lowering that rejects what the builder accepts
  fails TestNoGCCheckAgrees.

## Alternatives
- **A separate allow list of std functions and constructs in the
  checker**: readable, but it drifts from the builder; the builder is
  the only place that knows what lowers.
- **Report through the existing build error text**: no stable code, no
  span, no support class; agents could not act on it.
- **Warnings instead of errors**: the program would not build anyway.
