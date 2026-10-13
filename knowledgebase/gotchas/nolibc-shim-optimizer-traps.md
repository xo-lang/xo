# Defining C library functions in the runtime: the optimizer calls them back

Status: Observed (2026-10-13, decision 0206, std/llrt/xo_nolibc.c).

Symptoms, each in an XO_NOLIBC=1 build on linux/arm64:

1. A static hello world spun at 100 percent CPU in `calloc` (gdb: pc
   at calloc+12, called from ctx()).
2. The LLVM backend's link failed with `undefined reference to calloc`
   from `ld-temp.o`, though xo_nolibc.c defines calloc.
3. The LLVM link failed with `undefined reference to
   __aarch64_ldadd8_acq_rel` (and more of libgcc's outline atomics).
4. The shim's clock_gettime made the system call on arm64 though the
   vDSO was there (245 million kernel instructions for 500,000 calls).

Causes and fixes:

1. clang folds `malloc` followed by a zeroing `memset` into a call of
   `calloc`; inside calloc that is a tail call of itself, a loop. Fix:
   calloc is `NL_NOBUILTIN` (`__attribute__((no_builtin))`) and calls
   memset by name; memcpy, memset, memmove, memcmp, strlen and friends
   are NL_NOBUILTIN for the same reason (a loop recognized as memcpy).
2. With `-flto` the runtime is bitcode: LTO internalizes and drops
   C library definitions nothing references yet, then code generation
   introduces calls to them (calloc, memcpy). Fix: every C library
   function in the shim is `NL_USED` (`__attribute__((used))`);
   `#pragma clang attribute` cannot apply `used`. --gc-sections still
   drops what the program does not reach.
3. The LLVM IR carries clang's target attributes, which on Linux arm64
   include `+outline-atomics`; `-mno-outline-atomics` on the command
   line does not change bitcode attributes. Fix: with XO_NOLIBC=1 xo1
   asks clang for the attributes with `-mno-outline-atomics`.
4. arm64's vDSO has only DT_GNU_HASH, no DT_HASH, and the lookup took
   the symbol count from DT_HASH. Fix: without DT_HASH the symbol table
   is read up to the string table (it comes right before it).
