# rt.heap_large_resize overwrites the first word of a block used from its malloc'd address

Status: Observed (2026-10-14, B29: the HTTP/2 client's read buffer in
hclient.xo; the same pattern was in selects.xo, tasks.xo and mutex.xo)

Symptom: after a buffer grows, its first 8 bytes are garbage. The
https golden http_tls failed with `big: Protocol(http2: frame too
large)`: the next frame's length was read from the clobbered bytes.

Cause: a block the C frees is used from its malloc'd address,
`rt.heap_large(n) - 8` (gotchas/heap-large-prefix-not-malloc-address.md),
so its first word is data. `rt.heap_large_resize(b, n)` reallocs and
then writes the large block prefix (-n) into that first word, so the
`rt.heap_large_resize(b, n) - 8` pattern loses word 0.

Fix: grow such blocks with `raw.c_resize(b, n)`, which saves word 0
before the resize and restores it after (0 allocates). Use
`rt.heap_large_resize` directly only on blocks used from the address
after the prefix (`heap_large(n)`, released with `heap_release(b - 8)`).
