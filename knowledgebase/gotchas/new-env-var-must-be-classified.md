# A new XO_* environment variable fails -short until it is classified

Status: Observed (2026-10-21).

Symptom: a push's `-short` fails in internal/xotool with
`TestCodegenInputsClassified: environment variable XO_KEEP_ASM is not
classified`, though the branch's own gate passed (it built xo and ran
its proving tests, not internal/xotool).

Cause: internal/xotool codegen_test.go lists every environment variable
compiler/xo and the code generation read (codegenEnv) with its class:
keyed (in a cache key header), bypass (turns the cache off), content,
link, or none (no effect on generated code: paths, jobs, reporting,
cache and test switches). The test scans the sources and fails on a
variable the table lacks, so a variable that changes generated code
cannot slip past the caches' keys.

Fix: add the variable to codegenEnv in the same change, with its class
(and, if it can change generated code, in a key header or the bypass
first), and run `go test -run TestCodegenInputsClassified
./internal/xotool` before pushq. XO_KEEP_ASM, XO_CACHE_PRUNE_BUDGET,
XO_BORROW_CHECK and XO_LAZY_INLINE are none.
