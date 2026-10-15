// Proc.run (std.md 3.7, decision 0107), included by xo_std.c in programs
// that call it (XO_PROC, internal/build IOFlags). As std/rt's realProc.run
// (os/exec): the command is looked up in PATH when its name has no slash
// (not found: ProcErr.NotFound), its standard input is /dev/null, its
// standard output and error are read whole through pipes that wait on the
// event loop's poller, and cancelling the task kills it (ProcErr.Canceled).
// In the test world, FakeProc answers with the outputs on_run registered
// (std.md 8.4), and a command without one is NotFound.
//
// The result is a List[Str] that holds the outcome (no thread local
// state: a task's fiber may resume on another worker): the status ("0"
// ran, "1" not found, "2" an error, "3" canceled), then the exit code in
// decimal (or the error's message), then standard output and error when
// it ran.

#ifdef _WIN32
// Windows (decision 0197, stage 2): CreateProcessW instead of posix_spawn,
// with the command line quoted as Go's syscall.EscapeArg and the child's
// standard handles passed through an explicit handle list
// (STARTUPINFOEXW), so no other handle leaks into it. A process id is the
// process handle (pid_t), reaped by WaitForSingleObject. A pipe whose end
// the program reads or writes on the event loop is "pumped": the child
// gets an anonymous pipe, and a thread of its own copies between that
// pipe and a loopback socket pair whose other end the program uses (the
// poller waits on sockets only, xo_win.c). A pipe between two children
// or to the standard input feeder is a plain pipe.
typedef intptr_t pid_t;

// pr_rd and pr_wr read and write the program's end of a pumped pipe (a
// socket), errno set on failure.
static ssize_t pr_rd(int fd, void *b, size_t n) {
	int r = recv((SOCKET)fd, b, (int)n, 0);
	if (r == SOCKET_ERROR) {
		errno = xo_wsa_errno();
		return -1;
	}
	return r;
}

static ssize_t pr_wr(int fd, const void *b, size_t n) {
	int r = send((SOCKET)fd, b, (int)n, 0);
	if (r == SOCKET_ERROR) {
		errno = xo_wsa_errno();
		return -1;
	}
	return r;
}

// pr_w_text is the system's message for a Win32 error (Go's
// syscall.Errno.Error on Windows: FormatMessage without the line end).
static void pr_w_text(DWORD e, char *buf, size_t n) {
	wchar_t w[512];
	DWORD k = FormatMessageW(FORMAT_MESSAGE_FROM_SYSTEM | FORMAT_MESSAGE_IGNORE_INSERTS | FORMAT_MESSAGE_MAX_WIDTH_MASK, NULL, e,
		MAKELANGID(LANG_ENGLISH, SUBLANG_ENGLISH_US), w, 512, NULL);
	while (k > 0 && (w[k - 1] == L'\n' || w[k - 1] == L'\r' || w[k - 1] == L' ')) {
		k--;
	}
	int u = k == 0 ? -1 : xo_w_utf8(w, (int)k, buf, n - 1);
	if (u < 0) {
		snprintf(buf, n, "winapi error #%lu", (unsigned long)e);
		return;
	}
	buf[u] = 0;
}

// the status a process gets when it is killed: 128 plus SIGKILL's number,
// as on Unix (std.md 3.7)
#define PR_KILLED 137

// pr_reap reaps pid when it has exited (waiting when block): 1 and its
// exit code in *ws, else 0. The handle is closed once reaped.
static int pr_reap(pid_t pid, int *ws, int block) {
	HANDLE h = (HANDLE)pid;
	if (WaitForSingleObject(h, block ? INFINITE : 0) != WAIT_OBJECT_0) {
		return 0;
	}
	DWORD code = 0;
	GetExitCodeProcess(h, &code);
	*ws = (int)code;
	CloseHandle(h);
	return 1;
}

// pr_kill9 ends a started child (Child.kill, cancellation)
static void pr_kill9(pid_t pid) { TerminateProcess((HANDLE)pid, PR_KILLED); }

// pr_release forgets a child that is not reaped
static void pr_release(pid_t pid) { CloseHandle((HANDLE)pid); }

// pr_pipe2 makes a pipe not inherited by children (the handle list names
// what a child gets). nb -1: both ends are C runtime descriptors of the
// pipe (between two children, or to the input feeder); nb 0 or 1: that
// end is the program's, a nonblocking socket pumped to the pipe by a
// thread (xo_w_pump), and the other end is a descriptor of the pipe for
// the child.
static int pr_pipe2(int p[2], int nb) {
	p[0] = p[1] = -1;
	HANDLE r, w;
	if (!CreatePipe(&r, &w, NULL, 1 << 16)) {
		errno = xo_w_errno(GetLastError());
		return -1;
	}
	if (nb < 0) {
		p[0] = xo_w_fd(_open_osfhandle((intptr_t)r, _O_RDONLY | _O_BINARY));
		p[1] = xo_w_fd(_open_osfhandle((intptr_t)w, _O_WRONLY | _O_BINARY));
		return 0;
	}
	HANDLE child = nb == 0 ? w : r, pumped = nb == 0 ? r : w;
	int s = xo_w_pump(pumped, nb == 0, 1);
	if (s < 0) {
		CloseHandle(r);
		CloseHandle(w);
		errno = EMFILE;
		return -1;
	}
	p[1 - nb] = xo_w_fd(_open_osfhandle((intptr_t)child, (nb == 0 ? _O_WRONLY : _O_RDONLY) | _O_BINARY));
	p[nb] = s;
	return 0;
}
#else
#include <spawn.h>
#include <sys/stat.h>
#include <sys/wait.h>

extern char **environ;

#define pr_rd read
#define pr_wr write

// pr_reap reaps pid when it has exited (waiting when block): 1 and its
// wait status in *ws, else 0 (-1 on an error).
static int pr_reap(pid_t pid, int *ws, int block) {
	pid_t r = waitpid(pid, ws, block ? 0 : WNOHANG);
	if (r == pid) {
		return 1;
	}
	return r < 0 && errno != EINTR ? -1 : 0;
}

static void pr_kill9(pid_t pid) { kill(pid, SIGKILL); }

static void pr_release(pid_t pid) {
	int ws;
	waitpid(pid, &ws, WNOHANG);
}
#endif


// FakeProc.on_run's outputs are std/native/llrt/testworld.xo's (the
// zero-C audit's B14): Proc.run, Cmd.output and run, and pipelines in a
// test give them from Xo (the native prelude), and a fake child reads the
// entry testworld.xo keeps for its command until the test ends (words:
// the command, the status, the output, and the error, each text a block
// of its length and then the bytes), through cabi_tw.xo's export,
// which test binaries carry (the weak definition keeps other programs
// linking: they never run a fake).
__attribute__((weak)) int64_t xo_x_tw_proc_find(int64_t p, int64_t n) {
	(void)p, (void)n;
	return 0;
}

static xo_rlist *pr_outputs(int64_t code, const char *o, int64_t on, const char *e, int64_t en) {
	char t[32];
	int k = snprintf(t, sizeof t, "%lld", (long long)code);
	xo_rlist *l = n_str_list_new(4);
	l = n_str_list_add(l, "0", 1);
	l = n_str_list_add(l, t, k);
	l = n_str_list_add(l, o, on);
	return n_str_list_add(l, e, en);
}

static xo_rlist *pr_fail(char st, const char *msg) {
	xo_rlist *l = n_str_list_new(2);
	l = n_str_list_add(l, &st, 1);
	return n_str_list_add(l, msg, (int64_t)strlen(msg));
}

// pr_errmsg is the outcome of an error starting cmd, os/exec's message.
static xo_rlist *pr_errmsg(const char *cmd, int err) {
	char m[512];
	snprintf(m, sizeof m, "fork/exec %s: %s", cmd, strerror(err));
	// Go's errno texts start in lower case
	size_t k = strlen("fork/exec ") + strlen(cmd) + 2;
	if (k < sizeof m && m[k] >= 'A' && m[k] <= 'Z') {
		m[k] = (char)(m[k] + 32);
	}
	return pr_fail('2', m);
}

