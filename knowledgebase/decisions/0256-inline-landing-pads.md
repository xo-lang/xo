# 0256: inlining functions with landing pads

Status: Accepted (Xo Lang 2, 2026-10-17; the lead's queue). Supersedes
the "no landing pads of its own" rule of decision 0085's arm64 inliner
(compiler/arm64 inline.xo), for the calls this decision names.

## Context

Decision 0085 gave every value that can fault a landing (`Value.pad`, the
check after it), whose cleanup path releases what the function holds and
ends in the function's unwind exit, which raises the fault again
(`rt.fault_continue`) toward the caller's landing. The arm64 and amd64
inliner (decision 0184's cost model) took no function with landing pads
of its own: inline_call gave every copied value the call's landing, which
is right only when the callee holds nothing when it faults.

That kept generics over closures out of line: list.map and list.filter
call their function argument, which may fault, so the call carries a
landing pad. hof (`xs.map(fn).filter(fn).fold(0, fn)`) stayed 1.70 times
LLVM on arm64 after closures made in the caller became direct calls
(bench/results/amd64-vs-llvm-2026-10-17.md): fold, which has no pad, was
inlined and its closure call made direct, but map and filter were not.

## Decision

1. inline_call copies a callee's landing pads: a copied value whose
   callee value had a landing gets the copy of that check (set after the
   copy, since a check follows what it guards); a value without one gets
   the call's landing, as before. The callee's cleanup paths come along,
   and its unwind exit's `rt.fault_continue` has the call's landing, so a
   fault in the copy runs the callee's cleanup, then the caller's, as it
   did across the call.
2. A function with landing pads is inlined only at a call that passes a
   closure literal of no captures made in the caller (find_closure_call),
   within the loop limits (60 instructions, 24 blocks): there the copy
   lets the closure call become direct and be inlined in turn. Inlining
   every function with pads made hof 24% and maxby 16% faster but grew
   get 4%, push 5%, iter 9% and the binary 8%.
3. Not a function whose pads include an operation that faults before it
   takes its operands (list.set and list.take, keeps_operands): its
   cleanup releases those operands only when their root is owned, and the
   callee's owned parameter can be a borrowed root of the caller
   (retained before the call), so the copy's cleanup released a value the
   verifier saw as not held (the compiler's own modules.sort_diags,
   through list_insert). Relaxing the rule for borrowed roots in both RC
   and the verifier broke ordinary functions (arm64.generate_cached), so
   the exclusion stays.
4. Functions that catch their faults (`Func.catches`, task bodies) stay
   out of inlining entirely (2026-10-17, the closures change).

## Consequences

- On arm64 (instructions retired, the Mac): hof -23.7%, maxby -15.8%,
  the other collection workloads within 0.3%, the binary the same size.
- The module verifier checks every inlined function (XO_INLINE_STRICT=1
  in the tests and in the bootstrap's s2 and s3), so a pad copied wrong
  fails the build instead of running.
- The faults and deferred code goldens (fault_*, contract_*, conc_*)
  exercise the copied cleanups.

## Alternatives

- Specialize a generic on its closure argument before reference counting
  (a clone of list.map with the closure's call direct, whose landing then
  goes when the closure cannot fault): more machinery than copying the
  pads, and the copies do the same at closure sites.
- Inline every function with pads: rejected for the code growth above.
- Treat retained borrowed roots like owned ones at operations that keep
  their operands: rejected after it failed verification of ordinary
  functions.

## Addendum: one call site (Xo Lang 2, 2026-10-20; reverted the same day)

Reverted (2026-10-20): the arm64 backend's DWARF describes no variables
of an inlined function (compiler/arm64 dwvars.xo skips a variable with
an inline site), so a fault report's locals (decision 0268) lost the
inlined callee's (internal/llvm TestFaultLocals/arm64: step inlined into
main listed none). It lands again with inlined variables in the arm64
DWARF. As landed:

A function with landing pads and exactly one call site is inlined there
too, up to INLINE_ONE_PADDED_INSTRS (150) expanded instructions, unless
its body updates a list in place (list.set, push, pop, take): the copy
is the only body, so no hot loop gains a second one. The in-place rule:
copied, those updates fail the verifier's uniqueness proof (sums_of's
pushes inside a task scope, and one inlined into arm64.inline_cached,
broke xo's own build under XO_INLINE_STRICT=1). The cap: without it the
HTTP server's native text grew 4.6% for 1.5 to 2.2% fewer instructions
a request; with 150 it grows 1.0% for 0.7 to 1.9% on plain, plain-newconn
and tls (bench/results/inline-one-site-pads-2026-10-20.md). Found with
XO_INLINE_REPORT (each function's cost, sites, and how it is taken).
