# riscv64: a test binary needs __tls_get_addr

Status: Observed (2026-11-08, decision 0571 stage 3)

## Symptom

Every `xo test --backend=llvm --targets=linux/riscv64` build failed to
link, while program builds of the same code linked:

    ld.lld: error: undefined symbol: __tls_get_addr
    >>>               /tmp/prog-82fb5e.o:(xo_tlocal_at)

## Cause

`@xo_tlocal` (compiler/llvm sched.xo) was a plain `hidden thread_local`
global, whose access model clang picks from the relocation model. A
program build goes through LTO, and its link time code generation knows
the output is a static executable, so it uses local exec. A test
binary is built at -O0 without LTO, compiled as PIE (the toolchain's
default), so the access is general (or local) dynamic: a call to
`__tls_get_addr`. On arm64 and x86_64 lld relaxes that call to local
exec in an executable; on RISC-V it does not, and a static link without
a C library has no `__tls_get_addr`.

## Fix

On riscv64 the global is `thread_local(initialexec)`: a GOT slot that a
static link fills with the thread pointer offset at link time, valid in
an executable and in a shared object loaded at startup. A new
thread_local global on riscv64 needs the same model.
