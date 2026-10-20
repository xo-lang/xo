# 0276: The standard library follows Go's: scope, code, and tests

Status: Accepted (owner, 2026-10-20). Std work is assigned to the agent
Xo Lang Lib 1.

## Context

Xo is a backend language, and native is now the default backend
(decision 0270). Std grew module by module as programs needed it
(HTTP, TLS, JSON, SQL, crypto, time, CLI, CSV, TOML, regexp, archive),
and several of those were already ported from Go's standard library
(THIRD_PARTY_NOTICES: the cryptography, TLS, X.509, HTTP/1.1 and
HTTP/2, DNS, regexp, time zone rules, the Unicode tables). There is no
map of what std covers against what a backend needs, and no routine
for keeping ported code current. Gaps show up as surprises: std/sql has
only its in memory driver, and its real drivers were meant to be Go
packages bound with `use go`, which native builds cannot use.

Go's standard library is a stable, well tested, BSD licensed answer to
"what does a backend need", and Xo's semantics are close to Go's in
most of the areas std covers. The owner (2026-10-20): leverage the
existing library where possible, for both code and tests, since it is
very stable.

## Decision

1. **Scope.** Go's standard library is std's reference scope. A living
   coverage table, `knowledgebase/infrastructure/std-coverage.md`, maps
   each Go package to its Xo module, its status (done, partial,
   planned, not planned), and, for "not planned", one line saying why
   (for example go/*, debug/*, plugin, syscall, image). Packages a
   backend needs that Go keeps outside std (a Postgres driver,
   golang.org/x/crypto, x/net's WebSocket) are rows too, marked as
   outside Go's std.
2. **Code.** Where Go's implementation fits, std ports it to Xo rather
   than writing anew: the algorithms, parsers, state machines, tables,
   and their edge case handling. Each ported file names the Go file and
   version it follows, and THIRD_PARTY_NOTICES lists it.
3. **Tests.** Go's tests, test vectors, and fuzz corpora for a ported
   package are ported with it (as std `_test.xo` files and testdata),
   and a module that Go also implements gets a differential test where
   practical: the same inputs through Go and Xo, the same outputs (as
   TestH2Diff, FuzzHTTPDiff, and TestGoldenGoParity already do).
4. **API.** The API is Xo's, not Go's. Capabilities stay explicit
   (`uses net.dial`, an `Fs` or `Net` value; no package globals such as
   `http.DefaultClient`, `os.Getenv`, or a global random source),
   errors are typed `!` results, absence is `Option`, and shared
   resources are handles. Names and shapes follow Go's where that costs
   nothing, so a Go reader finds their way. Each new module's
   `spec/std.md` section is written and approved before its code, as
   now.
5. **Staying current.** Each Go release, the std agent reads its
   release notes and security fixes for the packages std ports, records
   which apply in the coverage table, and ports the fixes that do.
6. **Order.** Gaps are ranked by backend need, from a survey of
   representative services written from the spec, not by Go's package
   list. Native coverage comes first: a module is done when it works on
   every native target, the Go backend, and wasm where the capability
   exists.

## Consequences

- One table answers "does Xo have X" for users and agents, and the gaps
  it lists are the std agent's queue.
- Ported code inherits Go's maturity on edge cases, and ported tests
  catch the bugs a fresh implementation would have found the hard way.
- Every port carries an attribution obligation (the file header and
  THIRD_PARTY_NOTICES); a port without one is incomplete.
- Following Go's release notes is ongoing work, small per release.
- Std work outside std/native and std/llrt needs only vet, -short, and
  the module's own tests (`xo test --std-module`), so the std agent
  rarely needs a heavy slot. Work in std/native or std/llrt passes the
  fixed point as before.

## Alternatives

- **Mirror Go's APIs exactly.** Rejected: package globals and implicit
  effects would undo the capability model (spec section 3), and
  `(T, error)` with nil would lose what the checker proves about `!`
  and `Option`.
- **Bind Go's packages with `use go`.** Works only on the Go backend;
  native, the default, cannot use it.
- **Design each module from scratch.** Slower, and repeats edge case
  bugs Go has already fixed.
