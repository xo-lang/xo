# 0428: An edit build parses only the std declarations it needs

Status: Proposed (xo-lang-4, 2026-10-30; stub, design in progress)

## Context

An HTTP server edit build (4 files, 382 lines) re-lexes and re-parses 13 std
files (447 KB) every time: about 14 ms of CPU in parse_prefetch, profiled over
20 builds on the arm64 offload host. Ten of them get a full lazy parse because a
module declaring a std generic gets its whole tree (check/dump.xo
ensure_tree). Generic declarations are 11% of those bytes.

## Decision

To be written.
