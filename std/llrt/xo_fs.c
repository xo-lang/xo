// ---------------------------------------------------------------------------
// The Fs operations past read, write, and stat (std.md 3.1, decisions 0102,
// 0163, 0172) are Xo: std/native/llrt/fsops.xo over linux.xo, darwin.xo,
// and windows.xo (work item B16), the file and lock handles' records too
// (fsops.xo raw_file, raw_lock, freed through the services record). What
// stays here: Time's conversions, the cancellation check, and the stubs
// that keep fsops.xo's layer 0 arms linked.

// xo_time_of and xo_time_ns convert between a Time and its Unix
// nanoseconds, which is how a Time is lowered.
int64_t xo_time_of(int64_t ns) { return ns; }
int64_t xo_time_ns(int64_t t) { return t; }

// xo_fs_canceled reports whether an Fs operation reports Canceled: the
// task is canceled and not in deferred code (std/rt Task.Canceled is
// shield aware too).
int64_t xo_fs_canceled(void) { return ctx_canceled(ctx()); }

// ---------------------------------------------------------------------------
// fsops.xo's and sysio.xo's layer 0 arms (a wasm module's, whose own
// runtime serves them) name these on the native targets too: every arm of
// a `match` on target lowers, and a development build (XO_LLVM_OPT=O0,
// decision 0220) keeps the calls (gotchas/dead-target-arm-still-links.md).
// They are never run.
XO_NORETURN static void fs_dead(void) {
	static const char m[] = "a C Fs operation on a native target";
	xo_fault_raise(m, sizeof m - 1, "", 0);
}
#define FS_DEAD(t, name, ...)                                                                                              \
	t name(__VA_ARGS__) { fs_dead(); }
FS_DEAD(int64_t, xo_fs_stat, const char *p, int64_t n, int64_t follow, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_info, int64_t which)
FS_DEAD(xo_rlist *, xo_fs_names, const char *p, int64_t n, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_write, const char *p, int64_t n, const char *data, int64_t dn, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_rename, const char *a, int64_t an, const char *b, int64_t bn, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_remove, const char *p, int64_t n, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_chtimes, const char *p, int64_t n, int64_t t, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_case_sensitive, void)
FS_DEAD(int64_t, xo_fs_raw_mkdir, const char *p, int64_t n)
FS_DEAD(int64_t, xo_fs_raw_rmdir, const char *p, int64_t n)
FS_DEAD(int64_t, xo_fs_raw_unlink, const char *p, int64_t n)
FS_DEAD(int64_t, xo_fs_raw_kind, const char *p, int64_t n, int64_t follow)
FS_DEAD(xo_rlist *, xo_fs_raw_names, const char *p, int64_t n)
FS_DEAD(int64_t, xo_fs_raw_errno, void)
FS_DEAD(int64_t, xo_fs_raw_create, const char *p, int64_t n)
FS_DEAD(xo_rstr *, xo_fs_raw_getcwd, int64_t *len)
FS_DEAD(int64_t, xo_fs_raw_fail, int64_t err, const char *x, int64_t xn)
FS_DEAD(void *, xo_fs_open, const char *p, int64_t n, int64_t mode, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_file_closed, void *f)
FS_DEAD(xo_rlist *, xo_fs_file_read, void *f, int64_t max, const char *x, int64_t xn)
FS_DEAD(xo_rlist *, xo_fs_file_read_line, void *f, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_file_write, void *f, const char *p, int64_t n, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_file_close, void *f, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_copy, const char *a, int64_t an, const char *b, int64_t bn, const char *xa, int64_t xan, const char *xb, int64_t xbn)
FS_DEAD(int64_t, xo_fs_perm, const char *p, int64_t n, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_chmod, const char *p, int64_t n, int64_t mode, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_symlink, const char *t, int64_t tn, const char *p, int64_t n, const char *x, int64_t xn)
FS_DEAD(xo_rlist *, xo_fs_read_link, const char *p, int64_t n, const char *x, int64_t xn)
FS_DEAD(void *, xo_fs_lock, const char *p, int64_t n, int64_t wait, const char *x, int64_t xn)
FS_DEAD(void *, xo_fs_lock_shared, const char *p, int64_t n, int64_t wait, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_lock_held, void *l)
FS_DEAD(void, xo_fs_unlock, void *l)
