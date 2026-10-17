	.text
	.file	"main.xo"
	.globl	xo_main
	.p2align	4
	.type	xo_main,@function
xo_main:
	.cfi_startproc
	pushq	%rbp
	.cfi_def_cfa_offset 16
	.cfi_offset %rbp, -16
	movq	%rsp, %rbp
	.cfi_def_cfa_register %rbp
	pushq	%rbx
	subq	$24, %rsp
	movq	%rdi, %rbx
	testq	%rbx, %rbx
	je	.LBB0_3
	cmpq	$10, %rbx
	jg	.LBB0_long
.LBB0_1:
	leaq	.Lstr(%rip), %rdi
	movq	counter(%rip), %rax
	addq	$1, %rax
	movq	%rax, counter(%rip)
	callq	puts@PLT
	callq	helper
	movq	ext_ptr@GOTPCREL(%rip), %rcx
	movq	(%rcx), %rcx
	movq	%fs:0, %rax
	leaq	tls_var@TPOFF(%rax), %rax
	movq	(%rax), %rdx
1:
	lock cmpxchgq %rdx, (%rcx)
	jne	1b
	jmp	2f
	nop
2:
	decq	%rbx
	jne	.LBB0_1
.LBB0_3:
	addq	$24, %rsp
	popq	%rbx
	popq	%rbp
	.cfi_def_cfa %rsp, 8
	retq
.LBB0_long:
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	addq $1, %rax
	jmp	.LBB0_1
.Lfunc_end0:
	.size	xo_main, .Lfunc_end0-xo_main
	.cfi_endproc

	.p2align	4
	.type	helper,@function
helper:
	.cfi_startproc
	xorl	%eax, %eax
	retq
.Lfunc_end1:
	.size	helper, .Lfunc_end1-helper
	.cfi_endproc

	.section	.rodata.str1.1,"aMS",@progbits,1
.Lstr:
	.asciz	"hello"

	.data
	.globl	counter
	.p2align	3
counter:
	.quad	0
	.globl	table
table:
	.quad	helper
	.quad	xo_main+8

	.section	.tdata,"awT",@progbits
	.p2align	3
tls_var:
	.quad	7
	.section	".note.GNU-stack","",@progbits
