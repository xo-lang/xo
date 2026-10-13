
// ---------------------------------------------------------------------------
// The other Fs operations (std.md 3.1, decision 0102): system calls on
// paths std/native/prelude.xo already resolved against the capability's
// root and checked. Each returns 0 or the status of xo_fs_read (FsErr's
// tag plus one) with the path and message for xo_fs_msg; Go's os package
// semantics (std/rt caps.go).

#ifndef _WIN32 // Windows: xo_win.c
#include <dirent.h>
#include <sys/stat.h>
#include <sys/time.h>
#endif

static __thread int64_t n_fs_info[3]; // is_dir, size, modified (ns)

// fs_fail records err for path (the Xo path the error names) and returns
// its status.
static int64_t fs_fail(int err, const char *path) {
	int64_t st = 7;
	switch (err) {
	case ENOENT:
		st = 1;
		break;
	case EEXIST:
		st = 2;
		break;
	case ENOTEMPTY:
		st = 3;
		break;
	case ENOTDIR:
		st = 4;
		break;
	case EISDIR:
		st = 5;
		break;
	case EACCES:
	case EPERM:
		st = 6;
		break;
	}
	n_fs_status = st;
	free(n_fs_path);
	free(n_fs_err);
	n_fs_path = strdup(path);
	n_fs_err = strdup(strerror(err));
	if (!n_fs_path || !n_fs_err) {
		xo_oom();
	}
	if (n_fs_err[0] >= 'A' && n_fs_err[0] <= 'Z') {
		n_fs_err[0] = (char)(n_fs_err[0] + 32);
	}
	return st;
}

static int64_t fs_ok(void) {
	n_fs_status = 0;
	return 0;
}

// xo_fs_stat stats the OS path p (following a symbolic link when follow)
// for xo_fs_info; the error names the Xo path x.
int64_t xo_fs_stat(const char *p, int64_t n, int64_t follow, const char *x, int64_t xn) {
	char *path = cstr_of(p, n);
	struct stat sb;
	int r = follow ? stat(n == 0 ? "." : path, &sb) : lstat(n == 0 ? "." : path, &sb);
	int err = errno;
	free(path);
	if (r != 0) {
		char *xp = cstr_of(x, xn);
		int64_t st = fs_fail(err, xp);
		free(xp);
		return st;
	}
	n_fs_info[0] = S_ISDIR(sb.st_mode) ? 1 : 0;
	n_fs_info[1] = (int64_t)sb.st_size;
#ifdef __APPLE__
	n_fs_info[2] = (int64_t)sb.st_mtimespec.tv_sec * 1000000000 + sb.st_mtimespec.tv_nsec;
#else
	n_fs_info[2] = (int64_t)sb.st_mtim.tv_sec * 1000000000 + sb.st_mtim.tv_nsec;
#endif
	return fs_ok();
}

// xo_fs_info is a field of the last successful xo_fs_stat: 0 is_dir,
// 1 size, 2 modified.
int64_t xo_fs_info(int64_t which) { return n_fs_info[which & 3]; }

// xo_fs_names lists the entries of the OS directory p (not `.` and
// `..`, in no order); the status is set (the error names x).
xo_rlist *xo_fs_names(const char *p, int64_t n, const char *x, int64_t xn) {
	char *path = cstr_of(p, n);
	DIR *d = opendir(n == 0 ? "." : path);
	int err = errno;
	free(path);
	if (!d) {
		char *xp = cstr_of(x, xn);
		fs_fail(err, xp);
		free(xp);
		return n_str_list_new(0);
	}
	xo_rlist *l = n_str_list_new(8);
	struct dirent *e;
	while ((e = readdir(d)) != NULL) {
		if (strcmp(e->d_name, ".") == 0 || strcmp(e->d_name, "..") == 0) {
			continue;
		}
		l = n_str_list_add(l, e->d_name, (int64_t)strlen(e->d_name));
	}
	closedir(d);
	fs_ok();
	return l;
}

// xo_fs_write is os.WriteFile(p, data, 0644).
int64_t xo_fs_write(const char *p, int64_t n, const char *data, int64_t dn, const char *x, int64_t xn) {
	char *path = cstr_of(p, n);
	int fd = open(n == 0 ? "." : path, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 0644);
	int err = errno;
	free(path);
	if (fd >= 0) {
		int64_t off = 0;
		err = 0;
		while (off < dn) {
			ssize_t w = write(fd, data + off, (size_t)(dn - off));
			if (w < 0) {
				if (errno == EINTR) {
					continue;
				}
				err = errno;
				break;
			}
			off += w;
		}
		if (close(fd) != 0 && err == 0) {
			err = errno;
		}
		if (err == 0) {
			return fs_ok();
		}
	}
	char *xp = cstr_of(x, xn);
	int64_t st = fs_fail(err, xp);
	free(xp);
	return st;
}

