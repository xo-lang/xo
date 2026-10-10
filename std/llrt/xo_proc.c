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

#include <spawn.h>
#include <sys/stat.h>
#include <sys/wait.h>

extern char **environ;


// the outputs FakeProc.on_run registered, by command
typedef struct {
	char *cmd;
	int64_t code;
	char *out, *err;
	int64_t outn, errn;
} pr_fake;

static struct {
	xo_lock mu;
	pr_fake *v;
	int n;
} pr_fakes;

// xo_proc_on_run is `FakeProc.on_run(cmd, out)`: a later run of cmd in the
// test gives out (the last registration wins).
void xo_proc_on_run(const char *c, int64_t cn, int64_t code, const char *o, int64_t on, const char *e, int64_t en) {
	char *cmd = cstr_of(c, cn);
	char *out = malloc((size_t)on + 1), *err = malloc((size_t)en + 1);
	if (!out || !err) {
		xo_oom();
	}
	memcpy(out, o, (size_t)on);
	memcpy(err, e, (size_t)en);
	lk(&pr_fakes.mu);
	for (int i = 0; i < pr_fakes.n; i++) {
		pr_fake *f = &pr_fakes.v[i];
		if (strcmp(f->cmd, cmd) == 0) {
			free(f->out);
			free(f->err);
			free(cmd);
			f->code = code;
			f->out = out;
			f->err = err;
			f->outn = on;
			f->errn = en;
			unlk(&pr_fakes.mu);
			return;
		}
	}
	pr_fakes.v = tw_grow(pr_fakes.v, pr_fakes.n, sizeof(pr_fake));
	pr_fakes.v[pr_fakes.n++] = (pr_fake){cmd, code, out, err, on, en};
	unlk(&pr_fakes.mu);
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

// pr_drain reads what fd has now into b; 1 at the end of the output.
static int pr_drain(int fd, nbuf *b) {
	char t[65536];
	for (;;) {
		ssize_t n = read(fd, t, sizeof t);
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

static int pr_pipe(int p[2]) {
	if (pipe(p) < 0) {
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
	if (tw_is_fake(TW_REAL_PROC)) {
		lk(&pr_fakes.mu);
		for (int i = 0; i < pr_fakes.n; i++) {
			pr_fake *f = &pr_fakes.v[i];
			if (strcmp(f->cmd, cmd) == 0) {
				xo_rlist *l = pr_outputs(f->code, f->out, f->outn, f->err, f->errn);
				unlk(&pr_fakes.mu);
				free(cmd);
				return l;
			}
		}
		unlk(&pr_fakes.mu);
		free(cmd);
		return pr_fail('1', "");
	}
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
