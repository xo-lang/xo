# 0234: Exports with an exact C name, data symbols, and backend entry stubs

Status: Accepted (owner, 2026-10-15). Step 1 implemented 2026-10-15;
step 2 (main in Xo) waits for the 0.15.0 seed.

## Context

After seed 0.14.0 the runtime's C was std/llrt/xo_rt.c (244 lines) and
xo_nolibc.c (128): C by design only because Xo could not express a few
things (bench/results/zero-c-audit-refresh-2026-10-13.md, end state):

- `main`. The C library's start calls the symbol `main`; every Xo
  export was `xo_x_<name>` (decision 0207) except cabi_libc.xo's, which
  keep their C names only in an XO_NOLIBC build (decision 0230). C main
  also needed `xo_entry`'s address (the generated program entry), which
  rt.fn_addr (decision 0233) cannot name: it is not an export.
- Data symbols. An XO_NOLIBC build's C library defines `environ`,
  `stdin`, `stdout`, and `stderr`, which xo_nolibc.c defined in C.
- The XO_NOLIBC process entry (`_start`, the thread pointer, the switch
  to main's 1 GiB stack): inline assembly in xo_nolibc.c.
- The variadic printf family of an XO_NOLIBC build (`va_start` needs
  the callee's register save area), C shells over libcio.xo's vsnprintf
  and vfprintf.

The owner asked for the last runtime C to be tried (2026-10-15).

## Decision

1. **Exports with an exact C name.** std/native/llrt/cabi_c.xo is an
   export module whose top level functions are the C symbols of their
   names (not `xo_x_<name>`) in every native program (selfhost/ssa
   declare_exports; selfhost/check exports.xo keeps them out of the
   `xo_x_` namespace check). Its `main(argc, argv, envp) -> Int` is the
   program's C entry: argc is a C int (the low 32 bits of its word); the
   C libraries of Linux and macOS and the MSVC C runtime all pass envp
   as the third argument, so main reads no `environ` (on Windows
   process.xo reads Windows' own environment, so envp is not used).
2. **rt.sym_addr("name").** An intrinsic (std/native/rt.xo) for the
   address of a C level symbol under its exact name: `xo_entry`, the
   emitters' data symbols, or a C function. SSA: a FuncAddr whose aux
   is `&name` (the verifier accepts any non-empty name; rt.fn_addr's
   `@xo_x_<name>` keeps its export check). arm64: adrp/add for
   xo_entry and the module's data symbols, a GOT load for anything else
   (a symbol in a shared library has no fixed distance from the code).
   LLVM: `ptrtoint ptr @name`, declared `declare void @name()` unless
   the module defines it; the name is unquoted, as the split build
   matches a reference to its definition by text
   (gotchas/llvm-split-quoted-symbol-name.md). A fault when run in a
   wasm module.
3. **Data symbols.** In an XO_NOLIBC build (ssa.Module.nolibc) the
   emitters define `environ`, `stdin`, `stdout`, and `stderr`
   (ssa.nolibc_data): one zero word each, a global object in the arm64
   emitter's assembly and a `weak global i64` in LLVM IR (weak, so the
   split build's copies are one symbol and an unused one is kept), and
   cabi_nolibc.xo nl_start sets them through rt.sym_addr. Exporting Xo
   globals in general (a `pub` data declaration) was not needed: the
   C library's data symbols are a fixed list.
4. **Backend entry stubs** (Xo lang 5, add01079): the backends emit
   `_start` for an XO_NOLIBC build (ssa entry_stub_arm64 and
   entry_stub_amd64: arm64 assembly, LLVM module asm), which calls
   `main(argc, argv, envp)` with envp in x2/rdx. The C start stays
   selectable (XO_NOLIBC_C_START=1) until CI has run the stubs; it
   passes envp too from step 2 on.
5. **No printf family.** Nothing calls snprintf, sprintf, fprintf, or
   printf in an XO_NOLIBC build: such a build takes no `use c`
   (selfhost/xo main.xo, decision 0205), xo_rt.c and the emitters call
   no printf, and every mention in std/native is a comment. The four C
   shells, cabi_libc.xo's vsnprintf and vfprintf, and libcio.xo's
   formatter and va_list readers are deleted. A future C caller under
   XO_NOLIBC would fail to link, naming the function.

## Seeds: two steps

The 0.14.0 seed's compiler, which builds the next xo, knows neither
rt.sym_addr nor cabi_c.xo: it would export cabi_c's main as `xo_x_main`
and lower rt.sym_addr as a call of an undefined `xo_sym_addr`, and the
xo it builds would have no `main` once C main is deleted. So:

- **Step 1** (built by the 0.14.0 seed): the compiler's side (items 1
  to 3 in selfhost/), the data symbols and the printf deletion (only
  XO_NOLIBC builds, which no seed makes, use them), and rt.sym_addr's
  test (std/internal's rt_op "sym_addr": golden rt_addrs and
  test/codegen/fnaddr). Nothing the seed builds loads cabi_c.xo or
  calls rt.sym_addr, and C main stays.
- **Step 2** (after the 0.15.0 seed, cut from step 1): the native
  prelude imports cabi_c.xo; its main calls entry.xo init (the main
  thread's heap record, fault.xo and ctx.xo with the personality
  routine's address through rt.fn_addr, the event loop's record, and
  stop.xo with the C's services record) and entry.xo main with
  rt.sym_addr("xo_entry"). xo_rt.c loses main and fault_init; the C
  tests' constructor (XO_RT_NO_MAIN) calls cabi.xo rt_init (entry.xo
  init); cabi.xo's heap_start, rt_start, poll_svc, stop_init, and
  entry_main go. The C tests rename the program's `@main` in their
  exports IR (internal/llvm cOnlyWith), as they bring their own.
  XO_RT_TEST_THREADS (no test sets it) goes with C main.

Step 2 was checked before step 1 landed: the step 1 xo (from the 0.14.0
seed) built the step 2 tree, which built itself, and hello, rt_addrs,
the C runtime tests, and -short passed on the result, except the split
builds, which found the quoted symbol name fixed in step 1.

## What stays C

xo_rt.c: Windows' on_overflow (a vectored handler that runs on what is
left of an overflowed stack, decision 0197 stage 5) and the services
record that hands it to stop.xo (xo_c_stop_svc, a std/native/sys_c.xo
call from entry.xo init), the Windows headers and the asserts of the
constants and layouts sock.xo, cabi_net.xo, and poll.xo hard code, and
the C tests' constructor. xo_nolibc.c: the C start while
XO_NOLIBC_C_START is kept.

## Consequences

- Xo can define a C entry point and name any C level symbol's address;
  the C runtime is no longer needed to start a program.
- A cabi_c.xo export must have the exact signature C expects of that
  name; the checker does not know C prototypes.
- rt.sym_addr of a name nothing defines is a link error, as in C.
- Windows: main's envp and the Windows arm of the services record are
  UNTESTED (cross linked only).

## Alternatives

- Backend-emitted `main` (a stub like `_start`): would need its own
  code in both emitters for every OS; an Xo function exported under its
  C name reuses the export path.
- A weak Xo main beside the C tests' strong one: weak definitions differ
  per object format (COFF especially); renaming in the tests' IR is
  local to the tests.
- Keeping the printf family through an Xo or backend va_start shell per
  ABI (offered by Xo lang 5): no caller, so no code.
- One step with a -D flag that hides C main from the new driver only:
  the seed would still lower rt.sym_addr in cabi_c.xo as an undefined
  call.
