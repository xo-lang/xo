# A raw file's word is a bit pattern: read and write it wrapping

Status: Observed (2026-10-26, `xo prof` on a 2,127 sample profile)

Symptom: `xo prof FILE` stops on a profile that converts fine otherwise:

```
xo prof: integer overflow: 52554979092725760 * 256 does not fit Int
```

(or, on an older xo, it spins forever in `varint` with the CPU at 100%).

Cause: a raw profile's words are u64s (pcs, counts), and a word can be at or
above 2^63. Two places assumed they could not be:

- `u64` in compiler/xo/trace.xo built the word with checked arithmetic
  (`v * 256 + byte`), so the last multiply of a word with its top bit set
  faulted. The 2,127 sample profile held one such pc, 0xbab6799c00000000, in
  sample 540 (byte 28336), likely a stack walk artifact; the reader must
  still take it.
- `varint` in compiler/xo/prof.xo encoded a value with `v = v >> 7` and
  stopped at `v == 0`. For a negative Int (the wrapped word) the arithmetic
  shift keeps -1, the loop never ends, and the pprof builder grows without
  bound.

Fix: the reader is wrapping (`v.wrapping_mul(256).wrapping_add(...)`, so a
word at or above 2^63 reads as a negative Int), and the writer encodes a
negative value as its two's complement uint64, the ten byte protobuf form:
nine groups of seven bits then the top bit.

Trap for the next raw format: any reader of fixed width fields and any
varint writer must be written for the full bit pattern, not for "small"
values. Tests: compiler/xo/prof_test.xo, one for the read and one for the
ten byte write.
