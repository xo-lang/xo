# 0708: Proven evidence may come from linux/amd64

Status: Accepted (owner, 2026-11-17). Amends 0616 (testing evidence) and the
performance rows' host rule; written by the lead.

## Context

Decision 0616 and stdproven take every Proven run (fuzz, stress, race,
timing) on the linux/arm64 offload hosts. On 2026-11-17 those four hosts
were the bottleneck for std Proven: stdproven batches queued behind gates
and tiers, host 1 (timing) was held for over an hour, and disk ran low on
two hosts. The linux/amd64 host (WSL2 Ubuntu 24.04 on the owner's PC, 28
CPUs, 62 GB, lane `amd64`) sat idle. The owner (2026-11-17): use the x86-64
machine to expedite performance and testing to Proven; it does not matter
whether each module's evidence is from arm64 or amd64.

## Decision

- A module's fuzz, stress, race and performance evidence may each be taken
  on linux/amd64 instead of linux/arm64. One platform per kind per module is
  enough; the evidence file's header names the host and platform, as now.
- Backends on amd64: the native x86-64 backend and LLVM stand in for arm64
  and LLVM (fuzz "2 backends", stress on both backends).
- Performance on amd64 compares against the Go twin built and run on the same
  host in the same job, with 0616's and the perf rows' thresholds unchanged.
  A module with a perf row on both platforms is judged on its newer row.
- The amd64 lane is pass/fail and timing in one: a timing run there takes
  the lane exclusive, as on host 1.

## Consequences

- One more machine for evidence: fuzz and stress shards, race runs and timing
  rows can be routed to `amd64` when the arm64 lanes queue.
- stdproven (scripts/stdproven/run.sh, stdproven-offload) and stdcov accept
  amd64 evidence; the Go module cache on the amd64 host needs the modules the
  references and twins require (offload.md "Go modules for the twins").
- Proven still says nothing about the other platform for a kind taken on one;
  a bug only one backend shows is found by the tier and the goldens, not by
  the evidence.

## Alternatives

- arm64 only (0616 as written): leaves the idle host idle while Proven waits.
- Require both platforms per kind: doubles the evidence work.
