# 0416: Xo values implementing Go interfaces in `use go`

Status: Accepted (owner, 2026-10-29, answering the three open questions below; drafted by Lang 1)

## Context

A named Go interface in a bound signature is an opaque handle (decision
0041, spec 4.4; internal/gobind gen.go, handleFor): Xo can pass on an
`io.Reader` that Go handed it (an HTTP body), but cannot make one. So
every Go API that takes an interface for Xo to implement is closed:
`io.Copy(dst, src)` from an Xo source, `json.NewDecoder(r)` over Xo
bytes, `http.Handle(pattern, h)` with an Xo handler, `sort.Sort`,
`slog.Handler`, zap's `zapcore.WriteSyncer`, gzip over an Xo writer.
Functions already work as callbacks (gen.go callbackConv, rt.GoCallback;
decisions 0066, 0067: a call on the passing goroutine runs as the
passing task, a call from another goroutine as a child task of its own,
recorded under the deterministic scheduler, decision 0073), so
`http.HandleFunc` binds and `http.Handle` does not.

The Windows tester's survey (usego-coverage-2026-10-29.md) counts these
inside "function result or unmappable callback" and "interface{}"; the
case is small in count and large in use: it is how Go composes I/O.

## Decision

1. **An implementation is built from Xo functions.** For a named Go
   interface `I` whose every method maps as a callback (callbackConv's
   rules: parameters and one result that map, `(T, error)` as
   `T ! go.Error`), the binding declares a struct of functions and a
   constructor:

   ```
   // io (generated)
   pub struct WriterFns { write: fn(Bytes) -> Int ! go.Error uses fx }
   pub fn writer(f: WriterFns) -> Writer      // Writer: the existing handle
   ```

   The Go adapter defines `type xoWriter struct{ f ...; mu sync.Mutex }`
   with `Write(p []byte) (int, error)` calling `f.write` through
   rt.GoCallback, as a callback parameter is called today. The result is
   the ordinary `Writer` handle, so it passes wherever Go takes an
   `io.Writer`, and an Xo program writes its implementation as closures
   over its state (an Xo value captured by a closure, or a `Mutex[T]`).

   Functions, not an Xo interface or type: a closure captures what it
   needs and its effects are the callback's `uses fx` (fixed at the
   constructor call), with no new rule for "an Xo type satisfies a Go
   method set" and no question of which copy of a value Go holds (Xo
   values are values; Go keeps the implementation past the call).

2. **Buffers Go hands in are copied by direction.** A `[]byte` parameter
   of an implemented method is the reverse of bug 18's output buffer:
   - Go's `Read(p []byte) (n int, err error)`: the Xo function is
     `fn(max: Int) -> Bytes ! go.Error`; the adapter copies the returned
     bytes into `p` (at most `len(p)`; more is a fault) and returns
     `io.EOF` for an empty Bytes with no error. The same for `ReadAt`
     (offset added).
   - `Write(p []byte)`: `p` is copied into a Bytes (Go may reuse it after
     the call), as every []byte argument.
   The binder recognizes an output buffer by the rule bug 18 added
   (outBuffer: a Read* method's []byte, or a parameter named dst, with
   no []byte result).

