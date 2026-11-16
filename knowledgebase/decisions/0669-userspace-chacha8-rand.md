# 0669: the native Rand draws from a per-thread ChaCha8 keystream seeded by the OS

Status: Proposed (Xo Lang 1, 2026-11-15). Stub: filled in with the prototype and its host 1 --ab.

## Context

std/native/llrt/rand.xo makes one operating system entropy call per draw (bench/perf/random: 60 to 354 times Go's
math/rand/v2).
