# 0041: Use Go libraries until Xo has its own ecosystem

Status: Proposed (2026-10-08). Pending experiment in work queue item 6.

## Context
A new language lives or dies by its libraries. P1 compiles to Go, so calling
Go packages is mostly a binding and type mapping problem, not a runtime one.
Kotlin (Java), TypeScript (JavaScript), and Gleam (Erlang) grew this way.

## Proposed design
- Syntax: `use go "github.com/jackc/pgx/v5" as pgx`; Go module versions are
  pinned in `xo.mod`.
- `xo bind` reads the Go package with the Go standard library (`go/types` and
  `go list -export`) and generates Xo declaration files in the same bodiless
  form as `std/*.xo`. Code generation emits direct Go calls with conversions at
  the boundary.

Type mapping at the boundary:

| Go | Xo |
|---|---|
| `(T, error)` | `T ! go.Error` |
| pointer return that may be nil | `T?` (nil becomes `None`) |
| mutable Go objects (`*sql.DB`, `*http.Client`) | opaque handles, like capabilities |
| slices and maps | copied in and out to keep value semantics |
| `context.Context` parameter | supplied from the current scope (deadline and cancellation) |
| panic | fault, recovered at the call boundary |
| interfaces | Xo interfaces where the method set maps; otherwise handles |
| channels | `Chan[T]` handles |

## What Go code can still do to Xo's guarantees
- Authority: a Go library can open files or sockets without a capability.
  Calls into Go carry a visible `uses go` effect, and `xo.mod` declares what
  each Go package may do, so the escape is explicit and auditable.
- Concurrency: goroutines a library starts are not structured, and its
  internal state can race. Xo contains it behind handles; it cannot fix it.
- cgo: packages that need cgo break static cross compilation, so they are off
  by default.
- Record and replay: only Go calls whose results serialize can be recorded.
- Habit risk: agents may reach for Go idioms through the boundary. `xo vet`
  should report Go use per module.

## Experiment (work queue item 6)
Prototype `xo bind` against Go standard library packages first
(`crypto/sha256`, `regexp`, `strings`; no network needed), then one real
third party pure Go package (for example `github.com/google/uuid`). Measure
how much of a typical API maps cleanly and where hand written wrappers are
needed.
