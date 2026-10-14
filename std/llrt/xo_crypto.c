// xo_crypto.c: no C of std/crypto is left on the native backends: the
// hashes, base64, and verify_password are std/native/llrt/digest.xo
// (decisions 0193, 0195, 0198; SHA-512 the zero-C audit's B08),
// crypto.equal is std/native/llrt/ct.xo and Ed25519
// std/native/llrt/ed25519.xo (decision 0222, the audit's B09), the Rand
// capability std/native/llrt/rand.xo (B08). Included by xo_std.c.

// The prebuilt seed (0.10.0-4ab09c25) does not lower rt.ct_barrier
// (decision 0222) inline and calls this instead
// (gotchas/runtime-c-the-seed-still-calls.md): an empty asm, as the LLVM
// emitters' barrier. Delete it, and this file, once the seed has
// decision 0222.
int64_t xo_ct_barrier(int64_t x) {
	__asm__("" : "+r"(x));
	return x;
}
