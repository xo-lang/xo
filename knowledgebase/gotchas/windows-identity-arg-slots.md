# TestWindowsAmd64Identity fails on the hybrid Map's instances (arg_slots off on Windows)

Status: Observed (2026-10-21, Xo Lang 2; CI run 39654884531 and on the Mac)

Symptom: internal/arm64 TestWindowsAmd64Identity (not in -short; the full
tier) fails in map_key_fault and other goldens on xo.map.find, put,
rebuild, scan and xo.maph.putrest instances: `line 8: linux "subq $72,
%rsp", windows "subq $104, %rsp"` and similar, Windows' frame 16 to 96
bytes larger.

Cause: the pass arg_slots (regalloc.xo, x86 since 0aff5ec24) reads a stack
passed argument in its incoming slot; it is off on Windows x86 (the shadow
space above the stack arguments), where the argument is copied into the
frame instead (`movdqu 80(%rsp), %xmm15; movdqu %xmm15, 8(%rsp)`). The
hybrid Map's instances (decision 0291, switched on 2026-10-21) are the
first functions in the identity subset with more than six argument words.
Not a miscompile and not decision 0318: the xo before 0318 stage 3 fails
the same way. Turning arg_slots off on the Linux side of the test makes
more goldens differ (the pass also sets callers' word hints).

Fix: arg_slots on Windows with the incoming slots offset by the 32 byte
shadow space (queued with Xo Lang 2 after the allocator latency work);
until then the failure is known: do not bisect it.