// xo_fs_rename is os.Rename(a, b); the error names x.
int64_t xo_fs_rename(const char *a, int64_t an, const char *b, int64_t bn, const char *x, int64_t xn) {
	char *pa = cstr_of(a, an), *pb = cstr_of(b, bn);
	int r = rename(pa, pb);
	int err = errno;
	free(pa);
	free(pb);
	if (r == 0) {
		return fs_ok();
	}
	char *xp = cstr_of(x, xn);
	int64_t st = fs_fail(err, xp);
	free(xp);
	return st;
}

// fs_remove is os.Remove: unlink, then rmdir; the rmdir error unless it
// is ENOTDIR.
static int fs_remove(const char *path) {
	if (unlink(path) == 0) {
		return 0;
	}
	int e = errno;
	if (rmdir(path) == 0) {
		return 0;
	}
	if (errno != ENOTDIR) {
		e = errno;
	}
	return e;
}

int64_t xo_fs_remove(const char *p, int64_t n, const char *x, int64_t xn) {
	char *path = cstr_of(p, n);
	int err = fs_remove(n == 0 ? "." : path);
	free(path);
	if (err == 0) {
		return fs_ok();
	}
	char *xp = cstr_of(x, xn);
	int64_t st = fs_fail(err, xp);
	free(xp);
	return st;
}

// xo_fs_chtimes is os.Chtimes(p, t, t) with t in nanoseconds.
int64_t xo_fs_chtimes(const char *p, int64_t n, int64_t t, const char *x, int64_t xn) {
	char *path = cstr_of(p, n);
	struct timespec ts[2];
	int64_t s = t / 1000000000, ns = t % 1000000000;
	if (ns < 0) {
		ns += 1000000000;
		s--;
	}
	ts[0].tv_sec = (time_t)s;
	ts[0].tv_nsec = (long)ns;
	ts[1] = ts[0];
	int r = utimensat(AT_FDCWD, n == 0 ? "." : path, ts, 0);
	int err = errno;
	free(path);
	if (r == 0) {
		return fs_ok();
	}
	char *xp = cstr_of(x, xn);
	int64_t st = fs_fail(err, xp);
	free(xp);
	return st;
}

// ---------------------------------------------------------------------------
// The system calls under std/native/llrt/fspath.xo (decision 0193 step 6),
// which walks the paths of os.MkdirAll and os.RemoveAll and names the
// files of os.CreateTemp in Xo: each takes the OS path and returns 0 or
// the errno; xo_fs_raw_fail turns an errno into the status of the
// operation (the error naming the Xo path).

static __thread int64_t n_fs_errno;

int64_t xo_fs_raw_mkdir(const char *p, int64_t n) {
	char *path = cstr_of(p, n);
	int r = mkdir(n == 0 ? "." : path, 0755);
	int e = errno;
	free(path);
	return r == 0 ? 0 : e;
}

int64_t xo_fs_raw_rmdir(const char *p, int64_t n) {
	char *path = cstr_of(p, n);
	int r = rmdir(n == 0 ? "." : path);
	int e = errno;
	free(path);
	return r == 0 ? 0 : e;
}

int64_t xo_fs_raw_unlink(const char *p, int64_t n) {
	char *path = cstr_of(p, n);
	int r = unlink(n == 0 ? "." : path);
	int e = errno;
	free(path);
	return r == 0 ? 0 : e;
}

// xo_fs_raw_kind stats p (lstat unless follow): 1 a directory, 0 another
// file, minus the errno when it fails.
int64_t xo_fs_raw_kind(const char *p, int64_t n, int64_t follow) {
	char *path = cstr_of(p, n);
	struct stat sb;
	int r = follow ? stat(n == 0 ? "." : path, &sb) : lstat(n == 0 ? "." : path, &sb);
	int e = errno;
	free(path);
	if (r != 0) {
		return -(int64_t)(e ? e : EIO);
	}
	return S_ISDIR(sb.st_mode) ? 1 : 0;
}

