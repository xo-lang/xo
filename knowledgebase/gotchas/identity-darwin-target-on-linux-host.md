# scripts/identity on a Linux offload host fails on its darwin target

Status: Observed (Xo Lang 2, 2026-11-04).

## Symptom

`go run ./scripts/identity BASE NEW` on a linux/arm64 offload host stops
with `identity: objdump .../a/prog: exit status 1` after building the
first program; wrapped in `| tail`, the failure looked like success
(the gate's EXTRA step printed "extra ok").

## Cause

Its default targets are darwin/arm64 and linux/arm64, and it compares a
Mach-O pair by disassembling both with the host's `objdump -d`, which on
Linux (GNU objdump) does not read Mach-O.

## Fix

On a Linux host pass `--targets=linux/arm64`; compare darwin/arm64 on the
Mac. Run it with `set -o pipefail` when piping its output.