#ifdef _WIN32
// pr_has_slash reports whether a command names a path rather than a name
// looked up in PATH (exec.LookPath on Windows: any of \ / :)
static int pr_has_slash(const char *cmd) { return strpbrk(cmd, "/\\:") != NULL; }

// pr_w_exe reports whether path is an existing file (not a directory)
static int pr_w_exe(const char *path) {
	DWORD a = xo_w_attrs(path);
	return a != INVALID_FILE_ATTRIBUTES && !(a & FILE_ATTRIBUTE_DIRECTORY);
}

// pr_find_exe is exec's findExecutable on Windows: path itself when it
// has an extension of PATHEXT and exists, else path with each extension
// of PATHEXT in turn (malloc'd), or NULL.
static char *pr_find_exe(const char *path) {
	const char *pe = getenv("PATHEXT");
	if (!pe || !pe[0]) {
		pe = ".com;.exe;.bat;.cmd";
	}
	size_t pl = strlen(path);
	const char *dot = strrchr(path, '.');
	const char *sep = strpbrk(dot ? dot : "", "/\\");
	if (dot && !sep) {
		// an extension: as written, when it exists
		if (pr_w_exe(path)) {
			return cstr_of(path, (int64_t)pl);
		}
	}
	for (const char *p = pe; *p;) {
		const char *end = strchr(p, ';');
		size_t el = end ? (size_t)(end - p) : strlen(p);
		if (el > 0) {
			char *full = malloc(pl + el + 1);
			if (!full) {
				xo_oom();
			}
			memcpy(full, path, pl);
			for (size_t i = 0; i < el; i++) {
				char c = p[i];
				full[pl + i] = (char)(c >= 'A' && c <= 'Z' ? c + 32 : c);
			}
			full[pl + el] = 0;
			if (pr_w_exe(full)) {
				return full;
			}
			free(full);
		}
		if (!end) {
			break;
		}
		p = end + 1;
	}
	return NULL;
}

// pr_look_path is exec.LookPath on Windows: a command naming a path is
// found with PATHEXT's extensions; a name is looked up in each directory
// of PATH (; separated). The file (malloc'd), or NULL.
static char *pr_look_path(const char *cmd) {
	if (pr_has_slash(cmd)) {
		return pr_find_exe(cmd);
	}
	const char *path = getenv("PATH");
	if (!path) {
		return NULL;
	}
	size_t cl = strlen(cmd);
	for (const char *p = path;;) {
		const char *end = strchr(p, ';');
		size_t dl = end ? (size_t)(end - p) : strlen(p);
		if (dl >= 2 && p[0] == '"' && p[dl - 1] == '"') {
			p++;
			dl -= 2;
		}
		if (dl > 0) {
			char *full = malloc(dl + cl + 2);
			if (!full) {
				xo_oom();
			}
			memcpy(full, p, dl);
			full[dl] = '\\';
			memcpy(full + dl + 1, cmd, cl + 1);
			char *f = pr_find_exe(full);
			free(full);
			if (f) {
				return f;
			}
		}
		if (!end) {
			return NULL;
		}
		p = end + 1;
	}
}
#else
static int pr_has_slash(const char *cmd) { return strchr(cmd, '/') != NULL; }

// pr_look_path is exec.LookPath of a name without a slash: the first
// executable regular file of that name in PATH (malloc'd), or NULL.
static char *pr_look_path(const char *cmd) {
	const char *path = getenv("PATH");
	if (!path) {
		return NULL;
	}
	size_t cl = strlen(cmd);
	for (const char *p = path;;) {
		const char *end = strchr(p, ':');
		size_t dl = end ? (size_t)(end - p) : strlen(p);
		const char *dir = p;
		if (dl == 0) {
			dir = ".";
			dl = 1;
		}
		char *full = malloc(dl + cl + 2);
		if (!full) {
			xo_oom();
		}
		memcpy(full, dir, dl);
		full[dl] = '/';
		memcpy(full + dl + 1, cmd, cl + 1);
		struct stat sb;
		if (stat(full, &sb) == 0 && S_ISREG(sb.st_mode) && (sb.st_mode & 0111) && access(full, X_OK) == 0) {
			return full;
		}
		free(full);
		if (!end) {
			return NULL;
		}
		p = end + 1;
	}
}
#endif

// pr_drain reads what fd has now into b; 1 at the end of the output.
static int pr_drain(int fd, nbuf *b) {
	char t[65536];
	for (;;) {
		ssize_t n = pr_rd(fd, t, sizeof t);
		if (n > 0) {
			nbuf_put(b, t, (int64_t)n);
			continue;
		}
		if (n < 0 && errno == EINTR) {
			continue;
		}
		if (n < 0 && (errno == EAGAIN || errno == EWOULDBLOCK)) {
			return 0;
		}
		return 1;
	}
}

#ifndef _WIN32
static int pr_pipe(int p[2]) {
	if (xo_pipe(p) < 0) {
		return -1;
	}
	for (int i = 0; i < 2; i++) {
		fcntl(p[i], F_SETFD, FD_CLOEXEC);
	}
	fcntl(p[0], F_SETFL, fcntl(p[0], F_GETFL) | O_NONBLOCK);
	return 0;
}