// xo_fs_raw_names lists the entries of the directory p (not `.` and
// `..`); xo_fs_raw_errno is 0 or why it could not be read.
xo_rlist *xo_fs_raw_names(const char *p, int64_t n) {
	char *path = cstr_of(p, n);
	DIR *d = opendir(n == 0 ? "." : path);
	int err = errno;
	free(path);
	if (!d) {
		n_fs_errno = err ? err : EIO;
		return n_str_list_new(0);
	}
	xo_rlist *l = n_str_list_new(8);
	struct dirent *e;
	while ((e = readdir(d)) != NULL) {
		if (strcmp(e->d_name, ".") == 0 || strcmp(e->d_name, "..") == 0) {
			continue;
		}
		l = n_str_list_add(l, e->d_name, (int64_t)strlen(e->d_name));
	}
	closedir(d);
	n_fs_errno = 0;
	return l;
}

int64_t xo_fs_raw_errno(void) { return n_fs_errno; }

// xo_fs_raw_create creates the file p for os.CreateTemp (O_EXCL, 0600).
int64_t xo_fs_raw_create(const char *p, int64_t n) {
	char *path = cstr_of(p, n);
	int fd = open(path, O_RDWR | O_CREAT | O_EXCL | O_CLOEXEC, 0600);
	int e = errno;
	free(path);
	if (fd < 0) {
		return e;
	}
	close(fd);
	return 0;
}

// xo_fs_raw_rand is a random number below 2^32 (os.CreateTemp's names).
int64_t xo_fs_raw_rand(void) { return (int64_t)(uint32_t)cr_rand_u64(); }

// xo_fs_raw_getcwd is the working directory as the system gives it.
xo_rstr *xo_fs_raw_getcwd(int64_t *len) {
	char buf[4096];
	const char *w = getcwd(buf, sizeof buf);
	if (!w) {
		w = ".";
	}
	return nstr(w, (int64_t)strlen(w), len);
}

// xo_fs_raw_fail records err for the Xo path x: the operation's status;
// 0 records success.
int64_t xo_fs_raw_fail(int64_t err, const char *x, int64_t xn) {
	if (err == 0) {
		return fs_ok();
	}
	char *xp = cstr_of(x, xn);
	int64_t st = fs_fail((int)err, xp);
	free(xp);
	return st;
}

// xo_fs_case_sensitive is false on macOS and Windows (std/rt).
int64_t xo_fs_case_sensitive(void) {
#if defined(__APPLE__) || defined(_WIN32)
	return 0;
#else
	return 1;
#endif
}

// xo_time_of and xo_time_ns convert between a Time and its Unix
// nanoseconds, which is how a Time is lowered.
int64_t xo_time_of(int64_t ns) { return ns; }
int64_t xo_time_ns(int64_t t) { return t; }

// xo_fs_canceled reports whether an Fs operation reports Canceled: the
// task is canceled and not in deferred code (std/rt Task.Canceled is
// shield aware too).
int64_t xo_fs_canceled(void) { return ctx_canceled(ctx()); }

// ---------------------------------------------------------------------------
// File handles, copies, modes, links, and locks (std.md 3.1, decisions
// 0163 and 0172), as std/rt fsfile.go: std/native/prelude.xo resolves and
// checks the paths and keeps the handle's state messages; the functions
// here make the system calls. A read handle is buffered like Go's
// bufio.Reader of 64 KiB (a read larger than the buffer goes to the file
// when nothing is buffered).

#ifndef _WIN32
#include <sys/file.h>
#endif

#define FS_BUF (64 << 10)

typedef struct {
	xo_hdr h;
	int fd; // -1 once closed
	char *buf;
	int64_t r, w; // the buffered bytes are buf[r:w]
	int eof;
} xo_fsfile;

static void fsfile_free(void *p) {
	xo_fsfile *f = p;
	if (f->fd >= 0) {
		close(f->fd);
	}
	free(f->buf);
	xo_free(f);
}

static xo_fsfile *fsfile_new(int fd) {
	xo_fsfile *f = xo_alloc(sizeof(xo_fsfile));
	memset(f, 0, sizeof *f);
	h_init(f, fsfile_free);
	f->fd = fd;
	return f;
}

