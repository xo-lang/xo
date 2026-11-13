# 0627: Emit allocates from fresh memory: a per-thread free list stash

Status: Proposed (Xo Lang 1, 2026-11-13; stub, the probe and its measurements follow)

## Context

Decision 0448 stage 2 (the argument pool) costs the gcp aiplatform/apiv1 test build about 12% CPU, in emit only; keeping the old per-value lists alive gives it back (CPU 0.913) at the memory the pool saved (RSS 1.214), so emit pays for allocating its MIR from free lists the SSA passes scattered (ADR 0448, "The CPU follow-up"). This decision proposes a free list stash in the native allocator around each function's emit; filled in after the probe's --ab.
