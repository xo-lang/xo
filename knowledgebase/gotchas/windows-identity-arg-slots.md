# TestWindowsAmd64Identity fails on the hybrid Map's instances (arg_slots off on Windows)

Status: Observed (2026-10-21, Xo Lang 2; CI run 39654884531 and on the Mac); the Map part fixed the same day

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

Fix (2026-10-21): arg_slots on Windows too. Nothing between a caller's
stack words and its Xo callee is a Win64 frame there (Xo's calls are
System V, decision 0254 6c; the thunks keep their shadow space in their
own frames; the export shims build their own System V area), and the
exclusion had been a precaution (Xo lang 4). On the Mac 31 of the 31
subset goldens differed before, 1 after; the Wine goldens arg_slots, the
map ones, and the Wine subset pass (32 of 32). The test's leaf fault stub
fold also drops the Linux stub's frame for the variables a fault report
shows (x86emit.xo stub_saves, 48d132c35), which a Windows leaf's stub
(a jump, no frame) does not make.

Still differing: cli_fault's main (Linux's frame 16 bytes larger, the
register allocation different from a call on). Seen on main before the
fix; the likely cause is the calls of exports, which Linux makes through
the export's alias and Windows to the function itself (normalized in the
text, not in the allocation). A known difference: do not bisect it.
