# 0415: `use go` generics instantiated at bind time

Status: Accepted (owner, 2026-10-29, answering the three open questions below; drafted by Lang 1)

## Context

`use go` (decision 0041, spec 4.4) skips every generic Go function, generic
type and method of a generic type (internal/gobind gen.go catGeneric). The
Windows tester's coverage survey of v0.34.0 (2026-10-29,
xo-evaluation 2 knowledgebase/findings/usego-coverage-2026-10-29.md) puts
generics at 15% of the skips in both sets: 61 of 413 in 47 std packages,
424 of 2923 in 40 third-party modules, second after `any` (decision 0412).
Whole packages are unusable: `slices`, `maps`, `samber/lo`,
`hashicorp/golang-lru/v2` (0 of 28), and parts of go-redis, pgx and
zap. Generic APIs are growing in Go (iterators, `sync.OnceValue`,
`atomic.Pointer[T]`).

The binder reads a package once into a declaration file (`.xo/gobind/P.xo`)
and a Go adapter (`rt/xo_gobind_P.go`) that converts Xo values to Go
values at the boundary. A Go generic cannot be bound once: the adapter's
conversion depends on the type arguments (an Xo `List[Int]` is copied to
`[]int64`, a handle unwraps to its pointer, a struct converts field by
field), and only the program says which type arguments it uses.

Xo has generics of its own (spec 2.7): square brackets, interface
constraints, monomorphized, type arguments inferred at the call or given
(`json.decode[User](body)`).

## Decision

1. **Declarations are generic.** The binder writes a generic Go function
   or type as an Xo generic declaration whenever its signature maps with
   the type parameters left open: `slices.Index[S ~[]E, E comparable](s S,
   v E) int` becomes `fn index[E: Eq](s: List[E], v: E) -> Int`;
   `lru.Cache[K comparable, V any]` becomes a generic handle
   `Cache[K: Hash + Eq, V]` with its methods. Constraints map:
   `any` to none, `comparable` to `Eq` (and `Hash` where the type is a
   map key), `cmp.Ordered` and `constraints.Ordered` to `Ord`, a core type
   constraint `~[]E` to `List[E]` (the parameter itself disappears: Xo
   has no named slice types to pass), `~map[K]V` to `Map[K, V]`; any
   other constraint (a method set, a union of named types) keeps the
   function skipped with the reason "type constraint C has no Xo form".

2. **Instances come from the checker.** The checker types a call to a
   bound generic like any generic call (inference or explicit arguments)
   and records each instantiation the program uses, as Xo types, in the
   build's set of Go instances (per package, deduplicated, in the order
   first seen, so output is deterministic).

