# ELF runtime functions without the xo_ prefix are function starts too

Status: Observed (2026-10-25).

Symptom: the first cut of the hollow pass for linux/arm64 ELF
(decision 0371's ELF addendum) built every golden. In Docker, 15 of
them (the http ones, metrics_text, tls_client, trace_ring...) died with
"Trace/breakpoint trap" (exit 133): a hollowed body's `brk #0x484f`.

Cause: the nolibc runtime defines libc's functions under their own
names, with no `xo_` prefix: `memcpy:`, `free:`, `abort:`, `_start:`,
and others. The scan took only `xo_...:` and `main:` as global labels. A
`memcpy:` label was therefore inside the dead function above it, its
body was emptied with that function, and `bl memcpy` jumped into a brk.
Mach-O had no such hole, because every global there starts with `_`.

Fix: a pre-pass collects the ELF global labels without the prefix. They
start chunks, the token scan matches their calls, and `_start` (the
entry point) is a root. compiler/arm64 hollow_test.xo's ELF case has a
runtime-defined memcpy. The brk turned the missed reference into a
loud fault rather than a silent run into the next function.
