# 0554: Small List literals passed to a call stay on the stack; the callee copies one it keeps

Status: Proposed (Xo Lang 1, 2026-11-07; the lead asked for the ADR with the
measured ceiling before any code)

## Context

std/metrics takes a labelled update's values as a List: `c.inc([path])`,
`h.observe(v, [path])`. bench/perf/metrics builds that one-element List on
every call, as any user would. Go's client_golang takes `...string`, whose
backing array Go's escape analysis keeps on the stack; the Xo List is a
counted heap object (decision 0046): an allocation, a push, a release and a
free per update, plus the retain and release of the label Str it holds.

After two fixes on the same rows (frame environments for closures with
counted captures, 943947579; the Family borrowed in series_of, 705f0d5e5),
the survey's top gap (bench/results/xo-vs-go-survey-2026-11-07.md,
metrics/counter 4.25 times Go) is down to about 3 times Go.

Ceiling, measured before building (host 1, linux/arm64, at main with both
fixes, 0d761985c; medians of 9 alternating runs, startup included): the
bench as it is against a copy that builds the 50 one-element Lists once,
before the loop, and passes `lists[i % 50]`:

| Row | per call List | prebuilt | ratio |
|---|---|---|---|
| metrics counter (3M inc) | 205.3 ms | 150.4 ms | 0.733 |
| metrics histogram (2M observe) | 178.6 ms | 129.3 ms | 0.724 |

Go's counter is 69.8 ms (survey), so the List is about 40% of the
remaining gap, far above the 5% bar the lead set for leaving it.

Why decision 0395's rule (small Bytes and Str on the stack) does not reach
it: the List is passed to a call. counter_inc and counter_add inline into
the caller, but series_of does not (it is large), and 0395 keeps the heap
for any argument of a call, rightly, since the callee may keep it. Here the
callee does keep it, on one path: series_of's miss path stores the values
in the new Series (once per label set, 50 times in the bench's 3 million
calls).

## Decision

Version 1, arm64 and amd64 native (a pass after the inliner, beside 0395's
stack_small); LLVM, wasm and the Go backend keep the heap.

1. **Candidate.** A `list.new` with a constant capacity of at most 4
   elements, followed only by `list.push` calls on it (a List literal of at
   most 4 elements, expr.xo ListLit), of any element type.
2. **Caller rule.** The literal's value (and its aliases, as in 0395) is
   used only by Release, the read calls (len, index, get) and as an
   argument of a call whose parameter is **unstacking** (rule 4). It is
   never stored, returned, captured, carried into a loop header's
   parameter, or retained. Then:
   - The list lives in the frame (header plus capacity words, one slot per
     site, reused per execution as 0395 rule 4 argues), its count a new
     value STACK (obj.xo, distinct from IMMORTAL, so a static literal is
     never copied), on which retain and release change nothing.
   - The literal's releases, any of which is its last (it has no Retain),
     release its elements in place (as stack_env_release does for a
     closure's captures), or do nothing when the elements hold nothing
     counted.
3. **The callee copies what it keeps.** An unstacking parameter gets a
   copy-on-keep check, `rt.rl_unstack(p)`, before every use that keeps it:
   a consuming use in insert_rc's sense (an owned argument, a store into a
   struct, list, map or box, a capture, a return, a `var` update that
   writes it) of the parameter or of an alias of it (a Retain's operand, a
   block parameter it flows into). `rl_unstack` returns p when its count
   is not STACK and otherwise a heap copy with count 1 whose elements are
   retained (the caller's frame still releases its own). Reads and
   borrowed passes keep the stack list.
4. **Which parameters are unstacking.** Only List parameters that some
   candidate in the program is passed to (a whole-program fact the native
   back end has after inlining: the call sites of each function), so a
   function no literal reaches pays nothing. The caller rule accepts only
   arguments of parameters marked unstacking, and the mark is part of the
   callee's cache key, so a cached or separately compiled body without the
   checks (decision 0283's undecoded bodies, a std object built before the
   change) is never handed a stack List.
5. **Indirect calls** (a function value, an interface method) are never
   unstacking: the callee is not known.
6. **Verifier.** As 0395 rule 5: after the later passes, every stack List
   is checked again against rule 2 and every unstacking parameter's
   function against rule 3; a violation fails the build naming the
   function and value.

## Consequences

- metrics counter and histogram: up to the ceiling above (0.733 and
  0.724), less the frame stores of the literal; series_of's hit path
  keeps nothing, so it runs no rl_unstack.
- A function with unstacking parameters pays one load and compare of the
  count word on each keeping use, on those paths only.
- Every other call site that passes a small List literal to a function
  that is not inlined gets the same treatment; how many there are in std
  and in xo itself is to be counted (a dev listing of candidates) before
  the implementation's measurements.
- A List that is kept is built twice (in the frame, then the heap copy),
  on the keeping path only; series_of's miss runs once per label set.
- Verification: goldens on every backend (the stack List and the copy
  print the same), a codegen pattern (the bench's loop has no list
  allocation or free call), a golden whose callee keeps the List on some
  calls and not others, one that keeps an alias (a Retain then a store),
  and the fixed point.

## Alternatives

- **A single-value metrics API** (`c.inc1(path)`, or the held children of
  decision 0501, which already exist): fixes the bench only, and users must
  find it; kept as the fallback if this decision is rejected.
- **Static escape analysis alone** (stack only when the callee never keeps
  the parameter): series_of keeps it on its miss path, so it never fires
  here; it is rule 3 without the copy.
- **A stack bit checked at every retain** (the copy made wherever a stack
  object is retained): every retain in the program pays the check; rule 3
  limits it to the keeping uses of parameters a literal reaches.
- **Inlining series_of**: 883 lines of arm64 for a lookup with a cold
  insertion path; the inliner's size rules refuse it, and splitting it by
  hand only moves the call.
