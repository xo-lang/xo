# A direct rt.* call that no backend lowers stopped at the link

Status: Observed (2026-10-24, DS-1); a compile-time check since 2026-10-25.

Symptom: a std change that called an rt.* intrinsic directly
(rt.rs_push_rune outside BytesBuilder.write_rune) built through code
generation, then failed at the link with `Symbol not found:
_xo_rs_push_rune` and nothing naming the call.

Cause: compiler/arm64 calls.xo rt_call lowers an rt.* name it has no
inline lowering for to a call of its symbol, `_xo_NAME`. Nothing
defines that symbol unless a function of the program does.

Fix: after emit, a darwin build checks the assembly (compiler/arm64
undef.xo `undefined_xo`). A `_xo_` symbol that is named but defined
nowhere and not weak fails the build with the symbol, the function
naming it and the rt.* call. Call the intrinsic through its std wrapper,
or add the lowering. The link's own symbols are exempt: ld.xo's time
zone table (`_xo_tz_*`) and `use c` objects (`_xo_c_*`). A new
link-time symbol needs its prefix added there.
