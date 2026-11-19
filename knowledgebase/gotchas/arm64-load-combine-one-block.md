# arm64 load combining sees one block: inlined halves are not combined

Status: Observed (2026-11-19, task #417)

Symptom: `binary.get_u64_le` written as `get_u32_le(b, at) |
get_u32_le(b, at + 4).wrapping_shl(32)` compiled to two `ldr w` on arm64,
not one `ldr x`, though each half combined.

Cause: compiler/arm64/ldcomb.xo matches an Or tree whose values are all in
the root's block and read once. Each inlined call returns through a block
parameter, so the outer Or reads two parameters, not byte loads. A loop
that builds the word (`v = v.wrapping_shl(8) | Int(b[i + k])`) is not a
tree either.

Fix: write the load as one flat Or of its bytes, each shifted to its
place, the top byte with `wrapping_shl(56)` (a checked `<< 56` of a byte
can overflow, and the matcher takes a checked shift only when it cannot):
std/binary get_u64_le (2026-11-19; std/brotli ld64 reads its word with
internal.le_word since c1f178755).
