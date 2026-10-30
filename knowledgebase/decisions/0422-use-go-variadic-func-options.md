# 0422: `use go` binds a variadic of a function type (Go's functional options)

Status: Accepted (lead, 2026-10-29: "claim an ADR for variadic func options as List[callback] and do it"; drafted by Lang 2)

## Context

Go's functional options pattern ends a signature with a variadic of a
function type: `s3.NewFromConfig(cfg aws.Config, optFns ...func(*s3.Options))`,
`(*s3.Client).PutObject(ctx, params, optFns ...func(*s3.Options))`,
`grpc.NewClient(target string, opts ...DialOption)` (DialOption is an
interface there, but many SDKs use the func form). internal/gobind maps a
function typed parameter as a callback (gen.go callbackConv, rt.GoCallback;
decisions 0066, 0067) except when it is the variadic last parameter: the
`[]func(...)` went to mapType, whose function case is a skip. So the Go
workflows suite (bench/usego-workflows/aws, 2026-10-29) found no
s3.NewFromConfig, no s3.New and no Client operation in AWS SDK v2's s3
binding: 156 of its 1109 skips (s3 v1.114.6), nearly all operations a
program needs.

A variadic is a List in Xo (core 3.2), and `...any` is already a
`List[go.Any]` (decision 0412), so the options are a List of callbacks.

## Decision

1. A variadic last parameter whose element is an unnamed Go function type
   (`...func(*s3.Options)`) binds as `List[F]`, F being the Xo function type the element would have
   as a callback parameter (spec 4.4: handles for pointer parameters,
   `! go.Error` for a final error result, `uses fx`). The function takes
   the effect parameter `fx` as any function with a callback does.
2. The element type allows `go` itself (`fn(Options) uses (go, fx)`): an
   option sets the options through Go setters, and a list literal's
   closures share one effect variable, so a first element that performs
   nothing made the list pure and the next one, which calls a setter, an
   XO0701 (found by TestUseGoFuncOptions).
3. Each element is converted as a callback parameter is: its own
   rt.GoCallback, so a call during the Go call runs as the calling task and
   a later call from another goroutine as a child task (decision 0067),
   recorded by `--record`.
4. `[]` passes no options (Go's `f(x)`); an option is an Xo closure that
   changes the options through the handle's setters:
   `s3.new_from_config(cfg, [fn(o: s3.Options) { o.set_use_path_style(true) }])`.
5. A variadic of a named function type (zstd's `...EOption`) is
   unchanged: a List of by value handles, which Go's option helpers
   return and Xo passes back (gotchas/gobind-named-func-types-round-trip.md,
   Lang 1's function values).
6. The element's own skip rules stay: a variadic or generic function type,
   or one with a parameter or result that has no Xo form, still skips the
   whole function with that reason.

## Consequences

- AWS SDK v2's clients and operations bind; the same for SDKs that take
  `...func(*Options)` (Azure's and Google's option structs, many HTTP and
  database clients).
- A Go helper that returns an unnamed option (`func(...) func(*Options)`)
  gives a Go function value (`fn(..) uses go`, Lang 1's function values),
  which an Xo List of options can hold beside Xo closures where its type
  matches; otherwise a program writes the closure itself.
- No change to bindings that did not have such a parameter; the binding
  format version goes up, so cached bindings are made again once.

## Alternatives

- Drop the variadic from the binding (always pass none): simpler, and
  most calls pass no options, but it takes away per call options (path
  style, retries, a custom endpoint resolver) with no way back, and the
  binding would not show Go's signature.
- One Xo parameter per option: Go's options are open ended; no.
- An opaque `go.Func` value made by Go helpers only: needs function
  results bound first (not decided), and Xo closures would still not pass.
