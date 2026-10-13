# 0211: alias scopes for lists on LLVM

Status: Accepted (owner, 2026-10-13, as part of the zero-C work);
implemented 2026-10-13 (selfhost/llvm noalias.xo, mirrored in
internal/llvm noalias.go). Argon2id and BLAKE2b in Xo are the default
again (C with `XO_LLRT_C=digest`).

## Context

Every Xo list is a pointer to its object in the LLVM IR, and its
elements are loads and stores through it. LLVM cannot tell that the
stores to one `var List[Int]` parameter leave alone the words another
one reads, so the Argon2id in Xo (argon2_g's `bs`, `t`, `q`) stayed
1.155 times the C's instructions on LLVM while arm64 reached 0.980
(bench/results/raw-memory-2026-10-13.md, gotchas/
llvm-list-params-block-vectorizing.md), above decision 0180's bar (both
backends within 5 percent of the C). The language rules out the
aliasing (spec/core.md 5.1, 5.2: values never alias mutably; XO0503
forbids one variable to two `var` parameters), but the IR does not say
so, and the native backends implement value semantics by counting
references and copying on write, so two parameters can hold one object.

## Decision

Both LLVM emitters give list accesses LLVM alias scopes
(`!alias.scope` and `!noalias` metadata, scopes declared at the entry
with `llvm.experimental.noalias.scope.decl`), never `noalias` on the
parameters themselves.

1. **Classes.** A union-find over a function's List, Str, and Bytes
   values:
   - a U class starts at the `reuse` of a parameter (any List
     parameter) or at a new list (`list.new`, `rt.rl_empty`), and takes
     the values updated in place from it (`list.set`, `list.push`, a
     further `reuse`), block parameters that join only its values, and
     the `var` result of a direct call whose callee hands back the
     object it was given or a new list (item 3);
   - the RO class holds the borrowed List, Str, and Bytes parameters
     (not `var`, not passed by reference) and block parameters joining
     only them;
   - any other value (a raw parameter before its reuse, a field, an
     element, a tuple's part, another call's result) has no class, and a
     class joined with such a value, or holding RO and U values, has
     none either (poisoned).
2. **Scopes.** Each class with an access gets a scope (the RO class one
   for all of its values), numbered in the order of the accesses. A list
   access (list.len, index, get, set, push, pop, take) or a Bytes read
   (bytes.index, rt.bytes_word, rt.bytes_eq16, rt.bytes_high16) through
   a value of a class carries `!alias.scope` of its scope and
   `!noalias` of every other scope of the function; writes are scoped
   only through a U class. A function with fewer than two scopes gets
   nothing. The nodes (a distinct domain, distinct scopes, their lists)
   follow the functions, numbered from 100 * (function index + 1) (at
   most 32 scopes a function); SplitIR keeps them as `Meta` lines and
   the module splitter copies them into every module.
3. **Summaries.** Per function and `var` parameter: every return hands
   back the parameter itself, a new list, or a value of a U class whose
   only parameter root is that parameter. Computed for the whole module
   from all false until nothing changes (each only turns true, so a
   recursion stays false unless the induction holds).

The proof, in one call of the function, from two facts the reference
counting already guarantees and every in place update relies on: (a) a
list is written in place only through a value that holds its only
reference (each update follows a `reuse`, which copies a list whose
count is not 1, or a shared list another thread may hold); (b) a
borrowed parameter is backed by a reference its caller keeps for the
whole call, and every `var` or owned argument is a reference of its own
(a caller passing one list both ways retains it first; checked by the
noalias_lists golden).

- Two U classes never reach one object. Their objects are their
  parameter's own object, kept by a `reuse` that found the count 1, or
  objects allocated during the call (copies, grown lists, new lists, and
  the shared empty list, which no operation writes: an update grows it
  first). Two parameters holding one object both count it, so at most
  one of their reuses keeps it, after the other copied. Values from
  outside a class poison it instead of joining, so no other path leads
  an object into a second class; the summaries admit only callees that
  hand back the given object or a new one.
- U and RO never reach one object: an RO object exists before the call,
  and its caller's reference keeps it counted for the whole call, so no
  reuse finds it unique and no allocation of the call is it.
- The RO class is one scope; its accesses are reads.
- An object freed and another allocated at its address are different
  objects to LLVM, and calls (the release, the allocation) separate
  them.

## Consequences

- Argon2id in Xo, less setup over the C (instructions retired, Apple
  M-series, bench/perf/digest; bench/results/llvm-noalias-2026-10-13.md):
  LLVM 1.154 before, 1.132 with the scopes, 1.043 with the scopes and
  one source change (argon2_g checks `o + 127 < bs.len()` and
  `127 < q.len()` again after the permutation calls, so LLVM drops the
  index checks of the final xor and vectorizes it with `eor3`); arm64
  0.980. Decision 0180's bar holds on both backends, so Argon2id and
  BLAKE2b in Xo (commit a4168f52, `raw.zero_ints` for the zero lists)
  are the default again; `XO_LLRT_C=digest` builds the C
  (`xo_crypto_argon2id` back in IOFlags' list, both drivers).
- The same std without the scopes is 1.107 on LLVM: the guard alone
  does not do it.
- No other bench/perf workload moves by more than 1.5 percent on LLVM
  (scopes on over off; the system call heavy fsproc and httpparse vary
  most, proc 0.986, h1_get 0.988; results file);
  the gain is for code that writes one list while reading another
  through parameters or new lists.
- Compiler internal; no language or output change. The LLVM IR of
  every function with two scopes changes, so the recorded LLVM goldens
  (internal/selfhost/testdata/gogoldens/llvmdump) need a new recording.
- internal/llvm/testdata/golden/noalias_lists runs the same lists through
  two parameters, maps, closures, slices, calls that hand back their
  list or another one, and a loop rotating three lists, on the Go,
  LLVM, arm64, and wasm backends.

## Alternatives

- `noalias` on the LLVM parameters: unsound. Two `var` parameters (or a
  `var` and a borrowed one) can hold one object until the first reuse
  copies it, and the raw parameter is read (its count) and passed to the
  copy before that.
- Scopes on raw parameters too: unsound for the same reason: a raw read
  through one parameter and an in place write through another (after
  the first copied and released) would be claimed apart.
- `!noalias` on calls for the U classes whose values never leave the
  function's list operations ("sealed"; a callee cannot reach their
  objects): measured, not kept. It lets LLVM keep values across calls:
  Argon2id's xor written as plain loops went to 0.869 of the C on LLVM
  (and 1.400 on arm64), but the unrolled form went from 1.044 to 1.093
  in one run
  (stored words forwarded across the 16 permutation calls, then
  spilled). A later decision can bring it back with arm64 loops on par.
- String identified scopes (no numbered metadata): LLVM's inliner
  clones scopes by rebuilding their nodes, which gives the same uniqued
  node for a string scope, so two inlined calls would share scopes.
- Rewriting argon2_g's xor parts as loops: 0.869 on LLVM only with the
  call metadata; 1.129 without it, and 1.400 on arm64 either way; loops
  unrolled by 8 were worse on both (1.374 LLVM, 1.766 arm64).
