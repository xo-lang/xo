
// ---------------------------------------------------------------------------
// The other Fs operations (std.md 3.1, decision 0102): system calls on
// paths std/native/prelude.xo already resolved against the capability's
// root and checked. Each returns 0 or the status of xo_fs_read (FsErr's
// tag plus one) with the path and message for xo_fs_msg; Go's os package
// semantics (std/rt caps.go).

#include <dirent.h>
#include <sys/stat.h>
#include <sys/time.h>

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

// fs_remove_all is os.RemoveAll: nil for a missing path.
static int fs_remove_all(const char *path) {
	struct stat sb;
	if (lstat(path, &sb) != 0) {
		return errno == ENOENT ? 0 : errno;
	}
	if (!S_ISDIR(sb.st_mode)) {
		return unlink(path) == 0 || errno == ENOENT ? 0 : errno;
	}
	DIR *d = opendir(path);
	if (!d) {
		return errno;
	}
	int first = 0;
	struct dirent *e;
	size_t pl = strlen(path);
	while ((e = readdir(d)) != NULL) {
		if (strcmp(e->d_name, ".") == 0 || strcmp(e->d_name, "..") == 0) {
			continue;
		}
		size_t nl = strlen(e->d_name);
		char *c = malloc(pl + nl + 2);
		if (!c) {
			xo_oom();
		}
		memcpy(c, path, pl);
		c[pl] = '/';
		memcpy(c + pl + 1, e->d_name, nl + 1);
		int r = fs_remove_all(c);
		free(c);
		if (r != 0 && first == 0) {
			first = r;
		}
	}
	closedir(d);
	if (rmdir(path) != 0 && errno != ENOENT) {
		return first ? first : errno;
	}
	return 0;
}

int64_t xo_fs_remove_all(const char *p, int64_t n, const char *x, int64_t xn) {
	char *path = cstr_of(p, n);
	int err = fs_remove_all(n == 0 ? "." : path);
	free(path);
	if (err == 0) {
		return fs_ok();
	}
	char *xp = cstr_of(x, xn);
	int64_t st = fs_fail(err, xp);
	free(xp);
	return st;
}

// fs_mkdir_all is os.MkdirAll(path, 0755).
static int fs_mkdir_all(char *path) {
	struct stat sb;
	if (stat(path, &sb) == 0) {
		return S_ISDIR(sb.st_mode) ? 0 : ENOTDIR;
	}
	size_t l = strlen(path);
	while (l > 1 && path[l - 1] == '/') {
		l--;
	}
	size_t j = l;
	while (j > 0 && path[j - 1] != '/') {
		j--;
	}
	if (j > 1) {
		char saved = path[j - 1];
		path[j - 1] = 0;
		int r = fs_mkdir_all(path);
		path[j - 1] = saved;
		if (r != 0) {
			return r;
		}
	}
	if (mkdir(path, 0755) != 0) {
		int e = errno;
		if (stat(path, &sb) == 0 && S_ISDIR(sb.st_mode)) {
			return 0;
		}
		return e;
	}
	return 0;
}

int64_t xo_fs_mkdir_all(const char *p, int64_t n, const char *x, int64_t xn) {
	char *path = cstr_of(p, n);
	int err = fs_mkdir_all(path);
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

// xo_fs_create_temp is os.CreateTemp(dir, prefix + "*"): the new file's
// base name (prefix and a random number), empty with the status set on an
// error (which names x).
xo_rstr *xo_fs_create_temp(const char *d, int64_t dn, const char *pre, int64_t pn, const char *x, int64_t xn, int64_t *len) {
	char *dir = cstr_of(d, dn);
	int err = EEXIST;
	char name[512];
	for (int i = 0; i < 10000 && err == EEXIST; i++) {
		uint32_t r = (uint32_t)cr_rand_u64();
		int k = snprintf(name, sizeof name, "%.*s%u", (int)pn, pre, r);
		size_t full = strlen(dir) + (size_t)k + 2;
		char *path = malloc(full);
		if (!path) {
			xo_oom();
		}
		snprintf(path, full, "%s/%s", dn == 0 ? "." : dir, name);
		int fd = open(path, O_RDWR | O_CREAT | O_EXCL | O_CLOEXEC, 0600);
		err = fd >= 0 ? 0 : errno;
		if (fd >= 0) {
			close(fd);
		}
		free(path);
	}
	free(dir);
	if (err != 0) {
		char *xp = cstr_of(x, xn);
		fs_fail(err, xp);
		free(xp);
		*len = 0;
		return rs_new(0);
	}
	fs_ok();
	int64_t k = (int64_t)strlen(name);
	xo_rstr *s = rs_new(k);
	memcpy(s->data, name, (size_t)k);
	*len = k;
	return s;
}

// xo_fs_cwd is the working directory.
xo_rstr *xo_fs_cwd(int64_t *len) {
	char buf[4096];
	const char *w = getcwd(buf, sizeof buf);
	if (!w) {
		w = ".";
	}
	int64_t k = (int64_t)strlen(w);
	xo_rstr *s = rs_new(k);
	memcpy(s->data, w, (size_t)k);
	*len = k;
	return s;
}

// xo_fs_case_sensitive is false on macOS and Windows (std/rt).
int64_t xo_fs_case_sensitive(void) {
#ifdef __APPLE__
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
