# 0402: Fs.read_mapped, a file mapped read-only as an immutable Bytes

Status: Proposed (stub, 2026-10-29; Lang 1)

Context, decision and alternatives follow. The motivating cost: a separately compiled darwin link reads the whole std store
(29 to 31 MB) each edit build, 5 ms of fib's 23 ms link (link.read_store), while it uses only the live atoms' bytes.
