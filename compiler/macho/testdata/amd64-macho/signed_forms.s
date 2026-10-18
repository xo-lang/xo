	.section __TEXT,__text,regular,pure_instructions
	.globl _f
_f:
	leaq _counter(%rip), %rax
	cmpb $1, _counter(%rip)
	movl $7, _counter(%rip)
	leaq _counter+4(%rip), %rax
	cmpb $1, _counter+4(%rip)
	movl $7, _counter+4(%rip)
	leaq _counter+8(%rip), %rax
	cmpb $1, _counter+8(%rip)
	movl $7, _counter+8(%rip)
	leaq Ldata(%rip), %rax
	cmpb $1, Ldata(%rip)
	movl $7, Ldata(%rip)
	leaq Ldata+4(%rip), %rax
	cmpb $1, Ldata+4(%rip)
	movl $7, Ldata+4(%rip)
	leaq Ldata+8(%rip), %rax
	cmpb $1, Ldata+8(%rip)
	movl $7, Ldata+8(%rip)
	leaq Lstr(%rip), %rax
	cmpb $1, Lstr(%rip)
	movl $7, Lstr(%rip)
	leaq Lstr+4(%rip), %rax
	cmpb $1, Lstr+4(%rip)
	movl $7, Lstr+4(%rip)
	leaq Lstr+8(%rip), %rax
	cmpb $1, Lstr+8(%rip)
	movl $7, Lstr+8(%rip)
	retq
	.section __TEXT,__cstring,cstring_literals
Lstr:
	.asciz "hello world!"
	.section __DATA,__data
	.globl _counter
_counter:
	.quad 0
Ldata:
	.quad 0
	.quad 0
	.quad Lstr
	.quad Ldata
