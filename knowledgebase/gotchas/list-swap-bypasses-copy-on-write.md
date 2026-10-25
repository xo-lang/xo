# A raw element swap bypasses the list's copy-on-write

Status: Observed (2026-10-25, DS-1, decision 0374 step 3)

## Symptom

A sort on a `var` list that swapped its elements with `rt.list_swap`
sorted another binding's list as well:

```xo
var a = [3, 1, 2]
let b = a
a.sort_in_place()     // b == [1, 2, 3], which must stay [3, 1, 2]
```

The same code with ordinary element assignments (`xs[i] = x`) behaves:
`b` keeps its order.

## Cause

A list's copy-on-write lives in the generated code of the element
*store*: the store checks the receiver's count and calls `rl_copy`
(std/native/llrt/obj.xo:770, `xo_rl_copy`) for a shared list, so the
write lands in a copy the caller then holds. `rt.list_swap` is a raw word
swap of two elements of one array (compiler/arm64/calls.xo:2145,
compiler/llvm/call.xo:1241, compiler/wasm/call.xo:185); it neither counts
nor releases, by design, and it does not go through the store's check. So
the swaps write the array both bindings share.

## Fix

Force the copy once before the first swap, with a counted write of the
element that is already there:

```xo
let first = xs[0]
xs[0] = first
```

That store is what makes the receiver unique; every swap after it is
safe. It costs one retain and one release per call. Another shape is to
give the swap intrinsic a count check, which makes every swap a little
slower and needs a new seed because it changes a landed intrinsic.

## See also

- decisions/0374-list-sort-in-place.md (step 3, which primitives it
  compares and why).
- The copy-on-write itself: std/native/llrt/obj.xo `rl_copy`.
