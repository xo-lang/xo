# 0374: List.sort_in_place, a stable sort that mutates the receiver

Status: Proposed, steps 1 and 2 built (DS-1, 2026-10-25: rt.list_swap in
four lowerings with a caller on every backend and the golden
native_list_swap, then `List.sort_in_place` and `sort_in_place_by` in
std/prelude.xo, the native merge with half a list of scratch, the Go
backend's `slices.SortStableFunc` form, and the measurement in
bench/results/sortip-2026-10-25.md; step 3, the swap inside the merge,
is pending). The owner chose
option 1 on 2026-10-25: "superior, and we are not at v1 so API changes
are fine"; written for review before the implementation, per the lead.

## Context

`List.sort` is stable and returns a new list (spec/std.md:326), and its
Str form costs five words an element: 76.3 MB at 2,000,000 Strs on top of
the list itself (bench/results/sortstr-2026-10-24.md, probe
bench/perf/sort with a gen mode that only builds the list).

The floor is set by the signature, not by the algorithm: returning a new
list means the receiver's array (2n) plus a second array (2n), three
words an element, so no internal change to `sort` can go below that
(bench/results/sortstr-inplace-2026-10-25.md, "The floor for a
non-mutating sort"). The two stable in-place sorts were built and
measured against the current index sort at 2,000,000 Strs, and both lose:

| implementation | the sort adds | instructions | against today |
|---|---|---|---|
| index sort (today) | 76.3 MB | 3,292,000,000 | 1.00 |
| Go's sort.Stable (SymMerge with rotations) | 30.5 MB | 19,970,000,000 | 6.07 |
| bottom-up merge with a half list of scratch | 45.8 MB | 4,884,000,000 | 1.48 |

Go's unstable pdqsort (`slices.Sort`) is out: spec/std.md:326 promises
stability. The two losers say where the cost is: 62 percent of SymMerge's
time is reference counting inside the element swaps (each move of a Str
retains and releases it), so a sort of a mutating list only pays off if
its moves stop counting and releasing, which needs an element swap the
source cannot express (an assignment `xs[i] = xs[j]` counts, by design).

So the memory win needs a different *shape* of call, not a faster sort:
a list the caller lets the sort own, where the elements never need to
exist in two arrays at once.

## Decision

Add two mutating methods beside the existing ones, with the `var`
receiver std already uses for `push`, `pop`, `insert`, `remove_at` and
`set` (std/prelude.xo:519-522, spec/std.md:335-339):

```xo
fn (var xs: List[T]) sort_in_place()                    // T: Ord
fn (var xs: List[T]) sort_in_place_by[K: Ord, E, fx: Effects](key: fn(T) -> K ! E uses fx) -> () ! E uses fx
```

- **Stable**, the same promise `sort` makes, so the two forms order
  equal keys the same way and a caller can switch between them without
  changing its output.
- **Returns nothing** (a statement, like `push`); `xs.sort_in_place()`
  in an expression position is a type error. `sort` and `sort_by` keep
  their signatures and behavior exactly.
- **A shared list is copied first**, by the same uniqueness check that
  already copies for `push` and `xs[i] = x` (compiler/arm64/calls.xo:170,
  compiler/llvm/func.xo:1300): the caller's binding becomes a sorted
  copy, and another holder of the old list keeps its own order. So a
  uniquely owned list sorts with no second element array, and a shared
  one pays the same second array `sort` pays today.
- The generic form sorts with `T: Ord`; the keyed form takes `sort_by`'s
  key function and passes its effects and errors through the same effect
  variable.

## Implementation, staged like 0373

1. **A no-refcount element swap, backend only, landed before a seed.**
   `rt.list_swap(l, i, j)` swaps two elements of a list's own array as
   words, without counting or releasing what they reference. It is sound
   because a permutation of a list's elements does not change which
   values the list references: every element word is still referenced
   exactly as often, and the list never drops or shares one. Element
   width comes from the list header (`rt.list_esize` exists already).
   Landed and seeded before any std code calls it, the same two-step
   reason as decision 0373.
2. **The std methods on top**, a stable bottom-up merge with a scratch of
   half the list (the measured C form), which is correct on its own and
   is the fallback for element types a word swap does not cover.
3. Then the merge's moves switch to `rt.list_swap`, which removes the
   reference counting from the measured 1.48: the sorted result is the
   list's own array, the scratch is at most half a list, and the moves
   are a few instructions.

### Step 1 as built

- `rt.list_swap[T](xs: List[T], i: Int, j: Int)` (std/native/rt.xo).
- arm64 (amd64 retargets the same code): the two element addresses from
  `elem_addr`, two `load_elem`s then two `store_elem`s, so the element
  width is the emitter's own: a Bool's byte, a one word element, and a
  multi word element such as a two word Str or a struct all swap
  correctly. LLVM: two `elem_ptr`s and `elem_load`/`elem_store`, so a
  packed element (decision 0369: `List[UInt8]`, `List[Int16]`,
  `List[UInt32]`, `List[Float32]`) is loaded at its width, widened, and
  stored back narrowed; the first version loaded `g.ll(et)` and stored it
  raw, which overwrote a packed element's neighbours, and the packed rows
  below caught it. wasm: `elem_addr`,
  `load_leaves` and `store_leaves`. It is in inline.xo's intrinsic list,
  so a caller stays a leaf.
- Both indices are bounds checked (`elem_addr`/`bounds_check` and the
  faulting shape of `list.set`): the plan said unchecked, and a sort that
  handed a wrong index to an unchecked swap would corrupt a list
  silently, where the check is two compares a swap. The two elements are
  loaded before either is stored, so i == j writes back what it read.
- The tests are in `std/native/prelude_test.xo` (`xo test
  --std-module=native/prelude`, which internal/stdtest walks): a test
  module takes the name of a source module beside it, and native/rt's
  source is declarations only, so `xo test --std-module=native/rt` finds
  no tests at all; the obj module beside the lowering cannot host them
  because the reference count row reads `heapos.nfree` and obj importing
  native/llrt/heapos closes an import cycle through
  darwin/linux/sysio/windows. They cover Int, Bool, Str with a reference
  held outside the list across the swap, a struct, i == j, the first and
  last elements, and the reference count itself: two unique heap strings
  (a literal is immortal, so its count cannot move) swapped and dropped
  with the list, and the free-list total from `heapos.nfree` over all 73
  classes equal before and after, so a swap that counted an element twice
  or released one early fails. Five rows swap packed elements, since
  packing (0369) is on by default: `List[UInt8]` with 250 and 255,
  `List[Int16]` at the type's minimum -32768 (a wider or unsigned access
  would return 32768), `List[UInt32]` with its high bit set,
  `List[Float32]`, and a two word tuple; wasm keeps eight byte elements,
  so those rows take its other path there. The golden's packed row prints
  `255 250 2 1 0` with the mismatches at 0.
