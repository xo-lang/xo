# 0255: Preserve-most drop helpers on arm64

Status: Proposed (stub, 2026-10-17, xo-4; claimed before the work lands)

Context: releases of Str, List, and Handle values are calls of per type
drop helpers on arm64 (69% of the calls in bench/perf/tls's client),
and every call clobbers the caller saved registers, so the values live
across them spill (13% of the client's instructions are spill traffic).

Decision: to be filled in with the measured design.