// xo_proc_run runs cmd with args and returns its standard output and
// error (a list of two), or an empty list with the status set.
xo_rlist *xo_proc_run(const char *c, int64_t cn, xo_rlist *args) {
	xo_ctx *cx = ctx();
	if (ctx_canceled(cx)) {
		return pr_fail('3', "");
	}
	char *cmd = cstr_of(c, cn);
	char *file = cmd;
	if (!strchr(cmd, '/')) {
		file = pr_look_path(cmd);
		if (!file) {
			free(cmd);
			return pr_fail('1', "");
		}
	}
	int64_t na = args->len;
	char **argv = calloc((size_t)na + 2, sizeof(char *));
	if (!argv) {
		xo_oom();
	}
	argv[0] = cmd;
	for (int64_t i = 0; i < na; i++) {
		xo_rstr *s = ((xo_rstr **)rl_data(args))[2 * i];
		int64_t n = ((int64_t *)rl_data(args))[2 * i + 1];
		argv[i + 1] = cstr_of(s->data, n);
	}
	int po[2] = {-1, -1}, pe[2] = {-1, -1};
	int err = 0;
	pid_t pid = -1;
	if (pr_pipe(po) < 0 || pr_pipe(pe) < 0) {
		err = errno;
	} else {
		posix_spawn_file_actions_t fa;
		posix_spawnattr_t at;
		posix_spawn_file_actions_init(&fa);
		posix_spawn_file_actions_addopen(&fa, 0, "/dev/null", O_RDONLY, 0);
		posix_spawn_file_actions_adddup2(&fa, po[1], 1);
		posix_spawn_file_actions_adddup2(&fa, pe[1], 2);
		posix_spawnattr_init(&at);
		sigset_t none, def;
		sigemptyset(&none);
		sigemptyset(&def);
		sigaddset(&def, SIGPIPE);
		sigaddset(&def, SIGINT);
		sigaddset(&def, SIGTERM);
		short flags = POSIX_SPAWN_SETSIGMASK | POSIX_SPAWN_SETSIGDEF;
#ifdef POSIX_SPAWN_CLOEXEC_DEFAULT
		flags |= POSIX_SPAWN_CLOEXEC_DEFAULT;
#endif
		posix_spawnattr_setflags(&at, flags);
		posix_spawnattr_setsigmask(&at, &none);
		posix_spawnattr_setsigdefault(&at, &def);
		err = posix_spawn(&pid, file, &fa, &at, argv, environ);
		posix_spawn_file_actions_destroy(&fa);
		posix_spawnattr_destroy(&at);
	}
	for (int64_t i = 0; i <= na; i++) {
		if (argv[i] != cmd) {
			free(argv[i]);
		}
	}
	free(argv);
	if (po[1] >= 0) {
		close(po[1]);
	}
	if (pe[1] >= 0) {
		close(pe[1]);
	}
	if (err) {
		if (po[0] >= 0) {
			close(po[0]);
		}
		if (pe[0] >= 0) {
			close(pe[0]);
		}
		xo_rlist *l = pr_errmsg(file, err);
		if (file != cmd) {
			free(file);
		}
		free(cmd);
		return l;
	}
	if (file != cmd) {
		free(file);
	}
	free(cmd);
	nbuf out = {0}, errb = {0};
	xo_pd *pdo = pd_new(po[0], 0), *pde = pd_new(pe[0], 0);
	int fds[2] = {po[0], pe[0]};
	xo_pd *pds[2] = {pdo, pde};
	nbuf *bufs[2] = {&out, &errb};
	int open[2] = {1, 1};
	int canceled = 0;
	while (open[0] || open[1]) {
		if (ctx_canceled(cx)) {
			canceled = 1;
			break;
		}
		int got = 0;
		for (int i = 0; i < 2; i++) {
			if (open[i]) {
				int64_t before = bufs[i]->n;
				if (pr_drain(fds[i], bufs[i])) {
					open[i] = 0;
				}
				got |= bufs[i]->n != before || !open[i];
			}
		}
		if (got || !(open[0] || open[1])) {
			continue;
		}
		flush_pending();
		// wait on one pipe, in short steps while the other is open too
		int i = open[0] ? 0 : 1;
		if (pds[i]) {
			pd_wait_until(pds[i], 0, cx, open[1 - i] ? deadline(10000000) : 0);
		} else if (clock_wait(cx, 1000000)) {
			canceled = 1;
			break;
		}
	}
	if (canceled) {
		kill(pid, SIGKILL);
	}
	for (int i = 0; i < 2; i++) {
		if (pds[i]) {
			pd_free(pds[i]);
		} else {
			close(fds[i]);
		}
	}
	int ws = 0;
	for (int64_t step = 1000000;;) {
		pid_t r = waitpid(pid, &ws, WNOHANG);
		if (r == pid || (r < 0 && errno != EINTR)) {
			break;
		}
		if (r == 0) {
			if (!canceled && ctx_canceled(cx)) {
				canceled = 1;
				kill(pid, SIGKILL);
			}
			struct timespec ts = {0, (long)step};
			if (canceled) {
				nanosleep(&ts, NULL);
			} else {
				clock_wait(cx, step);
			}
			if (step < 50000000) {
				step *= 2;
			}
		}
	}
	if (canceled) {
		free(out.p);
		free(errb.p);
		return pr_fail('3', "");
	}
	int64_t code = WIFEXITED(ws) ? WEXITSTATUS(ws) : -1;
	xo_rlist *l = pr_outputs(code, out.p ? out.p : "", out.n, errb.p ? errb.p : "", errb.n);
	free(out.p);
	free(errb.p);
	return l;
}
#endif

// ---------------------------------------------------------------------------
// Commands, children, and pipelines (std.md 3.8, decisions 0161 and 0172),
// as std/rt proccmd.go's realProc and fakeProc. std/native/prelude.xo
// checks the attenuation and builds the specs; the functions here start
// the commands with posix_spawn (file actions for the directory and the
// standard streams), wait on the event loop's poller for their pipes, and
// poll waitpid. Standard input data goes through a pipe written by a
// thread of its own, as os/exec copies it from a goroutine.

#ifdef __linux__
// glibc 2.29 and later (the Linux runtime pack needs 2.31)
extern int posix_spawn_file_actions_addchdir_np(posix_spawn_file_actions_t *, const char *);
#endif

// PR_INHERIT as a descriptor of pr_start: the program's own descriptor of
// that number; PR_DUP_OUT as standard error: a copy of standard output.
enum { PR_NULL = -1, PR_INHERIT = -2, PR_DUP_OUT = -3 };

// pr_cmd is a command of a spec (std/native/rt.xo proc_exec).
typedef struct {
	char *name;
	char *dir; // NULL: the program's
	int flags; // 1 env set, 2 inherit, 4 merge_stderr
	char **argv;
	char **envp;   // NULL: the program's environment
	char *cmdline; // Windows: the command line std/native/llrt/procspec.xo built
} pr_cmd;

static char *pr_lstr(xo_rlist *l, int64_t i) {
	if (i >= l->len) {
		return cstr_of("", 0);
	}
	xo_rstr *s = ((xo_rstr **)rl_data(l))[2 * i];
	int64_t n = ((int64_t *)rl_data(l))[2 * i + 1];
	return cstr_of(s->data, n);
}

static int64_t pr_lint(xo_rlist *l, int64_t i) {
	char *s = pr_lstr(l, i);
	int64_t v = strtoll(s, NULL, 10);
	free(s);
	return v < 0 ? 0 : v;
}

// pr_parse reads the command at *i of the spec l.
static void pr_parse(xo_rlist *l, int64_t *i, pr_cmd *c) {
	c->name = pr_lstr(l, (*i)++);
	c->dir = pr_lstr(l, (*i)++);
	if (!c->dir[0]) {
		free(c->dir);
		c->dir = NULL;
	}
	c->flags = (int)pr_lint(l, (*i)++);
	int64_t na = pr_lint(l, (*i)++);
	c->argv = calloc((size_t)na + 2, sizeof(char *));
	if (!c->argv) {
		xo_oom();
	}
	c->argv[0] = cstr_of(c->name, (int64_t)strlen(c->name));
	for (int64_t k = 0; k < na; k++) {
		c->argv[k + 1] = pr_lstr(l, (*i)++);
	}
	int64_t ne = pr_lint(l, (*i)++);
	int64_t e0 = *i;
	*i += ne;
	// the variables added: none, procspec.xo merged them
	*i += pr_lint(l, *i) + 1;
	int env_set = c->flags & 1;
	c->envp = NULL;
	// built by std/native/llrt/procspec.xo (decision 0193 step 6; flag 8):
	// the environment is final (flag 1: there is one, else the program's),
	// the Windows command line follows
	c->cmdline = pr_lstr(l, (*i)++);
	if (env_set) {
		c->envp = calloc((size_t)ne + 1, sizeof(char *));
		if (!c->envp) {
			xo_oom();
		}
		for (int64_t j = 0; j < ne; j++) {
			c->envp[j] = pr_lstr(l, e0 + j);
		}
	}
}

static void pr_cmd_free(pr_cmd *c) {
	free(c->name);
	free(c->dir);
	free(c->cmdline);
	if (c->argv) {
		for (char **a = c->argv; *a; a++) {
			free(*a);
		}
		free(c->argv);
	}
	if (c->envp) {
		for (char **e = c->envp; *e; e++) {
			free(*e);
		}
		free(c->envp);
	}
}

// pr_pipe2 makes a pipe whose ends close on exec; the end nb (0 or 1, -1
// neither) is nonblocking.
#ifndef _WIN32
static int pr_pipe2(int p[2], int nb) {
	if (xo_pipe(p) < 0) {
		p[0] = p[1] = -1;
		return -1;
	}
	for (int i = 0; i < 2; i++) {
		fcntl(p[i], F_SETFD, FD_CLOEXEC);
	}
	if (nb >= 0) {
		fcntl(p[nb], F_SETFL, fcntl(p[nb], F_GETFL) | O_NONBLOCK);
	}
	return 0;
}
#endif

static void pr_close(int *fd) {
	if (*fd >= 0) {
		close(*fd);
		*fd = -1;
	}
}

