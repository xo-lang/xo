package arm64

import "strings"

// The task switch and thread state of decision 0212 (selfhost/arm64
// sched.xo is the self hosted copy, which this mirrors): rt.stack_switch
// is a call of xo_stack_switch, rt.fiber_start_addr the address of
// xo_fiber_entry, and rt.tlocal_get and rt.tlocal_set calls of
// xo_tlocal_get and xo_tlocal_set (the C runtime calls those too), all
// emitted once in a program that uses them, as instruction sequences:
// the switch is std/llrt/xo_rt.c's C path's xo_fiber_switch (AAPCS64:
// x19 to x28, fp, lr, d8 to d15 saved on the stack, whose pointer is
// stored at the first argument, then restored from the second); the
// thread locals are 16 words in the thread's static TLS block, reached
// through a TLV descriptor on Darwin and the local exec model on Linux.

// stackSwitch is the body of xo_stack_switch.
var stackSwitch = []string{
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

// schedAsm is the text of the functions and the thread local table on
// os.
func schedAsm(os OS) string {
	var b strings.Builder
	fn := func(name string, global bool, body []string) {
		if os == Darwin {
			if global {
				b.WriteString("\t.globl _" + name + "\n")
			}
			b.WriteString("\t.p2align 2\n_" + name + ":\n")
		} else {
			if global {
				b.WriteString("\t.globl " + name + "\n\t.hidden " + name + "\n")
			}
			b.WriteString("\t.type " + name + ", @function\n\t.p2align 2\n" + name + ":\n")
		}
		for _, l := range body {
			b.WriteString("\t" + l + "\n")
		}
	}
	if os == Darwin {
		b.WriteString("\t.section __TEXT,__text,regular,pure_instructions\n")
	} else {
		b.WriteString("\t.text\n")
	}
	fn("xo_stack_switch", true, stackSwitch)
	// a new fiber's first frame holds its argument in x19 and its
	// function in x20 (std/native/llrt/sched.xo fiber_frame)
	fn("xo_fiber_entry", false, []string{"mov x0, x19", "blr x20", "brk #1"})
	if os == Darwin {
		// the TLV descriptor's thunk returns the table's address in x0
		// and keeps every register but x0, x16, x17, and lr (Darwin's
		// TLV convention): the index and the return address wait in x9
		// and x10, with no frame
		tlv := []string{"mov x10, x30", "adrp x0, _xo_tlocal@TLVPPAGE", "ldr x0, [x0, _xo_tlocal@TLVPPAGEOFF]", "ldr x8, [x0]", "blr x8"}
		fn("xo_tlocal_get", true, append(append([]string{"mov x9, x0"}, tlv...), "ldr x0, [x0, x9, lsl #3]", "ret x10"))
		fn("xo_tlocal_set", true, append(append([]string{"mov x9, x0", "mov x11, x1"}, tlv...), "str x11, [x0, x9, lsl #3]", "ret x10"))
		fn("xo_tlocal_addr", true, append(tlv, "ret x10"))
		b.WriteString("\t.section __DATA,__thread_data,thread_local_regular\n\t.p2align 3\n_xo_tlocal_init:\n")
		b.WriteString(strings.Repeat("\t.quad 0\n", 16))
		b.WriteString("\t.section __DATA,__thread_vars,thread_local_variables\n\t.globl _xo_tlocal\n\t.p2align 3\n_xo_tlocal:\n")
		b.WriteString("\t.quad __tlv_bootstrap\n\t.quad 0\n\t.quad _xo_tlocal_init\n")
		return b.String()
	}
	tp := []string{"mrs x9, tpidr_el0", "add x9, x9, :tprel_hi12:xo_tlocal", "add x9, x9, :tprel_lo12_nc:xo_tlocal"}
	fn("xo_tlocal_get", true, append(tp, "ldr x0, [x9, x0, lsl #3]", "ret"))
	fn("xo_tlocal_set", true, append(tp, "str x1, [x9, x0, lsl #3]", "ret"))
	fn("xo_tlocal_addr", true, append(tp, "mov x0, x9", "ret"))
	b.WriteString("\t.section .tdata,\"awT\",@progbits\n\t.globl xo_tlocal\n\t.hidden xo_tlocal\n\t.p2align 3\n\t.type xo_tlocal, @tls_object\n\t.size xo_tlocal, 128\nxo_tlocal:\n")
	b.WriteString(strings.Repeat("\t.xword 0\n", 16))
	return b.String()
}
