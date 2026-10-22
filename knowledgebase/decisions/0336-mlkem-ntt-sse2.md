# 0336: ML-KEM's NTT loops in SSE2 on x86-64

Status: Proposed (Xo Lang 4, 2026-10-22)

Stub: cpu.mlkem_ntt, cpu.mlkem_inverse_ntt and cpu.mlkem_mul_acc as
decision 0231/0329 intrinsics, lowered to SSE2 (two coefficients an xmm
register) by both x86-64 emitters. Filled in with the change.
