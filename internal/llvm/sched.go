package llvm

import (
	"runtime"
	"strings"
)

// The task switch and thread state of decision 0212 (selfhost/llvm
// sched.xo is the self hosted copy, which this mirrors): rt.stack_switch
// is a call of xo_stack_switch and rt.fiber_start_addr the address of
// xo_fiber_start, naked functions whose bodies are the instruction
// sequences std/llrt/xo_rt.c's C path uses (switchAsm); rt.tlocal_get and
// rt.tlocal_set are calls of xo_tlocal_get and xo_tlocal_set, never inlined (a
// fiber may go on on another thread after a switch, and a caller must
// not keep a thread local's address), over a table of 16 words in a
// thread_local global that only xo_tlocal_addr names (a split build,
// decision 0168, copies a global into each module that uses it).

// schedSwitch defines xo_stack_switch and xo_fiber_start at the first
// use, for the host target (the Go driver's LLVM builds are for the
// host).
func (g *sgen) schedSwitch() {
	if g.emitted["@xo_stack_switch"] {
		return
	}
	g.emitted["@xo_stack_switch"] = true
	sw, start := switchAsm(runtime.GOOS, g.goarch())
	g.helpers.WriteString(nakedFn("void @xo_stack_switch(i64 %0, i64 %1)", sw))
	g.helpers.WriteString(nakedFn("void @xo_fiber_start()", start))
}

// goarch is the target architecture of the module (the host when unset).
func (g *sgen) goarch() string {
	if g.m.GOARCH != "" {
		return g.m.GOARCH
	}
	return runtime.GOARCH
}

// schedTlocal defines the thread local table and its functions at the first
// use.
func (g *sgen) schedTlocal() {
	if g.emitted["@xo_tlocal_get"] {
		return
	}
	g.emitted["@xo_tlocal_get"] = true
	g.emitted["@xo_tlocal_set"] = true
	g.helpers.WriteString(tlocalIR)
}

// tlocalIR is the thread local table of decision 0212 in IR.
const tlocalIR = `@.xo_tlocal = internal thread_local global [16 x i64] zeroinitializer, align 8

define internal ptr @xo_tlocal_addr(i64 %k) nounwind {
  %p = getelementptr [16 x i64], ptr @.xo_tlocal, i64 0, i64 %k
  ret ptr %p
}

define i64 @xo_tlocal_get(i64 %k) noinline nounwind {
  %p = call ptr @xo_tlocal_addr(i64 %k)
  %v = load i64, ptr %p, align 8
  ret i64 %v
}

define void @xo_tlocal_set(i64 %k, i64 %v) noinline nounwind {
  %p = call ptr @xo_tlocal_addr(i64 %k)
  store i64 %v, ptr %p, align 8
  ret void
}

`

// nakedFn is a function of sig whose body is the assembly lines asm.
func nakedFn(sig string, asm []string) string {
	body := strings.Join(asm, "\\0A")
	body = strings.ReplaceAll(body, "$", "$$")
	return "define " + sig + " naked noinline nounwind {\n  call void asm sideeffect \"" + body + "\", \"\"()\n  unreachable\n}\n\n"
}

// switchAsm returns the bodies of xo_stack_switch and xo_fiber_start on
// goos/goarch: xo_rt.c's xo_fiber_switch and xo_fiber_start.
//
// arm64 (AAPCS64): x19 to x28, fp, lr, and d8 to d15 are callee saved
// (x18 is the platform's on Darwin and Windows, where it points at the
// TEB: not touched); on Windows the TEB's stack bounds (StackBase [x18,
// #8], StackLimit [x18, #16], DeallocationStack [x18, #0x1478]) switch
// with the stack. x86_64 System V: rbx, rbp, r12 to r15, and the control
// words of MXCSR and the x87 unit; Windows x64 also rdi, rsi, xmm6 to
// xmm15, and the TEB's bounds (gs:0x08, gs:0x10, gs:0x1478). A new
// fiber's first frame (std/native/llrt/sched.xo fiber_frame) holds its
// argument and function in the first two registers restored.
func switchAsm(goos, goarch string) (sw, start []string) {
	if goarch == "amd64" {
		if goos == "windows" {
			return amd64WinSwitch, []string{"subq $32, %rsp", "movq %rbx, %rcx", "callq *%r12", "ud2"}
		}
		return amd64Switch, []string{"movq %rbx, %rdi", "callq *%r12", "ud2"}
	}
	start = []string{"mov x0, x19", "blr x20", "brk #1"}
	if goos == "windows" {
		return arm64WinSwitch, start
	}
	return Arm64Switch, start
}

// Arm64Switch is xo_stack_switch on arm64 other than Windows (also the
// arm64 emitters', internal/arm64).
var Arm64Switch = []string{
	"sub sp, sp, #160",
	"stp x19, x20, [sp, #0]",
	"stp x21, x22, [sp, #16]",
	"stp x23, x24, [sp, #32]",
	"stp x25, x26, [sp, #48]",
	"stp x27, x28, [sp, #64]",
	"stp x29, x30, [sp, #80]",
	"stp d8, d9, [sp, #96]",
	"stp d10, d11, [sp, #112]",
	"stp d12, d13, [sp, #128]",
	"stp d14, d15, [sp, #144]",
	"mov x9, sp",
	"str x9, [x0]",
	"mov sp, x1",
	"ldp x19, x20, [sp, #0]",
	"ldp x21, x22, [sp, #16]",
	"ldp x23, x24, [sp, #32]",
	"ldp x25, x26, [sp, #48]",
	"ldp x27, x28, [sp, #64]",
	"ldp x29, x30, [sp, #80]",
	"ldp d8, d9, [sp, #96]",
	"ldp d10, d11, [sp, #112]",
	"ldp d12, d13, [sp, #128]",
	"ldp d14, d15, [sp, #144]",
	"add sp, sp, #160",
	"ret",
}

