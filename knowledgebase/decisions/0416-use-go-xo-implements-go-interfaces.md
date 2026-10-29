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
