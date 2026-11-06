# 0529: Cache pack read once with lazy decode for warm builds

Status: Proposed (xo-lang-4, 2026-11-06; owner-approved work item, item C option b: "making std's cache reads cheap")

## Context

A warm `xo build` of hello on linux/arm64 takes about 0.22 s, of which
about 0.19 s is reading, decoding and replaying the back-end and front
end cache entries of std and the program (about 24 MB read per build;
bench/results item C, to be committed). The test suites pay it once per
golden (arm64 648 builds, llvm 596, std 353). The target is set by the
step 1 breakdown (decode and replay of std's records and key
computation, not byte reads alone).

## Decision

To be filled after the step 1 breakdown (phase times, XO_CACHE_STATS and
CPU profiles of warm hello and golden builds on host 1).

## Consequences

To be filled.

## Alternatives

To be filled.
