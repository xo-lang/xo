# 0368: specialize a function for a constant argument (a closure of nothing, or a literal) at its call site

Status: Proposed (Xo Lang 1, 2026-10-24; for the owner via the lead;
nothing built; step 1 of the measurement plan first)

## Context

Two of today's native-vs-LLVM gaps (the bench/perf sweep of 2026-10-24,
notes sweep-native-vs-llvm-2026-10-24.txt) have one cause: a callee that
the native inliner does not take, called with an argument that is a
constant at the call site, which LLVM propagates into the callee.

- unicode fieldsfunc (1.279 of LLVM's instructions; XO_PROF: native 359
  samples against 143): `s.split_where(fn(r) { r.is_space() })` reaches
  prelude where_spans[E] (std/native/prelude.xo), which calls `f(r)?`
  once per rune through the closure value. where_spans is too big to
  inline into str_split_where, so arm64's devirtualize (a call through a
  Closure made in the same function) never sees the closure, and
  unicode.is_space runs as its own call (54 ms of samples against 8).
  LLVM inlines the chain and calls is_space directly. trimfunc's
  trim_where, any_where and the other `_where` prelude functions take a
  closure the same way.
- callbacks for (1.445): `u.name == want` in by_for, inlined into walk,
  where `want` is walk's parameter; main passes the literal "nobody".
  Native does not inline walk, so the compare is against a parameter and
  calls obj.str_eq for every name of the same length; tw411 (755ef724d)
  compares with a literal in line, but only a literal the compiler sees
  at the compare.

## Decision (proposed)

A module-level SSA pass, `specialize_args`, before inlining:

1. A call site is a candidate when an argument is a constant: a Closure
   value with no captures (a function of nothing), or a constant Str,
   Int or Bool, and the callee is a function of this module or of std
   whose body the compiler has (not an export, not recursive through
   this parameter).
2. The callee is cloned with that parameter replaced by the constant
   (`where_spans[E]#is_space`, keyed by the callee and the constant),
   once per distinct (callee, parameter, constant); every call site
   with the same constant calls the clone. The existing passes then do
   the rest: devirtualize turns `f(r)` into a direct call of the
   closure's function, the inliner takes it, fold_consts and
   str_eq_const see the literal.
3. Budget: a clone only for a callee of at most N SSA instructions
   (N to be set by step 2's measurement; INLINE_FEW_BUDGET's 500 is the
   starting point), and at most M clones per module, the largest call
   counts in loops first; the build stays correct with
   XO_PASSES=-specialize_args. Cache keys include the clones (they are
   functions of the module).
4. Not specialized: a parameter the callee stores or returns (the
   constant would escape into a value whose identity matters), and
   closures with captures (step 4 of a later decision, if wanted).

## Measurement plan

1. Before building: in a scratch copy of std, hand-clone where_spans for
   the is_space call site (a where_spans_space with `unicode.is_space(r)`
   in place of `f(r)?`, and a str_split_where that calls it), build
   bench/perf/unicode's fieldsfunc with it against the same xo with the
   unchanged std, and time both in a quiet window (`xo bench --ab`,
   instructions and wall); likewise walk cloned with want = "nobody" for
   callbacks for. If fieldsfunc's wall does not drop to about LLVM's
   (within 1.2 of it) or callbacks for's by 10% or more, stop and
   record it here.
2. Built: every golden (arm64, LLVM; XO_INLINE_STRICT=1), the stdtest
   corpora of the modules whose `_where` functions clone (strwhere,
   bytes, strings), progen, the fixed point, xo's __text native and
   LLVM (the clones' size), a cold self build's time (the pass's cost),
   and quiet windows on bench/perf unicode, callbacks, json and text.

## Consequences

- Closure-taking std functions (the `_where` family, sort with a key,
  find_where) run with their predicate inlined when the call site passes
  a function of nothing, as LLVM's builds do.
- Code size grows by the clones: the budget bounds it, and step 2
  reports it.

## Alternatives

- Make each `_where` function small enough to inline (split its loop
  into a helper per rune class): std-only, but every consumer, and the
  callbacks case is not a closure.
- A per-std-function special case (fields_where for is_space): narrow.
- Leave it: fieldsfunc at about 2.5 times LLVM's time on native.