// pr_start starts c with standard input, output, and error on the
// descriptors in, out, and err (PR_NULL /dev/null, PR_INHERIT the
// program's, PR_DUP_OUT error as output): 0 started, else the status of
// an outcome ('1' not found, '2' an error) with *msg its detail
// (malloc'd).
#ifdef _WIN32
static char pr_w_spawn(pr_cmd *c, const char *file, const int *fds, pid_t *pid, char **msg);
#endif

static char pr_start(pr_cmd *c, int in, int out, int err, pid_t *pid, char **msg) {
	char *file = c->name;
#ifdef _WIN32
	file = pr_look_path(c->name); // PATHEXT's extensions apply to paths too
	if (!file) {
		*msg = cstr_of(c->name, (int64_t)strlen(c->name));
		return '1';
	}
#else
	if (!strchr(c->name, '/')) {
		file = pr_look_path(c->name);
		if (!file) {
			*msg = cstr_of(c->name, (int64_t)strlen(c->name));
			return '1';
		}
	}
#endif
	if (c->dir) {
		// os.StartProcess checks the directory first
		struct stat sb;
		if (stat(c->dir, &sb) != 0) {
			int e = errno;
			if (file != c->name) {
				free(file);
			}
			if (e == ENOENT) {
				*msg = cstr_of(c->name, (int64_t)strlen(c->name));
				return '1';
			}
			char m[1024];
			snprintf(m, sizeof m, "chdir %s: %s", c->dir, strerror(e));
			size_t k = strlen("chdir ") + strlen(c->dir) + 2;
			if (k < sizeof m && m[k] >= 'A' && m[k] <= 'Z') {
				m[k] = (char)(m[k] + 32);
			}
			*msg = cstr_of(m, (int64_t)strlen(m));
			return '2';
		}
	}
	if (out == PR_INHERIT || err == PR_INHERIT || err == PR_DUP_OUT) {
		xo_flush(); // the program's output so far comes first
	}
#ifdef _WIN32
	int wfds[3] = {in, out, err};
	char wst = pr_w_spawn(c, file, wfds, pid, msg);
	if (file != c->name) {
		free(file);
	}
	return wst;
#else
	posix_spawn_file_actions_t fa;
	posix_spawnattr_t at;
	posix_spawn_file_actions_init(&fa);
	int fds[3] = {in, out, err};
	for (int k = 0; k < 3; k++) {
		int fd = fds[k];
		if (fd == PR_NULL) {
			posix_spawn_file_actions_addopen(&fa, k, "/dev/null", k == 0 ? O_RDONLY : O_WRONLY, 0);
		} else if (fd == PR_INHERIT) {
#ifdef __APPLE__
			posix_spawn_file_actions_addinherit_np(&fa, k);
#endif
		} else if (fd == PR_DUP_OUT) {
			posix_spawn_file_actions_adddup2(&fa, 1, 2);
		} else {
			posix_spawn_file_actions_adddup2(&fa, fd, k);
		}
	}
	if (c->dir) {
		posix_spawn_file_actions_addchdir_np(&fa, c->dir);
	}
	posix_spawnattr_init(&at);
	sigset_t none, def;
	sigemptyset(&none);
	sigemptyset(&def);
	sigaddset(&def, SIGPIPE);
	sigaddset(&def, SIGINT);
	sigaddset(&def, SIGTERM);
	short flags = POSIX_SPAWN_SETSIGMASK | POSIX_SPAWN_SETSIGDEF;
#ifdef POSIX_SPAWN_CLOEXEC_DEFAULT
	flags |= POSIX_SPAWN_CLOEXEC_DEFAULT;
#endif
	posix_spawnattr_setflags(&at, flags);
	posix_spawnattr_setsigmask(&at, &none);
	posix_spawnattr_setsigdefault(&at, &def);
	int e = posix_spawn(pid, file, &fa, &at, c->argv, c->envp ? c->envp : environ);
	posix_spawn_file_actions_destroy(&fa);
	posix_spawnattr_destroy(&at);
	char st = '0';
	if (e == ENOENT) {
		*msg = cstr_of(c->name, (int64_t)strlen(c->name));
		st = '1';
	} else if (e != 0) {
		char m[1024];
		snprintf(m, sizeof m, "fork/exec %s: %s", file, strerror(e));
		size_t k = strlen("fork/exec ") + strlen(file) + 2;
		if (k < sizeof m && m[k] >= 'A' && m[k] <= 'Z') {
			m[k] = (char)(m[k] + 32);
		}
		*msg = cstr_of(m, (int64_t)strlen(m));
		st = '2';
	}
	if (file != c->name) {
		free(file);
	}
	return st;
#endif
}

#ifdef _WIN32
// pr_w_inheritable is an inheritable copy of h (NULL when h is not a
// handle)
static HANDLE pr_w_inheritable(HANDLE h) {
	HANDLE d = NULL;
	if (h == NULL || h == INVALID_HANDLE_VALUE ||
		!DuplicateHandle(GetCurrentProcess(), h, GetCurrentProcess(), &d, 0, TRUE, DUPLICATE_SAME_ACCESS)) {
		return NULL;
	}
	return d;
}

// pr_w_null opens NUL for a child's standard stream k, inheritable
static HANDLE pr_w_null(int k) {
	SECURITY_ATTRIBUTES sa = {sizeof sa, NULL, TRUE};
	HANDLE h = CreateFileW(L"NUL", k == 0 ? GENERIC_READ : GENERIC_WRITE, FILE_SHARE_READ | FILE_SHARE_WRITE, &sa,
		OPEN_EXISTING, 0, NULL);
	return h == INVALID_HANDLE_VALUE ? NULL : h;
}

