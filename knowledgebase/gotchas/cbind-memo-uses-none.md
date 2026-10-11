# cbind: `ffi` and `uses none` shared one binding in a process

Status: Observed (2026-10-11, SSA stage 6, found by the self hosted
checker's port of `use c`)

Symptom: a program with `use c "tiny.h" link "tiny.c"` and `use c
"tiny.h" link "tiny.c" as quiet uses none` bound both lines to the
same module (`rt.c_tiny_425a8eec_*` for both); only one declaration
file was written to `.xo/cbind`. The `uses none` line got the `ffi`
binding, so its calls carried an effect the line declared away; in the
other order an `ffi` line would get the binding without effects.

Cause: `cbind.Bind` memoizes bindings per process under a hash of the
header, links, effects, and cache directory, formatting `opts.Uses`
with `%q`. A nil slice (no `uses`: `ffi`) and an empty one (`uses
none`) both print `[]`, so the second line found the first line's
binding. The binding's own key (`Binding.Key`) did tell them apart.

Fix: the memo key also says whether Uses is nil
(internal/cbind/cbind.go Bind); test `TestBindMemoUsesNone`. When a
hash or cache key is built with `fmt`, check that every distinction the
value carries survives the formatting (nil against empty, a pointer
against its target).
