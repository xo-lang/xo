# Loopback HTTP on the WSL2 amd64 offload host runs 20 times slow, Go included

Status: Observed (2026-11-02, Xo Lang 2)

## Symptom

bench/perf/jitter's /alloc on the linux/amd64 offload host (WSL2 Ubuntu on a
Ryzen 9 9955HX; server on CPUs 0-5, the loadgen on 6-11, OFF_EXCLUSIVE=1)
served 8,000 to 9,400 requests a second at 600 to 660 us of server CPU a
request, about 18 times the cost on the linux/arm64 hosts (33 to 36 us), with
5% run to run noise. An XO_STACK_SHIFT sweep there showed nothing but noise.

## Cause

The host, not Xo: Go's own jitter server (/alloc-hand, the same pinning and
loadgen, two rounds) did worse, 3,600 requests a second at 870 to 920 us a
request, against Xo linger's 8,100 to 8,600 at 633 to 657 us. Loopback TCP in
WSL2's VM costs far more than on bare Linux; where exactly (the Hyper-V
network path, the WSL kernel's loopback, Windows' power or scheduler policy)
was not measured.

## What it puts in doubt

HTTP throughput rows measured on that host:
bench/results/a1-poll-batch-inject-2026-11-02.md's linux/amd64 rows (P req/s
-3.4%, R req/s +10.9% and CPU -9.9%, R +8.2% and +9.0%) compare two Xo builds
under the same bottleneck; their ratios may hold or may be the host's noise,
and the absolute numbers say nothing about Xo on amd64. The release-vs-go
scripts have no amd64 HTTP section (their http section runs on the first
linux/arm64 host), so the release comparisons are not affected.

## Fix

Measure HTTP throughput on the linux/arm64 hosts or bare Linux; use the amd64
host for correctness and CPU-bound timings (infrastructure/offload.md).