var arm64WinSwitch = []string{
	"sub sp, sp, #192",
	"stp x19, x20, [sp, #0]",
	"stp x21, x22, [sp, #16]",
	"stp x23, x24, [sp, #32]",
	"stp x25, x26, [sp, #48]",
	"stp x27, x28, [sp, #64]",
	"stp x29, x30, [sp, #80]",
	"stp d8, d9, [sp, #96]",
	"stp d10, d11, [sp, #112]",
	"stp d12, d13, [sp, #128]",
	"stp d14, d15, [sp, #144]",
	"ldr x9, [x18, #8]",
	"ldr x10, [x18, #16]",
	"stp x9, x10, [sp, #160]",
	"ldr x9, [x18, #0x1478]",
	"str x9, [sp, #176]",
	"mov x9, sp",
	"str x9, [x0]",
	"mov sp, x1",
	"ldp x9, x10, [sp, #160]",
	"str x9, [x18, #8]",
	"str x10, [x18, #16]",
	"ldr x9, [sp, #176]",
	"str x9, [x18, #0x1478]",
	"ldp x19, x20, [sp, #0]",
	"ldp x21, x22, [sp, #16]",
	"ldp x23, x24, [sp, #32]",
	"ldp x25, x26, [sp, #48]",
	"ldp x27, x28, [sp, #64]",
	"ldp x29, x30, [sp, #80]",
	"ldp d8, d9, [sp, #96]",
	"ldp d10, d11, [sp, #112]",
	"ldp d12, d13, [sp, #128]",
	"ldp d14, d15, [sp, #144]",
	"add sp, sp, #192",
	"ret",
}

var amd64WinSwitch = []string{
	"pushq %rbp",
	"pushq %rbx",
	"pushq %rdi",
	"pushq %rsi",
	"pushq %r12",
	"pushq %r13",
	"pushq %r14",
	"pushq %r15",
	"subq $192, %rsp",
	"movq %gs:0x1478, %rax",
	"movq %rax, 184(%rsp)",
	"movq %gs:0x10, %rax",
	"movq %rax, 176(%rsp)",
	"movq %gs:0x08, %rax",
	"movq %rax, 168(%rsp)",
	"movdqu %xmm6, 8(%rsp)",
	"movdqu %xmm7, 24(%rsp)",
	"movdqu %xmm8, 40(%rsp)",
	"movdqu %xmm9, 56(%rsp)",
	"movdqu %xmm10, 72(%rsp)",
	"movdqu %xmm11, 88(%rsp)",
	"movdqu %xmm12, 104(%rsp)",
	"movdqu %xmm13, 120(%rsp)",
	"movdqu %xmm14, 136(%rsp)",
	"movdqu %xmm15, 152(%rsp)",
	"stmxcsr (%rsp)",
	"fnstcw 4(%rsp)",
	"movq %rsp, (%rcx)",
	"movq %rdx, %rsp",
	"ldmxcsr (%rsp)",
	"fldcw 4(%rsp)",
	"movdqu 8(%rsp), %xmm6",
	"movdqu 24(%rsp), %xmm7",
	"movdqu 40(%rsp), %xmm8",
	"movdqu 56(%rsp), %xmm9",
	"movdqu 72(%rsp), %xmm10",
	"movdqu 88(%rsp), %xmm11",
	"movdqu 104(%rsp), %xmm12",
	"movdqu 120(%rsp), %xmm13",
	"movdqu 136(%rsp), %xmm14",
	"movdqu 152(%rsp), %xmm15",
	"movq 168(%rsp), %rax",
	"movq %rax, %gs:0x08",
	"movq 176(%rsp), %rax",
	"movq %rax, %gs:0x10",
	"movq 184(%rsp), %rax",
	"movq %rax, %gs:0x1478",
	"addq $192, %rsp",
	"popq %r15",
	"popq %r14",
	"popq %r13",
	"popq %r12",
	"popq %rsi",
	"popq %rdi",
	"popq %rbx",
	"popq %rbp",
	"ret",
}

var amd64Switch = []string{
	"pushq %rbp",
	"pushq %rbx",
	"pushq %r12",
	"pushq %r13",
	"pushq %r14",
	"pushq %r15",
	"subq $8, %rsp",
	"stmxcsr (%rsp)",
	"fnstcw 4(%rsp)",
	"movq %rsp, (%rdi)",
	"movq %rsi, %rsp",
	"ldmxcsr (%rsp)",
	"fldcw 4(%rsp)",
	"addq $8, %rsp",
	"popq %r15",
	"popq %r14",
	"popq %r13",
	"popq %r12",
	"popq %rbx",
	"popq %rbp",
	"ret",
}
