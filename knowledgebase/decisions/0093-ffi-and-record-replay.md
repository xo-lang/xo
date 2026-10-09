# 0093: FFI: `use go` on the Go backend, `use c` on native backends, recorded at the boundary (Appendix B question 5)

Status: Accepted (2026-10-09, by the owner; proposed by the spec audit).
Stage 1 implemented (2026-10-09): `use go` calls are recorded and
replayed on the Go backend (notes below). Stage 2 (`use c`, `xo bind
--c`, the `ffi` effect) is not built.

## Context

Spec Appendix B question 5: "FFI design and how `ffi` interacts with
record/replay." Core 6.1 lists an `ffi` effect ("Foreign calls (P3)") that
nothing performs: the checker accepts `uses ffi`, no construct needs it.

What exists (evidence):

- **Go packages** (`use go`, decision 0041, core 4.4): bindings generated
  by `xo bind`, a `go` effect that `pub` functions declare, values copied
  at the boundary, opaque handles, panics as faults. 123/125 functions and
  92/95 methods of six std packages bind
  (`bench/results/go-interop-2026-10-08.md`); `uuid` binds and runs.
- **Record and replay** (core 11.3): Go calls are *not* recorded; they
  run again during a replay. Xo callbacks that Go calls on its own
  goroutines are recorded and replayed at the same scheduling point
  (decision 0073). So a Go library that reads a file or the clock makes
  a replay diverge silently.
- **Native backends** (decision 0083): `use go` is `XO1201` *impossible*
  (Go code needs Go's runtime). The native runtimes already call C for
  crypto (`std/llrt/xo_crypto.c`), JSON, HTTP parsing, and Unicode, but
  only as part of std; user code has no way to call C.
- Authority: Go code can open files and sockets without a capability; the
  `go` effect marks the escape in signatures (core 4.4).

## Options

1. **One `ffi` mechanism for all backends** (C ABI everywhere, cgo on the
   Go backend). Breaks static cross compilation on the Go backend (cgo),
   which decision 0041 avoided on purpose.
2. **Per backend foreign language, one effect discipline** (recommended):
   `use go` stays the Go backend's FFI with effect `go`; native backends get
   `use c "path/to/header.h"` (bindings generated from a C header, like
   `xo bind`) with effect `ffi`. Each is `XO1201` impossible on the other
   backend family, as `use go` is today.
3. **No user FFI before 1.0**: only std. Keeps the 1.0 surface small but
   leaves the native backends without an escape hatch.

## Recommendation

Option 2, staged, with these rules for both `go` and `ffi`:

- **Boundary types.** Scalars, `Str`, `Bytes`, `List`/`Map` of those
  (copied), tuples and flat structs of those (copied), and opaque handles.
  No pointers into Xo values; foreign code never sees Xo memory after the
  call returns (value semantics, core 5.1).
- **Effects.** Every foreign call performs its family's effect (`go` or
  `ffi`), which `pub` functions declare (as `go` today). A binding may
  narrow it with a declared capability effect when a human vouches for it
  (`use c "zlib.h" uses none`), mirroring the per dependency effect
  ceilings of decision 0086.
- **Record and replay.** A foreign call is an effect call at the boundary:
  with `--record`, its arguments' hash and its results (when every result
  type is a boundary value) are written to the trace, and `xo replay`
  returns the recorded results without calling. A call whose results
  include a handle cannot be replayed by value: it runs again, and the
  trace marks it, so `xo replay` reports "re executed foreign call
  `pgx.connect`" instead of diverging silently. Bindings may mark a
  function `replay rerun` (pure functions such as `regexp.match_string`)
  to skip recording its results.
- **Same rule for Go.** Apply the recording rule to `use go` calls too,
  replacing today's "Go calls are not recorded" (core 11.3). This is the
  change that makes replays of programs using Go libraries trustworthy.
- **Faults.** A foreign crash that can be caught (Go panic, C function
  returning an error code per binding) is a fault at the call site; a C
  segfault is not recoverable and ends the process with the crash bundle
  (native) as today.
- **Staging.** First the recording rule for `use go` (P2 work on the
  existing recorder), then `use c` for native backends with `xo bind
  --c`.

