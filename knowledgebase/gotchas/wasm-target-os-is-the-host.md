# A wasm build's `target.os` is the host's, and `match target` arms are all lowered

Status: Observed (2026-10-13, decision 0206).

Symptom: after std/native/llrt/sysio.xo chose Linux system calls with
`match target.os { TargetOs.Linux => ... }`, TestSelfhostWasm on a Mac
reported archive_formats and native_bytes as unsupported (`the wasm
backend does not support the call rt.tw_fake`), a call only the Linux
arm makes, and 127 instead of 128 goldens passed.

Cause: two things together. (1) The SSA builders build `target` as a
constant struct and lower every arm of a `match` on it; no SSA pass
folds the match (spec core 12.2 says only the chosen arm is code
generated: the native backends' optimizers drop the dead arm, the wasm
emitter and compiler/arm64 do not). So every arm must lower on every
backend. (2) xo1 `--backend=wasm` and the compiler/wasm harness build
with the compiler's own `target` (the host's OS and architecture), so on
a Linux host a WASI module would even take the Linux arm at run time.

Fix: sysio.xo tests `target.arch` too (`raw()`: Linux and not Wasm32);
xo1 builds wasm modules, and the harness its goldens, with the wasm
architecture (`build_program_target(..., "", "wasm")`), keeping the
host's OS so want.txt (the host's run) still matches; the wasm emitter
turns the system call intrinsics into a fault instead of rejecting the
module. TestSelfhostWasm: 129 of 149 pass. Not done: wasm builds'
`target.os` should be Wasi (core 12.2); changing it changes what
goldens such as method_values print under the harness. When writing
target dependent runtime code, make every arm lower on every backend,
or guard on both `target.os` and `target.arch`.
