// Processes (std.md 3.7 and 3.8, decisions 0107, 0161, 0172), included
// by xo_std.c in programs that start commands (XO_PROC): Proc.run,
// Cmd.output, run, and spawn, pipelines, and children are
// std/native/llrt/proc.xo's on every OS (the zero-C audit's B25: stage 4
// macOS and Linux, stage 5 Windows). What stays here is a child's record:
// the object header and proc.xo's words (CHILD_X_SIZE bytes, proc.xo
// CH_SIZE), freed by proc.xo child_free through cabi_proc.xo's export (a
// program that only fakes commands links the weak stand in: its fake
// children hold no descriptors).
enum { CHILD_X_SIZE = 240 };

__attribute__((weak)) void xo_x_proc_child_free(int64_t ch) { (void)ch; }

static void child_free_x(void *p) {
	xo_x_proc_child_free((int64_t)(intptr_t)p);
	xo_free(p);
}

void *xo_proc_child_new(void) {
	void *ch = xo_alloc(CHILD_X_SIZE);
	memset(ch, 0, CHILD_X_SIZE);
	h_init(ch, child_free_x);
	return ch;
}
