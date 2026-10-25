# 0368: specialize a function for a constant argument (a closure of nothing, or a literal) at its call site

Status: Accepted (owner, 2026-10-24 ~18:15, after step 1 passed; built
the same day, see "As built").

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

## Step 1 result (2026-10-24)

One xo (tw417, with f6e2fd3d5's split_where change), only std or the bench
copy differing; quiet `xo bench --ab`, 15 rounds, load 1:

| row | wall | instructions |
|---|---|---|
| fieldsfunc, where_spans hand cloned for is_space, against f6e2fd3d5's std | 0.661 (0.644 to 0.668) | 0.813 |
| fieldsfunc with the clone, against LLVM (A) | 0.931 (0.922 to 0.943): native faster | 1.019 |
| fieldsfunc without it, against LLVM (A) | 1.446 | 1.253 |
| callbacks for, walk passing the literal "nobody", against the bench | 0.806 (0.791 to 0.831) | 0.739 |
| callbacks for with the literal, against LLVM (A) | 1.357 | 1.063 |
| callbacks for without it, against LLVM (A) | 1.685 | 1.437 |

Both bars cleared: fieldsfunc from 1.45 of LLVM's wall to 0.93 (the bar
was within 1.2), callbacks for 19% faster (the bar 10%). What callbacks
for keeps against LLVM (1.36 of its wall at 1.06 of its instructions) is
not the loop shape (decision 0367's step 1 found that free on M-series);
to be profiled with the pass built.

## As built (2026-10-24)

compiler/ssa specialize.xo, `Builder.specialize_args`, run after the
cleanup and before the reference counting (in run_passes, and in xo's
lower after passes_off is read), so the reference counting's and the
inliner's per-function caches see the clones as ordinary functions; off
under XO_PASSES=-specialize_args and on wasm. It applies to every
backend but wasm (LLVM gets clones too, which its own propagation would
have found).

- Candidates: an `Op.Call` to a function of the module (func_at) that is
  not the caller, has a body of at most SPEC_MAX_VALUES (600) values, is
  not called through values (indirect), not an entry or an export, has
  no captures, and does not call itself; the parameter an entry
  parameter that is not `var`; one constant parameter a call.
- Constants: a Closure with no captures (keyed by its function's name),
  or a Str or Bytes literal (keyed by its bytes) when the callee compares
  that parameter: an operand of str.eq, str.cmp, Eq or Ne, in the callee
  or in a callee it passes the parameter to, three levels down (walk
  passes `want` to by_for, which compares it). Int and Bool literals are
  not candidates: the runtime passes them at most call sites
  (budget.charge's count), and with them the first build spent all 256
  clones on runtime functions with no gain; Str literals that a callee
  only passes on (fault locations: llrt_str_slice_chk's) likewise spent
  the budget until spec_compared.
- Closures first, then literals, each over the module and the clones as
  they are added (a clone's constant reaches its own calls:
  str_split_where's closure clones where_spans). At most SPEC_MAX_CLONES
  (256) a module; one clone per (callee, parameter, constant), named
  `callee#specN`, its cache key cleared.
- The clone: the callee's copy with the constant's operation at the
  start of the entry block and every use of the parameter replaced; the
  parameter stays (unused), so call sites keep their arguments. A
  retain or release of the constant is of an immortal value (rc.xo
  imm_value: a Str constant, a closure of nothing).
- bench/perf/unicode gets 5 live clones (where_spans, trim_where,
  map_runes twice, dwline inline_chain). Unquiet instructions, the same
  xo with the pass off as A: unicode fieldsfunc 0.842, trimfunc 0.801,
  upper 1.0; callbacks for 0.740, any and ifor 1.0; json small and maps
  1.0.
- Measured (2026-10-24, after the full tier): quiet `xo bench --ab`, one
  xo, the pass off (XO_PASSES=-specialize_args) as A, 15 rounds, load 3:

  | row | wall | instructions |
  |---|---|---|
  | unicode fieldsfunc | 0.823 (0.804 to 0.832) | 0.842 |
  | unicode trimfunc | 0.852 (0.829 to 0.86) | 0.800 |
  | callbacks for | 0.816 (0.8 to 0.839) | 0.740 |
  | json small / maps | 0.989 / 0.989 | 1.0 / 1.0 |
  | bytessearch index | 1.012, noise | 1.0 |
  | unrelated: rsa sign2048, digest sha256_big, text time_fmt and float_fmt, sort ints and strs | 0.996 to 1.003, all within noise | 0.999 to 1.0 |

  xo's own __text (s2 of the fixed point): 59,747,540 with the pass,
  59,670,844 without, +76,696 bytes (+0.13%). The gate: the ssa and arm64
  xo tests, the codegen patterns, every arm64 golden and the amd64 cross
  builds, every LLVM golden with Go parity, every stdtest corpus on arm64
  and LLVM, cmd/xo, the fixed point (no inline fallbacks), the Docker
  amd64 sample (54 of 54), and golden specialize_args (a closure down a
  chain, a compared literal two calls down, non-constant arguments, a var
  parameter, recursion, a closure with captures). `xo vet --cost` skips
  the clones (they repeat their originals' lines) and names a call of a
  clone by its callee (compiler/ssa cost.xo).

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

## Addendum: literals only where nothing else uses them (2026-10-24)

Sizing native code for the Xo Lead (size step 2) found 45 `#spec` clones
in a release bench/perf/http/xo. They took 63,008 bytes, 60,612 of them
copies of a Map's generated operations: `xo.map.put`,
`xo.maph.slowput`, `xo.mlog.record` and the functions they call, made
for the constant Str keys of `m["id"] = ...`. Those operations compare
the key, so spec_compared took them, but they hash and store it first,
and the literal folds nothing there.

The rule is now spec_uses (compiler/ssa specialize.xo), as the Lead
asked: a general rule, not a list of callees.

- A Str literal clones the callee only when it compares the parameter
  and every other use is neutral.
- Neutral uses: a Str.len, a closure's capture, and passing it to a
  callee whose own uses are neutral or compare, three levels down.
- Any other use counts against: a hash, a store, or a call of a function
  with no body.

Results, the old rule (main's xo, 3a435412a) against the new one,
release builds, minimum of 5 runs:

| row | before | after |
|---|---|---|
| bench/perf/http/xo __text | 1,519,088 (45 clones) | 1,467,848 (1 clone) |
| unicode fieldsfunc instructions | 176,267,676 | 176,269,610 |
| unicode trimfunc instructions | 52,830,942 | 52,793,159 |
| callbacks for instructions | 189,292,852 | 189,159,622 |
| callbacks any instructions | 940,045,937 | 940,012,388 |
| xo itself, s2 of the fixed point, __text | 60,128,952 | 59,996,112 |

The callbacks and unicode binaries keep their clones and their size.
The first cut, compares only with every use a compare, lost callbacks
for's clone (189 to 256 million instructions): walk also hands `want`
to by_any's closure. A size cap does not separate the cases either: map
find has 48 SSA values and walk 131. compiler/ssa specialize_test.xo
covers the rule.
