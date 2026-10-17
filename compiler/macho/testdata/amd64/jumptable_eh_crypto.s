	.text
	.globl	dispatch
	.p2align	4
	.type	dispatch,@function
dispatch:
	.cfi_startproc
	.cfi_personality 155, DW.ref.xo_x_personality
	.cfi_lsda 27, .Lexc0
	pushq	%rbx
	.cfi_adjust_cfa_offset 8
	.cfi_offset %rbx, -16
	cmpq	$3, %rdi
	ja	.LBB_def
	leaq	.LJTI0(%rip), %rax
	movslq	(%rax,%rdi,4), %rcx
	addq	%rax, %rcx
	jmpq	*%rcx
.LBB_0:
	movsd	.LCPI0(%rip), %xmm0
	addsd	%xmm1, %xmm0
	cvttsd2si	%xmm0, %rax
	jmp	.LBB_out
.LBB_1:
	movq	tls_dyn@GOTTPOFF(%rip), %rax
	movq	%fs:(%rax), %rax
	jmp	.LBB_out
.LBB_2:
	movdqu	(%rsi), %xmm0
	aesenc	%xmm1, %xmm0
	pclmulqdq	$17, %xmm2, %xmm0
	sha256rnds2	%xmm0, %xmm3, %xmm4
	movdqu	%xmm0, (%rsi)
	adcxq	%rdx, %rax
	mulxq	%rcx, %r8, %r9
	jmp	.LBB_out
.LBB_3:
	callq	*%rdx
.LBB_def:
	xorl	%eax, %eax
.LBB_out:
	popq	%rbx
	.cfi_adjust_cfa_offset -8
	retq
.Lfunc_end:
	.size	dispatch, .Lfunc_end-dispatch
	.cfi_endproc
	.section	.rodata,"a",@progbits
	.p2align	2
.LJTI0:
	.long	.LBB_0-.LJTI0
	.long	.LBB_1-.LJTI0
	.long	.LBB_2-.LJTI0
	.long	.LBB_3-.LJTI0
	.p2align	3
.LCPI0:
	.quad	0x3ff0000000000000
	.section	.gcc_except_table,"a",@progbits
	.p2align	2
.Lexc0:
	.byte	255
	.byte	255
	.byte	1
	.uleb128 .Lcst_end-.Lcst_begin
.Lcst_begin:
.Lcst_end:
	.hidden	DW.ref.xo_x_personality
	.weak	DW.ref.xo_x_personality
	.section	.data.DW.ref.xo_x_personality,"awG",@progbits,DW.ref.xo_x_personality,comdat
	.p2align	3
	.type	DW.ref.xo_x_personality,@object
	.size	DW.ref.xo_x_personality, 8
DW.ref.xo_x_personality:
	.quad	xo_x_personality
	.section	".note.GNU-stack","",@progbits
