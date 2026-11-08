# 0574: C library build modes (exported functions, shared and static libraries)

Status: Proposed (Xo Lang 4, 2026-11-08; owner decides). The owner
approved the direction, an ADR, and a stage 1 prototype (2026-11-08,
through the lead); the prototype's measurements fill in the
Consequences.

## Context

Xo builds programs only. The C entry is std/native/llrt/cabi_c.xo
`main` (decision 0234), which runs entry.init (the main thread's heap,
faults, context, the poller, the stop record) and entry.main (the
process start, main on a fiber, the scheduler, the end of program flush
and leak check). A C, Python, Ruby or Node program cannot call Xo code.
Go offers this with `//export` and `-buildmode=c-shared` / `c-archive`.

Half of the mechanism exists, internal to std:
- C ABI exports (decision 0207): every top level function of a cabi
  marker module is an Entry function built as a root after the program
  (compiler/ssa declare_exports), named `xo_x_<name>`, with Int and
  Float parameters, an Int, Float, nothing or Str result, and no faults.
- Exact C names (decisions 0230, 0234): cabi_libc and cabi_c export
  bare C names; rt.sym_addr; data symbols; backend entry stubs.
- The other direction, Xo calling C, is `use c` (decisions 0093, 0116):
  bindings from a header, scalars, Str, Bytes, flat structs and opaque
  handles at the boundary, the effect `ffi`.

What is missing:
- a user visible marker;
- a library link (no main, a shared object or an archive);
- a header;
- runtime start from a foreign caller (heaps are per thread:
  obj.heap_init with the thread's record);
- the boundary type rules for user types;
- faults at the boundary;
- how an export gets its capabilities (Os).

There is no shared object support in Xo's own linkers (no `-shared`,
`-dynamiclib`, ET_DYN or MH_DYLIB in compiler/), so stage 1 links with
the C compiler (the `--ld=cc` path).

Reference design: Go's cgo export rules (cmd/cgo, "C references to
Go"; runtime/cgo):
- exported functions get C names;
- strings cross as {pointer, length} (GoString);
- C may not keep Go pointers after the call (the cgo pointer passing
  rules, checked by GODEBUG=cgocheck);
- the runtime starts in a library constructor, and a C thread entering
  Go gets an extra M (needm);
- a Go panic that reaches the C frame aborts the process;
- c-archive writes a `.a` and `_cgo_export.h`.

## Decision (proposed)

1. **Marker: an `as c "NAME"` clause.** A top level `pub fn` whose
   signature is followed by `as c "c_name"` is a C export under that
   symbol:

   ```
   /// The sum of a and b, for C.
   pub fn add(a: Int, b: Int) -> Int as c "geo_add" {
     a + b
   }
   ```

   The clause comes after the result and before `uses` and `requires`.
   It mirrors the field wire names (`kind as "type"`, decision 0277)
   and `use c "<header>" ... as name`, Xo's other boundary with C.
   The C name is required and spelled out: there is no mangling to
   guess, and grep finds the symbol.

   The checker rejects, each with a new XO code:
   - a method, a generic function, a `var` parameter, or a parameter or
     result outside the boundary types (3);
   - two exports of one C name;
   - a name in the reserved `xo_` namespace, or one that is not a C
     identifier;
   - an export in a module other than the root package of the build
     (a dependency's exports would be surprising; revisit later).

   Spec 3.2 gains the clause, and a new section "C exports" covers the
   boundary types, faults and threads.
2. **Build modes.**
   - `xo build --lib=shared` writes `libNAME.so` (linux),
     `libNAME.dylib` (darwin), or `NAME.dll` plus its import library
     (windows).
   - `xo build --lib=static` writes `libNAME.a`.
   - Both write `NAME.h`: the exports' C prototypes, the boundary
     typedefs (`XoBytes`, `XoHandle`), and `xo_init`, `xo_free` and
     `xo_last_error`.
   - A library has no entry: `--lib` on a package that declares
     `fn main` is an error (a new XO code), so a program is not shipped
     as a library by mistake.
   - A program build ignores the clause: the function is an ordinary
     `pub fn`, so a package can be both a library and a program's
     dependency.
   - Only the exports and the `xo_` API are public. The runtime's own C
     names (the cabi exports, `xo_x_*`) are hidden: an exported symbols
     list on darwin, a version script on linux.
3. **Boundary types.**

   | Xo | C | ownership |
   |---|---|---|
   | Int | int64_t | value |
   | Float | double | value |
   | Bool | bool (uint8) | value |
   | Bytes, Str (parameter) | `XoBytes {const uint8_t *p; int64_t n}` | borrowed for the call; Xo copies what it keeps (a view would outlive the C buffer) |
   | Bytes, Str (result) | `XoBytes` | owned by the caller, freed with `xo_free(p)`; a Str is UTF-8, n bytes, with a NUL past n for convenience |
   | any other type | `XoHandle` (uint64) | an index into a per library table holding a counted reference; `xo_release(h)` drops it; a stale handle faults |
   | `T ! E` result | T's C type, with `int32_t *err` appended | 0 on success, else an error code; `xo_last_error()` returns the text (thread local, owned by the library) |

   Optional and List results go through handles in stage 2 (an
   accessor API comes later).
4. **Faults.** A fault never unwinds into C. Each export's stub runs
   the body under a fault boundary. A fault records its message (thread
   local, `xo_last_error`) and writes the error code when the result has
   one. Otherwise it aborts the process with the fault report, as Go
   does for a panic reaching C. Stage 1 always aborts; error codes come
   in stage 2.
5. **Effects and capabilities.** In stage 1 an export's `uses` clause
   may name only pure effects (no `Os`). The library's capabilities come
   from `xo_init(argc, argv, envp)`, which builds the Os once and hands
   it to the package's `fn init(os: Os)` if it declares one; the package
   keeps it in its own state. An export that needs I/O reads the Os the
   package kept. (Alternative: every export takes an `os: Os` parameter
   that the stub fills from the library's Os. That is simpler for the
   author but hides the capability flow from C.)
6. **Runtime start and threads.**
   - `xo_init` runs entry.init's steps and starts the scheduler's pool
     without a main fiber. It is idempotent and thread safe.
   - Stage 1 requires `xo_init`. Stage 2 runs it lazily from the first
     export call (a once flag in the stub) and from a library
     constructor on darwin and linux, as Go's c-shared does.
   - A call from a thread Xo did not start attaches the thread on its
     first call: a heap record from heapos and the thread's TLS, as
     heap_thread does for Xo's own threads (Go's needm). The body runs on
     that thread's stack, not on a fiber.
   - A body that spawns tasks or does blocking I/O uses the pool, and
     the export waits on the result, as main waits.
   - A thread's record is kept until thread exit (a pthread key
     destructor frees it).
   - A library has no leak check at exit.