- A codegen pattern is still impossible (`xo dev codegen` matches a
  function by name and nothing calls the intrinsic yet, the blind spot
  0373's step 1 had), so a caller on every backend comes from
  `internal.crypto_op("rt.list_swap")` and the golden
  `internal/llvm/testdata/golden/native_list_swap`: std/native/internal.xo
  `probe_list_swap` runs the swaps through the intrinsic for arm64, LLVM
  and wasm, std/rt's Go switch answers the same text, and the wasm run's
  `XO_LEAKCHECK=1` is the reference count check there.
- The intrinsic is lowered in `inline_rt` (arm64) and `inline_rt` (LLVM),
  and dispatched from `compiler/ssa/json.xo` `generic_intrinsic` because it
  takes a type argument. Not in `call_op`/`call`, which match on names
  too: both route `rt.` to `rt_call` before their match, so an arm there
  never runs, which the first gate run found as an undefined symbol.
  internal/repotest's reachability check now reports that shape.
- **Precondition, recorded after step 3's first build found it:** the
  swap does no copy-on-write, because that lives in the element store's
  generated code, so the receiver's array must be unique when a caller
  swaps. A swap-based sort otherwise writes a list another binding
  shares; the std caller makes the list unique first with one counted
  element write (gotchas/list-swap-bypasses-copy-on-write.md). The
  intrinsic's doc in std/native/rt.xo states it, and step 3's design
  below chooses between that std-side write and a count check in the
  lowering.

### Step 2 as built

- `std/prelude.xo` declares both forms beside the other `var` methods
  (`sort_in_place`, and `sort_in_place_by` passing effects and errors
  through `E`/`fx`, the shape `update` on a Map already has), and
  `spec/std.md`'s mutating block gains them with the stability promise.
- Native (`std/native/prelude.xo`): `merge_sort_in_place` and
  `merge_runs_in_place` take `sort`'s shape (runs of 16 by insertion,
  then bottom up merges, a pair already in order skipped), but the merge
  copies its left run into a scratch list that only grows to the width it
  needs, so at most half the list, and writes the merge back from the
  run's start. `sort_in_place` is that with `<`; `sort_in_place_by`
  computes the keys once, sorts an index list with the in-place merge,
  and then permutes the elements inside the receiver one cycle at a time,
  so the keyed form also never builds a second element array.