// pr_w_spawn starts file with c's arguments, environment, and directory
// and the standard streams fds (pr_start's): CreateProcessW with exactly
// those three handles inherited.
static char pr_w_spawn(pr_cmd *c, const char *file, const int *fds, pid_t *pid, char **msg) {
	static const DWORD stds[3] = {STD_INPUT_HANDLE, STD_OUTPUT_HANDLE, STD_ERROR_HANDLE};
	HANDLE hs[3] = {NULL, NULL, NULL};
	for (int k = 0; k < 3; k++) {
		int fd = fds[k];
		if (fd == PR_DUP_OUT) {
			hs[k] = pr_w_inheritable(hs[1]);
		} else if (fd == PR_INHERIT) {
			hs[k] = pr_w_inheritable(GetStdHandle(stds[k]));
		} else if (fd >= 0) {
			hs[k] = pr_w_inheritable((HANDLE)_get_osfhandle(xo_w_raw(fd)));
		}
		if (!hs[k]) {
			hs[k] = pr_w_null(k); // PR_NULL, or no such stream
		}
	}
	nbuf cl = {0};
	nbuf_str(&cl, c->cmdline); // procspec.xo's
	nbuf_byte(&cl, 0);
	wchar_t *wcl = xo_w_path(cl.p);
	wchar_t *wfile = xo_w_path(file);
	free(cl.p);
	wchar_t *wdir = NULL;
	if (c->dir) {
		wchar_t *rel = xo_w_path(c->dir);
		if (rel) {
			DWORD n = GetFullPathNameW(rel, 0, NULL, NULL);
			wdir = n ? malloc((size_t)n * sizeof(wchar_t)) : NULL;
			if (wdir && !GetFullPathNameW(rel, n, wdir, NULL)) {
				free(wdir);
				wdir = NULL;
			}
			free(rel);
		}
	}
	wchar_t *wenv = NULL;
	if (c->envp) {
		// NAME=VALUE\0 ... \0, UTF-16
		size_t n = 0, cap = 256;
		wenv = malloc(cap * sizeof(wchar_t));
		if (!wenv) {
			xo_oom();
		}
		for (char **e = c->envp; *e; e++) {
			int k = MultiByteToWideChar(CP_UTF8, 0, *e, -1, NULL, 0);
			if (k <= 1) {
				continue;
			}
			while (n + (size_t)k + 2 > cap) {
				cap *= 2;
				wenv = realloc(wenv, cap * sizeof(wchar_t));
				if (!wenv) {
					xo_oom();
				}
			}
			MultiByteToWideChar(CP_UTF8, 0, *e, -1, wenv + n, k);
			n += (size_t)k;
		}
		wenv[n++] = 0;
		if (n == 1) {
			wenv[n++] = 0; // an empty block is two terminators
		}
	}
	// the handle list: each handle once
	HANDLE list[3];
	int nl = 0;
	for (int k = 0; k < 3; k++) {
		int dup = 0;
		for (int j = 0; j < nl; j++) {
			dup |= list[j] == hs[k];
		}
		if (hs[k] && !dup) {
			list[nl++] = hs[k];
		}
	}
	SIZE_T asz = 0;
	InitializeProcThreadAttributeList(NULL, 1, 0, &asz);
	LPPROC_THREAD_ATTRIBUTE_LIST al = malloc(asz);
	if (!al) {
		xo_oom();
	}
	BOOL ok = InitializeProcThreadAttributeList(al, 1, 0, &asz) &&
		(nl == 0 || UpdateProcThreadAttribute(al, 0, PROC_THREAD_ATTRIBUTE_HANDLE_LIST, list, (SIZE_T)nl * sizeof(HANDLE), NULL, NULL));
	STARTUPINFOEXW si;
	memset(&si, 0, sizeof si);
	si.StartupInfo.cb = sizeof si;
	si.StartupInfo.dwFlags = STARTF_USESTDHANDLES;
	si.StartupInfo.hStdInput = hs[0];
	si.StartupInfo.hStdOutput = hs[1];
	si.StartupInfo.hStdError = hs[2];
	si.lpAttributeList = al;
	PROCESS_INFORMATION pi;
	memset(&pi, 0, sizeof pi);
	DWORD e = 0;
	if (!ok || !wcl || !wfile) {
		e = ok ? ERROR_INVALID_PARAMETER : GetLastError();
	} else if (!CreateProcessW(wfile, wcl, NULL, NULL, nl > 0, CREATE_UNICODE_ENVIRONMENT | EXTENDED_STARTUPINFO_PRESENT, wenv,
		wdir, &si.StartupInfo, &pi)) {
		e = GetLastError();
	}
	DeleteProcThreadAttributeList(al);
	free(al);
	free(wcl);
	free(wfile);
	free(wdir);
	free(wenv);
	for (int k = 0; k < 3; k++) {
		if (hs[k]) {
			CloseHandle(hs[k]);
		}
	}
	if (e == 0) {
		CloseHandle(pi.hThread);
		*pid = (pid_t)pi.hProcess;
		return '0';
	}
	if (e == ERROR_FILE_NOT_FOUND || e == ERROR_PATH_NOT_FOUND) {
		*msg = cstr_of(c->name, (int64_t)strlen(c->name));
		return '1';
	}
	char t[512], m[1024];
	pr_w_text(e, t, sizeof t);
	snprintf(m, sizeof m, "fork/exec %s: %s", file, t);
	*msg = cstr_of(m, (int64_t)strlen(m));
	return '2';
}
#endif

// pr_code is the status of a finished command: its exit code, or 128
// plus the signal that ended it.
static int64_t pr_code(int ws) {
#ifdef _WIN32
	return (int64_t)(uint32_t)ws; // the exit code (os.ProcessState.ExitCode)
#else
	if (WIFEXITED(ws)) {
		return WEXITSTATUS(ws);
	}
	if (WIFSIGNALED(ws)) {
		return 128 + WTERMSIG(ws);
	}
	return -1;
#endif
}

// pr_wait waits for pid, killing it when the task is canceled (*canceled
// tells): its wait status.
static int pr_wait(xo_ctx *cx, pid_t pid, int *canceled) {
	int ws = 0;
	for (int64_t step = 1000000;;) {
		int r = pr_reap(pid, &ws, 0);
		if (r != 0) {
			return ws;
		}
		if (r == 0) {
			if (!*canceled && ctx_canceled(cx)) {
				*canceled = 1;
				pr_kill9(pid);
			}
			if (*canceled) {
				struct timespec ts = {0, (long)step};
				nanosleep(&ts, NULL);
			} else {
				flush_pending();
				clock_wait(cx, step);
			}
			if (step < 50000000) {
				step *= 2;
			}
		}
	}
}

// pr_feed writes a command's standard input data from a thread of its
// own and closes the pipe (os/exec's goroutine).
typedef struct {
	int fd;
	char *p;
	int64_t n;
} pr_feeder;

static void *pr_feed_main(void *a) {
	pr_feeder *f = a;
	write_all(f->fd, f->p, (size_t)f->n);
	close(f->fd);
	free(f->p);
	free(f);
	return NULL;
}

static void pr_feed(int fd, const char *p, int64_t n) {
	pr_feeder *f = malloc(sizeof *f);
	char *copy = malloc((size_t)n + 1);
	if (!f || !copy) {
		xo_oom();
	}
	memcpy(copy, p, (size_t)n);
	*f = (pr_feeder){fd, copy, n};
	pthread_t t;
	pthread_attr_t at;
	pthread_attr_init(&at);
	pthread_attr_setdetachstate(&at, PTHREAD_CREATE_DETACHED);
	if (pthread_create(&t, &at, pr_feed_main, f) != 0) {
		pr_feed_main(f);
	}
	pthread_attr_destroy(&at);
}

// pr_collect reads the descriptors fds (nonblocking) into bufs until each
// ends: 1 when the task was canceled first.
static int pr_collect(xo_ctx *cx, int *fds, nbuf **bufs, int n) {
	xo_pd **pds = calloc((size_t)n + 1, sizeof(xo_pd *));
	int *open = calloc((size_t)n + 1, sizeof(int));
	if (!pds || !open) {
		xo_oom();
	}
	int left = 0;
	for (int i = 0; i < n; i++) {
		pds[i] = pd_new(fds[i], 0);
		open[i] = 1;
		left++;
	}
	int canceled = 0;
	while (left > 0) {
		if (ctx_canceled(cx)) {
			canceled = 1;
			break;
		}
		int got = 0;
		for (int i = 0; i < n; i++) {
			if (open[i]) {
				int64_t before = bufs[i]->n;
				if (pr_drain(fds[i], bufs[i])) {
					open[i] = 0;
					left--;
				}
				got |= bufs[i]->n != before || !open[i];
			}
		}
		if (got || left == 0) {
			continue;
		}
		flush_pending();
		// wait on one, in short steps while another is open too
		int w = 0;
		while (!open[w]) {
			w++;
		}
		if (pds[w]) {
			pd_wait_until(pds[w], 0, cx, left > 1 ? deadline(10000000) : 0);
		} else if (clock_wait(cx, 1000000)) {
			canceled = 1;
			break;
		}
	}
	for (int i = 0; i < n; i++) {
		if (pds[i]) {
			pd_free(pds[i]);
		} else {
			close(fds[i]);
		}
	}
	free(pds);
	free(open);
	return canceled;
}

#ifdef _WIN32
// pr_w_quote appends s quoted as procspec.xo's quote (Go's
// syscall.EscapeArg, and also when s holds a line end or another space).
static void pr_w_quote(nbuf *b, const char *s) {
	size_t n = strlen(s);
	if (n == 0) {
		nbuf_str(b, "\"\"");
		return;
	}
	int bs = 0, sp = 0;
	for (size_t i = 0; i < n; i++) {
		unsigned char c = (unsigned char)s[i];
		bs |= c == '"' || c == '\\';
		sp |= c == ' ' || (c >= 9 && c <= 13);
	}
	if (!bs && !sp) {
		nbuf_str(b, s);
		return;
	}
	if (!bs) {
		nbuf_byte(b, '"');
		nbuf_str(b, s);
		nbuf_byte(b, '"');
		return;
	}
	if (sp) {
		nbuf_byte(b, '"');
	}
	int slashes = 0;
	for (size_t i = 0; i < n; i++) {
		char c = s[i];
		if (c == '\\') {
			slashes++;
		} else if (c == '"') {
			for (; slashes > 0; slashes--) {
				nbuf_byte(b, '\\');
			}
			nbuf_byte(b, '\\');
		} else {
			slashes = 0;
		}
		nbuf_byte(b, c);
	}
	if (sp) {
		for (; slashes > 0; slashes--) {
			nbuf_byte(b, '\\');
		}
		nbuf_byte(b, '"');
	}
}