// xo_fs_open opens the OS path p: mode 0 for reading, 1 created or
// truncated, 2 appended to (created if missing); the error names x.
xo_fsfile *xo_fs_open(const char *p, int64_t n, int64_t mode, const char *x, int64_t xn) {
	char *path = cstr_of(p, n);
	int flags = O_RDONLY;
	if (mode == 1) {
		flags = O_WRONLY | O_CREAT | O_TRUNC;
	} else if (mode == 2) {
		flags = O_WRONLY | O_CREAT | O_APPEND;
	}
	int err = 0;
	if (mode == 0) {
		struct stat sb;
		if (stat(n == 0 ? "." : path, &sb) == 0 && S_ISDIR(sb.st_mode)) {
			err = EISDIR;
		}
	}
	int fd = -1;
	if (!err) {
		fd = open(n == 0 ? "." : path, flags | O_CLOEXEC, 0666);
		if (fd < 0) {
			err = errno;
		}
	}
	free(path);
	if (fd < 0) {
		char *xp = cstr_of(x, xn);
		fs_fail(err, xp);
		free(xp);
		return fsfile_new(-1);
	}
	fs_ok();
	return fsfile_new(fd);
}

int64_t xo_fs_file_closed(xo_fsfile *f) { return f->fd < 0; }

// fsfile_fill reads once into the empty buffer: the count, 0 at the end,
// -errno.
static int64_t fsfile_fill(xo_fsfile *f) {
	if (!f->buf) {
		f->buf = malloc(FS_BUF);
		if (!f->buf) {
			xo_oom();
		}
	}
	f->r = f->w = 0;
	for (;;) {
		ssize_t k = read(f->fd, f->buf, FS_BUF);
		if (k < 0 && errno == EINTR) {
			continue;
		}
		if (k < 0) {
			return -errno;
		}
		f->w = k;
		return k;
	}
}

// xo_fs_file_read is `f.read(max)` (max > 0): the bytes in a list of one,
// or the empty list at the end with the status 0, or an error's status.
xo_rlist *xo_fs_file_read(xo_fsfile *f, int64_t max, const char *x, int64_t xn) {
	if (max > (1 << 20)) {
		max = 1 << 20;
	}
	if (f->r == f->w) {
		if (max >= FS_BUF) {
			char *b = malloc((size_t)max);
			if (!b) {
				xo_oom();
			}
			ssize_t k;
			do {
				k = read(f->fd, b, (size_t)max);
			} while (k < 0 && errno == EINTR);
			if (k < 0) {
				int e = errno;
				free(b);
				char *xp = cstr_of(x, xn);
				fs_fail(e, xp);
				free(xp);
				return n_str_list_new(0);
			}
			fs_ok();
			xo_rlist *l = n_str_list_new(1);
			if (k > 0) {
				l = n_str_list_add(l, b, k);
			}
			free(b);
			return l;
		}
		int64_t k = fsfile_fill(f);
		if (k < 0) {
			char *xp = cstr_of(x, xn);
			fs_fail((int)-k, xp);
			free(xp);
			return n_str_list_new(0);
		}
		if (k == 0) {
			fs_ok();
			return n_str_list_new(0);
		}
	}
	int64_t k = f->w - f->r < max ? f->w - f->r : max;
	xo_rlist *l = n_str_list_add(n_str_list_new(1), f->buf + f->r, k);
	f->r += k;
	fs_ok();
	return l;
}

// xo_fs_file_read_line is `f.read_line()`: the line without \n or \r\n
// in a list of one, or the empty list at the end (status 0) or on an
// error.
xo_rlist *xo_fs_file_read_line(xo_fsfile *f, const char *x, int64_t xn) {
	nbuf line = {0};
	for (;;) {
		if (f->r == f->w) {
			int64_t k = fsfile_fill(f);
			if (k < 0) {
				if (line.n > 0) {
					break; // bufio: the partial line, the error on the next call
				}
				free(line.p);
				char *xp = cstr_of(x, xn);
				fs_fail((int)-k, xp);
				free(xp);
				return n_str_list_new(0);
			}
			if (k == 0) {
				if (line.n == 0) {
					fs_ok();
					return n_str_list_new(0);
				}
				break;
			}
		}
		char *s = f->buf + f->r;
		char *nl = memchr(s, '\n', (size_t)(f->w - f->r));
		if (nl) {
			nbuf_put(&line, s, nl - s);
			f->r += nl - s + 1;
			break;
		}
		nbuf_put(&line, s, f->w - f->r);
		f->r = f->w;
	}
	int64_t n = line.n;
	if (n > 0 && line.p[n - 1] == '\r') {
		n--;
	}
	xo_rlist *l = n_str_list_add(n_str_list_new(1), line.p ? line.p : "", n);
	free(line.p);
	fs_ok();
	return l;
}

