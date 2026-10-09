# 0072: Map and Set on the native backends as structs of lists

Status: Accepted (2026-10-08). Implemented in `internal/ssa/build_map.go`
(with `xo_str_hash` and `xo_str_quote` in `std/llrt/xo_rt.c`). Goldens:
`internal/llvm/testdata/golden/maps`, `map_key_fault`.

## Context
Map and Set (std.md 2.4) were build errors on the native backends. The
Go backend's Map is an insertion ordered entry log with a hash index
(`std/rt/map.go`), with value semantics. The native HTTP server needs
maps (headers, routes, JSON objects), and the native backends already
have reference counted lists with in place updates of unique values
(Perceus) and sharing between tasks (decision 0058).

## Decision
1. **A Map is an SSA struct of lists**, no new kind and no backend code:
   `keys: List[K]`, `vals: List[V]` (in insertion order, removed entries
   too), `live: List[Bool]`, `slots: List[Int]` (open addressing, a power
   of two long: 0 empty, else entry index + 1), `n: Int`. `Set[T]` is the
   same with `V = ()`. Copies share the lists; the first write to a
   shared list copies it, a unique one is updated in place.
2. **Operations are SSA functions generated per type** (`xo.map.find`,
   `put`, `remove`, `rebuild`, `keys`, `values`, `entries`), and keys
   hash with generated `xo.hash[T]` functions (Int: a multiplicative
   mix; Str: FNV-1a with a final mix in the runtime; structs, tuples,
   enums, lists, and boxes combine their parts). Equality of keys is the
   structural `==`.
3. **Removal** marks the entry dead and replaces its key and value with
   zero values (so they are released); its slot stays, and lookups skip
   it. When entries (live or dead) would pass three quarters of the
   table, `put` rebuilds the lists from the live entries in order, with
   a table at least twice the live count.
4. Lowered: `{k: v}` and `#{x}` literals, `len`, `is_empty`, `get`,
   `contains`, `get_or_fault` (Str and Int keys; the Go backend's
   message), `put`, `remove`, `update`, `keys`, `values`, `entries`,
   `to_list`, `add`, `m[k] = v`, and `for (k, v) in m` / `for x in s`
   (the value the map had when the loop started).
5. Not lowered (build errors that name them): `==` on a value that holds
   a Map or Set (its fields are not its value), Float, function, and
   handle keys, `filter`, and Set's `union`, `intersect`, `difference`,
   `any`, `all`, `map`.

## Consequences
- `maps` (adapted from internal/gen's `map_value_semantics`) prints the
  same on the Go, LLVM, and arm64 backends with no leaks, also with
  share probes and under ThreadSanitizer.
- Lookups do one hash and compare keys only of live entries in the
  probe sequence; a map with many removals keeps dead entries until the
  next rebuild.
- A generic list of operations means each Map type gets its own copy of
  the functions (monomorphized, like generics, decision 0048).

## Alternatives
- **A runtime hash map in C with callbacks**: the runtime would need
  each key's layout, hash, and equality, against decision 0046's
  scalars and pointers interface.
- **A new SSA kind**: both backends would need layout, reference
  counting, sharing, and equality code for it.
- **Linear search without an index**: simpler, quadratic for large maps.
