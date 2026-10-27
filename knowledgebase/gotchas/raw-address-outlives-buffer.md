# raw.bytes_addr(b) and raw.data(b) do not keep b alive: an address bound to a variable dangles

Status: Observed (2026-10-27: an intermittent SIGSEGV in cold xo builds, `lz4.compress` at its table's memset, internal/arm64
TestDebugInfoSameCode/http_tls in the hourly run, 1 of 32 cold builds under load). Fixed in std/native/llrt lz4.xo and chacha.xo.

Symptom: SIGSEGV (code 2, a protection fault) at `stnp q0, q0, [x0]`, the first store of libsystem's memset, on a 256 KiB
buffer `raw.bytes_buf` had just returned; the fault address was the block's data (`...020`). Nothing in the heap changed a
mapping's protection; the mapping was gone.

Cause: `let tab = raw.bytes_buf(n)` then `let t = raw.bytes_addr(tab)` and later uses of the Int `t` only. Reference counting
releases a value after its last use, so `tab` was released at the line that took its address. A block of 256 KiB or more is a
large block: its release goes to the large block cache (heapos.xo, 16 entries, 64 MiB), or, when the cache is full or its
entries are not within a quarter to the whole of the request, to `munmap`. Usually it was cached, still mapped, and the
program worked; with a full cache the memory was unmapped under the memset. The debug event ring and a SIGSTOPped
`vmmap` of the held process (XO_CRASH_SLEEP) showed the order: map, block returned, unmap, fault.

Fix: a use of the buffer after its last raw access, `keep_bytes(tab)` (darwin.xo keep, aesgcm.xo keep_bytes, flate.xo
keep_bytes are the same idiom). `rawAddrIssues` in internal/repotest (TestRawAddrKeepsBuffer) fails the build of the
repository when a function of std/native binds the address of a local buffer to a variable and never mentions the
buffer again (an address used inside its own statement, a literal and a parameter are fine). The golden
internal/llvm/testdata/golden/lz4_cache_full fills the cache first (20 blocks of 3 MiB), so the table gets a new mapping
and its release is an unmap: it faulted 3 of 3 before the fix.

How to avoid: in the runtime's Xo, never hold an Int address of a buffer past the buffer's last mention; end the
raw access with `keep_bytes(b)` (Bytes) or a use of the Str. A test that depends on the freed memory being gone needs the
cache full, not just a release.
