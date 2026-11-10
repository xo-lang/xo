# 0586: A comparator sort, `sort_with`, and descending order

Status: Accepted (owner, 2026-11-10, as written, relayed by the lead;
proposed by Xo Lang 1 from owner tester feedback).
Related: 0374 (`sort_in_place`), std.md 2 (List).

## Context

`List` sorts by its elements' `Ord` (`sort`) or by a key (`sort_by(key)`),
both stable. An owner tester reached for Go's and JavaScript's comparator
form, `xs.sort(fn(a, b) { a > b })`, and got three errors. Two needs have
no direct form today:

- an order that is not one key's (sort by count descending, then by name
  ascending) needs a tuple key with a negated count;
- descending order means negating the key, which faults for `Int.min`
  (XO-F overflow) and does not exist for `Str`.

Go has `slices.SortFunc` and `slices.SortStableFunc(s, cmp func(a, b T)
int)` with `cmp.Compare` and `cmp.Or`; Rust has `sort_by(|a, b|
a.cmp(b))` returning `Ordering` and `Reverse`.

## Decision (proposed)

1. **`Ordering`**, a prelude enum `Ordering { Less, Equal, Greater }`, and
   for every `Ord` type `fn (a: T) cmp(b: T) -> Ordering` (the order `<`
   uses, core 4). `Ordering` has `reverse() -> Ordering` and
   `then(o: Ordering) -> Ordering` (the first that is not `Equal`, Go's
   `cmp.Or`, Rust's `then`).
2. **`sort_with(cmp: fn(T, T) -> Ordering ! E uses fx) -> List[T] ! E uses fx`**
   and **`sort_in_place_with`**, stable (equal elements keep their input
   order), like Go's `slices.SortStableFunc`. A comparator that is not a
   strict weak order gives some permutation, never a fault or a loop
   (as Go). Its effects and errors propagate as `sort_by`'s do.
3. **Descending without negation:** `xs.sort_with(fn(a, b) { b.cmp(a) })`,
   or `sort_by_desc(key)` and `sort_in_place_by_desc(key)`, stable, which
   order by `key` from greatest to least (no key negation, so `Int.min`
   and `Str` keys work).
4. **Habits:** `xs.sort(fn(a, b) { a > b })` and `sort(fn(a, b) -> Int
   ...)` become one XO0601 that names `sort_with` with the rewritten
   comparator (`a > b` to `b.cmp(a)`, `a < b` to `a.cmp(b)`), and
   `sort_by` for a key.

## Consequences

- Multi-key orders read as Go's: `a.count.cmp(b.count).reverse().then(a.name.cmp(b.name))`.
- `Ordering` and `cmp` are new prelude names; a program's own `cmp`
  method on its type shadows the prelude's as any method does.
- Every backend implements `sort_with` with the same stable merge sort as
  `sort_by` (the comparator in place of the key compare).

## Alternatives

- **`fn(T, T) -> Int`** (negative, zero, positive, as Go's `cmp`): no new
  type, but a sign convention is easy to get backwards, and `a - b`
  comparators overflow.
- **`fn(T, T) -> Bool` "less"** (C++, Go's old `sort.Slice`): what the
  tester wrote; cannot say "equal", so stability and `then` chains are
  awkward.
- **Only `sort_by_desc`**: covers descending, not multi-key orders that
  mix directions without tuple keys.

## Implementation (2026-11-10, Xo Lang 1)

Points 1 to 3 as written: `Ordering`, `cmp` (the checker's builtin method on every Ord type), `reverse`,
`then`, `sort_with`, `sort_in_place_with`, `sort_by_desc`, `sort_in_place_by_desc` on every backend (native:
std/native/prelude.xo's merge sort with the comparator; Go: std/rt/ordering.go), golden sort_with.
Point 4 keeps the habit's code XO0403 (an argument the method does not take; the habit already used it, and
a code's meaning never changes) where the text above says XO0601: the message names `sort_with`, the note
shows the comparator rewritten (`a > b` to `b.cmp(a)`, `a < b` to `a.cmp(b)`, `>=`/`<=` alike), and when the
body's last expression is one comparison a fix renames the method and rewrites the comparison.