- Go (`std/rt/list.go`): `List_sort_in_place` is `List_own` then
  `slices.SortStableFunc` with `Cmp`; `List_sort_in_place_by` computes
  the keys, sorts a permutation with `Cmp`, and gives the receiver the
  reordered array with a fresh block, the shape `List_sort_by` already
  has. Its nine probe modes print the same checksums as arm64's.
- Tests: `std/native/prelude_test.xo` gains eleven, on arm64 and LLVM
  (25 in the module): empty and one element, a sorted list, a reversed
  one, all equal, the sizes around the 16 element run, a shared receiver
  keeping its order while the sorted binding changes, Strs, agreement
  with `sort` and with `sort_by` on 60 structs, and a failing key leaving
  the list alone. They are not in a `std/prelude_test.xo` beside the
  declarations because a `--std-module=prelude` test that uses a `var`
  list method is reported as XO1201 "support is planned" for every
  expression in it, which an old method shows too (a two line test whose
  body is `var xs = [3, 1]` and `xs.push(4)`); that is not this change's,
  and it is the lead's to route.
- Corpus: `scripts/stdcorpus/prelude.go` runs both in-place forms over
  `SORT_STR_CASES` against Go's `sort.Strings` (the shapes a merge has to
  get right, the 7 byte prefix cases included), a stability row for each
  form, and the copy-on-write row; 12 corpus tests pass on arm64 and LLVM.
- Measurement: bench/results/sortip-2026-10-25.md, 2,000,000 Ints, Strs
  and UInt8, the sort's cost as the sorted run minus its gen run. The
  extra max RSS goes 80.0 to 22.7 MB on Strs (0.28), 16.0 to 14.3 MB on
  Ints and 2.0 to 1.7 MB on UInt8; instructions go 1.42, 1.12 and 1.18
  times `sort`'s, which is the counting step 3 removes; Strs are 0.83 of
  `sort`'s wall (0.63 to 0.52 s), Ints and UInt8 within the timer's
  resolution. `sort_in_place_by` is measured against `sort_by`, the fair
  comparison because both pay a keys and an index array: 1.12, 1.09 and
  1.12 times its instructions and 0.86, 0.89 and 0.81 of its extra RSS.

### Step 3 design: how the moves become swaps (measured, not landed)

The plan said "the merge's moves switch to `rt.list_swap`". `rt.list_swap`
swaps two elements of ONE list's array, and std Xo cannot make a list hold
the scratch: List has no truncate, resize or set_len (std/prelude.xo:497
has only `with_capacity`), so a list cannot be grown by n/2 placeholders
and shortened again. Two shapes were then possible, and both were built and
measured (bench/results/sortip-swap-2026-10-25.md, 2M Ints, Strs and UInt8,
instructions and RSS; wall indicative):

