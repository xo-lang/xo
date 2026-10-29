# A layout order set on the emitter's loop changed nothing: the parallel path forms the parts by index

Status: Observed (2026-10-29, xo-lang-6, decision 0403 stage 1).

## Symptom

`xo build --layout` printed "1 of 1 sampled functions matched, 563
functions ordered" and the binary's bytes differed from the build
without the file, yet `nm -n` listed the functions in the same order:
the hot function had not moved.

## Cause

Two paths produce the text. gen.xo's program loop (one part) and
fcache.xo's parallel path, which compiles the functions in parallel and
groups them into parts by index (parallel.xo part_lists, `for (fi, f) in
m.funcs.indexed()`); the parts' order is the output's. The order on the
emitter's loop reached only the one-part path; the bytes differed
because the layout file's hash lengthened the build info. The link side
had the same shape twice: the link context copied the order before the
file was read (an empty list), and the atom lookup matched a symbol's
value against the section's address, which a store atom's never equals.

## Fix

One order for both paths: parallel.xo fn_order_of, used by part_lists
and gen.xo. The link context takes the order at the link. An atom is
named by any defined symbol of its section. Each proven by nm -n on the
built binary, not by the bytes differing.

## Rule

An ordering change is proven by the order (nm -n, the symbol list), not
by "the bytes changed": the build id changes with any new input. A
value handed through a context is set after its source is read.