// xo_fs_file_write is `f.write(data)`: every byte, or an error's status.
int64_t xo_fs_file_write(xo_fsfile *f, const char *p, int64_t n, const char *x, int64_t xn) {
	int64_t off = 0;
	while (off < n) {
		ssize_t w = write(f->fd, p + off, (size_t)(n - off));
		if (w < 0) {
			if (errno == EINTR) {
				continue;
			}
			int e = errno;
			char *xp = cstr_of(x, xn);
			int64_t st = fs_fail(e, xp);
			free(xp);
			return st;
		}
		off += w;
	}
	return fs_ok();
}

// xo_fs_file_close closes f (a second close does nothing).
int64_t xo_fs_file_close(xo_fsfile *f, const char *x, int64_t xn) {
	if (f->fd < 0) {
		return fs_ok();
	}
	int r = close(f->fd);
	int e = errno;
	f->fd = -1;
	if (r != 0 && e != EINTR) {
		char *xp = cstr_of(x, xn);
		int64_t st = fs_fail(e, xp);
		free(xp);
		return st;
	}
	return fs_ok();
}

// xo_fs_copy is std/rt's realFs.copy: the contents of a into b (created
// or truncated) with a's permission bits; errors name xa or xb.
int64_t xo_fs_copy(const char *a, int64_t an, const char *b, int64_t bn, const char *xa, int64_t xan, const char *xb, int64_t xbn) {
	char *pa = cstr_of(a, an), *pb = cstr_of(b, bn);
	int err = 0, to = 0;
	int in = open(pa, O_RDONLY | O_CLOEXEC);
	struct stat sb;
	int out = -1;
	if (in < 0) {
		err = errno;
	} else if (fstat(in, &sb) != 0) {
		err = errno;
	} else if (S_ISDIR(sb.st_mode)) {
		err = EISDIR;
	} else {
		mode_t perm = sb.st_mode & 0777;
		out = open(pb, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, perm);
		if (out < 0) {
			err = errno;
			to = 1;
		} else {
			char *buf = malloc(FS_BUF);
			if (!buf) {
				xo_oom();
			}
			for (;;) {
				ssize_t k = read(in, buf, FS_BUF);
				if (k < 0 && errno == EINTR) {
					continue;
				}
				if (k < 0) {
					err = errno;
					to = 1;
					break;
				}
				if (k == 0) {
					break;
				}
				const char *q = buf;
				while (k > 0) {
					ssize_t w = write(out, q, (size_t)k);
					if (w < 0 && errno == EINTR) {
						continue;
					}
					if (w < 0) {
						err = errno;
						to = 1;
						break;
					}
					q += w;
					k -= w;
				}
				if (err) {
					break;
				}
			}
			free(buf);
			if (close(out) != 0 && !err) {
				err = errno;
				to = 1;
			}
			out = -1;
			if (!err) {
				chmod(pb, perm); // also over an existing file
			}
		}
	}
	if (in >= 0) {
		close(in);
	}
	free(pa);
	free(pb);
	if (!err) {
		return fs_ok();
	}
	char *xp = to ? cstr_of(xb, xbn) : cstr_of(xa, xan);
	int64_t st = fs_fail(err, xp);
	free(xp);
	return st;
}

// xo_fs_perm is the permission bits of p (following links), or -1 with
// the status set.
int64_t xo_fs_perm(const char *p, int64_t n, const char *x, int64_t xn) {
	char *path = cstr_of(p, n);
	struct stat sb;
	int r = stat(n == 0 ? "." : path, &sb);
	int e = errno;
	free(path);
	if (r != 0) {
		char *xp = cstr_of(x, xn);
		fs_fail(e, xp);
		free(xp);
		return -1;
	}
	fs_ok();
	return sb.st_mode & 0777;
}

int64_t xo_fs_chmod(const char *p, int64_t n, int64_t mode, const char *x, int64_t xn) {
	char *path = cstr_of(p, n);
	int r = chmod(n == 0 ? "." : path, (mode_t)(mode & 0777));
	int e = errno;
	free(path);
	if (r == 0) {
		return fs_ok();
	}
	char *xp = cstr_of(x, xn);
	int64_t st = fs_fail(e, xp);
	free(xp);
	return st;
}