// pr_w_cmdline is the command line of argv (NULL ended) as procspec.xo
// builds Proc.cmd's: each argument quoted, joined by spaces. Proc.run
// has no spec, so it is built here; without it pr_w_spawn read a NULL
// command line (net_only's Proc.run faulted, 2026-10-14, gotcha
// windows-proc-run-null-cmdline).
static char *pr_w_cmdline(char **argv) {
	nbuf b = {0};
	for (int i = 0; argv[i]; i++) {
		if (i > 0) {
			nbuf_byte(&b, ' ');
		}
		pr_w_quote(&b, argv[i]);
	}
	nbuf_byte(&b, 0);
	return b.p;
}

// xo_proc_run on Windows: the command with its standard input NUL and
// its output and error collected, through pr_start (as xo_proc_exec's
// Cmd.output).
xo_rlist *xo_proc_run(const char *c, int64_t cn, xo_rlist *args) {
	xo_ctx *cx = ctx();
	if (ctx_canceled(cx)) {
		return pr_fail('3', "");
	}
	char *cmd = cstr_of(c, cn);
	pr_cmd pc;
	memset(&pc, 0, sizeof pc);
	pc.name = cmd;
	int64_t na = args->len;
	pc.argv = calloc((size_t)na + 2, sizeof(char *));
	if (!pc.argv) {
		xo_oom();
	}
	pc.argv[0] = cstr_of(cmd, (int64_t)strlen(cmd));
	for (int64_t i = 0; i < na; i++) {
		pc.argv[i + 1] = pr_lstr(args, i);
	}
	pc.cmdline = pr_w_cmdline(pc.argv);
	int o[2] = {-1, -1}, e[2] = {-1, -1};
	char st = '2';
	char *msg = NULL;
	pid_t pid = 0;
	if (pr_pipe2(o, 0) < 0 || pr_pipe2(e, 0) < 0) {
		msg = cstr_of(strerror(errno), (int64_t)strlen(strerror(errno)));
	} else {
		st = pr_start(&pc, PR_NULL, o[1], e[1], &pid, &msg);
	}
	pr_close(&o[1]);
	pr_close(&e[1]);
	if (st != '0') {
		pr_close(&o[0]);
		pr_close(&e[0]);
		pr_cmd_free(&pc);
		// Proc.run's NotFound carries no name (the prelude adds it)
		xo_rlist *l = pr_fail(st, st == '1' ? "" : (msg ? msg : ""));
		free(msg);
		return l;
	}
	pr_cmd_free(&pc);
	nbuf out = {0}, errb = {0};
	int rd[2] = {o[0], e[0]};
	nbuf *bufs[2] = {&out, &errb};
	int canceled = pr_collect(cx, rd, bufs, 2);
	if (canceled) {
		pr_kill9(pid);
	}
	int ws = pr_wait(cx, pid, &canceled);
	if (canceled) {
		free(out.p);
		free(errb.p);
		return pr_fail('3', "");
	}
	xo_rlist *l = pr_outputs(pr_code(ws), out.p ? out.p : "", out.n, errb.p ? errb.p : "", errb.n);
	free(out.p);
	free(errb.p);
	return l;
}
#endif

// pr_specs parses every command of specs (*n of them, malloc'd).
static pr_cmd *pr_specs(xo_rlist *specs, int *n) {
	pr_cmd *cs = NULL;
	int k = 0;
	for (int64_t i = 0; i < specs->len;) {
		cs = tw_grow(cs, k, sizeof(pr_cmd));
		memset(&cs[k], 0, sizeof(pr_cmd));
		pr_parse(specs, &i, &cs[k]);
		k++;
	}
	*n = k;
	return cs;
}

static void pr_specs_free(pr_cmd *cs, int n) {
	for (int i = 0; i < n; i++) {
		pr_cmd_free(&cs[i]);
	}
	free(cs);
}

// xo_proc_exec runs the commands of specs: mode 0 one command with its
// output collected (Cmd.output), 1 one command on the terminal (Cmd.run),
// 2 a pipeline (Proc.pipeline) whose last output and every error stream
// are collected; input is the first command's standard input when
// has_in. The outcome: "0", the status (pipefail: the rightmost
// failure), standard output, standard error.
xo_rlist *xo_proc_exec(xo_rlist *specs, const char *in, int64_t inn, int64_t has_in, int64_t mode) {
	xo_ctx *cx = ctx();
	if (ctx_canceled(cx)) {
		return pr_fail('3', "");
	}
	int n = 0;
	pr_cmd *cs = pr_specs(specs, &n);
	if (n == 0) {
		free(cs);
		return pr_outputs(0, "", 0, "", 0);
	}
	int total = n;
	if (mode != 2) {
		n = 1;
	}
	pid_t *pids = calloc((size_t)n, sizeof(pid_t));
	int *rd = calloc((size_t)n + 1, sizeof(int)); // the read ends to collect
	nbuf **bufs = calloc((size_t)n + 1, sizeof(nbuf *));
	nbuf *errb = calloc((size_t)n, sizeof(nbuf));
	nbuf out = {0};
	if (!pids || !rd || !bufs || !errb) {
		xo_oom();
	}
	int nrd = 0;
	int feed[2] = {-1, -1};
	int prev = PR_NULL; // the next command's standard input
	if (has_in) {
		if (pr_pipe2(feed, -1) == 0) {
			prev = feed[0];
		}
	} else if (mode == 1 || (mode == 0 && (cs[0].flags & 2))) {
		prev = PR_INHERIT;
	}
	int started = 0;
	char st = '0';
	char *msg = NULL;
	for (int i = 0; i < n; i++) {
		int merge = cs[i].flags & 4;
		int o[2] = {-1, -1}, e[2] = {-1, -1};
		int ofd, efd;
		if (mode == 1) {
			ofd = PR_INHERIT;
			efd = merge ? PR_DUP_OUT : PR_INHERIT;
		} else {
			if (pr_pipe2(o, i == n - 1 ? 0 : -1) < 0) {
				st = '2';
				msg = cstr_of(strerror(errno), (int64_t)strlen(strerror(errno)));
				pr_close(&prev);
				break;
			}
			ofd = o[1];
			efd = o[1];
			if (!merge && pr_pipe2(e, 0) == 0) {
				efd = e[1];
			}
		}
		st = pr_start(&cs[i], prev, ofd, efd, &pids[i], &msg);
		if (prev >= 0) {
			close(prev);
		}
		prev = PR_NULL;
		pr_close(&o[1]);
		pr_close(&e[1]);
		if (st != '0') {
			pr_close(&o[0]);
			pr_close(&e[0]);
			break;
		}
		started++;
		if (e[0] >= 0) {
			bufs[nrd] = &errb[i];
			rd[nrd++] = e[0];
		}
		if (o[0] >= 0) {
			if (i == n - 1) {
				bufs[nrd] = &out;
				rd[nrd++] = o[0];
			} else {
				prev = o[0];
			}
		}
	}
	pr_close(&prev);
	if (feed[1] >= 0) {
		if (started > 0) {
			pr_feed(feed[1], in, inn);
		} else {
			close(feed[1]);
		}
	}
	int canceled = 0;
	if (st != '0') {
		// a command did not start: the ones before it are killed
		for (int i = 0; i < started; i++) {
			pr_kill9(pids[i]);
		}
	}
	if (nrd > 0) {
		canceled = pr_collect(cx, rd, bufs, nrd);
	}
	int64_t code = 0;
	for (int i = 0; i < started; i++) {
		if (canceled) {
			pr_kill9(pids[i]);
		}
		int64_t c = pr_code(pr_wait(cx, pids[i], &canceled));
		if (c != 0) {
			code = c;
		}
	}
	xo_rlist *l;
	if (st != '0') {
		l = pr_fail(st, msg ? msg : "");
	} else if (canceled) {
		l = pr_fail('3', "");
	} else {
		nbuf eb = {0};
		for (int i = 0; i < n; i++) {
			nbuf_put(&eb, errb[i].p, errb[i].n);
		}
		l = pr_outputs(code, out.p ? out.p : "", out.n, eb.p ? eb.p : "", eb.n);
		free(eb.p);
	}
	free(msg);
	for (int i = 0; i < n; i++) {
		free(errb[i].p);
	}
	free(out.p);
	free(errb);
	free(bufs);
	free(rd);
	free(pids);
	pr_specs_free(cs, total);
	return l;
}