3. **The binder instantiates.** At build (not at check), xo hands the
   binder the instance list; xo-gobind maps each Xo type argument to its
   Go type (Int to int64 or the Go type the declaration names, Str to
   string, a bound handle to its Go type, a List to a slice), checks it
   against the Go constraint with go/types (types.Instantiate with
   validate), and writes one adapter function per instance into
   `rt/xo_gobind_P_inst.go`, named by a hash of the instance, calling
   `pkg.F[GoArgs...]` with the conversions every non-generic call uses. A
   type argument with no Go form (an Xo enum, a closure with effects, an
   Xo struct with no Go twin) is `XO1106` at the call (decision 0412's
   "has no Go value", go_any's table, compiler/check goany.xo), unless
   the constraint is `any` (decision 4a); the checker reports it, so `xo
   check` catches it without running the binder.

4. **Xo types as type arguments.** An Xo struct or enum passed as `T` to
   `lo.Map` or kept in an `lru.Cache[Str, User]` has no Go type. Two
   forms are possible; this decision takes the first and leaves the
   second for 0412's go.Any work:
   a. **Opaque:** where the Go code never looks inside `T` (its
      constraint is `any`, so Go can only store, copy and return it), the
      instance uses a Go type that holds the Xo value as a handle
      (`xoval`, a reference the Go backend's runtime keeps alive). Most
      container and functional APIs (`lru`, `lo`, `slices.Reverse`) are
      this. `comparable` with an Xo type is refused (Go's `==` on the
      handle is identity, not Xo's equality).
   b. **Converted:** a Go struct generated from the Xo struct (deferred).

5. **Methods on generic types.** A method of a generic handle is bound
   with its receiver's parameters; its instances follow the handle's
   (one adapter per method per handle instance the program uses).

6. **Iterators** (`iter.Seq[V]`, `iter.Seq2[K, V]`) are generic types: a
   function returning one is bound when V maps, as Xo's existing
   iterator form (gen.go's iterator case), instantiated the same way.

7. **Caching.** The instance file is part of the build's Go tree, keyed
   by the instance list, not part of the binding cache: the binding
   (declarations) stays per package and is unchanged by the program, so
   adding a call does not rebind the package.

## Consequences

- Expected coverage: the survey's generic skips are at most 15% of skips
  in both sets (std 61, third party 424); constraint shapes outside step
  1's table stay skipped, so the gain is measured, not assumed: the build
  reruns the survey's 87 packages before and after (bench/results) and
  reports how many generic skips each constraint shape keeps.
- `use go` builds only on the Go backend, which already generates Go per
  program, so per-program instances need no new build stage; xo-gobind
  runs once more per build when the instance list changed (cached by its
  hash).
- No new diagnostic code: a type argument with no Go value is XO1106
  (decision 0412), with the message naming the type argument.
- spec 4.4 gains the constraint table and the opaque rule; spec 10 the
  new code.
- Tests: internal/gobind (the declaration of each constraint shape;
  instantiation and its validation errors), cmd/xo through the offline
  GOPROXY tree (a synthetic generic module: a container with `any`, a
  function with `cmp.Ordered`, a `~[]E` function, a method set
  constraint that stays skipped), and `slices`/`maps` from Go's std.

## Alternatives

- **Instances listed in xo.mod** (`bind go slices Index[int]`): no
  checker work, but the user writes Go types by hand and keeps the list
  in step with the code; inference from calls does it for them.
- **A generic Go adapter** (`func index[E comparable](...)`) with the
  conversions done through reflection or an interface per type: one
  adapter per function, but the conversion of `List[E]` to `[]E` needs
  E's Go type at compile time anyway, and reflection costs every call.
- **Binding a fixed set of common instances** (Int, Str, Float): no
  checker work, misses every handle and user type, and multiplies the
  binding's size by the set.
- **Leave generics skipped and wait for Xo-native ports** (std already
  has List, Map, sort and iterator methods covering most of `slices` and
  `maps`): right for those two, but not for third-party containers and
  clients (lru, go-redis's generic commands) that have no Xo port.

## The owner's answers (2026-10-29)

1. Is 4a (Xo values carried opaquely through Go generics with `any`
   constraints) acceptable, given a Go library could keep the value past
   the call (a cache does exactly that, which is the point)? **Yes, only where the constraint is `any`** (4a).
2. Should `slices` and `maps` be bound at all, or should the binder point
   at the Xo prelude method instead (a note in the skipped list: "use
   List.index")? **Do not bind them**: `slices` and `maps` stay skipped, and the
   skipped list names the prelude's List or Map method instead.
3. Is instantiation at build time (step 3) acceptable for `xo check`
   speed, given check validates through the mapping table only? **Yes**, at build time.

## As built, stage 1: generic functions (Lang 1, 2026-10-29)

- Steps 2 and 3 are one step: the adapter is itself a Go generic that forwards the Xo declaration's type
  parameters (`func Go_P_index[E comparable](tk *Task, a0 List[E], a1 E) int64`) and instantiates the Go
  function explicitly (`p.Index[[]E, E](GoSliceCopy(a0), a1)`). The Go backend already calls a bodiless
  generic as `Prefix_fn[T](...)` with T's Go representation, so Go's compiler makes each instance at build:
  no instance list, no second binder run, and the binding stays per package. A value of a type parameter
  crosses as Xo's representation (identity), which is what 4a asks for under `any` and is Go's own value for
  the comparable and ordered types the checker admits.
- `comparable` maps to `Eq` alone (`T: Hash + Eq` is not Xo syntax, XO0177); Map has no key constraint in
  its declaration.
- The checker's call constraint loop (compiler/check call.xo, goany.xo need_go_comparable) reports XO1106
  for an `Eq` or `Ord` type argument of a Go binding's generic whose Go value Go cannot compare or order
  (a List, Map, Xo struct or enum, or an ordered Bool), before Go's build would fail.
- A generic call is re-executed in a replay (decision 0093's handle mode), since a type argument may be a
  handle the signature cannot show.
- Generic types (`lru.Cache[K, V]`, methods of generic receivers, iterators of generic types) are stage 2.
- Binding format 10.
