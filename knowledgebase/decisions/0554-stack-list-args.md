# 0554: Small List literals passed to a call stay on the stack; the callee copies one it keeps

Status: Accepted with addendum (owner, 2026-11-07); proposed by Xo Lang 1 the
same day, the lead asked for the ADR with the measured ceiling before any code

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
   function no literal reaches pays nothing, and that some path of the
   function does not keep (decision 0149's consumed on every path, for
   Lists): a callee that keeps its List on every path (the compiler's
   `b.add(op, t, aux, [args])`, which stores its arguments in the value)
   would only pay a frame build and a copy. The caller rule accepts only
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

### As implemented (prototype, 2026-11-07): changes from the text above

The prototype (compiler/arm64/stacklist.xo, arm64 only, pass
`stack_lists`) differs from rules 1, 3 and 4 in three ways; these change
what was accepted and are reported to the lead with their numbers before
landing.

- **Element types.** Only List[Str] and List[List] literals and
  parameters (stack_list_type): their elements are whole words the frame
  slot stores as they are. Other element types keep the heap.
- **Every function protects its parameters (rule 4 replaced).** Each
  function's List[Str] and List[List] parameters with a consuming use get
  the copy-on-keep check, whether or not a literal reaches the function,
  and a caller passes a literal to any module function. Rule 4's
  whole-program mark would make a function's body depend on its callers,
  so the inline cache (which serves a function's post-inline body by its
  own key) would hand a warm build a body compiled for a different set of
  callers than a cold build: a stack List could reach a body without the
  checks. Protecting every function keeps each body a function of its own
  key. The cost moves from "functions a literal reaches" to "every
  function with such a parameter and a keeping use"; its size is in the
  measurements below.
- **Where the check goes (rule 3 refined).** A parameter with one
  keeping use, owned or borrowed, is copied right before that use
  (`rt.rl_unstack`), so the paths that keep nothing (series_of's hit)
  copy nothing. For an owned parameter the copy consumes it and every
  later use names the value that reaches it: the copy where only the
  copy's paths reach, the parameter where none does, and a new block
  parameter where both meet (stacklist.xo redefine, SSA repair of one
  redefinition), so a fault's shared cleanup releases one value on every
  path. An owned parameter with several keeping uses, or one that is
  retained, is copied once at the entry instead (the first prototype did
  this for every owned parameter: series_of then copied on every call and
  the metrics counter measured 0.987, noise, against the 0.733 ceiling).
  A borrowed parameter's copy takes the retain insert_rc put before the
  use; a single-predecessor landing pad's release moves to the copy.
  `rt.rl_unstack` is an inline count compare with a call
  (`xo_x_obj_rl_unstack_n`) only for a frame list.
- **Verifier (rule 6).** The existing RC verifier checks the result:
  `rt.rl_stack` and `rt.rl_stack_put` are immortal roots (imm_at),
  `rl_stack_put` consumes its element, `rl_unstack` consumes its operand;
  the self build verifies under XO_INLINE_STRICT=1.
- **Landing pad releases (rule 1 widened).** A literal whose element
  expression can fault has the list made so far released in that
  element's landing pad, so the single-use rule rejected it (the metrics
  counter's `[value]` among them: the first measurements ran with the
  pass taking no literal there). Each value of the literal's chain may
  now have releases besides its next push; they become
  `rt.rl_stack_drop K` for the K elements put by then.
- **Var parameters.** A literal passed as a callee's `var` parameter is
  not a candidate: the callee changes it in place, and a push there
  grows the frame list and frees its frame memory (found by the self
  build: closures.xo env_drop's `var b: List[Str] = [...]` reached
  frame_down's push once landing pad releases were allowed).

## Addendum (owner, 2026-11-07): the local rule as built, and its numbers

The owner accepted the prototype's local rule in place of rule 4's
whole-program mark, on the follow-up numbers below. As built
(compiler/arm64/stacklist.xo, arm64 and amd64 native backends):

- **Every function protects its parameters.** Each List[Str] or
  List[List] parameter with a keeping use (stored, captured, returned,
  consumed by a call that keeps it) is checked before it is kept,
  whether or not a literal reaches the function, so a function's body
  depends only on its own inline cache key.
- **The copy sits at a single keeping use.** With one keeping use the
  `rt.rl_unstack` goes right before it, and for an owned parameter the
  SSA is repaired (on-demand block parameters where the copied and the
  original value meet), so a path that keeps nothing copies nothing;
  several keeping uses, or a retain, copy once at the entry.
- **Landing pad releases are allowed in the literal's chain.** An
  element expression that can fault releases the list made so far in
  its landing pad; those releases become `rt.rl_stack_drop K` for the K
  elements put by then. Without this the metrics counter's `[value]` was
  never taken (the first measurements below ran with the pass taking no
  literal there).
- **A literal passed as a `var` parameter is excluded.** The callee
  changes it in place, and a push there grows the frame list and frees
  its frame memory (the self build found it: closures.xo env_drop's
  `var b: List[Str] = [...]` reached frame_down's push).

Numbers (`xo bench --ab` on the first linux/arm64 host, base presize
b686b0392, which made `[x]` at its size, against presize plus this
rule 87bec9ab7):

| row | wall | 95% interval | n |
|---|---|---|---|
| bench/perf/metrics counter | 0.760 | 0.747 to 0.794 | 6 |
| bench/perf/metrics histogram | 0.756 | 0.738 to 0.773 | 6 |
| xo self build | 1.011 (noise) | 0.999 to 1.034 | 7 |
| xo self build cpu | 1.007 (noise) | 0.992 to 1.019 | 7 |
| xo self build max RSS | 0.999 (noise) | 0.989 to 1.004 | 7 |
| regexp compile | 0.996 | 0.987 to 1.0 | 7 |
| path join | 0.994 (noise) | 0.979 to 1.015 | 7 |
| template render | 1.001 (noise) | 0.998 to 1.005 | 6 |
| strsplit split | 0.950 (noise) | 0.9 to 1.026 | 7 |

`rt.rl_unstack` runs 3,720,660 times in one self build of xo by the
built xo (an instrumented scratch build counting every call), and copies
a frame list 0 times: the compiler pays only the inline count compare.
An earlier n=3 self build run read 1.012 (1.012 to 1.049), so the cost
is at most about 1 to 2% of xo's wall, against 24% on the metrics hot
path. xo's __text grew 0.15% at the first prototype (37,630,892 to
37,686,848 bytes, 307 rl_unstack sites); it was not measured again after
the two fixes, and no byte-identity run was made for a program with no
candidate (every function with a keeping parameter changes).

## Consequences

- metrics counter and histogram: up to the ceiling above (0.733 and
  0.724), less the frame stores of the literal; series_of's hit path
  keeps nothing, so it runs no rl_unstack.
- A function with unstacking parameters pays one load and compare of the
  count word on each keeping use, on those paths only.
- Every other call site that passes a small List literal to a function
  that is not inlined and does not always keep it gets the same
  treatment. A text count (2026-11-07, lines with a bracketed literal
  argument, slices included, so an upper bound) is 183 lines in std and
  2,551 in the compiler, most of the latter `b.add(..., [args])` calls,
  which keep their List on every path and are excluded by rule 4; the
  implementation lists the sites the rule takes before measuring.
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
