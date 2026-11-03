# wasm's heap_large_resize took the data address, callers pass the prefix

Status: Observed (2026-11-03, decision 0447 R2a sizing; golden
stdin_long_line)

Symptom: on wasip1/wasm and wasip2/wasm, standard input past about 4 KiB
of buffered input came back corrupted: a 20,000-byte first line read
right, the 100-byte lines after it did not ("each as written false").
Any wasm runtime buffer grown with `rt.heap_large_resize` (stdin.xo put,
twrun, det) was affected. Not in a release: standard input on wasm came
in b16428830, after v0.36.0 (a read faulted before), and no other caller
of the resize was found that a v0.36.0 wasm program reaches.

Cause: `rt.heap_large_resize(b, n)` and `rt.heap_release(b)` take the
block's prefix, 8 bytes before its data, as on the native heap
(gotchas/heap-large-prefix-not-malloc-address.md), and every caller
passes `addr - 8`. compiler/wasm rt.xo's `$heap_large_resize` treated
its argument as the data address: it read the old size 16 bytes before
it (8 bytes off) and copied from 8 bytes before the data. A wrong size
that happened to be large hid the first problem; the shifted copy
corrupted every grown buffer. `$heap_release` was a no-op, so blocks
were never reused either (a 64 KiB block leaked per stdin read).

Fix: the WAT functions take the prefix (data = b + 8), and released
blocks go on power-of-two free lists that heap_large takes from first
(compiler/wasm rt.xo, 2026-11-03). A new wasm lowering of a native
runtime intrinsic must follow the native argument convention, not the
WAT helper's: read the intrinsic's doc in std/native/rt.xo and two
callers first.
