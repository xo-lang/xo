# 0491: `use go`: embedded struct fields, and a warning for a List or Map Go may change

Status: Proposed (Xo Lang 2, 2026-11-04, from the owner's v0.36.0 evaluation; for the owner to decide, nothing built)

## Context

The owner's evaluations of v0.36.0 (darwin/amd64 and Windows) found three
gaps in `use go`. The lead asked for one draft covering them:

1. **Embedded Go struct fields are not bound.** zap's
   `observer.LoggedEntry` is `struct { zapcore.Entry; Context []zapcore.Field }`.
   On 0.38.1 its binding has `context()`, `set_context(v)` and
   `context_map()`, but no way to reach the entry: neither the embedded
   field (`le.Entry` in Go) nor the fields it promotes (`le.Message`,
   `le.Level`, `le.Time`, `le.LoggerName`). The binder skips every
   embedded field without a skip line (internal/gobind gen.go, the
   field loop's `fv.Embedded()`). Methods promoted through an embedded
   field *are* bound, since the binder walks `types.NewMethodSet`.
   Spec 4.4 says nothing about either.
2. **Named Go map types (jwt's `MapClaims`)**: already done.
   9a4b4390e (2026-10-30, in v0.36.0) added `T.of(m)` and `to_map()`, and
   spec 4.4 states it. The evaluation's jwt program ran on 2026-10-29,
   before that change. Checked on 0.38.1 on 2026-11-04 (Go backend):
   `jwt.MapClaims.of({"sub": go.any("me")})`, then `as_claims()`,
   `as_signing_method()` and `signed_string(go.any(b"key"))`, prints a
   signed token. Nothing here to decide.
3. **Silent copies.** A Go function that changes its argument in place,
   given an Xo List or Map as a Go interface value (`any`, or an
   interface the List's converted Go value satisfies), changes a copy.
   The List is unchanged, with no error. The common case was `sort`:
   since v0.36.0, `sort.slice`, `slice_stable`, `ints`, `strings` and
   `float64s` are skipped, and a call is XO1102 naming List.sort,
   sort_by and sort_in_place. The general case remains:
   `reflect`-based fillers, a library's `Shuffle(any)`, anything that
   writes through an `any` that holds a slice or map. Spec 4.4 says
   "slices and maps are copied in and out" in one clause of a long
   bullet.

## Decision (proposed)

### A. Embedded fields are bound, and so are the fields they promote

1. An exported embedded field of a bound struct handle gets the getter
   and setter of any field, named after the field's Go name (its type's
   name). For LoggedEntry: `le.entry() -> zapcore.Entry` and
   `le.set_entry(v)`. An embedded struct held by value is a handle that
   aliases the outer struct's field (Go's `&le.Entry`), so a change
   through it is seen in the outer value, as Go's selector behaves. An
   embedded `*T` is `T?` (nil is None), and an embedded interface is
   its interface handle.
2. Every field Go's selector reaches through embedded structs gets a
   getter and setter on the outer handle (`le.message()`, `le.level()`,
   `le.set_message(s)`). The binder resolves them with
   `types.LookupFieldOrMethod` on the outer type, so Go's rules decide:
   - the shallowest name wins;
   - two at the same depth are ambiguous and are not bound;
   - a name a method or a shallower field already has is a collision
     skip with a skip line, as for methods today.
   A promoted field of an unexported embedded type is bound when the
   field itself is exported, as Go allows.
3. Promoted methods stay as they are. Spec 4.4 gains one bullet that
   states all three: embedded fields, promoted fields, promoted methods.
4. The binding format number goes up, and gobind tests cover
   LoggedEntry's shape, a depth-2 promotion, an ambiguous name, an
   embedded pointer and an embedded interface.

### B. A warning where Go may change a copy

1. A new warning `XO1107`. A List, Map or Set value passed to a Go
   parameter of interface type, explicitly (`go.any(xs)`) or by the
   implicit conversion of a `...any` list literal's element, gets:
   "Go receives a copy of this List; a change Go makes to it is not
   seen here (spec 4.4)". The fix note names what gets a result back:
   a Go function that returns the value, or `go.out[T]()` for a filler.
2. **Where it is reported (the owner's choice; option (b) recommended).**
   (a) `xo check`, everywhere. (b) `xo vet` only. (c) `xo check`, with a
   read-only allowlist that suppresses it.
   Most `any` parameters only read: `json.marshal`, `fmt.sprintf`, a
   logger's fields, `sql` arguments. Reporting all of them in check
   (a) would be noise on the commonest `use go` calls. The allowlist
   (c) is a list to maintain. Vet (b) finds the rare in-place writer
   without taxing the common reads, and an agent runs `xo vet`
   before shipping. A per-parameter `@reads` annotation in the binding
   could later move it into check.
3. Spec 4.4 gets its own sentence, not a clause: "A List, Map or Set
   given to Go is a copy: a Go function that changes its argument in
   place (through `any`, an interface, or a slice parameter) changes the
   copy, and the Xo value stays as it was; take the result Go returns, or
   use `go.out[T]()` for a function that fills a value." The `use go`
   guide (docs and llms.txt) carries the same sentence with the
   sort example.

## Consequences

- LoggedEntry and similar wrappers (zap's observer, testing helpers,
  options structs that embed a base) become usable with no Go wrapper.
- Promoted-field accessors add names to a handle; a name that collides
  is skipped with a line, never silently shadowed.
- An aliasing getter for a value-embedded struct is new for field
  getters. Today a struct field getter returns a handle to a copy, or
  to the pointer for a `*T` field. The aliasing matches Go and keeps
  `le.entry().set_message(s)` meaningful. Option A1's alternative
  below keeps copies instead.
- B costs nothing at run time. Under option (b) a program that never
  runs `xo vet` gets the spec sentence only.

## Alternatives

- **Embedded struct getter returns a copy** (as `let`): simpler and
  consistent with other getters, but `le.entry().set_message(s)` would
  then change a copy, the same silent-copy trap as B.
- **Promote fields only, no embedded-field accessor**: covers the
  common read (`le.message()`) but not passing the embedded value on
  (`zapcore.Entry` to an encoder).
- **Make the copy an alias** (pass a Go slice backed by the Xo List's
  memory): no copy, so no trap, but Xo's List is reference counted with
  copy on write, and Go holding its memory past the call breaks both
  (the same reason decision 0419 copies pointers to basic types).
- **XO1107 as an error**: refuses correct read-only calls; rejected.