int64_t xo_fs_symlink(const char *t, int64_t tn, const char *p, int64_t n, const char *x, int64_t xn) {
	char *target = cstr_of(t, tn), *path = cstr_of(p, n);
	int r = symlink(target, path);
	int e = errno;
	free(target);
	free(path);
	if (r == 0) {
		return fs_ok();
	}
	char *xp = cstr_of(x, xn);
	int64_t st = fs_fail(e, xp);
	free(xp);
	return st;
}

// xo_fs_read_link is the target of the link p in a list of one, the empty
// list when p is not a link (status 0) or on an error.
xo_rlist *xo_fs_read_link(const char *p, int64_t n, const char *x, int64_t xn) {
	char *path = cstr_of(p, n);
	struct stat sb;
	int err = 0;
	xo_rlist *l = n_str_list_new(1);
	if (lstat(n == 0 ? "." : path, &sb) != 0) {
		err = errno;
	} else if (S_ISLNK(sb.st_mode)) {
		char buf[4096];
		ssize_t k = readlink(path, buf, sizeof buf);
		if (k < 0) {
			err = errno;
		} else {
			l = n_str_list_add(l, buf, k);
		}
	}
	free(path);
	if (err) {
		char *xp = cstr_of(x, xn);
		fs_fail(err, xp);
		free(xp);
		return l;
	}
	fs_ok();
	return l;
}

// Advisory locks: a lock handle holds the descriptor flock(2) locked.

typedef struct {
	xo_hdr h;
	int fd;
	int held;
} xo_fslock;

static void fslock_free(void *p) {
	xo_fslock *l = p;
	if (l->fd >= 0) {
		close(l->fd); // releases the lock
	}
	xo_free(l);
}

// xo_fs_lock takes the lock of p (created if missing): wait tells to
// wait (polling every 10 ms, a cancellation point). The handle holds the
// lock when xo_fs_lock_held says so; else the status tells an error
// (8: canceled) or 0 (try_lock found it held).
static xo_fslock *fs_lock_op(const char *p, int64_t n, int64_t wait, const char *x, int64_t xn, int op) {
	xo_fslock *l = xo_alloc(sizeof(xo_fslock));
	memset(l, 0, sizeof *l);
	h_init(l, fslock_free);
	char *path = cstr_of(p, n);
	l->fd = open(n == 0 ? "." : path, O_RDWR | O_CREAT | O_CLOEXEC, 0666);
	int e = errno;
	free(path);
	if (l->fd < 0) {
		char *xp = cstr_of(x, xn);
		fs_fail(e, xp);
		free(xp);
		return l;
	}
	xo_ctx *c = ctx();
	for (;;) {
		if (flock(l->fd, op | LOCK_NB) == 0) {
			l->held = 1;
			fs_ok();
			return l;
		}
		e = errno;
		if (e == EINTR) {
			continue;
		}
		if (e != EWOULDBLOCK && e != EAGAIN) {
			close(l->fd);
			l->fd = -1;
			char *xp = cstr_of(x, xn);
			fs_fail(e, xp);
			free(xp);
			return l;
		}
		if (!wait) {
			close(l->fd);
			l->fd = -1;
			fs_ok();
			return l;
		}
		flush_pending();
		if (clock_wait(c, 10000000) || ctx_canceled(c)) {
			close(l->fd);
			l->fd = -1;
			n_fs_status = 8;
			return l;
		}
	}
}

xo_fslock *xo_fs_lock(const char *p, int64_t n, int64_t wait, const char *x, int64_t xn) {
	return fs_lock_op(p, n, wait, x, xn, LOCK_EX);
}

// xo_fs_lock_shared is xo_fs_lock's shared lock (std/internal's
// lock_shared, decision 0214): LOCK_SH, which the Windows flock shim
// takes with LockFileEx without LOCKFILE_EXCLUSIVE_LOCK.
xo_fslock *xo_fs_lock_shared(const char *p, int64_t n, int64_t wait, const char *x, int64_t xn) {
	return fs_lock_op(p, n, wait, x, xn, LOCK_SH);
}

int64_t xo_fs_lock_held(xo_fslock *l) { return l->held; }

// xo_fs_unlock is `l.unlock()`; a second unlock does nothing.
void xo_fs_unlock(xo_fslock *l) {
	if (!l->held) {
		return;
	}
	l->held = 0;
	flock(l->fd, LOCK_UN);
	close(l->fd);
	l->fd = -1;
}
