# 0371: empty the functions nothing reaches after code generation (arm64 hollow pass)

Status: Accepted (Xo Lead, 2026-10-24, as size step 1; built the same
day: compiler/arm64 hollow.xo).

## Context

compiler/arm64 reach.xo's `drop_unreached` removes dead functions before
code generation (hello world kept 441 of 1,015, 2026-10-18). It cannot
drop:

- every C ABI export (`Func.export`, the `xo_x_` stubs of
  std/native/llrt/cabi*.xo, decision 0207), since C code may call one by
  name;
- the functions only an export reaches;
- the helpers the emitter makes itself (`_xo_h*`: a type's drop or
  equality), since the emitter calls them and the runtime by symbol
  name, which the SSA does not show.

So a release hello world linked 71 export stubs and the code behind
them: netconn's dial, selects, chan sends, http stream frees. A program
whose link has no C objects never calls an export, and the assembly
names every reference the SSA hides.

## Decision

A pass over the program's assembly, after emit and before assembling
(compiler/xo main.xo, `arm64.hollow_unreached` over each part):

1. **Functions.** A function runs from a global label (`_name:`) in
   `__TEXT,__text` to the next one or the next section switch. Global
   labels with no instruction between them name one body: a drop
   helper's `_xo_f_drop_T:` and `_xo_h...:`. A shared local label in a
   function (`Lxo_drop1:`, which other functions branch to) is a name of
   that function. Its declaration lines (`.globl`, `.private_extern`,
   `.p2align`) name a symbol without using it.
2. **Roots.**
   - `_main`.
   - Every symbol named outside a function's text: data, the unwind and
     exception tables, the unsafe range tables, the export stubs' tables.
     `__DWARF` is not a root, since it names every function.
   - The unsafe range markers (`_xo_utext_*`, `_xo_utend_*`).
   - Reachability is a worklist over the symbols each function's lines
     name.
3. **Hollowing.** A function no root reaches keeps its labels and its
   layout and symbol directives, so the tables that name its labels
   still assemble. It loses its instructions, data and CFI, so no zero
   length FDE lands on the next function. Its body is one
   `brk #0x484f` ("HO", `HOLLOW_BRK`), 4 bytes: a path the scan missed
   faults at once, with an immediate that names this pass, instead of
   running into the next function.
4. **Platforms.** It runs only when all of these hold:
   - darwin/arm64 (Mach-O symbols start with `_`, which the scan relies
     on);
   - the program has no `use c` module, since a C object could call an
     export by name;
   - the build is not `xo asm`, whose output stays the emitter's.

   amd64 (AT&T text, Mach-O or ELF) and linux/arm64 ELF are left as they
   are until each is verified.
5. **Switch.** `XO_PASSES=-hollow` turns it off. The build stays correct
   either way.

## Result (2026-10-24, on 3a435412a, release builds)

| program | __text off | __text on | change |
|---|---|---|---|
| hello world | 235,612 | 164,896 | -30.0% |
| bench/perf/http/xo (CRUD) | 1,519,088 | 1,473,008 | -3.0% |
| xo itself (s2 of the fixed point) | 60,128,952 | 60,097,632 | -0.05% |

CRUD's 11 tests pass with the pass on, and hello runs; the fixed point
holds with the pass on and off (s2 and s3 identical).
internal/arm64 TestHollowUnreached builds an in-process std/http router
program. It checks that an unreached C ABI export
(`_xo_x_scope_fail_site`) and an unreached drop helper
(`_xo_f_drop_..._RootsData_3f`) are each one `brk #0x484f` (and not
with the pass off), and that the program prints the same either way.
compiler/arm64 hollow_test.xo covers the scan on a synthetic assembly:
the aliased labels, a shared local label, and a data root.

## Consequences

- Each dead function keeps its alignment padding and a 4 byte brk.
  The address space a hollowed function took is not reclaimed beyond
  that.
- The dead functions' unwind entries, exception tables and
  `_xo_unsafe_*` rows stay, now naming an empty range. Dropping them is
  a follow-up.
- A reference the scan does not see shows up as a SIGTRAP at a
  `brk #0x484f`. Two examples: an address computed at run time, or a
  name looked up by profiler symbols. The first cut hit one such case
  (a `bl Lxo_drop1` into a helper), which the shared local label rule
  fixes.
- The per-function backend cache is unaffected: the pass runs on the
  assembled parts' text, after the cache.

## Alternatives

- Teach reach.xo the emitter's symbol references (drop helpers, runtime
  calls) and drop exports when no C is linked. Rejected for step 1:
  every emitter change would have to keep that list in step, and the
  assembly already has it.
- Linker dead stripping (`-dead_strip`). The internal Mach-O linker has
  no atom model. Its sections are whole, so it would first need
  subsections_via_symbols per function. That is larger than this pass,
  and a candidate for a later step.
- An empty body (no brk). A missed reference would then run into the
  next function's code. Rejected (Lead, 2026-10-24).
