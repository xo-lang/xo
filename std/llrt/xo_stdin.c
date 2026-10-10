// Standard input (std.md 3.6, decisions 0088 and 0102): read_line,
// read_all, and lines wait on the event loop's poller, so a read is a
// cancellation point and a stop request ends it (IoErr.Canceled), as
// std/rt's stdin.go. Input read before a reader gave up stays buffered for
// the next read. Readers take turns. When the poller cannot watch the
// descriptor (a regular file, /dev/null on Linux), reads go to read(2)
// directly: they do not block for long.

#include <poll.h>

static struct {
	int busy;    // a reader holds the turn
	int init;    // the descriptor was looked at
	xo_pd *pd;   // the poller's registration, or NULL
	char *buf;   // read but not delivered
	size_t len, cap;
	int eof;     // the end of input follows buf
	int err;     // a read error (errno) follows buf
} sin_st;

static __thread int64_t n_stdin_status;
static __thread int n_stdin_errno;

static void sin_put(const char *p, size_t n) {
	if (sin_st.len + n > sin_st.cap) {
		size_t c = sin_st.cap ? sin_st.cap * 2 : 4096;
		while (c < sin_st.len + n) {
			c *= 2;
		}
		char *b = realloc(sin_st.buf, c);
		if (!b) {
			xo_oom();
		}
		sin_st.buf = b;
		sin_st.cap = c;
	}
	memcpy(sin_st.buf + sin_st.len, p, n);
	sin_st.len += n;
}

// sin_take returns the first n buffered bytes as a string.
static xo_rstr *sin_take(size_t n, size_t skip, int64_t *len) {
	xo_rstr *s = rs_new((int64_t)n);
	memcpy(s->data, sin_st.buf, n);
	memmove(sin_st.buf, sin_st.buf + n + skip, sin_st.len - n - skip);
	sin_st.len -= n + skip;
	*len = (int64_t)n;
	return s;
}

static int sin_stopped(xo_ctx *c) { return ctx_canceled(c) || xo_stop_requested(); }

// A test's standard input (FakeStdio.set_input, std.md 8.4): the text and
// how much of it was read; std/rt's FakeStdio.
static struct {
	pthread_mutex_t mu;
	char *text;
	size_t len, off;
} tw_in = {.mu = PTHREAD_MUTEX_INITIALIZER};

// xo_tw_set_input is `stdio.set_input(text)`: reads start over on text.
void xo_tw_set_input(const char *p, int64_t n) {
	char *t = malloc((size_t)n + 1);
	if (!t) {
		xo_oom();
	}
	memcpy(t, p, (size_t)n);
	pthread_mutex_lock(&tw_in.mu);
	free(tw_in.text);
	tw_in.text = t;
	tw_in.len = (size_t)n;
	tw_in.off = 0;
	pthread_mutex_unlock(&tw_in.mu);
}

static xo_rstr *tw_stdin_read(int64_t all, int64_t *len) {
	pthread_mutex_lock(&tw_in.mu);
	const char *rest = tw_in.text ? tw_in.text + tw_in.off : "";
	size_t n = tw_in.len - tw_in.off, take = n, skip = 0;
	if (!all) {
		if (n == 0) {
			pthread_mutex_unlock(&tw_in.mu);
			n_stdin_status = 1;
			return rs_new(0);
		}
		const char *nl = memchr(rest, '\n', n);
		if (nl) {
			take = (size_t)(nl - rest);
			skip = 1;
		}
	}
	size_t k = !all && take > 0 && rest[take - 1] == '\r' ? take - 1 : take;
	xo_rstr *s = rs_new((int64_t)k);
	memcpy(s->data, rest, k);
	tw_in.off += take + skip;
	pthread_mutex_unlock(&tw_in.mu);
	*len = (int64_t)k;
	n_stdin_status = 0;
	return s;
}

// sin_fill reads what is available into the buffer; it waits on the
// poller (in steps, to see a stop request) when nothing is. 1 when the
// reader was canceled or a stop was requested.
static int sin_fill(xo_ctx *c) {
	if (!sin_st.init) {
		sin_st.init = 1;
		struct stat sb;
		if (fstat(0, &sb) == 0 && !S_ISREG(sb.st_mode)) {
			sin_st.pd = pd_new(0, 0);
		}
	}
	char t[65536];
	for (;;) {
		if (sin_stopped(c)) {
			return 1;
		}
		if (sin_st.pd) {
			struct pollfd p = {.fd = 0, .events = POLLIN};
			int r = poll(&p, 1, 0);
			if (r == 0) {
				flush_pending();
				int w = pd_wait_until(sin_st.pd, 0, c, deadline(50000000));
				if (w == PD_CANCELED) {
					return 1;
				}
				continue;
			}
		}
		ssize_t n = read(0, t, sizeof t);
		if (n < 0) {
			if (errno == EINTR || errno == EAGAIN) {
				continue;
			}
			sin_st.err = errno;
			return 0;
		}
		if (n == 0) {
			sin_st.eof = 1;
			return 0;
		}
		sin_put(t, (size_t)n);
		return 0;
	}
}

// xo_stdin_read reads a line (without its newline and a `\r` before it)
// or, when all, the rest of the input. xo_stdin_status then tells: 0 the
// text, 1 the end of input (no line), 2 a read error (xo_stdin_msg), 3
// canceled or stopped.
xo_rstr *xo_stdin_read(int64_t all, int64_t *len) {
	xo_ctx *c = ctx();
	*len = 0;
	n_stdin_status = 3;
	if (sin_stopped(c)) {
		return rs_new(0);
	}
	if (tw.on) {
		return tw_stdin_read(all, len);
	}
	// take the turn
	while (__atomic_exchange_n(&sin_st.busy, 1, __ATOMIC_ACQUIRE)) {
		if (clock_wait(c, 1000000) || xo_stop_requested()) {
			return rs_new(0);
		}
	}
	xo_rstr *s = NULL;
	for (;;) {
		if (!all) {
			char *nl = sin_st.len ? memchr(sin_st.buf, '\n', sin_st.len) : NULL;
			if (nl) {
				size_t n = (size_t)(nl - sin_st.buf);
				size_t k = n > 0 && sin_st.buf[n - 1] == '\r' ? n - 1 : n;
				s = sin_take(k, n - k + 1, len);
				n_stdin_status = 0;
				break;
			}
		}
		if (sin_st.eof || sin_st.err) {
			if (sin_st.len == 0 && sin_st.err) {
				n_stdin_errno = sin_st.err;
				sin_st.err = 0;
				n_stdin_status = 2;
				break;
			}
			size_t n = sin_st.len;
			size_t k = !all && n > 0 && sin_st.buf[n - 1] == '\r' ? n - 1 : n;
			s = sin_take(k, n - k, len);
			n_stdin_status = all || n > 0 ? 0 : 1;
			break;
		}
		if (sin_fill(c)) {
			break;
		}
	}
	__atomic_store_n(&sin_st.busy, 0, __ATOMIC_RELEASE);
	return s ? s : rs_new(0);
}

int64_t xo_stdin_status(void) { return n_stdin_status; }

// xo_stdin_msg is the message of the last read error.
xo_rstr *xo_stdin_msg(int64_t *len) {
	const char *m = strerror(n_stdin_errno);
	int64_t n = (int64_t)strlen(m);
	xo_rstr *s = rs_new(n);
	memcpy(s->data, m, (size_t)n);
	if (n > 0 && s->data[0] >= 'A' && s->data[0] <= 'Z') {
		s->data[0] = (char)(s->data[0] + 32);
	}
	*len = n;
	return s;
}
