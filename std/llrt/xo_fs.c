// ---------------------------------------------------------------------------
// The Fs operations (std.md 3.1, decisions 0102, 0163, 0172) are Xo:
// std/native/llrt/fsops.xo and sysio.xo over linux.xo, darwin.xo, and
// windows.xo (work item B16), the file and lock handles' records too
// (fsops.xo raw_file, raw_lock, freed through the services record). A
// Time's conversions are std's own arithmetic (Time.UNIX_EPOCH), the
// cancellation check the native prelude's llrt_fs_canceled (poll.xo).
//
// Seed compatibility (zero-C audit end pass): everything here. This
// compiler lowers rt.fs_canceled to llrt_fs_canceled and the calls of
// fsops.xo's and sysio.xo's layer 0 arms (a wasm module's) to a fault on
// the native targets (selfhost/ssa call.xo wasm_only_rt), so it never
// names these and never sets XO_FS; the seed (0.11.0) does, for every
// arm of a `match` on target (gotchas/dead-target-arm-still-links.md).
// Delete the file, XO_FS, and its include when the seed has this change.

int64_t xo_fs_canceled(void) { return ctx_canceled(ctx()); }

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
