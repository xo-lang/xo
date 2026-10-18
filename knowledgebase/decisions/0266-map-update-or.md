# 0266: point copy-modify-store at Map.update (no new API)

Status: Accepted (owner, 2026-10-18; proposed by Xo Lang 2 from the
owner's language survey through the lead)

## Context

The survey asked for `Map.update_or(k, default, fn(var v) {...})` to
replace copy-modify-store:

```
var ids = by_name.get(f.name) ?? []
ids.push(i)
by_name[f.name] = ids
```

That method already exists. std.md 2.4 has
`fn ([var] m: Map[K, V]) update[R](k: K, init: V, f: fn(var V) -> R) -> R`:
it reads k (or init), runs f on it in place, stores it, and returns f's
value. The agent that asked for update_or (Xo Lang 2) had written the
three-line form in compiler/arm64 reach.xo the same day without finding
it. The gap is discoverability, not the API.

The three-line form is worse than verbose: forgetting the last line
loses the write silently (value semantics: `ids` is a copy), and it
looks the key up twice.

## Decision

1. No new Map method. `update` stays the one in-place helper; its doc
   line in std.md 2.4 gets the copy-modify-store example above and its
   `update` form:
   `by_name.update(f.name, [], fn(var ids: List[Int]) { ids.push(i) })`.
2. A vet hint, XO0532 (advisory): a `var x = m.get(k) ?? d` (or
   `m.get(k) ?? d` into a `var`) whose variable is then mutated through a
   `var` method or field assignment and stored back with `m[k] = x` (or
   `m.put(k, x)`) in the same block, with no other use of x, names
   `m.update(k, d, fn(var x) { ... })`, with the rewrite as its fix.
3. A second hint in the same check: the mutation without the store back
   (`var x = m.get(k) ?? d; x.push(v)` and x never stored or read
   again) is XO0533, "this changes a copy of the map's value; store it
   back or use m.update". That one is the lost-write bug.
4. `xo explain XO0532` and `XO0533` describe both forms.

## Consequences

- No API growth; agents get pointed at the existing method by the tool
  they already run.
- XO0533 catches a real bug class (a mutation of a copy, dropped) that
  XO0209 (discarded pure results) does not cover, since push is not
  pure.

## Alternatives

- `update_or` as asked: a second name for update.
- Entry API (`m.entry(k).or_insert(d)` returning a mutable reference):
  needs references, which value semantics avoids.
- Docs only: the agent read std.md and still missed it; a hint at the
  point of writing is what reaches it.

## Implementation (2026-10-18)

compiler/cli/core vet.xo copy_store: in every block, `var x = m.get(k)
?? d` on a Map, followed by statements that only change x (a call of a
method with a `var` receiver on x, `x op= v`, or an assignment to a field
or element of x), then `m[k] = x` with the same m and k text is XO0532
(a note, with the `m.update` rewrite as its fix). Any other statement
between them ends the match. `m.put(k, x)` as the store is not matched
yet. Tests: cmd/xo TestVetCopyStore; `xo explain XO0532`.

Point 3 (XO0533) was not added: the checker already reports the lost
write as the error XO0520, "mutation of `x` is never written back"
(core 5.2), found when TestVetCopyStore's program failed to check. An
error is stronger than a vet warning, and a second code for the same
mistake would break the one code per meaning rule (spec 10).
