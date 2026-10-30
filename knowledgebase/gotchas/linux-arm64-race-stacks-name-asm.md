# Linux arm64 --race stacks name prog.s lines, not main.xo lines

Status: Observed 2026-10-29 (Linux arm64 offload run, TestRaceStacksArm64).

Symptom: on Linux the arm64 backend's `--race` build reports frames such as
`xo_f_internal_2ecrypto__op prog.s:3474`, with no `main.xo:LINE`, so
TestRaceStacksArm64 finds none of its `xo_f_touch main.xo:` frames.

Cause: darwin builds are linked by `xo build` with Xo's linker and carry the
`.loc` line table; the Linux test links the emitted assembly with clang and
ThreadSanitizer's runtime, which symbolizes by the assembly's own lines.

Fix: the test skips on Linux with this reason. To enable it, emit `.file` and
`.loc` directives for the Linux assembly path (compiler/arm64) and assert the
`main.xo` frames there.
