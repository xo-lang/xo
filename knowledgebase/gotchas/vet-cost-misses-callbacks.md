# `xo vet --cost` says nothing about callback walks

Status: Observed (2026-10-22, Xo Publish, while measuring decision 0337).

## Symptom

`xs.any(fn(u) { u.name == want })` over a list of structs with string
fields costs 3.3 times the instructions of the same `for` loop (5.5
times on a list shared by two tasks), and `xo vet --cost` reports no
note for it, nor for `for u in xs { t += f(u) }` with `f` a `fn`
parameter.

## Cause

XO0908 (compiler/ssa cost.xo elem_count_per_iter) looks for a Retain
whose retained value a CallValue uses. A struct element is retained
field by field (`retain` of `field 1` and `field 2`) while the CallValue
takes the struct, so the test never matches. And the prelude's callback
methods (list_any, list_filter, list_map, list_fold) hold the loop in
the prelude, which cost.xo does not read: it notes the root module's
functions only.

## Fix

Until decision 0337 (function values borrow their arguments) is decided:
treat a callback over a list of heap holding structs in a hot path as
counting each heap field up and down per element; write a `for` loop
there, which borrows (decision 0186). Check a function with
`XO_DUMP_FN=NAME XO_PASSES=-inline xo build --backend=arm64` and count
the `retain` lines in its loop. If 0337 is deferred, XO0908 should follow
a field's Retain to its element and note the prelude callback's call
site.
