# A method of a value type is not a value

Symptom: Observed 2026-10-23, while wiring a predicate into
`Str.find_where` and its *Func siblings: `s.trim_where(Rune.is_space)`
and `s.contains_where(Rune.is_space)` failed with

```
XO0601: name `is_space` not found
note: `is_space` is a method; call it on a value of type `Rune`
```

Cause: a method becomes a function value only when a receiver is bound
(`x.m` of the `method_values` golden, `let has = "hello".contains`,
`let has = m.hits`), and the receiver's remaining parameters follow. A
type name names no receiver, so `Rune.is_space` has nothing to bind: the
checker's associated-name lookup finds static methods (`Rune.try`,
`Bytes.from_hex`) only. The same holds for every method of a struct,
enum, list or Str; the note's wording is what tells the two apart.

Fix: pass a closure or a named function, which also reads better at the
call site because the predicate has a name:

```
fn is_space_rune(r: Rune) -> Bool { r.is_space() }
s.trim_where(is_space_rune)              // a named function
s.split_where(fn(r) { r.is_space() })    // a closure
s.contains_where(m.hits)                 // a bound method value
```

If an unbound method value is ever wanted, it is a language change (a
section 3.4 rule and a checker feature), not a std one.
