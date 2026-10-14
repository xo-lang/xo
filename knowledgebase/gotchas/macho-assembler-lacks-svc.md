# selfhost/macho cannot assemble `svc #0`, so every linux/arm64 `--ld=macho` build fails

Status: Observed (2026-10-13), not fixed.

Symptom: with the xo1 built from 78a06532 (cache key 36a0f09c54a6cb61),
`xo1 build --backend=arm64 --target=linux/arm64 --ld=macho` fails on
every program, even internal/llvm/testdata/golden/arith, with
`xo: assembler: line 8034: unsupported instruction: svc #0`.
scripts/elf-goldens.py reports all 179 programs as build failures
(bench/results/xo-dev-harness-2026-10-13.md). The darwin builds and the
cc assembled linux builds are not affected.

Cause: decision 0206 makes the arm64 emitter write raw Linux system
calls (`svc #0` in selfhost/arm64/gen.xo's xo_sigreturn and xo_clone,
and the inline `rt.sys6` calls in calls.xo), and the self hosted
assembler in selfhost/macho has no encoding for `svc`. The same lag as
knowledgebase/gotchas/selfhost-assembler-lags-emitters.md: no test
assembles a linux/arm64 program with selfhost/macho on the Mac, and the
ELF goldens need Docker, which is not running on the Mac by default.

Fix (to do): add `svc #imm16` (0xD4000001 | imm16 << 5) to
selfhost/macho's instruction table, then rerun
`python3 scripts/elf-goldens.py <xo1>` (Docker up) or its `xo dev
goldens --target=linux/arm64 --ld=macho` successor (decision 0209). A
build only test (no run, no Docker) of one linux/arm64 `--ld=macho`
program in the Mac tier would catch the next new instruction.
