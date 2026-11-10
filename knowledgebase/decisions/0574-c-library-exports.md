# 0574: C library build modes (exported functions, shared and static libraries)

Status: Accepted (owner, 2026-11-08, on the lead's recommendation, with a separate `XoStr` header type (same layout as the string header) so the UTF-8 rule is visible to C programmers). Proposed (Xo Lang 4, 2026-11-08; owner decides). The owner
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
     typedefs (`XoBytes`, `XoStr`, `XoHandle`), and `xo_init`, `xo_free` and
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
   | Bytes (parameter) | `XoBytes {const uint8_t *p; int64_t n}` | borrowed for the call; Xo copies what it keeps (a view would outlive the C buffer) |
   | Str (parameter) | `XoStr {const char *p; int64_t n}` (the owner's acceptance, 2026-11-08: its own type, same layout, so the UTF-8 rule shows in the header) | borrowed and copied as Bytes; n bytes of UTF-8, no NUL needed; each byte of an invalid sequence arrives as U+FFFD (scan.text_at, decision 0259) |
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
     without a main fiber, and switches the allocator to one heap record
     per thread (the stage 1 prototype found the single threaded record
     crashes under concurrent C callers). It is idempotent and thread
     safe.
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
  stable once released. Stage 1 shipped behind `XO_CLIB=1` until the
  owner accepted the header format; the owner accepted it on 2026-11-08
  (with XoStr) and the flag is gone (2026-11-08): `--lib` and the `as c`
  clause work without it.
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

## Stage 1 prototype (2026-11-08, branch l4-clib, not on main)

The prototype is built and passing; it does not push until the owner
accepts this ADR, and then pushes with spec/core.md's grammar and 3.2
text and the XO1518 row in the same change.

What it has:
- the `as c "NAME"` clause (parser `CExportClause`, AST codec v9) and
  its checks (XO1518: not `pub`, a method, generic, `var` parameters, an
  error result, a parameter or result outside the boundary types, a name
  that is not a C identifier or starts with `xo_`, more than 8 integer
  words of parameters);
- `xo build --lib=shared` behind `XO_CLIB=1`, arm64 native backend,
  linked with cc (`-dynamiclib -install_name @rpath/libNAME.dylib`;
  `-shared -Bsymbolic -soname libNAME.so`), no DWARF segment, no hollow
  pass (it roots at `_main`), `NAME.h` beside the library;
- `xo build --lib=static`: each part assembled with cc, the parts and
  the constructor joined into one object (`cc -r`: a C link takes an
  archive member only for a symbol it needs), then `ar rcs`; on darwin
  the runtime's weak references the object leaves undefined (the
  ThreadSanitizer fiber hooks, which a program's link passes as `-U`)
  are absolute symbols at 0 in a member of their own, so the C
  program's link resolves them as a program's link would; on linux the
  C program links with `-lm -lpthread -ldl`;
- `xo_init(argc, argv, envp)` in cabi_c.xo (entry.init, then
  entry.start), in place of `main`, and an empty SSA `main` for the
  backends' xo_entry; in a shared library it is also the constructor
  (`__mod_init_func`, `.init_array`), which the loader calls with the
  process's arguments, and a second call does nothing, so C need not
  call it (stage 2's lazy start, brought forward; cmd/xo TestCLibNoInit
  and TestCLibStatic);
- Bytes and Str parameters: a generated wrapper `xo_cx_NAME` takes
  `XoBytes {p, n}` as two words and copies the bytes (raw.str_at)
  before calling the function; a Str parameter is `XoStr` in the header
  and is copied by scan.text_at, which makes invalid UTF-8 U+FFFD
  (2026-11-08, on the owner's acceptance);
- only `xo_init` and the exports are the shared library's symbols
  (decision 2's hidden runtime names, brought forward from stage 2);
- linux: thread locals by initial exec (`:gottprel:`, a `tls_ie` flag
  in the function and part cache keys) in calls.xo tl_base, the
  scheduler's xo_tlocal_* and the preempt stub; local exec
  (`:tprel_hi12:`) cannot link into a shared object (ld.lld:
  "relocation R_AARCH64_TLSLE_ADD_TPREL_HI12 against xo_tlocal cannot
  be used with -shared").

The proof is the library below, called from C and from Python:

```
pub fn add(a: Int, b: Int) -> Int as c "geo_add" { a + b }
pub fn norm(x: Float, y: Float) -> Float as c "geo_norm" { (x * x + y * y).sqrt() }
pub fn list_sum(n: Int) -> Int as c "geo_list_sum" { ... a List of n Ints, summed ... }
pub fn byte_sum(b: Bytes) -> Int as c "geo_byte_sum" { ... }
pub fn str_len(w: Float, s: Str, k: Int) -> Float as c "geo_str_len" { w * Float(s.len() + k) }
```

- The C test (cmd/xo TestCLibShared on the branch) includes the
  generated `geo.h`, calls `xo_init`, and checks each export: 42 from
  `geo_add(2, 40)`, 5.0 from `geo_norm(3, 4)`, 4999950000 from
  `geo_list_sum(100000)` (the heap from xo_init), a 64 byte buffer's sum
  and an empty `{NULL, 0}` through `geo_byte_sum`, and a Str's words
  between a double and an int64 through `geo_str_len`.
- The Python test (`python3 -I test_ctypes.py LIBPATH`):

  ```python
  # Stage 1 Python caller of libgeo through ctypes (decision 0574):
  # python3 -I test_ctypes.py LIBPATH
  import ctypes, math, sys
  lib = ctypes.CDLL(sys.argv[1])
  lib.xo_init.argtypes = [ctypes.c_int, ctypes.c_void_p, ctypes.c_void_p]
  lib.xo_init.restype = ctypes.c_int64
  lib.geo_add.argtypes = [ctypes.c_int64, ctypes.c_int64]
  lib.geo_add.restype = ctypes.c_int64
  lib.geo_norm.argtypes = [ctypes.c_double, ctypes.c_double]
  lib.geo_norm.restype = ctypes.c_double
  lib.geo_list_sum.argtypes = [ctypes.c_int64]
  lib.geo_list_sum.restype = ctypes.c_int64
  class XoBytes(ctypes.Structure):
      _fields_ = [("p", ctypes.c_char_p), ("n", ctypes.c_int64)]
  lib.geo_byte_sum.argtypes = [XoBytes]
  lib.geo_byte_sum.restype = ctypes.c_int64
  lib.geo_str_len.argtypes = [ctypes.c_double, XoBytes, ctypes.c_int64]
  lib.geo_str_len.restype = ctypes.c_double
  assert lib.xo_init(0, None, None) == 0
  assert lib.geo_add(2, 40) == 42
  assert lib.geo_add(-5, 3) == -2
  assert math.isclose(lib.geo_norm(3.0, 4.0), 5.0)
  assert lib.geo_list_sum(100000) == 4999950000
  data = bytes(range(200))
  assert lib.geo_byte_sum(XoBytes(data, len(data))) == sum(data)
  assert lib.geo_str_len(2.0, XoBytes(b"h\xc3\xa9llo", 6), 1) == 14.0
  print("ok")
  ```
- TestCLibStatic links the same library and C test from `--lib=static`,
  and a C caller that never calls `xo_init`.
- TestCLibRejects checks XO1518 for seven bad exports and that
  `--lib=shared` refuses a package with `fn main`.

Both pass on darwin/arm64 (the Mac) and linux/arm64 (offload host 2,
glibc, ld.lld; the .so has its SONAME, no TEXTREL, and needs only libc
and libm). The shared library exports only `xo_init` and the exports
(an exported symbols list on darwin, a version script with `local: *`
on linux; TestCLibShared checks the list with nm); without them the
linux library exported 201 symbols, the runtime's C names among them.

Threads: an export called from a C thread Xo did not start ran (one
thread at a time), but four C threads calling an allocating export at
once crashed (SIGSEGV): after entry.start the runtime is still single
threaded, and the allocator hands every thread the main thread's heap
record (obj.heap_word's single threaded word). The prototype's xo_init
now makes the switch pool.xo makes before its second thread
(cabi.xo heap_threads: one heap record per thread, made on the thread's
first allocation by heap_thread, and standard output's lock).
With it, four C threads making 20000 allocating calls each return
the right sums (3 runs on the Mac; cmd/xo TestCLibThreads on the branch).
A thread's record is never freed in stage 1: heapos.thread_heap maps
one and links it into the records' list for good (Xo's own threads live
as long as the process). A C thread that calls an allocating export and
exits costs about 32 KB (max RSS 2.4 MB, then 34.6 MB after 1000 such
threads, on the Mac); stage 2 reuses a record freed by a pthread key
destructor. A fault in an export (`[10, 20][5]`) prints the fault
report and exits 70, from the main thread and from a foreign one, as
decision 4 has it for stage 1. Its traceback first named every frame
from the C executable's symbols (`run`, `main`): the symbolizer read
image 0 (macOS) or /proc/self/exe (Linux). The prototype reads the
image holding Xo's code (dladdr of xo_entry on macOS, the dynamic
linker's link map on Linux) for the names and lines, and ends the
traceback at the first frame outside it, named by the dynamic linker:
`boom.boom`, then `main (C)` (cmd/xo TestCLibFault).

Call overhead (100M calls of `geo_add`, 10M calls of `geo_byte_sum`
over 64 bytes; against the same functions in C, `noinline` in the
executable (c_) and in a C shared library called through the dynamic
linker as the Xo library is (cs_); the C caller at -O2):

| host | geo_add | cs_add | c_add | geo_byte_sum (64 B) | geo_byte_len (64 B) | cs_byte_sum | c_byte_sum |
|---|---|---|---|---|---|---|---|
| darwin/arm64 (Mac, 1 run) | 1.27 ns | 1.02 ns | 0.76 ns | 24.6 ns | 20.6 ns | 4.1 ns | 4.1 ns |
| linux/arm64 (offload host 2, the pass/fail host, under load; median of 5 runs) | 0.94 ns | not run | 0.47 ns | 24.8 ns | not run | not run | 16.3 ns |
| linux/arm64 (offload host 1, the timing host; median of 5 runs, 2026-11-08) | 1.28 ns | 1.02 ns | 0.77 ns | 34.3 ns | not run | 18.1 ns | 17.9 ns |
| darwin/amd64 (the Intel laptop, cross built on the Mac; median of 3 runs, 2026-11-08) | 2.14 ns | 1.45 ns | 0.94 ns | 46.4 ns | 15.4 ns | 10.4 ns | 10.5 ns |

On host 1 the Int call costs a quarter nanosecond over a C library's, as
on the Mac. On the Intel laptop it costs 0.7 ns over a C library's, and
`geo_byte_sum` less `geo_byte_len` (31 ns) is the x86 backend's 64 byte
loop, not the boundary: the arm64 backend's loop is about 4 ns on the
Mac, C's 10 ns on the laptop. That loop is an x86 code generation item,
outside this decision.

(`geo_byte_len` returns `b.len()`: the boundary's copy alone.)

An Int call into the Xo library costs about a quarter nanosecond more
than the same call into a C library. The export's body is C's plus Xo's
overflow check (`adds x0, x0, x1; b.vs <fault>; ret`, no frame, no
saved registers), and the export's symbol is a `b` to the function's own
label: one more branch than C's. The Bytes call's extra cost is the copy:
on the Mac, `geo_byte_sum` less `geo_byte_len` is C's loop time, so the
20 ns over C
is the wrapper's allocation, memcpy and free of the 64 bytes (all
inlined into `xo_cx_geo_byte_sum` but the allocator's call). A plain Xo
program's `b.clone()` of 64 bytes takes 16 to 17 ns on the same Mac (3
runs of 10M), so the library path adds about 4 ns and the rest is Xo's
own allocation, copy and free. C's malloc, memcpy and free of 64 bytes
takes 15.5 to 16.6 ns on the same Mac (a profile of the Xo clone: the
allocation 37%, libc's memmove 27%), so Xo's allocator is at C's speed
and the only saving left is not copying: a borrowed Bytes over the C
buffer for the call, when the function is checked not to keep it (Xo
cannot keep a view of C memory past the call), else the copy.

Stage 2 progress (branch, held with stage 1 until this decision is
accepted; 2026-11-08):

- amd64: `--lib` builds with the amd64 backend (linux thread locals by
  initial exec, `movq xo_tlocal@GOTTPOFF(%rip)`, as arm64's `:gottprel:`).
  The seven cmd/xo TestCLib tests pass on the linux/amd64 offload host;
  on the Intel laptop the C caller, four concurrent C threads, and 1000
  exiting C threads (max RSS 1032 to 1116 KB) pass.
- Error results: `-> T ! E` exports as table row `T ! E` above, with 1
  as the code: `int32_t *err` last (0 on success, 1 on failure, NULL
  not set), 0 returned on failure, and the error's Display text for
  `xo_last_error()`, kept in the calling thread's heap record (freed by
  the thread's next error or its exit). cmd/xo TestCLibErrors (Int,
  Float and unit results, a NULL err).
- Constructor order: an export of a static library called from a
  constructor that runs before the library's own (the program's link
  order) crashed; a static library's exports now start the runtime
  first (`xo_start_early`), and the constructor's later `xo_init` adds
  the process's arguments and environment. Shared libraries skip the
  check (the loader runs a dependency's constructor first; the check
  cost 0.2 to 0.3 ns a call on the Mac).
- Handles: a parameter or result of any type but `Bool` and function
  types crosses as an `XoHandle` (`typedef uint64_t XoHandle`), a slot
  of the library's table (std/native/llrt/clibh.xo) holding the value as
  an `rt.Any` with one count, under a lock, with the slot's generation in
  the handle's high bits: `xo_release(h)` drops it, and a released or
  forged handle, or one holding another type, faults. A result is shared
  before it is kept (any C thread may use it). cmd/xo TestCLibHandles;
  8 C threads making, reading and releasing 320,000 handles: no wrong
  result, max RSS flat at about 3 MB. A call through a handle costs 12.4
  ns on the Mac (28 ns before `rt.any_get` read the Any's value in place
  instead of copying it); the rest is the lock and the shared count.
- Optional results and parameters (2026-11-10): an `Option[T]` whose T
  crosses as a handle is 0 for `None`, else T's own handle (the header
  writes `XoHandle /* T or 0 */`), so C tests it against 0 and reads it
  with T's exports; a C caller passes 0 for `None`. An Option of `Int`,
  `Float`, `Str` or `Bytes` stays a handle of the Option until an
  accessor API (stage 3). A handle result of a type with nothing on the
  heap (an `Option[Int]`, a struct of Ints) failed the build (the
  verifier: a share of a value not on the heap); it is shared only when
  it holds a heap value now. List results were already handles (the
  `List[Int]` rows below). cmd/xo TestCLibHandles.
- Bytes and Str results (2026-11-10), the table's row: the export's
  wrapper copies the bytes into C's malloc with a NUL past them and
  returns `XoBytes` or `XoStr` {p, n} in two registers (x0 and x1; rax
  and rdx), which the caller frees with `xo_free(p)` (NULL does
  nothing); an error result returns {NULL, 0}. The checker had rejected
  them (XO1518) while the table listed them. cmd/xo TestCLibHandles
  (ctr_label, ctr_tail, ctr_label_or).
- LLVM (2026-11-10; target 4 above): `--backend=llvm --lib=shared|static`.
  The wrapper is the SSA builder's, as for the native backends; a shared
  library links through clang with the same exports list and
  constructor, and a static library compiles each module to an object
  with no LTO (a C program's link runs none over an archive) and archives
  it with the constructor (ar_static). On linux the thread local table is
  initial exec, as the native backends' (the general dynamic model calls
  __tls_get_addr on every access from a shared library). cmd/xo
  TestCLibLLVM: the geo and ctr libraries, shared and static, with their
  C callers (Mac).
- The TestCLibThreadExit build failure of 2026-11-08 (exit 1): found.
  Separate compilation linked a library against std's cached program
  variant, which lacks the runtime's library entries; the first library
  build missed the cache and started that variant's build in the
  background, and a later one linked against it. A library is now always
  built whole, and the front end cache's key has the library mode
  (gotchas/clib-sepcomp-variant-link.md).

Stage 2 call costs (2026-11-09; a C caller at -O2 against the same
functions in a C shared library; 20M calls a row, 2M for the error, make
and List rows; ns a call; host 1 = the linux/arm64 timing host, median
of 5 runs; Mac Studio = the M-series Mac Studio, 3 runs, under load,
its handle rows from the build before the release fast path below):

| row | host 1 | host 1, C | Mac Studio | Mac Studio, C |
|---|---|---|---|---|
| `Int ! Str`, success | 2.26 | 2.24 | 2.24 | 2.26 |
| `Int ! Str`, error | 5.4 (30.0 before) | 1.74 | 6.5 (31.8 before) | 1.52 |
| error, then `xo_last_error()` | 6.7 (30.1 before) | 2.99 | 7.8 (32.2 before) | 2.43 |
| a read through a handle | 6.4 (18.7 before) | 0.99 | 4.7 (10.5 before) | 0.69 |
| make a struct handle, read it, release it | 92 (107 before) | 12.3 | 53 to 65 (69 before) | 28 to 32 |
| make a List[Int] handle (10 items), sum it, release it | 132 (129 before) | 16.3 | 81 to 92 (93 before) | 30 to 36 |

An error result keeps the thread's error text buffer for the next error
(its capacity in the block's first word) and passes a Str error as its
own text, where it formatted the error into a new Str and allocated and
freed the C copy on every error.

A handle parameter is read in place (the "after" rows): the export's
wrapper starts a read of the handle table (clibh.read_begin: the
thread's heap record shows the table's epoch, then a full fence), takes
the value's list with no count (anyval.clib_peek, then rt.obj_peek, an
rt.obj_at that takes no count and that the SSA's reference counting
treats as a borrowed root), passes its element as the argument (counted
only where the function keeps it) and ends the read after the call.
xo_release puts the released value on a list, freed once no read that
began before the release is running, and the table is chunks that never
move, so a read takes no lock; a release with no read running and
nothing waiting drops its value at once (the List row was 141 ns on
host 1 while each release built a list of what it freed). cmd/xo
TestCLibHandleChurn: 4 C threads
read one handle while 4 make, read and release their own.

Stage 2, Bytes and Str (2026-11-10; the same harness with three more rows,
a C caller at -O2 against the same functions in a C shared library, host 1,
median of 5 runs, 20M calls (2M for upper), ns a call; before = f3f8e4641,
buffers = 9822bd846 (the per-thread parameter buffers), then 8a27337e4 (one
read of the buffer table a call, and an all ASCII Str skips the UTF-8 scan)):

| row | before | buffers | one table read, ASCII | C |
|---|---|---|---|---|
| `byte_sum(b: Bytes) -> Int`, 64 bytes | 33.3 | 23.5 | 21.8 | 17.6 |
| `str_len(s: Str) -> Int`, 16 bytes | 25.7 | 10.7 | 7.0 | 1.0 |
| `upper(s: Str) -> Str`, 32 bytes, then `xo_free` | 36.5 | 30.0 | 27.3 | 14.2 |

A view of C's buffer in place of the copy does not fit decision 0330's
layout: a Str's data sits at its object's header plus a 32 bit offset, and
counting reads and writes that header, which C memory does not have. The
buffer keeps the copy but not the allocation. What is left of the 16 byte
Str call (perf on host 1): the copy into the buffer 36%, giving the buffer
back 16%, the thread record's lookup 20%.

The create path (make a struct handle, read it, release it, 92 ns against
C's 12.3): perf on host 1 puts it in the shared counts (rc.dec of the
value, 17.6% self; the struct's Str field dropped through an atomic
decrement, 10.9%) and the four allocations a make takes (the value, its
list of one, the Any and its tag; obj.rl_free 13%). The fence and the
c_read_min walk are not the cost: a build without them (unsafe, measure
only, 2026-11-09) ran 91 ns. A slot that holds a heap value's object
directly (no Any, no list of one, no tag copy) is the next design to
measure; the shared counts stay, since any C thread may use a handle.

The slot that holds the value, measured first (2026-11-10, host 1, the
same harness, median of 5, make + count + release of a struct handle):
main 89.8 ns; the Any keeping its type's constant tag instead of a copy
(one allocation pair fewer) 90.2 ns; three extra allocation pairs a make
107.5 ns, so a pair here costs at most about 5.9 ns. A slot holding the
value's object drops the list of one and the Any as well: at most about
12 ns of the 77 over C, under the 15 ns that would justify the design, so
it is not built. The rest is the shared counts (rc.dec of the value and
its Str field's atomic decrement, about 28% of the profile) and the
handle lock and epoch; a cheaper path there would have to stop sharing a
handle's value until a second thread reads it.

Tried and dropped on the way (2026-11-09):
- The lock alone: a read without the table's lock, but still counting
  the Any, its list and the value (4 reading and 4 releasing C threads:
  no wrong value, max RSS flat at 3.7 MB), measured 20.8 ns against 21.0
  on host 1 and 10.7 against 10.5 on the Mac Studio. perf on host 1 put
  the time in the counts: six atomic operations a read (the Any, its list
  and the value each counted and dropped), about 3 ns each on Graviton.
- rt.Any keeping its type's tag as the generated constant (no heap copy
  per Any, the tag compared by address first): no change on host 1, and
  it touches every program's rt.Any.

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
