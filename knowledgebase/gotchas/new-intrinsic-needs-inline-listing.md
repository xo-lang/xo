# A new lowered rt intrinsic must be listed for the inliner

Status: Observed (2026-10-30)

## Symptom

pushq stops with "fast repo checks failed" and
`TestInlineIntrinsicsListed` (internal/repotest/inlineleaf_test.go)
prints "lowered by calls.xo but in neither inline_intrinsic nor
inlineNotLeaf (add each where it belongs): rt.cpuid7_ecx rt.crc32_fold
rt.crc32_fold_ymm". The change's own gate passed: it ran goldens, the
fixed point and codegen patterns, but not internal/repotest.

## Cause

compiler/arm64 inline.xo decides which calls a leaf may contain: an
`rt.*` call the native emitter lowers to instructions either keeps the
caller a leaf (listed in inline.xo's inline_intrinsic) or not (listed in
the test's inlineNotLeaf with a reason: "cpuid", "block kernel", ...).
The test reads every `"rt.NAME" =>` arm of compiler/arm64 calls.xo and
fails on one in neither list.

## Fix

In the change that adds the `"rt.NAME"` arm to calls.xo, add NAME to
inline_intrinsic (a plain instruction with no clobbers beyond its
result, such as rt.crc32cx) or to inlineNotLeaf (a template with a
clobber list or a system call, such as rt.crc32_fold's block kernel or
rt.cpuid7_ecx). Run the fast checks in the gate:
`go test ./internal/repotest -run 'TestInlineIntrinsicsListed|TestStaticWords|TestCodegenInputsClassified'`.