7. **Targets, in order.**
   1. linux/arm64 and darwin/arm64 (stage 1, native backend, linked with
      cc);
   2. linux/amd64 and darwin/amd64;
   3. windows/amd64 (DLL exports, a .def or `__declspec` table);
   4. LLVM;
   5. wasm (component exports, a separate decision);
   6. Xo's own linkers writing MH_DYLIB and ET_DYN (removing the cc
      dependency).
8. **Stages.** Each stage is its own commit with its own proof.
   1. Behind `XO_CLIB=1`: the clause, Int, Float and Bytes parameters,
      Int and Float results, `xo_init` required, faults abort, darwin/arm64
      and linux/arm64, `--lib=shared` and `--lib=static`; a C test and a
      Python ctypes test; call overhead measured against C calling C.
   2. All boundary types, handles, error codes, lazy start, foreign
      thread attach, hidden runtime symbols.
   3. amd64 and windows.

## Consequences

- A new public surface (spec, XO codes, header format) that has to stay
  stable once released. Stage 1 ships behind `XO_CLIB=1` until the
  owner accepts the header format.
- Library builds keep a second runtime start path (`xo_init`) in sync
  with entry.init; a test builds both.
- Handles add a table and a lock per library; Int, Float and Bytes calls
  do not touch it.
- Call overhead: to be measured in stage 1 (`xo bench --ab` on the
  linux/arm64 host: C calling an exported `add(a: Int, b: Int) -> Int`
  and a Bytes sum, 10M calls, against the same functions in C).
- The `export` habit hint (compiler/parser habits_decl.xo: "`export` is
  TypeScript syntax; Xo exports a declaration with `pub`") stays as it
  is. A keyword `export fn` would have turned that habit into a C symbol
  without a diagnostic.

## Alternatives

- **`export fn` / `export "c_name" fn`** (this ADR's first draft): reads
  like Go's `//export`, but `export` is the habit of TypeScript, which
  the parser maps to `pub` with a fix. An agent writing `export fn` out
  of habit would get a C symbol in place of a hint.
- **The module as the marker** (decision 0207's rule for std): a
  library's exports are every pub function of one module. No syntax
  change, but it exports helpers by accident and has no place for the C
  name.
- **An `exports` list in xo.mod**: keeps source files unchanged, but the
  boundary type errors would point at xo.mod, not the declaration.
- **`uses cexport`**, a discipline like `uses realtime` (6.5): no
  place for the C name, and `uses` is about effects.
- **NUL-terminated `char *` strings**: what most C APIs use, but Xo
  Bytes may contain NUL, and the length would be recomputed per call.
  Go chose {p, n} for the same reason. The header can add an `xo_cstr`
  helper.
- **Unwinding a fault into C as a longjmp**: undefined for C++ callers
  and for frames compiled without unwind tables; Go aborts.
