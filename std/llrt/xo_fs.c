// ---------------------------------------------------------------------------
// The Fs operations past read, write, and stat (std.md 3.1, decisions 0102,
// 0163, 0172) are Xo: std/native/llrt/fsops.xo over linux.xo, darwin.xo,
// and windows.xo (work item B16). What stays here: the records of file and
// lock handles, whose release closes the descriptor, and the stubs that
// keep fsops.xo's layer 0 arms linked.

// xo_time_of and xo_time_ns convert between a Time and its Unix
// nanoseconds, which is how a Time is lowered.
int64_t xo_time_of(int64_t ns) { return ns; }
int64_t xo_time_ns(int64_t t) { return t; }

// xo_fs_canceled reports whether an Fs operation reports Canceled: the
// task is canceled and not in deferred code (std/rt Task.Canceled is
// shield aware too).
int64_t xo_fs_canceled(void) { return ctx_canceled(ctx()); }

// ---------------------------------------------------------------------------
// File and lock handles (std.md 3.1, decisions 0163 and 0172): fsops.xo
// opens the descriptor (a HANDLE on Windows) and reads and writes the
// records' words; a handle dropped without close still closes it.

#define FS_BUF (64 << 10)

// The layout fsops.xo reads and writes: fd at 16, buf at 24, r at 32, w
// at 40.
typedef struct {
	xo_hdr h;
	int64_t fd; // -1 once closed
	char *buf;
	int64_t r, w; // the buffered bytes are buf[r:w]
} xo_fsfile;

// fs_close closes the descriptor of a handle record: on Windows the
// HANDLE fsops.xo opened.
#ifdef _WIN32
#define fs_close(fd) CloseHandle((HANDLE)(intptr_t)(fd))
#else
#define fs_close(fd) close((int)(fd))
#endif

static void fsfile_free(void *p) {
	xo_fsfile *f = p;
	if (f->fd >= 0) {
		fs_close(f->fd);
	}
	free(f->buf);
	xo_free(f);
}

// xo_fs_raw_file is the handle of the descriptor fd that fsops.xo opened
// (with the 64 KiB read buffer when buffered): the record is fsops.xo's,
// freeing it closes fd.
xo_fsfile *xo_fs_raw_file(int64_t fd, int64_t buffered) {
	xo_fsfile *f = xo_alloc(sizeof(xo_fsfile));
	memset(f, 0, sizeof *f);
	h_init(f, fsfile_free);
	f->fd = fd;
	if (buffered && !(f->buf = malloc(FS_BUF))) {
		xo_oom();
	}
	return f;
}

// A lock handle holds the descriptor locked: fd at 16, held at 24.
typedef struct {
	xo_hdr h;
	int64_t fd;
	int64_t held;
} xo_fslock;

static void fslock_free(void *p) {
	xo_fslock *l = p;
	if (l->fd >= 0) {
		fs_close(l->fd); // releases the lock
	}
	xo_free(l);
}

// xo_fs_raw_lock is the lock handle of the descriptor fd that fsops.xo
// opened (-1 for none), held or not: freeing it closes fd.
xo_fslock *xo_fs_raw_lock(int64_t fd, int64_t held) {
	xo_fslock *l = xo_alloc(sizeof(xo_fslock));
	h_init(l, fslock_free);
	l->fd = fd;
	l->held = held;
	return l;
}

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
FS_DEAD(xo_fsfile *, xo_fs_open, const char *p, int64_t n, int64_t mode, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_file_closed, xo_fsfile *f)
FS_DEAD(xo_rlist *, xo_fs_file_read, xo_fsfile *f, int64_t max, const char *x, int64_t xn)
FS_DEAD(xo_rlist *, xo_fs_file_read_line, xo_fsfile *f, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_file_write, xo_fsfile *f, const char *p, int64_t n, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_file_close, xo_fsfile *f, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_copy, const char *a, int64_t an, const char *b, int64_t bn, const char *xa, int64_t xan, const char *xb, int64_t xbn)
FS_DEAD(int64_t, xo_fs_perm, const char *p, int64_t n, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_chmod, const char *p, int64_t n, int64_t mode, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_symlink, const char *t, int64_t tn, const char *p, int64_t n, const char *x, int64_t xn)
FS_DEAD(xo_rlist *, xo_fs_read_link, const char *p, int64_t n, const char *x, int64_t xn)
FS_DEAD(xo_fslock *, xo_fs_lock, const char *p, int64_t n, int64_t wait, const char *x, int64_t xn)
FS_DEAD(xo_fslock *, xo_fs_lock_shared, const char *p, int64_t n, int64_t wait, const char *x, int64_t xn)
FS_DEAD(int64_t, xo_fs_lock_held, xo_fslock *l)
FS_DEAD(void, xo_fs_unlock, xo_fslock *l)