3. **Calls on one implementation are serialized.** Go may call it from
   several goroutines at once (an `http.Handler` serving concurrent
   requests is the norm). Each call runs as a task (0067); the adapter's
   mutex is not taken by default, since serializing an HTTP handler
   defeats it: the closures' captured state follows Xo's rules for
   sharing between tasks (an immutable capture, or a `Mutex[T]`), the
   same as a function passed to `spawn`. A captured `var` is refused at
   the constructor, as for any callback Go may call from another
   goroutine (to check: the checker's rule for callbacks today).

4. **Embedded and large interfaces.** An interface with an embedded one
   (`io.ReadWriter`, `io.ReadCloser`) flattens its methods into one
   Fns struct. An interface with an unexported method, or a method that
   does not map, gets no constructor and a skipped-list line
   ("interface I: method M cannot be implemented in Xo: reason").

5. **Equality, identity, and replay.** The handle compares by Go's `==`
   (pointer identity of the adapter struct, as other handles by
   pointer). Under `xo run --record`, calls into the implementation are
   callbacks and are recorded and replayed as callbacks are (0093).

6. **Scope of the first version:** io (Reader, Writer, Closer, the
   combinations, ReaderAt, WriterTo where it maps), net/http (Handler,
   RoundTripper), sort.Interface, fmt.Stringer, error (an Xo error
   passed to Go as `error`: its `Error()` from the Xo error's message),
   slog.Handler where its methods map (its `Record` takes `any`
   attributes: waits on 0412). Measured on the 87 packages as 0412.

## Consequences

- `http.Handle` with an Xo handler, `io.Copy` from or to Xo, gzip and
  json streams over Xo data, `sort.Sort`, custom Go errors: the
  composition points of Go's I/O open to Xo programs.
- One more generated Go type per implementable interface, emitted only
  when the program calls its constructor (the binding declares all; the
  adapter is per program, like 0412's instances).
- A Go library holds Xo closures past the call: their lifetime is the Go
  object's, kept alive by the Go backend's GC (the Go backend's values
  are Go values already).
- spec 4.4 gains the Fns form and the Read rule; std is not affected.
- Tests: internal/gobind (the declaration and adapter for io.Writer,
  io.Reader, http.Handler, an embedded interface, an unmappable one),
  cmd/xo goldens on the Go backend (io.Copy from an Xo reader to an Xo
  writer; an http.Handler serving two concurrent requests through
  httptest; sort.Sort over an Xo list; an Xo error through errors.Is),
  and a record and replay of the handler.

## Alternatives

- **An Xo type implements the Go interface by its methods** (an Xo
  `impl` matching a Go method set, converted implicitly at a call): more
  like Go, but Go keeps the value past the call and may call it from
  other goroutines, which an Xo value (copied, mutated through `var`)
  cannot answer without the same closure capture underneath; and it adds
  an implicit conversion (spec 2.10 has few). Can be layered later as
  sugar over the Fns constructor.
- **Hand-written Go wrappers** (today's only way): every program that
  wants `http.Handle` writes and binds a Go file.
- **Bind only io via std adapters** (an Xo `Reader` to Go `io.Reader` in
  std/rt): covers the most common case with no binder work, but nothing
  else (Handler, sort, slog).

## The owner's answers (2026-10-29)

1. Functions (Fns structs) as the first form, with method-set
   implementation as later sugar, or straight to method sets? **Function structs first**; method sets later as
   sugar.
2. Serialize calls per implementation by default (safe, slow for
   handlers) or follow spawn's sharing rules (decision 3, proposed)? **Spawn's sharing rules**, no default
   serialization.
3. Scope of the first version (decision 6): io, net/http, sort, fmt,
   error; anything to add or drop? **As drafted.**

## As built, step 1: callbacks that return errors (Lang 1, 2026-10-29)

- Prerequisite of the Fns structs (each implemented method that returns `error` is such a callback), and
  useful alone: a callback parameter whose Go type ends in `error` (`func(int) error`, `func(string) (int,
  error)`) binds as `fn(..) -> T ! go.Error uses fx` (`-> () ! go.Error` with no other result), where it
  was skipped before. internal/gobind callbackConv; the adapter returns `GoErrorOf(r.E)` on an Err.
- std/go gains `fn error(message: Str) -> Error` (rt Go_error, Go's errors.New). rt GoErrorOf gives back
  the Go error a go.Error was made from, so an error that came from Go (io.EOF, a sentinel) keeps its
  identity for `errors.Is`; a replayed error, which has only its text, becomes errors.New(text).
- Binding format 12.

## As built, step 2: implementations by named function parameters (Lang 1, 2026-10-29)

- Changed from decision 1 (the lead, 2026-10-29, owner told with a veto): the constructor takes one
  function parameter per method, named after it, instead of a `Fns` struct; same substance (functions, not
  method sets), no new codegen for a binding-declared struct with fields. Called with named arguments:

  ```
  let w = io.writer(write: fn(p: Bytes) -> Int ! go.Error {
    sink.send(p)
    p.len()
  })
  let r = io.reader(read: fn(max: Int) -> Bytes ! go.Error { next_chunk(max) })
  let h = http.handler(serve_http: fn(w, r) { w.write_header(204) })
  ```

  A stored, reusable set of functions (the Fns value) can come later if wanted.
- internal/gobind bindImpl: for each interface handle of the package (not generic, not another package's),
  a Go signature with one function parameter per method (Go's method order, sorted by name) is bound by
  bindFunc as a function `xoNew_<handle>` of the adapter, so each parameter is an ordinary callback
  (callbackConv with step 1's error results; effects, replay and the task rules come with it). The adapter
  type `xoImpl_<handle>` holds the Go functions and its methods call them. The constructor is named
  after the interface (`writer`), or `implement_writer` when a function has the name; an interface with an
  unexported or variadic method, or one whose method does not map, is skipped ("interface I cannot be
  implemented in Xo: reason").
- Read rule: a method whose first parameter outBuffer recognizes, with `(int, error)` results, is
  `fn(Int, ...) -> Bytes ! go.Error`, given len(p); the adapter copies, panics (a fault) past len(p), and
  returns io.EOF for no bytes and no error.
- Tests: gobind TestImplementations (io, sort, net/http declarations and the io adapter), cmd/xo
  TestUseGoImplements (io.copy between Xo implementations, an Xo go.error through io.write_string,
  sort.sort over a Mutex[List[Int]]) and TestUseGoImplementsHandler (an Xo http.Handler serving two
  concurrent httptest requests).
- Decision 3 needs no new rule: closures capture by value and writing a captured `var` is XO0502 (core
  3.3), so a callback shares state with its creator only through a Mutex or Atomic.
- Upcasts (the lead, 2026-10-29: binder only, no checker change): an interface handle did not convert to an
  interface it embeds or satisfies (`resp.body()` is `io.ReadCloser`, `io.read_all` takes `io.Reader`:
  XO0401). internal/gobind bindUpcasts declares `fn (x: H) as_<iface>() -> I uses go` for each handle H whose
  Go type (`*T` for a struct) is assignable to interface I, I being an interface of H's package or one of
  upcastTargets (io's I/O interfaces, fmt.Stringer) when the package depends on io or fmt; a name already
  taken is skipped. The targets are looked up in the dependency imported whole through the binder's own
  importer, since an import's scope from export data holds only what the bound package names
  (gotchas/gobind-export-data-partial-scope.md). Binding format 14.
- Binding format 13.

## As built, step 4: Go's error as a value (Lang 1, 2026-10-29)

- Go's `error` outside a function's final result (a parameter, a field, a result before the last) maps to
  `go.Error?` both ways (rt GoErrOpt, GoErrorOfOpt; nil is None), which opens errors.Is and As, error
  parameters and fields; an `error` that is the only result keeps `! go.Error` (errors.new, join, unwrap:
  nil is Ok). A package variable of type error is `fn name() -> go.Error` (rt GoErr of it; a nil one
  faults), where sentinel errors were not bound. Measured: "error outside the final result" was 202 of the
  508 third-party skips and 16 of std's 280 after step 2 (survey of 2026-10-29, ~/xo-lead/notes/lang1/
  survey); the after numbers are in bench/results/usego-coverage-2026-10-29b.md.
- Binding format 15. Tests: gobind TestErrorValues, cmd/xo TestUseGoErrorValues.

## As built: function-typed fields and variables (Lang 1, 2026-10-29)

- A struct field or package variable whose type is a Go function type
  binds `set_<name>(v: fn(..) uses fx)` (format 17 bound no getter; the
  next item below adds it). gobind
  bindFuncSetter binds it through bindFunc as a call of a Go function
  literal that assigns its argument, so the callback conversion, the
  error result rule of step 1, the effect variable and the replay mode
  (callback) are the ones a callback parameter gets. An unexported named
  function type, or a callback that does not map (several results), is
  skipped as before.
- Binding format 17. Tests: gobind TestFuncSetters, cmd/xo
  TestUseGoFuncSetters (synthetic example.com/hook in
  testdata/goproxy).

## Amendment: methods of named basic types (Lang 1, 2026-10-29; lead approved, owner FYI with a veto)

- A named basic type (`time.Month`, `fs.FileMode`, `slog.Level`) maps to its
  basic type, so its methods had no Xo receiver and were skipped (survey 3:
  46 std, 30 third party). Following this decision's function first rule,
  each value receiver method is a package function taking the value first.
- Naming: `<type>_<method>`, each part in snake case (`Month.String` is
  `month_string`, `FileMode.IsDir` is `file_mode_is_dir`); the receiver's
  Go name, else `v`, names the first parameter.
- A name clash: package functions, variables and constants are bound first
  and keep their names; a method whose name is taken is skipped with
  "name `x` is taken" (a counted skip, so the survey shows it).
- A pointer receiver method (`(*Level).Set`, `UnmarshalText`) changes the
  value and Xo passes a copy, so it stays skipped (the copy rule of 0419).
- The adapter calls the method expression (`time.Month.String(time.Month(m))`).
- Not chosen now: an Xo newtype per named basic type (`Month` a distinct
  type with methods). It is truer to Go and gives `m.string()`, but every
  function that takes or returns the type would change from `Int` to the
  newtype, which breaks existing programs' bindings and needs conversions
  at each use; revisit if the owner wants method syntax there.
- Binding format 20. Tests: gobind TestNamedBasicMethods, cmd/xo
  TestUseGoNamedBasicMethods.

## As built: Go function values in Xo (Lang 1, 2026-10-29)

- A Go function value Go gives Xo (a result, a field getter, a variable
  read) is an Xo function `(fn(..) -> R ! go.Error uses go)`: gobind
  mapResult tries mapType and, for a function type it skips, uses
  funcValueConv, whose toXo wraps the Go function in a Go closure of the
  Xo function representation (`func(*Task, ..) Result[R, E]`) that enters
  Go (GoEnter, GoExit: a Go panic, a nil function's included, is a
  fault) and converts arguments and results as an adapter does. It has
  no toGo; a parameter of function type stays a callback. Skipped: a
  variadic or generic function value, one with a parameter that cannot
  go to Go (a function); several results came in format 24 (below).
- Only an unnamed function type is a function value. A named one stays
  a by value handle: it goes back to Go, which a function value cannot
  (functional options such as zstd's `EOption`, returned by
  `with_concurrent_blocks` and passed to `new_writer`), and keeps its
  methods and upcasts (http.HandlerFunc). A first build that made method
  less named types function values lost 14 third-party functions in the
  survey, all functional options.
- A function returning one is replayed as a handle call (it re executes):
  the conv carries the package in hpkgs.
- The type is parenthesized because a trailing `uses` after a result
  function type would be the returned function's (spec 15, "Function
  types in return position").
- Binding format 22. Tests: gobind TestFuncValues, TestFuncSetters (the
  getters), cmd/xo TestUseGoFuncValues (synthetic example.com/fnval).

## As built: several results as a tuple (Lang 1, 2026-10-29)

- A callback (callbackConv) or a Go function value (funcValueConv) with
  2 to 6 results, not counting a final error, maps them to an Xo tuple
  `(A, B)` (rt TupleN, fields F0..): the callback's Go function returns
  `r.V.F0, r.V.F1` (zero values and GoErrorOf(r.E) on Err), the function
  value's call builds the tuple from r0, r1. A result that cannot go to
  Go (an iterator) still skips a callback. More than 6 is skipped as a
  bound function's results are.
- Binding format 24. Tests: gobind TestCallbackErrors (two), TestFuncSetters
  (pick), TestFuncValues (pair), cmd/xo TestUseGoTupleResults (synthetic example.com/multi).

## Addendum (2026-10-30): upcasts to other packages' interfaces

Status: Accepted (lead, 2026-10-29: "a 0416 addendum, lead-approved, owner
FYI with veto"; Lang 2).

The upcasts (`as_X`, bindUpcasts) covered a package's own interfaces and a
fixed list from io and fmt. The Go workflows suite (2026-10-29) found that
chi's `*Mux` could not be passed where Go takes net/http's `Handler`
(`Mux.Mount`, `httptest.NewServer`): Go's assignability does not cross the
handle boundary, and `Handler` is neither chi's nor io's.

Decision: a handle also has `as_X` for each interface of another package
that its package's own bound signatures name (a foreign interface handle
the binding refers to), when Go would assign the handle's type to it:
chi's `Mux` gets `as_handler() -> http.Handler` because `Mux.Handle`
takes one. Only named interfaces, non generic, in a fixed order (by Xo
name); a name clash is a counted skip as before.

Not chosen: every interface of every imported package (many methods no
program uses, and an import's interfaces change the binding without its
signatures changing); a by-name list per package like upcastTargets (it
would never cover third-party interfaces).

Tests: cmd/xo TestUseGoForeignUpcast (synthetic example.com/upmux: a Mux
given to its own Get(http.Handler) and to httptest.NewServer).

### Widening (2026-10-30): the interfaces of imported packages

Status: Accepted (lead, 2026-10-30: "go ahead with the widening (format
28), lead-approved, owner FYI with veto"; Lang 2). It revises the "Not
chosen" line above for imported packages.

The Go workflows suite's aws workflow still failed after the addendum:
`aws.Config.set_credentials` takes `aws.CredentialsProvider`, and
`credentials.StaticCredentialsProvider` implements it, but the credentials
package's own signatures never name that interface, so no
`as_credentials_provider` was bound.

Decision: a handle also has `as_X` for each exported, non generic interface
with methods (not a constraint) of a package its package imports directly,
named by its signatures or not, when Go would assign the handle's type to
it. An imported package's interface is bound only when one of the
package's handles implements it, so an import adds no foreign handle a
program could not use; internal packages (`.../internal/...`) offer
nothing (no binding can import them). The imports are go list's (its
`Imports`): a package read from export data lists only those its API
refers to (gotchas/export-data-imports-api-only.md). They are walked by path and each
scope's names in order, so the binding stays deterministic. Binding format
28.

Cost, measured 2026-10-30 on the Go workflows suite (generated binding
lines under .xo/gobind, the `as_` methods among them, and the built Go
backend program; A is main's xo, B the widening):

| workflow | lines A | lines B | growth | as_ A | as_ B | program A | program B |
|---|---|---|---|---|---|---|---|
| aws | 160125 | 161887 | 1.1% | 359 | 437 | did not check | 10326274 |
| chi | 65878 | 65959 | 0.1% | 250 | 261 | 8113298 | 8113298 |
| zap | 68502 | 68577 | 0.1% | 248 | 258 | 6129394 | 6129394 |
| pgx | 95218 | 101189 | 6.3% | 527 | 698 | (0412 pending) | (0412 pending) |

Programs do not grow: Go's linker drops the methods a program never calls.
The aws workflow passes with B (it did not check with A).

Not chosen: interfaces only of imported packages that some other bound
function takes (it needs every binding's signatures at once, and a program
can still pass a value to a field, as aws.Config.Credentials, that no
function names).

Tests: cmd/xo TestUseGoImportedInterfaceUpcast (synthetic
example.com/upwide: impl.Hello implements iface.Greeter, which impl imports
and never names; `iface.say(impl.new("xo").as_greeter())`); the Go
workflows suite's aws workflow.
