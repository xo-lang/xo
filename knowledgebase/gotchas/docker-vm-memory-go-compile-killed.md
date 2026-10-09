# Parallel Go builds of the generated runtime get killed in the Docker VM

Status: Observed (2026-10-09)

Symptom: in the Linux container, `go test ./internal/arm64
./internal/llvm` failed four TestGolden cases with `build (go): go build
for linux/arm64 failed (code generator bug): xoprog/rt:
.../compile: signal: killed`.

Cause: the Docker Desktop VM had 8 GiB of memory and 12 CPUs. Each golden
builds a program with the Go backend, which compiles the large `rt`
package; with two test packages at once and 12 parallel tests each, the
compilers ran the VM out of memory and the kernel's OOM killer stopped
them. Not a code generator bug, though the message says so.

Fix: `scripts/linux-test.sh` passes `-p 2` (whole module) or `-p 1`
(a single heavy package). Pass the same when running the suite by hand in
a container, or give Docker Desktop more memory.

Observed again (2026-10-09, afternoon): with `-p 2`, TestHybridRandom
(added that morning) still had its compilers killed, even run alone,
because its subtests run one `go vet` of a generated program per CPU
(`-parallel` defaults to GOMAXPROCS, 12). It passes with `-parallel 2`.
The script's defaults now also pass `-parallel 4`. On EC2 machines with
30 GiB (infra/ci-aws) the default parallelism passes.
