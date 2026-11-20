# perf annotate prints nothing on linux/arm64 host 2; use perf script hot IPs

Status: Observed (2026-11-20, task #429, xo-4c)

Symptom: on host 2, `perf record` and `perf report --stdio` work, but `perf annotate --stdio -s
SYMBOL` prints no lines for a Go symbol (objdump is installed). `perf stat` gives task-clock,
context switches and migrations; cycles and L1-dcache-load-misses come back as not supported.

Cause: unverified. Host 2 exposes no hardware counters to perf (records fall back to a
software clock), and annotate finds no matching source for Go's symbol names.

Fix: count samples per instruction address and map them with Go's disassembler:
`perf record -q -o p.data ./PROG ARGS; perf script -i p.data -F ip | sort | uniq -c | sort -rn | head -25`,
then `go tool objdump -s 'PKG\.FUNC$' PROG` on the Mac and look up the addresses (the binary is
not PIE, so they match). Sampling skid puts a stall's samples on the instruction after the
slow one or at the next call's entry.
