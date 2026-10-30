# std/go changes

## 2026-10-30: behavior: a Go library records the Xo line as its caller

- Kind: behavior
- Commit: this commit
- Found by: the owner's Windows testing of zap (`"caller":"rt/xo_gobind_go_uber_org_zap.go:1687"`), 1b6771a9c's caller skip
- Go reference: runtime.Caller; zap's AddCaller, log's Lshortfile, slog's AddSource
- Test: internal/gobind TestSiteForms; bench/usego-workflows/zap ("caller is the Xo line", the exact line, no add_caller_skip)
- Decision: 0441

On the Go backend a bound call runs its adapter's body in a function
literal at the call, under the Xo call's `//line` (gobind writes one
`//xo:site` record per non generic adapter; the compiler pastes it), so
a library that records its caller sees `main.xo:LINE`, not the adapter
in rt. A program that added a caller skip for the adapter frame
(`zap.add_caller_skip(1)`) now skips one frame too many and drops it.
Bindings are version 33: older ones are rebound on the next build.
Calls through an interface value and of generic functions still run the
adapter in rt. Cost on the linux/arm64 host
(bench/results/go-site-forms-2026-10-30.md): the edit loop's rebuild
wall zap 0.991 and chi 1.004 of the base (within noise), the binary the
same size, the program's generated Go 189 (zap) and 274 (chi) lines
larger (about 2% of the tree).

## 2026-10-30: fix: go.out[T]() of a List or Map allocates T's Go type

- Kind: fix
- Commit: this commit
- Found by: the owner's darwin testing of 0.35.0 (b21) and Windows testing (bug 22)
- Go reference: encoding/json Unmarshal into *[]int64 and *map[string]int64; decision 0412 point 4
- Test: internal/gen golden go_out_collections

`go.out[List[Int]]()` was a Go `*[]any` and `go.out[Map[Str, Int]]()` a
`*map[any]any`, because the Go type came from an empty Xo value, so
`json.unmarshal` into them failed or `get()` faulted ("Go wrote a
[]interface {}"). The Go type now comes from T (`*[]int64`,
`*map[string]int64`, `*map[string][]string` for nested collections), and
`get()` converts the Go value back element by element; a Go map's keys
enter the Map in sorted order.

## 2026-10-30: fix: a fault Go recovers reads as Xo's fault text

- Kind: fix
- Commit: this commit
- Found by: the owner's Windows testing (w21: net/http logged `&{7 3 105 main.xo:20:13}`)
- Go reference: net/http's conn.serve recover (logs the value with %v)
- Test: std/rt TestFaultValueText; internal/gen golden go_http_fault

The checked operations' panic values (an index out of range, an
overflow, a division by zero) render through Error and String as the
fault's message and location, so a Go recover outside Xo's runtime that
prints the value shows "index 10 out of range for List of length 3 at
main.xo:23:24". The fault's own report (AsFault) is unchanged.
## 2026-10-30: docs: a Go library that records its caller sees the binding's adapter

- Kind: docs
- Commit: this commit
- Found by: the owner's Windows testing (zap's caller pointed into generated Go)
- Go reference: runtime.Caller; zap.AddCallerSkip
- Test: bench/usego-workflows/zap (the caller is main.xo:LINE with the skip)

Every bound call runs through one adapter frame in std/rt (the
conversions and the panic boundary), so a library that walks the stack
to its caller names the adapter unless told to skip one more frame:
`cfg.build([zap.add_caller_skip(1)])`. The generated call site carries
its `//line`; nothing in the generator was missing. Spec core.md 4.4;
gotcha go-adapter-frame-caller-skip.md.

## 2026-10-29: feature: go.Recv, receiving from Go channels

- Kind: feature
- Commit: this commit
- Found by: the use go coverage survey (26 third-party channel skips:
  fsnotify's Events and Errors, go-redis's PubSub.Channel)
- Go reference: Go's channel receive and select; decision 0421
- Test: cmd/xo TestUseGoChannels; internal/gobind TestChannels

A Go channel a Go function, field or variable gives Xo is a
`go.Recv[T]`: `recv()` waits for the next value (None once closed),
`try_recv()` does not wait, and `each(f)` receives until it is closed. A
waiting receive is a cancellation point. Xo does not send on, close or pass
back a Go channel; time's timer channels are not bound.

## 2026-10-29: feature: go.error, and callbacks that return a Go error

- Kind: feature
- Commit: this commit
- Found by: decision 0416 step 1 (an Xo implementation of a Go interface
  returns Go errors from its methods)
- Go reference: errors.New; decision 0416
- Test: cmd/xo TestUseGoCallbackErrors; internal/gobind TestCallbackErrors

A Go callback parameter whose function type ends in `error` now binds:
the Xo function returns `T ! go.Error`, and its `Err` is the callback's Go
error. `go.error(message)` makes a `go.Error` (Go's `errors.New`); one that
came from Go goes back as the same Go error.

## 2026-10-29: feature: go.Any, Go's `any` in `use go` bindings

- Kind: feature
- Commit: this commit
- Found by: the Windows tester's `use go` coverage survey of v0.34.0
  (2026-10-29): `any` was 38% of std's skips and 70% of third party's
- Go reference: Go's empty interface; fmt.Sprintf, database/sql
  Rows.Scan, encoding/json Unmarshal; decision 0412
- Test: cmd/xo TestUseGoAny and TestUseGoAnyNoGoValue; internal/gobind

A `use go` binding now binds a function whose signature has Go's `any`:
the value is a `go.Any`, made with `go.any(x)` (implicit for the elements
of a list literal written for `...any`, as `fmt.sprintf("%d", [n])`) and
read with `as_int()`, `as_str()`, `as_list()`, `as_map()`, `as_handle[T]()`.
`go.out[T]()` is a Go pointer a Go function fills through `any`
(`json.unmarshal(data, v.any())`, then `v.get()`).