- **Rotation merge (built, rejected).** Runs of 16 by swap insertion, then
  merges that place the longer run's middle element with a binary search in
  the other run and rotate the block between them with three reversals of
  swaps, recursing. No scratch at all, stable, and it passes the module's
  25 tests and the corpus on arm64 and LLVM. It costs 5.16, 2.71 and 3.44
  times `sort`'s instructions and 4.62, 2.03 and 2.94 times step 2's: a
  rotation merge is O(n log^2 n) moves where the half-buffer merge is
  O(n log n), and the ~30 instructions a Str move saves do not pay for 1.3
  to 2.3 extra moves a level. Its only win is zero extra RSS, and only on
  Strs does that show in wall (1.08 s against `sort`'s 1.02).
- **Half-buffer merge with a two-list swap (preferred, needs a seed).**
  Step 2's merge with every move a swap between the list and the scratch,
  which needs one new intrinsic, `rt.list_swap2`.

**`rt.list_swap2(a, i, b, j)`**: swaps element i of a's array with element
j of b's array as words, with no count and no release, the same lowering
family as `list_swap` (two element addresses, two loads, two stores). The
soundness argument is the same and rests on the pair of arrays together:
every swap leaves the multiset of references held by `list ∪ scratch`
unchanged, so nothing needs counting; an element is only ever overwritten in
a slot that has already been consumed (both cursors move forward, and the
value a swap writes into the dead slot is never read again).

**Where the scratch's content comes from, and what it costs.** `list_swap2`
bounds-checks j like `list_swap`, so the scratch must have at least as many
elements as the left run before the first swap, and a generic `T` has no
default value, so the slots are filled with copies of a list element:

```xo
var scratch: List[T] = []
while scratch.len() < left_max {
  scratch.push(xs[0])          // one counted retain a slot
}
```

That is `n/2` counted retains at the start of a sort (the widest merge's
left run is at most half the list) and `n/2` releases when the scratch is
dropped at the end. It is cheap against what it replaces: at 2M elements a
sort makes about 21 moves an element, so the 1M count pairs of the init are
about one fortieth of the ~42M counted moves step 2 pays, and they are
paid once, not per level.

There is no raw uninitialized route in std Xo: `List.with_capacity(n)`
gives an allocation with those slots but a length of 0, so a swap into them
reads or writes past the logical end and the bounds check refuses it; making
the slots addressable means changing a list's length, which is the
`truncate`/`set_len` alternative below, rejected. So the initializer above
is the design, and its counting is stated rather than hidden.

The invariant, stated exactly: at the start of the sort, `list ∪ scratch`
holds the list's elements once each plus `left_max` copies of `xs[0]`, every
one of those references counted; each swap leaves that multiset unchanged;
and the scratch's drop at the end releases exactly the copies, so every
count returns to where it started. "Each element exactly once" in the
soundness paragraph above is the same statement with the copies excluded.

**What the scratch holds between levels and at the end.** After the
left-run move-out and the merge, the merged elements are all back in the
list, and the scratch's slots that were used hold the cast-off copies of
`xs[0]` again; the copies are never read by the merge, only written over and
then swapped back, so the scratch between levels holds `left_max` copies of
one element and nothing else. At the end of the sort it holds exactly those
copies, and dropping it is what releases them.

Cases the design has to answer:

- **a == b.** Legal and it degenerates to `list_swap`. The sort never calls
  it that way, but the lowering must not assume distinct objects: read both
  element addresses from their own headers and let the same pointer work.
- **b shared.** Unsound, and by construction impossible to hit: the scratch
  is made inside the sort and no other binding ever holds it, so its count
  is 1. The intrinsic's doc states the contract (b must be uniquely owned),
  the same way `list_swap`'s now states its own precondition.
- **Different element widths.** The call site's two lists are both
  `List[T]` (the scratch is declared `List[T]`), so one element size serves
  both, a packed `T` (a Bool, a small struct) included because the size comes
  from the type. A pair of different element types would corrupt memory, so
  the checker should reject a call whose two list types are not the same
  instantiation, and the lowering reads the size from that type once.
- **Copy-on-write.** a is the receiver and must be unique, which is the
  `list_swap` precondition (gotchas/list-swap-bypasses-copy-on-write.md);
  the sort forces it with one counted element write before the first swap.
  b needs nothing: it is fresh.
- **Bounds.** Both indices are checked, as `list_swap`'s are.

**The expected instruction ratio, before it is built.** Step 2 measures
1.42 (Strs), 1.12 (Ints) and 1.18 (UInt8) times `sort`, and that is what
step 3 has to move:

- **Strs:** expected to fall below step 2's 1.42 and toward, possibly below,
  `sort`'s 1.00. A counted Str move is a retain and a release (each with its
  own inline test and an atomic slow path on a shared object) plus two
  stores; a swap is two loads, two stores and two bounds compares. The init
  adds `n/2` count pairs against ~21 moves an element.
