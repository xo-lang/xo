# A frame in the red zone met code that writes below rsp

Status: Observed (2026-10-21)

**Symptom.** With leaf red-zone frames on System V amd64 (Frame.rz:
a function that makes no call keeps its frame in the 128 bytes below rsp,
the frame pointer kept), the server build of bench/perf/http failed
`internal: amd64: xo.hash[Str] calls or pushes with its frame in the red
zone`, and xo's own build stayed fine.

**Cause.** Three things write below rsp without a `call` in the
function's own instruction templates, which the first pre-scan
(x86_rz_ok) read:
- a memory to memory parallel move, `pushq A; popq B` (x86emit.xo);
- a fault stub (`callq` of the raise, after `subq` and stores of the
  caller saved registers), emitted after the body;
- a template that moves rsp itself (SHA-NI's xmm saves: `subq $176,
  %rsp`, stores at (%rsp)), not seen in a build: the Docker goldens run
  without SHA-NI.

**Fix.** The memory to memory move steps over the red zone (`leaq
-128(%rsp), %rsp`, the push and pop 128 bytes further, `leaq 128(%rsp),
%rsp`, as the preemption stub does). x86_rz_ok refuses a function with a
fault check or a template naming %rsp. The post-check x86_rz_unsafe,
which read the emitted text and caught the first two, also fails on a
`sub`, `lea` or `and` into rsp outside the step over. Keep the post-check
when adding a template that touches the stack.
