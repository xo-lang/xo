# 0374: List.sort_in_place, a stable sort that mutates the receiver

Status: Proposed (the owner chose option 1 on 2026-10-25: "superior, and
we are not at v1 so API changes are fine"; written for review before the
implementation, per the lead).

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
