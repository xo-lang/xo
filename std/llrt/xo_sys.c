// ---------------------------------------------------------------------------
// The Xo system call layers (std/native/llrt/sysio.xo, fsops.xo; decision
// 0206) take nothing from the C runtime: the test world's switches are
// twflags.xo's, the clock's scratch and the outcome of a file operation
// sysio.xo's, getauxval linux.xo's (work items B16 and B24 of the zero-C
// audit). Their layer 0 arms (a wasm module's, whose own runtime serves
// them) name these on the native targets too
// (gotchas/dead-target-arm-still-links.md); they are never run.

XO_NORETURN static void sys_dead(void) {
	static const char m[] = "a wasm layer call on a native target";
	xo_fault_raise(m, sizeof m - 1, "", 0);
}

void xo_fs_set_err(int64_t st, const char *p, int64_t pn, const char *m, int64_t mn) {
	(void)st, (void)p, (void)pn, (void)m, (void)mn;
	sys_dead();
}
int64_t xo_fs_status(void) { sys_dead(); }
xo_rlist *xo_fs_msg(void) { sys_dead(); }
xo_rlist *xo_fs_read(const char *p, int64_t n) {
	(void)p, (void)n;
	sys_dead();
}
