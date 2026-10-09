# A lazily formatted error frame can print a value changed after the `?`

Status: Observed (2026-10-09, while implementing decision 0092 on the Go
backend).

## Symptom

With frames that keep parameter values and format them only when the
trace is printed, a caller that changes its arguments after the call
returned an error, and later propagates that stored error, saw the new
values in the frame:

- `fn pair(a: Int, xs: List[Int])` called with the caller's `var ys`;
  after `ys[0] = 9` the frame printed `xs=[9, 2, 3]` instead of
  `[1, 2, 3]` (golden `error_frames_borrow`). The same for a `var`
  List parameter written by the caller afterwards
  (`error_frames_capture`).
- A `var b: Buf[Int]` parameter frozen by the caller, whose List was then
  written, printed `Buf[55, 1]`; an `Atomic[Int]` set afterwards printed
  its new value.

## Cause

Copying the value into the frame is not enough when the copy shares
memory that can still be written in place. A List passed to a plain
parameter is lent (a borrow, `internal/gen/own.go`), and a `var` List
parameter is the caller's own list: neither block is marked shared, so
the caller's next element write happens in place, under the frame.
`Buf.freeze` hands its array to a new, unshared List. An Atomic's Debug
text is its current value.

## Fix

`internal/gen/expr.go` `frameValue`: every captured value goes through
`ownCode` (marks its Lists shared, so the next write in place copies),
and the types whose Debug text can change in place (`gen.frameEager`:
`Buf`, `Atomic`, anything holding them, unspecialized type parameters)
are formatted at capture, bounded to 65 bytes (`rt.FrameTextOf`). A new
mutable runtime type with in place writes, or a Debug text that reads
live state, must be added to `frameEager`. The two goldens fail if
either step is removed.
