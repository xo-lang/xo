# x86 template clobbers must not name xmm14 or xmm15

Status: Observed 2026-11-19 (task: random's x86-64 ChaCha8 kernel).

## Symptom

The amd64 golden sample failed native_json only: `show[Str]` of
`"é😀"` printed `ok: "w6nwn5iA"` (the base64 JSON of Bytes) instead of
the string. A binary of the same golden cross built on the Mac printed
the right bytes; only an xo built natively on linux/amd64 from the
change (offload-amd64.sh) compiled it wrong, every time. Random's own
output matched Go's word for word.

## Cause

The new rt.chacha8_rand4 kernel writes all sixteen xmm registers, and
`ssa.chacha8rand_x86_xmms()` listed 0 to 15, which the native emitter
pushed into Inst.clobbers as FP_BASE + 14 and FP_BASE + 15. On x86_64
those are the emitter's scratch (regalloc.xo, decision 0254): the
allocator never hands them out, and naming them in a template's
clobbers miscompiled code inside the xo that ran the kernel (xo's own
refill of its random state), so that xo compiled the golden wrong.

## Fix

Clobber lists for the native x86 emitter name only allocatable
registers (`fp_caller_t`, xmm0 to xmm13); a template may still write
xmm14 and xmm15 as scratch. The LLVM backend's inline asm clobbers are
separate and do name all sixteen. Test:
compiler/arm64/x86clobbers_test.xo. A display trap on the way: the
offload output re-encodes non-ASCII, so `"é😀"` showed as `"éð"`; read
bytes with `od -c` on the host.
