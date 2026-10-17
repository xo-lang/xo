# A static link has no .eh_frame_hdr unless asked

Status: Anticipated (2026-10-14; avoided up front by decision 0221, not hit).

**Symptom**: with XO_NOLIBC=1 a fault ends the program with status 70
without running deferred code, although unwind.xo is linked: the Xo
unwinder finds no PT_GNU_EH_FRAME segment and treats it as the end of the
stack.

**Cause**: the GCC and Clang drivers pass `--eh-frame-hdr` to the linker
only for dynamic (or static-pie) links; a plain `-static` link relies on
crtbegin.o registering .eh_frame with libgcc, and `-nostdlib` drops both.
So the binary has .eh_frame but no header table or segment.

**Fix**: compiler/xo link.xo sys_link_args passes `-Wl,--eh-frame-hdr`
(and `-Wl,--defsym=xo_personality=xo_x_personality`) for XO_NOLIBC links.
Check a binary with `readelf -lW prog | grep GNU_EH_FRAME`. The ELF
linker of decision 0182 must write the segment too before it links static
programs.
