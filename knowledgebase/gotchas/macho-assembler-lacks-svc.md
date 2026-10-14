# selfhost/macho could not assemble `svc #0`, so every linux/arm64 `--ld=macho` build failed

Status: Observed (2026-10-13), fixed (2026-10-13).

Symptom: with the xo1 built from 78a06532 (cache key 36a0f09c54a6cb61),
`xo1 build --backend=arm64 --target=linux/arm64 --ld=macho` failed on
every program, even internal/llvm/testdata/golden/arith, with
`xo: assembler: line 8034: unsupported instruction: svc #0`.
scripts/elf-goldens.py reported all 179 programs as build failures
(bench/results/xo-dev-harness-2026-10-13.md). The darwin builds and the
cc assembled linux builds were not affected.

Cause: decision 0206 makes the arm64 emitter write raw Linux system
calls (`svc #0` in selfhost/arm64/gen.xo's xo_sigreturn and xo_clone,
and the inline `rt.sys6` calls in calls.xo), and the self hosted
assembler in selfhost/macho had no encoding for `svc`. The same lag as
knowledgebase/gotchas/selfhost-assembler-lags-emitters.md: no test
assembled a linux/arm64 program with selfhost/macho on the Mac, and the
ELF goldens need Docker, which is not running on the Mac by default.

Fix: selfhost/macho/enc.xo encodes `svc #imm16` (0xD4000001 | imm16 <<
5), with `svc #0` and `svc #0x80` in macho_test.xo's encoding table
(`xo test selfhost/macho`: 14 passed). An ELF `--ld=macho` build on the
Mac then needs a linux/arm64 runtime pack (XO_RTPACK, or lib/xo/rtpack);
on a linux/arm64 host the ELF goldens run directly (`xo dev goldens
--flags='--target=linux/arm64 --ld=macho'`, decision 0209).
