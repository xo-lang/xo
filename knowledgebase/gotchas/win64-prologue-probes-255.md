# A Win64 prologue of unrolled stack probes passed 255 bytes

Status: Observed (2026-10-22)

Symptom: a windows/amd64 build of a function with a frame of about
90 KiB (a generated function of 24,000 lines) failed in Xo's assembler
with "a prologue of 365 bytes: UNWIND_INFO holds at most 255".

Cause: compiler/arm64 x86emit.xo x86_prologue touched each page of a
frame past 4 KiB with its own `subq $4096, %rsp` and `movq $0, (%rsp)`
(11 bytes a page), all inside the prologue. A Win64 UNWIND_INFO records
the prologue's size in one byte, so past about 68 KiB of frame the
prologue could not be described.

Fix: a frame past PROBE_UNROLL pages (8, as arm64's probe_sp) probes in
a loop: r10 the frame's end, r11 walking down a page at a time with
`testq %r11, (%r11)`, then one `subq` (about 30 bytes whatever the
size). r10 and r11 hold no argument in either calling convention.
internal/arm64 TestX86FrameProbeLoop checks huge_frame's prologue.
