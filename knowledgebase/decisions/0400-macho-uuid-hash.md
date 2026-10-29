# 0400: the Mach-O UUID from the code signature's page hashes

Status: Accepted (Lead, 2026-10-28)

## Context

compiler/macho link.xo write took the executable's LC_UUID from a SHA-256 of the whole
image (the header as zeros, then the rest), a second full pass over the bytes beside the
code signature's SHA-256 page hashes. Skipping it saved 2.4 ms of a separately compiled
HTTP server's 41 ms store link (finish 21.5 to 19.1 ms, n=80 interleaved) and 0.6 ms of
fib's 16 ms. A UUID only needs to be unique per content and deterministic; the one reader
is the runtime's line reader (std/native/llrt/dwline.xo), which pairs the running image
with its file or its dSYM, both written by the same link. Nothing compares UUIDs across
xo versions (the backend cache keys on inputs; scripts/identity compares Mach-O by
disassembly).

## Decision

Every code signature page past the header (from the first page boundary at or after the
header and load commands) is independent of the UUID, which lives in the header. The link
hashes those pages first (sign.xo tail_page_hashes), takes the UUID as the SHA-256 of the
header as zeros, the image bytes before that boundary, and those page hashes in order,
and passes the hashes to the signature (code_signature_with), which hashes only the
pages that hold the header. One SHA-256 pass over the image instead of two; the UUID
still depends on every byte but the header's. Measured (quiet interleaved A/B, n=80, the
separately compiled store link): the HTTP server's link 0.898 (finish 25.5 to 22.2 ms),
fib's 0.964 (finish 5.5 to 4.8 ms). Proof: codesign --verify, a binary's and its dSYM's
UUID equal (xo's --dsym and dsymutil's), the fixed point (s2 = s3), identity 202 of 202
(Mach-O compared by disassembly), TestDebugLineDarwin, TestStdStoreUnwind.

## Consequences

Every darwin binary's UUID changes once. Its strength is SHA-256's as before. A binary and
its dSYM still share one UUID (the same link writes both).

## Alternatives

A cheaper non-cryptographic hash of the image (ld64 uses MD5): still a second pass, and a
new hash in std; reusing the signature's hashes removes the pass.

## Note: ELF

The ELF linker's GNU build id (elfsplit.xo build_id_note, --debug-info=split only) is a
SHA-256 of the loaded sections, the DWARF and the symbols, copied into one buffer first;
there is no signature to share. A split build could hash incrementally (crypto.Hasher)
without the copy; not done here.
