# 0329: ChaCha20's key stream four blocks at a time in vector registers

Status: Proposed

Stub: cpu.chacha_blocks4 (std/native/llrt cpu.xo, a portable body like
cpu.aes_enc10's), lowered as an rt intrinsic: a NEON core on arm64
(four blocks as 4 x 32 bit lanes), LLVM vector IR, amd64 SSE or the
portable body, wasm the portable body; std/native/llrt chacha.xo's
xor_stream calls it for each 256 bytes. TLS ChaCha20-Poly1305 bulk runs
at 0.44 of Go (bench/results/tls-h2-vs-go-2026-10-21.md).
