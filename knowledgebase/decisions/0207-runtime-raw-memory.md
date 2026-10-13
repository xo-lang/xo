# 0207: Raw memory and C ABI exports for the runtime

Status: Accepted (2026-10-13), stage 1 implemented: the C ABI export
mechanism of decision 0204 and the raw memory intrinsics of decision
0195 prerequisite 2, internal to std (decision 0074). Stage 2 (design
below): reference counts implemented 2026-10-13 (std/native/llrt/rc.xo,
the default; `XO_LLRT_C=rc` keeps the C until a full tier passes on
main), with `raw.zero_ints`; the allocator not started (what it needs:
results file, stage 2).
Files: `std/native/llrt/cabi.xo`, `cabi_tls.xo`, `cabi_time.xo`,
`raw.xo`, `netpolicy.xo`, `tlsname.xo`, `std/native/rt.xo` (raw memory),
internal/ssa `rawmem.go`, selfhost/ssa `build.xo` (declare_exports) and
`call.xo` (raw_intrinsic, layout_value), the emitters (internal/arm64
isel.go inlineRaw and arm64.go exports; internal/llvm fromssa.go
inlineRaw and exports; selfhost/arm64 calls.xo and gen.xo;
selfhost/llvm call.xo and gen.xo; selfhost/wasm call.xo and rt2.xo),
std/llrt (xo_rt.c, xo_std.c, xo_bytes.c, xo_http.c, xo_time.c,
xo_tls.c). Tests: TestCABIDiff (new), the C only runtime tests through
`cOnly`, the goldens. Evidence: `bench/results/raw-memory-2026-10-13.md`.

## Context

Decision 0195 prerequisite 2 (raw memory) unblocks 6,574 lines of
std/llrt (the zero-C audit), and decision 0204 lets the C runtime call
runtime functions compiled from Xo, so the audit's "(0) held by C
callers" group (1,806 lines) can move before its callers and leaf
pieces (the allocator, reference counts) can move before the C that
calls them. Both must stay internal: no user visible language or spec
change.

## Decision

1. **The export marker is a module.** Every top level function of the
   runtime module `native/llrt/cabi` is a C ABI export named
   `xo_x_<name>`; so is every function of a module
   `native/llrt/cabi_<feature>`. Only std's implementation modules can
   load runtime modules (decision 0080), so user code cannot export. The
   native prelude imports cabi (the checkers exempt these imports from
   XO0110); a feature's implementation module imports its cabi_<feature>
   (std/native/http.xo imports cabi_tls, std/native/time.xo cabi_time).
2. **Shapes.** Parameters are Int or Float (an address is an Int; C
   passes `(int64_t)(intptr_t)p`); the result is an Int, a Float,
   nothing, or a Str returned as its string object with its length
   stored through a pointer the C caller passes after the parameters, as
   the C runtime returns strings. The SSA builders report anything else
   unsupported. An export may not fault (a fault would unwind through
   its C caller); exports write text into C buffers (`raw.put_cstr`)
   rather than handing C strings to free where they can.
3. **Rooting.** The SSA builders (internal/ssa declareExports,
   selfhost/ssa declare_exports) build the exports as roots after the
   program's own code, in the order of module paths and then of names:
   cabi's always, a cabi_<feature>'s only when the program built code of
   a module that imports it (module loading is no signal: std/testing
   loads std/http into every program,
   gotchas/std-testing-loads-http-everywhere.md). Each is an Entry
   function (owned parameters, no by-reference parameters) with
   `Func.Export` set. The wasm backend builds none (no C calls them).