- **Ints and UInt8:** expected to be a few percent WORSE than step 2's 1.12
  and 1.18, because there is nothing to count: a move is one store and a
  swap is two loads, two stores and two compares. If the measurement shows
  that loss is real, the open option is a compile-time choice of merge by
  element type (plain moves when `T` holds nothing counted), which this ADR
  leaves to the numbers rather than to guesswork.
- The number to report is the same table, same probe, with the `sort` and
  step 2 columns beside it, and the init's `n/2` counts called out on their
  own line.

The alternative, a length-changing method (`List.truncate(n)` or
`set_len(n)` keeping the scratch beyond the logical end of one list), was
compared and rejected: it needs a method that changes a list's length in
place with capacity semantics, it must release the elements it cuts off
(n/2 releases a sort, which is the same init cost as the copies above, so
no win there), it
interacts with the unique/capacity invariants and with `len()` inside the
merge (the logical length and the scratch's room would differ), and it
still needs the same uniqueness force. That is more std surface and more
invariants for the same shape, and it locks the scratch into the receiver's
own allocation, so a sort could not use a smaller scratch.

So step 3 is `list_swap2` plus the half-buffer merge, landed and gated
before the next seed, with step 2 the form that ships until then.

## Consequences

- Memory, unique receiver: at most n/2 words of scratch, and none once
  step 3 lands, against 5n for `sort` today (76.3 MB to 0 at 2,000,000
  Strs). Shared receiver: 2n, the same as `sort`, because the copy is
  the point.
- Instructions: the half buffer measured 1.48 times the index sort; with
  the swap intrinsic the moves stop counting, which is where 62 percent
  of SymMerge's time went, and the projection is about the index sort's
  count at 3n rather than 5n of memory. That projection is measured, not
  assumed, before step 3 lands.
- spec/std.md gains the two signatures in section 2.3's mutating block
  and the stability promise for the in-place forms, in the same commit
  as the implementation; std/prelude.xo keeps its declarations beside
  the other `var` methods.
- Tests: internal/stdtest's prelude corpus gains rows for both forms
  (already sorted, reversed, all equal, keys sharing a seven byte
  prefix, one key a prefix of another, 0, 1, 2, 12, 13 and 50 keys),
  with Go's `slices.SortStableFunc` as the oracle; plus two CoW rows: a
  shared original keeps its order while the sorted binding changes, and
  a unique list's identity (its array address) is unchanged by the sort.

## Alternatives

1. Leave `sort` alone: the measured status quo, 5n words and the
   fastest stable sort today.
2. Change `sort` itself to copy-on-write and sort in place internally:
   bounded by 3n (the receiver plus the new array) and measured 48
   percent more instructions for 40 percent less scratch. It also
   changes nothing for the caller, who cannot say "I am done with this
   list".
3. Unstable in-place pdqsort: the cheapest in memory and time (Go's
   `slices.Sort`), refused because it breaks the stability promise, and
   instead offered as a separate decision if a caller ever needs it.
4. A list type that owns its elements explicitly (a Move or a unique
   ownership annotation): the general fix for the copy, a language
   change, out of scope here.
