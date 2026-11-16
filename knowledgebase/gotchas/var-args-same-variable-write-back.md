# Two `var` arguments from one variable: the native backends kept only the last write

Status: Observed (2026-11-15, Xo Lang 3's brotli encoder; fixed the same day, task #70)

## Symptom

`set_both(s.a, s.b)`, with `var` parameters, left `s.a` unchanged and
set `s.b` on arm64 and LLVM; the go backend set both. No diagnostic. The
same for any two disjoint places of one variable that decision 0111
allows: two fields (also nested, also through a `var` parameter), Int or
tuple fields, two literal list indexes, a `var` receiver on one field
with a `var` argument from another (`h.t.swap_in(h.w)`), a call through
a function value, three fields at once (only the last survived). std/brotli's
`command_prefix_code1(histo, fe.cmd_depth, fe.cmd_bits, fe.cmd_code)`
lost its depth and bits updates, so multi-fragment quality 0 streams
were invalid.

## Cause

compiler/ssa lowers a `var` argument by copying in and out (spec 5.2):
the call returns each place's new value and the caller stores it back
through the place (Place, place_set). A Place caches the value of each
level it read before the call (the Go place's cur() cache). Each write
back rebuilt the variable from its own cache, the variable as it was
before the call, so the second store replaced the first: `s =
SetField(old s, b, new b)` after `s = SetField(old s, a, new a)`. The
go backend passes pointers, which share the variable.

## Fix

call.xo write_back (direct calls and calls through function values): a
place whose root variable an earlier place of the same call already
stored to drops its cached levels and its taken elements and reads the
variable again, so each store starts from the previous one's result.
Places of different variables keep their caches. Golden
internal/llvm/testdata/golden/var_args_disjoint_places (every backend).

## How to avoid

A lowering that copies in and out must thread the stores of one call
through the variable they share; a cache of the variable taken before
the call is stale after the first store. Test any new place shape with
two places of one variable, not one.
