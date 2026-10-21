# `xo dev goldens` passed a program the Go tests failed on the inline verifier

Status: Observed (2026-10-21, golden http_server, commit 21f0ff815)

Symptom: a gate that ran `xo dev goldens --backend=arm64` passed, but the
Go tests (internal/arm64 TestGoldenRun, TestGoldenRunPoison,
TestGoldenRunHeapGuard, and Xo lang 4's amd64 run) failed http_server:
"the inlined arm64 module does not verify (... v1643 uses v1528, which is
not alive); failing the build (XO_INLINE_STRICT=1)". Xo Lang 3 bisected it
to the commit; their gates were blocked meanwhile.

Cause: xo builds an arm64 module whose inlining does not verify without
inlining and only warns, unless XO_INLINE_STRICT=1 (compiler/xo main.xo
inline_or_say; gotchas/arm64-inline-verify-fallback.md). The Go tests set
it (internal/xotool's init); `xo dev goldens` did not, so the same
program passed one and failed the other.

Fix: `xo dev goldens` (and `xo dev codegen`'s `xo asm`) now pass
XO_INLINE_STRICT=1 to every xo they start unless the caller's environment
sets it (XO_INLINE_STRICT=0 keeps the warning), and the pass cache keys
the setting. cmd/xo TestDevGoldensInlineStrict checks what a fake xo is
given.
