# Generated IR was compiled for LLVM's generic CPU

Status: Observed (2026-10-10, decision 0119)

Symptom: an LLVM backend binary on darwin/arm64 had exclusive load and
store loops (`ldaxr`/`stlxr`) for every shared reference count of
generated code (`otool -tv bin | grep -c ldaxr`: 20 in a small spawning
program), while the C runtime in the same binary used LSE (`ldaddal`,
`stadd`).

Cause: the IR names no CPU, and under `-flto` the linker's code
generation uses the target's generic CPU for every function without a
`target-cpu` attribute. clang gives its own C functions such attributes
(apple-m1 on macOS), so only the generated functions were generic; LTO
also does not inline a function with more target features into one with
fewer.

Fix: the build copies clang's default target CPU and features into the
IR's attribute groups (`build.ClangTargetAttrs`). After it, the program
has no exclusive loops. When changing how IR is compiled (flags, LTO,
another linker), disassemble a binary and compare the generated code's
atomics with the runtime's.