## Consequences

- Core 6.1's `ffi` row gets a meaning: the effect of `use c` calls.
  Core 4.4 and 11.3 change for recording of Go calls. Appendix B question 5
  is removed.
- The trace format gains foreign call records; old traces stay readable
  (no foreign records means rerun, as today).
- Native backends get an escape hatch without a Go runtime.
- A C binding generator (headers to Xo declarations) is new work, similar
  in size to `internal/gobind`.

## Alternatives

- C everywhere through cgo: loses static cross compilation on the Go
  backend.
- No FFI before 1.0: the native backends stay closed; reconsider only if
  the owner wants the smallest possible 1.0.
- Keep not recording Go calls: replays of programs with Go I/O diverge
  silently, contradicting design goal 7.

## Implementation notes, stage 1 (2026-10-09)

- Where: `xo bind` (internal/gobind, binding format version 5) decides a
  replay mode per function when it writes the adapter, and the adapter
  calls `rt.GoFxValue` or `rt.GoFxRerun` (std/rt/gofx.go) only when
  `rt.GoFxOn()` (a recording, ring buffers, or a replay): an ordinary run
  pays one branch per Go call. Each declaration's doc comment carries a
  `/// Replay: ...` line saying which mode applies.
- By value: every parameter and result is a boundary value and there is
  no callback. The trace effect is `go.call(<Xo name>, <hash>)`, the hash
  being the first 8 bytes of SHA-256 over the encoded arguments (the
  arguments are not stored: they may be large or secret). Results go
  through the effect codec; `go.Error` encodes its text and Go type, and
  a replayed `go.Error` answers `go_type()` with the recorded type. A Go
  panic (a fault) is recorded with `"x": "fault"` and raised again. The
  replay still enters and leaves the Go call in the scheduler
  (`goEnter`/`goExit`), so scheduling decisions match the recording.
- Refinement of the rule above: re execution covers not only results that
  hold a handle but also calls whose receiver or arguments are handles,
  and calls that take an Xo callback. A handle's Go object exists in the
  replay because the call that made it ran again; answering a later
  method call by value would skip its side effects on that object, and
  some are observable inside Go: `sync.WaitGroup.wait` returns unit, but
  answering it by value would let main run ahead of the callbacks it
  waits for (the program of `TestReplayGoCallback`; reasoned, not
  observed, since the rule was chosen before that run). Such calls record
  `go.call(<name>)` with `"x": "handle"` or `"callback"` before the call,
  and the replay checks the point and prints ``xo replay: re executed
  foreign call `<name>` (...)`` once per name.
- `replay rerun`: no wrapper, nothing recorded. gobind marks the
  functions of a fixed list of pure std packages (`strconv`, `strings`,
  `bytes`, `unicode*`, `math*`, `regexp*`, `path`, `html`, `net/url`,
  `encoding/hex`, `encoding/base64`, `encoding/base32`), except those
  taking or returning a handle of another package (`regexp`'s
  `io.RuneReader` methods). Users mark more with xo.mod lines `replay
  rerun go <Go path> [name | Type.method ...]` (no names: every
  function). A binding with such a list is cached under a name of its
  own (`<path>.rerun-<hash>.xo`) and its header records the list, so a
  changed line rebinds and projects sharing `$XO_GOBIND_DIR` do not
  overwrite each other's bindings.
- Trace format: the header gains `"foreign": 1`, effects gain the
  optional field `"x"`. A trace without `"foreign"` replays with every Go
  call run again (the old behavior), and old toolchains ignore both
  fields. Ring buffers (`XO_RING`) and crash bundles record Go calls the
  same way.
- Field getters and setters of Go structs (`GoBoundary`) and Go
  variables read through a binding are not recorded: they read Go state
  that the re executed calls rebuilt.
- Tests: `internal/gen` `TestReplayGoCalls` (os.getpid and os.read_file
  answered offline, strconv and regexp rerun, exec.command re executed
  and reported, an old trace, a changed argument, an xo.mod rerun line),
  `internal/gobind` `TestReplayModes`, `TestReadRerun`.
