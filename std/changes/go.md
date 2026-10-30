# std/go changes

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
