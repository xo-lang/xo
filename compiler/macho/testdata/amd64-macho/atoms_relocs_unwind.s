	.section __TEXT,__text,regular,pure_instructions
	.globl _main
	.p2align 4
_main:
	.cfi_startproc
	.cfi_personality 155, _xo_personality
	.cfi_lsda 16, Lexcept0
	pushq %rbp
	movq %rsp, %rbp
	pushq %rbx
	.cfi_def_cfa %rbp, 16
	.cfi_offset %rbp, -16
	.cfi_offset %rbx, -24
	leaq Lstr(%rip), %rdi
	movl $0, %eax
	callq _puts
	callq _helper
	callq _main
	movq _ext@GOTPCREL(%rip), %rcx
	movq _xo_tlocal@TLVP(%rip), %rdi
	callq *(%rdi)
	movl $7, Ldata(%rip)
	movl $7, Lstr(%rip)
	movl $7, _counter+4(%rip)
	movq _counter(%rip), %rax
	cmpq $3, %rax
	je Lmain_end
	jmp Lmain_end
Lmain_end:
	popq %rbx
	popq %rbp
	retq
	.cfi_endproc
	.p2align 4
	.globl _helper
	.private_extern _helper
_helper:
	.cfi_startproc
	pushq %rbp
	.cfi_def_cfa_offset 16
	.cfi_offset %rbp, -16
	movq %rsp, %rbp
	.cfi_def_cfa_register %rbp
	pushq %rbx
	pushq %r12
	.cfi_offset %rbx, -24
	.cfi_offset %r12, -32
	testq %rdi, %rdi
	jne Lmain_end
	popq %r12
	popq %rbx
	popq %rbp
	jmp _helper
	.cfi_endproc
	.p2align 4
_leaf:
	.cfi_startproc
	retq
	.cfi_endproc
	.section __TEXT,__gcc_except_tab
Lexcept0:
	.byte 255
	.byte 155
	.byte 0
	.section __TEXT,__cstring,cstring_literals
Lstr:
	.asciz "hi"
	.section __DATA,__data
	.globl _counter
	.p2align 3
_counter:
	.quad 0
Ldata:
	.long 0
	.long 0
	.quad Lstr
	.quad _helper
	.quad Lmain_end
