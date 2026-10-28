# A label just before a function's label is in the atom before it

Status: Observed (2026-10-28)

## Symptom

An XO_STD_ROOTS (std store) build of `async_preempt_alloc` crashed with
SIGSEGV in `alloc.alloc` (reached from `obj.reserve_slow`) in 2 of 40
runs, a corrupted heap; the same program built normally passed 40 of 40.
The store gates had passed it 5 of 5 several times on a quiet machine.

## Cause

compiler/arm64 push_fn_text brackets a runtime function whole for
asynchronous preemption: `L..._du<2k>:` (the unsafe range's start), then
the function's text with its own label, then `L..._du<2k+1>:`. The start
label comes before the function's label. A normal build's table is per
file (offsets from the file's first text label), so that is harmless.
The store build's per-function tables (apreempt.xo unsafe_fn_tables)
split the labels at each function label, so the start went to the
function before. That function got an unpaired label. The bracketed
function got only its end, so it had no table at all: every bracketed
runtime function, the allocator among them, could be preempted
anywhere. In atoms mode (`.subsections_via_symbols`) such a label is
also in the atom before the function, which the link may place anywhere.

## Fix

unsafe_fn_tables gives a start label (an even `_du` number) with no
instruction between it and the next function's label to that function,
and its range starts at the function's label. An end label there stays
with the function before. The index grew from 402 to 410 functions;
`async_preempt_alloc` store builds passed 60 of 60.
Generally: in atoms mode, never let a label meant for a function
precede the function's own symbol.
