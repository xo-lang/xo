# arm64: a wide result read in place was overwritten by a drop helper's stack arguments

Status: Observed (2026-11-12, azure armnetwork with decision 0612's partial borrow)

## Symptom

azure armnetwork's poller-of-pager tests faulted on arm64 only (SIGSEGV
at address 0 in `inc thunk_x9`, in runtime.Pipeline.send): the Pager that
poll_until_done returned had a zeroed `pipeline` (policies, roots). LLVM
passed. XO_FREE_CHECK=1, XO_HEAP_GUARD=1 and XO_BORROW_CHECK=1 found
nothing (no freed memory was read: the words were overwritten, not
freed), and the retains and releases balanced in SSA. A `let` of the
result before returning it, or any probe, made the fault go away.

## Cause

A call's sret result of 32 words or more comes back in the caller's
outgoing (stack argument) area at sp+0 (compiler/arm64/byref.xo
sret_slot). When every use of it is in the call's block before the next
call, the caller reads it there in place, lazily (plan_sret_lazy). That
check treated only `call` and `callvalue` as calls. A `retain` or
`release` of a struct, tuple or enum wider than eight words calls its
dup or drop helper with every word (calls.xo retain and release), the
words past x7 stored at sp+0 upward, over the result. With 0612 the
caller of `Pager.starting_with` released the borrowed Pager's Pipeline
and Request after the call and before reading the result.

## Fix

plan_sret_lazy ends the in place window at any op whose operands pass
more than eight words (byref.xo stack_args_op), not only at calls. Golden
internal/llvm/testdata/golden/sret_lazy_release reproduces it (faults
with xo 0.50.1).

## How it was found

lldb on the test binary (`XO_TEST=0 lldb mini612.test`; break with
`breakpoint set -K false -n <symbol without the leading _>`): the
argument at `starting_with`'s entry and its result were intact, but the
registers fn3 returned were zero; the asm between the call and the copy
showed `mov x17, sp` stores for two release helpers' stack arguments.
A reduced azure module (one client method plus its test, 140 lines)
reproduced it deterministically where synthetic programs did not, until
the methods were kept out of line with more call sites.