// ---------------------------------------------------------------------------
// Children (Cmd.spawn)

typedef struct {
	xo_hdr h;
	pid_t pid;
	char st; // the start: '0' started, '1' not found, '2' an error, '3' canceled
	char *msg;
	int in_fd; // its input's pipe (the write end), -1 when not a pipe
	int in_closed;
	xo_pd *in_pd;
	int fd[2]; // the read ends of its output and error stream, -1 when not pipes
	xo_pd *pd[2];
	nbuf ahead; // output read ahead by read_line, from ahead_at
	int64_t ahead_at;
	int eof;
	int reaped;
	int64_t code;
	int fake; // FakeProc: fbuf holds the registered output and errors
	nbuf fbuf[2];
	int64_t fat[2];
} xo_child;

static void child_free(void *p) {
	xo_child *ch = p;
	if (ch->in_pd) {
		pd_free(ch->in_pd);
	} else {
		pr_close(&ch->in_fd);
	}
	for (int i = 0; i < 2; i++) {
		if (ch->pd[i]) {
			pd_free(ch->pd[i]);
		} else {
			pr_close(&ch->fd[i]);
		}
		free(ch->fbuf[i].p);
	}
	if (!ch->fake && ch->st == '0' && !ch->reaped) {
		pr_release(ch->pid);
	}
	free(ch->ahead.p);
	free(ch->msg);
	xo_free(ch);
}

// xo_proc_spawn starts the command of spec (Cmd.spawn): its input is the
// data when has_in, else the program's with inherit, else a pipe; its
// output and error stream are pipes, or the program's with inherit.
xo_child *xo_proc_spawn(xo_rlist *spec, const char *in, int64_t inn, int64_t has_in) {
	xo_child *ch = xo_alloc(sizeof(xo_child));
	memset(ch, 0, sizeof *ch);
	h_init(ch, child_free);
	ch->in_fd = ch->fd[0] = ch->fd[1] = -1;
	ch->st = '0';
	xo_ctx *cx = ctx();
	if (ctx_canceled(cx)) {
		ch->st = '3';
		return ch;
	}
	int n = 0;
	pr_cmd *cs = pr_specs(spec, &n);
	if (n == 0) {
		free(cs);
		ch->st = '2';
		ch->msg = cstr_of("no command", 10);
		return ch;
	}
	pr_cmd *c = &cs[0];
	int inherit = c->flags & 2, merge = c->flags & 4;
	if (tw_is_fake(TW_REAL_PROC)) {
		const int64_t *f = (const int64_t *)(intptr_t)xo_x_tw_proc_find((int64_t)(intptr_t)c->name, (int64_t)strlen(c->name));
		if (!f) {
			ch->st = '1';
			ch->msg = cstr_of(c->name, (int64_t)strlen(c->name));
		} else {
			const int64_t *o = (const int64_t *)(intptr_t)f[2], *e = (const int64_t *)(intptr_t)f[3];
			ch->fake = 1;
			ch->code = f[1];
			nbuf_put(&ch->fbuf[0], (const char *)(o + 1), o[0]);
			nbuf_put(&ch->fbuf[merge ? 0 : 1], (const char *)(e + 1), e[0]);
		}
		pr_specs_free(cs, n);
		return ch;
	}
	int fi[2] = {-1, -1}, pi[2] = {-1, -1}, o[2] = {-1, -1}, e[2] = {-1, -1};
	int ifd = PR_NULL, ofd = PR_INHERIT, efd = merge ? PR_DUP_OUT : PR_INHERIT;
	if (has_in) {
		if (pr_pipe2(fi, -1) == 0) {
			ifd = fi[0];
		}
	} else if (inherit) {
		ifd = PR_INHERIT;
	} else if (pr_pipe2(pi, 1) == 0) {
		ifd = pi[0];
	}
	if (!inherit) {
		if (pr_pipe2(o, 0) == 0) {
			ofd = efd = o[1];
		}
		if (!merge && pr_pipe2(e, 0) == 0) {
			efd = e[1];
		}
	}
	ch->st = pr_start(c, ifd, ofd, efd, &ch->pid, &ch->msg);
	pr_close(&fi[0]);
	pr_close(&pi[0]);
	pr_close(&o[1]);
	pr_close(&e[1]);
	pr_specs_free(cs, n);
	if (ch->st != '0') {
		pr_close(&fi[1]);
		pr_close(&pi[1]);
		pr_close(&o[0]);
		pr_close(&e[0]);
		return ch;
	}
	if (fi[1] >= 0) {
		pr_feed(fi[1], in, inn);
	}
	// writes to its input poll (a pipe's write end is not registered
	// with the poller)
	ch->in_fd = pi[1];
	ch->fd[0] = o[0];
	ch->fd[1] = e[0];
	for (int i = 0; i < 2; i++) {
		if (ch->fd[i] >= 0) {
			ch->pd[i] = pd_new(ch->fd[i], 0);
		}
	}
	return ch;
}

// xo_proc_child_start is how the start went: "0", or the failure.
xo_rlist *xo_proc_child_start(xo_child *ch) {
	if (ch->st == '0') {
		return pr_fail('0', "");
	}
	return pr_fail(ch->st, ch->msg ? ch->msg : "");
}

#ifdef _WIN32
int64_t xo_proc_child_pid(xo_child *ch) { return ch->fake || ch->st != '0' ? 0 : (int64_t)GetProcessId((HANDLE)ch->pid); }
#else
int64_t xo_proc_child_pid(xo_child *ch) { return ch->fake ? 0 : (int64_t)ch->pid; }
#endif

// pr_data is the outcome "0" with the n bytes at p.
static xo_rlist *pr_data(const char *p, int64_t n) {
	xo_rlist *l = n_str_list_new(2);
	l = n_str_list_add(l, "0", 1);
	return n_str_list_add(l, p, n);
}

// pr_io is the outcome of an error with message "<what>: <errno text>".
static xo_rlist *pr_io(const char *what, int e) {
	char m[512];
	snprintf(m, sizeof m, "%s: %s", what, strerror(e));
	size_t k = strlen(what) + 2;
	if (k < sizeof m && m[k] >= 'A' && m[k] <= 'Z') {
		m[k] = (char)(m[k] + 32);
	}
	return pr_fail('2', m);
}