4. **Emitters.** arm64 (both emitters): Xo functions already follow
   AAPCS64 for Int and Float words, so the export is a global symbol that
   branches to the function; a Str result gets a frame that keeps the
   pointer in x19 and stores x1 through it. LLVM (both): an external
   function of the default (C) convention that calls the internal one
   and, for a Str, stores the length. C declares each export in xo_rt.c
   (or the feature's file); C only tests link an empty program's IR
   for them (internal/llvm `cOnly`).
5. **Raw memory intrinsics** (std/native/rt.xo): `mem_load(a, off, w)`
   (zero extended), `mem_store(a, off, v, w)` with w a constant 1, 2, 4,
   or 8 (both SSA builders reject another); `mem_copy`, `mem_move`,
   `mem_set`, `mem_cmp` (the platform's memcpy, memmove, memset, memcmp
   on arm64, LLVM's intrinsics and memcmp on LLVM, `memory.copy`,
   `memory.fill`, and a loop in the wasm runtime); `layout(k)`, folded
   by the SSA builders from one table (internal/ssa `Layout`,
   selfhost/ssa `layout_value`; native and wasm columns: string data,
   string capacity, list elements, list length, list capacity, count,
   address bytes, count bytes); `obj_addr[T](x)` (a Str's, List's, or
   handle's object, no count change); `obj_at[T](a)` (a List or handle at
   an address, retained once so the result's release leaves the count
   as it was); `rs_set_len` (decision 0201's). All unchecked. The
   atomics of decision 0200 take an address as their container
   (`slot_*[Int](a, i, ...)`: the word at a + 8 * i) in every emitter.
6. **What moved** (results file, table): number text and fault texts of
   xo_rt.c, UTF-8 decode and encode and IsSpace, Int parse, Duration and
   Time text, base64 digits and days_from_civil of xo_std.c, the Bytes
   index fault, the zip directory reader of xo_time.c, Net.only's address
   policy, and xo_tls.c's PEM decoding, name checks, hostname error text,
   and alert text. std/llrt: 883 lines deleted, 151 added, and 176 more
   compiled only for `XO_LLRT_C=text`. The test world stores (412 lines)
   are C state under C locks and wait for (1) and (2).
7. **The bar** (decision 0193 item 5) holds for everything moved:
   bench/perf/text and bench/perf/httpparse within 5 percent on both
   backends, trim 0.978 and 0.985 after unicode.is_space tests its ranges
   first, h1_get 1.006 and 1.008.

## Design: reference counts and the allocator in Xo (stage 2)

Not implemented; the order and the rules for the step after this one.

1. **Reference counts first.** `xo_rc_inc`, `xo_rc_dec`, the shared
   count paths, `xo_rc_claim`, `xo_rc_free`'s dispatch, and the list and
   string free walkers move to a runtime module `native/llrt/rc.xo` as
   cabi exports (the C and the generated code both call them; the
   emitters' inline fast paths stay inline and keep calling the slow
   path by its C symbol, now an export). The count word is
   `slot_load/slot_add/slot_cas[Int](obj, 0, ...)` at
   `obj + layout(5)`: the thread local path a plain mem_load/mem_store,
   the shared path (bit 62) atomic, the immortal check a compare; the
   orders are those of xo_rt.c's `rc_dec_shared` (acq_rel on the last
   decrement). Children are released through the element callbacks the
   C already passes (`xo_elem_fn`), called as function addresses.
2. **Then the allocator.** Size classes (`xo_alloc`'s table), the per
   thread heap (the heap pointer is `__thread` today: decision 0195's
   (6) gives `rt.tls_get/tls_set`; until it lands the heap stays reached
   through a C accessor export of one word), the free lists as intrusive
   singly linked lists in the blocks (`mem_load/mem_store` of the next
   pointer at the block), the central lists under the C mutex first and
   `slot_cas` later, and the leak counters (`allocs`, `frees`) as words
   of the heap record. Large blocks stay malloc (a platform call).
3. **The no allocation, no counting rule.** Code in rc.xo and the
   allocator's module must not allocate or count: every function there
   takes and returns only Int and Float words (addresses), uses no Str,
   List, Map, closure, or interpolation, and is checked as a real-time
   function (decision 0133's check already proves a function allocation
   free; the module's functions are marked so by being in these marker
   modules), plus a rule in the SSA builders that a value from `obj_at`
   or an address is never counted (no `obj_at` in these modules at all).
   A violation is a build error of std, not a runtime surprise.
4. **Verification.** The leak check (every golden ends with `xo
   leakcheck: 0 live`, LLVM and arm64), the C only stress tests
   (rc_threads.c, tasks.c through `cOnly`) under ThreadSanitizer where it
   runs (TestRuntimeThreads, TestGoldenThreads with four threads per
   golden), TestCABIDiff extended with counts and allocation statistics
   compared with the C path for one release (`XO_LLRT_C=rc` keeps the C
   copy selectable, decision 0193 item 3), the bar on bench/perf (an
   allocation heavy workload set: list push, string append, map insert,
   task spawn), and a fuzz of interleaved retain, release, share, and
   claim across threads against a reference model.

## Stage 2 as implemented (2026-10-13)

- **Zero filled lists**: `raw.zero_ints(n)` (with_capacity, one
  mem_set, the length stored at `layout(3)` with the width of
  `layout(6)`), used by archive's zeros, regexp's queue, and jsontext's
  key table. With it the Argon2id in Xo is 0.980 of the C's
  instructions (less setup) on arm64 but 1.155 on LLVM, so decision
  0180's bar still fails and Argon2id and BLAKE2b stay C; the LLVM cause
  is the unvectorized xor loops (the `var` List parameters may alias,
  gotchas/llvm-list-params-block-vectorizing.md).
- **Reference counts** as design 1, with these differences: the module
  is `native/llrt/rc` (inc, dec, dec_shared, claim, mark) and the
  exports are cabi.xo's `rc_*` wrappers (cabi's marker rule; rc.xo is
  not itself a marker); xo_rc_free's dispatch and the free walkers stay
  C (they call element callbacks by address, which Xo cannot yet: an
  `rt.call_addr` intrinsic is the missing piece). Generated code keeps
  its inline paths, so only the C runtime's own count operations moved.
  The C path is rc_c.xo calling `rt.rc_*_c` (-DXO_RC_C).
- **Design 3** is enforced in the self hosted SSA builder
  (selfhost/ssa contract.xo `check_no_alloc`): scalar values only, no
  fault, no closure or box, no unwind block, and calls only of inline
  intrinsics; a call of another function is refused because the
  builders add a landing check after every call while building
  (gotchas/call-adds-landing-check.md). The Go builders, frozen, do not
  check it (std is the same source for both).
- **Design 4**: rc_threads.c's `model` (random interleaved retain,
  release, claim, and mark across 8 threads against each thread's own
  knowledge of its references) and the whole file run under
  ThreadSanitizer for both paths (TestRuntimeThreads); the bar on
  bench/perf/nested, bench/perf/xo, text, json, and conc holds within 5
  percent on both backends (worst steady case text debug on arm64,
  1.037 less setup). Not done: TestCABIDiff's count and allocation
  statistics against the C path.

## Consequences

- C may call Xo, and Xo may read and write C memory: the remaining C
  can be moved leaf first where that is easier.
- Every native program carries cabi.xo's closure: a hello world on
  arm64 grows by 20,720 bytes; LLVM builds do not grow (LTO). The SSA
  goldens and `-short` of internal/ssa grow with it
  (gotchas/exports-grow-every-program.md).
- TLS name checks follow Go's net.ParseIP exactly (no leading zeros in
  IPv4 fields) and lose the C's 256 byte name limit.
- `rt.rune_is_space` and `rt.net_admits` are gone (their callers are Xo).

## Alternatives

- **A syntax marker** (an attribute on `fn`): rejected, a language
  change; the module is enough and keeps the exports in one place.
- **Root a feature's exports by the modules loaded**: wrong, every
  program loads std/http (gotcha above).
- **Weak C references to exports a program does not build**: Mach-O
  rejects undefined weak symbols outside dylibs at static link time, and
  selfhost/macho would need it too.
- **Strings across the boundary as Str parameters**: the RC pass infers
  borrowing per function, so a C caller could not know whether to keep
  its reference; addresses and lengths avoid the question.