// xo_proc_child_write is `c.write(data)`: to its input's pipe.
xo_rlist *xo_proc_child_write(xo_child *ch, const char *p, int64_t n) {
	xo_ctx *cx = ctx();
	if (ctx_canceled(cx)) {
		return pr_fail('3', "");
	}
	if (ch->fake) {
		return pr_fail('0', "");
	}
	if (ch->in_fd < 0 && !ch->in_closed) {
		return pr_fail('2', "the child's input is not a pipe (stdin data or inherit was set)");
	}
	if (ch->in_closed) {
		return pr_fail('2', "write |1: file already closed");
	}
	int64_t off = 0;
	while (off < n) {
		ssize_t w = pr_wr(ch->in_fd, p + off, (size_t)(n - off));
		if (w > 0) {
			off += w;
			continue;
		}
		if (w < 0 && errno == EINTR) {
			continue;
		}
		if (w < 0 && (errno == EAGAIN || errno == EWOULDBLOCK)) {
			flush_pending();
			if (ch->in_pd) {
				if (pd_wait(ch->in_pd, 1, cx) == PD_CANCELED) {
					return pr_fail('3', "");
				}
			} else if (clock_wait(cx, 1000000)) {
				return pr_fail('3', "");
			}
			continue;
		}
		return pr_io("write |1", w < 0 ? errno : EIO);
	}
	return pr_fail('0', "");
}

// xo_proc_child_close_stdin is `c.close_stdin()`.
void xo_proc_child_close_stdin(xo_child *ch) {
	if (ch->in_fd < 0 || ch->in_closed) {
		return;
	}
	ch->in_closed = 1;
	if (ch->in_pd) {
		pd_free(ch->in_pd);
		ch->in_pd = NULL;
		ch->in_fd = -1;
	} else {
		pr_close(&ch->in_fd);
	}
}

// ch_read reads at most n bytes of stream i into b, waiting on the
// poller: the count, 0 at the end, -1 canceled, or -2 - errno.
static int64_t ch_read(xo_ctx *cx, xo_child *ch, int i, char *b, int64_t n) {
	for (;;) {
		ssize_t r = pr_rd(ch->fd[i], b, (size_t)n);
		if (r >= 0) {
			return r;
		}
		if (errno == EINTR) {
			continue;
		}
		if (errno != EAGAIN && errno != EWOULDBLOCK) {
			return -2 - errno;
		}
		flush_pending();
		if (ch->pd[i]) {
			int w = pd_wait(ch->pd[i], 0, cx);
			if (w == PD_CANCELED) {
				return -1;
			}
			if (w == PD_CLOSED) {
				return 0;
			}
		} else if (clock_wait(cx, 1000000)) {
			return -1;
		}
	}
}

// xo_proc_child_read is `c.read(max)` (err false) or `c.read_err(max)`:
// "0" and at most max bytes (at most 1 MiB), "e" at the end.
xo_rlist *xo_proc_child_read(xo_child *ch, int64_t err, int64_t max) {
	xo_ctx *cx = ctx();
	int i = err ? 1 : 0;
	if (ch->fake) {
		if (ctx_canceled(cx)) {
			return pr_fail('3', "");
		}
		nbuf *b = &ch->fbuf[i];
		int64_t left = b->n - ch->fat[i];
		if (left <= 0) {
			return pr_fail('e', "");
		}
		if (max <= 0) {
			return pr_data("", 0);
		}
		int64_t k = left < max ? left : max;
		xo_rlist *l = pr_data(b->p + ch->fat[i], k);
		ch->fat[i] += k;
		return l;
	}
	if (!err && ch->ahead.n - ch->ahead_at > 0) {
		// bytes read ahead by read_line come first
		int64_t left = ch->ahead.n - ch->ahead_at;
		int64_t k = left < max ? left : max;
		if (k < 0) {
			k = 0;
		}
		xo_rlist *l = pr_data(ch->ahead.p + ch->ahead_at, k);
		ch->ahead_at += k;
		return l;
	}
	if (ctx_canceled(cx)) {
		return pr_fail('3', "");
	}
	if (ch->fd[i] < 0) {
		return pr_fail('2', err ? "the child's error stream is not a pipe (inherit or merge_stderr was set)" : "the child's output is not a pipe (inherit or merge_stderr was set)");
	}
	if (max <= 0) {
		return pr_data("", 0);
	}
	int64_t n = max < (1 << 20) ? max : (1 << 20);
	char *b = malloc((size_t)n);
	if (!b) {
		xo_oom();
	}
	int64_t r = ch_read(cx, ch, i, b, n);
	xo_rlist *l;
	if (r > 0) {
		l = pr_data(b, r);
	} else if (r == 0) {
		l = pr_fail('e', "");
	} else if (r == -1) {
		l = pr_fail('3', "");
	} else {
		l = pr_io("read |0", (int)(-2 - r));
	}
	free(b);
	return l;
}

// pr_line is the outcome of a line of n bytes at p, without \r at its end.
static xo_rlist *pr_line(const char *p, int64_t n) {
	if (n > 0 && p[n - 1] == '\r') {
		n--;
	}
	return pr_data(p, n);
}

// xo_proc_child_read_line is `c.read_line()`: "0" and a line of its output
// without \n or \r\n, "e" at the end.
xo_rlist *xo_proc_child_read_line(xo_child *ch) {
	xo_ctx *cx = ctx();
	if (ch->fake) {
		if (ctx_canceled(cx)) {
			return pr_fail('3', "");
		}
		nbuf *b = &ch->fbuf[0];
		int64_t at = ch->fat[0];
		if (at >= b->n) {
			return pr_fail('e', "");
		}
		const char *s = b->p + at;
		const char *nl = memchr(s, '\n', (size_t)(b->n - at));
		int64_t k = nl ? (int64_t)(nl - s) : b->n - at;
		ch->fat[0] = at + k + (nl ? 1 : 0);
		return pr_line(s, k);
	}
	if (ch->fd[0] < 0) {
		return pr_fail('2', "the child's output is not a pipe (inherit was set)");
	}
	for (;;) {
		int64_t left = ch->ahead.n - ch->ahead_at;
		const char *s = ch->ahead.p ? ch->ahead.p + ch->ahead_at : "";
		const char *nl = left > 0 ? memchr(s, '\n', (size_t)left) : NULL;
		if (nl || (ch->eof && left > 0)) {
			int64_t k = nl ? (int64_t)(nl - s) : left;
			xo_rlist *l = pr_line(s, k);
			ch->ahead_at += k + (nl ? 1 : 0);
			if (ch->ahead_at == ch->ahead.n) {
				ch->ahead.n = ch->ahead_at = 0;
			}
			return l;
		}
		if (ch->eof) {
			return pr_fail('e', "");
		}
		if (ctx_canceled(cx)) {
			return pr_fail('3', "");
		}
		if (ch->ahead_at > 0) {
			memmove(ch->ahead.p, ch->ahead.p + ch->ahead_at, (size_t)left);
			ch->ahead.n = left;
			ch->ahead_at = 0;
		}
		nbuf_grow(&ch->ahead, 4096);
		int64_t r = ch_read(cx, ch, 0, ch->ahead.p + ch->ahead.n, 4096);
		if (r == -1) {
			return pr_fail('3', "");
		}
		if (r < -1) {
			return pr_io("read |0", (int)(-2 - r));
		}
		if (r == 0) {
			ch->eof = 1;
		}
		ch->ahead.n += r;
	}
}

// xo_proc_child_wait is `c.wait()`: closes its input, waits, and gives
// "0" and its status.
xo_rlist *xo_proc_child_wait(xo_child *ch) {
	char t[32];
	if (!ch->fake && !ch->reaped) {
		xo_proc_child_close_stdin(ch);
		xo_ctx *cx = ctx();
		int canceled = 0;
		int ws = pr_wait(cx, ch->pid, &canceled);
		ch->reaped = 1;
		ch->code = pr_code(ws);
		if (canceled) {
			return pr_fail('3', "");
		}
	}
	snprintf(t, sizeof t, "%lld", (long long)ch->code);
	return pr_fail('0', t);
}

// xo_proc_child_kill is `c.kill()`.
void xo_proc_child_kill(xo_child *ch) {
	if (!ch->fake && ch->st == '0' && !ch->reaped) {
		pr_kill9(ch->pid);
	}
}
