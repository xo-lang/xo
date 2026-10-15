// Standard input (std.md 3.6, decisions 0088 and 0102): read_line,
// read_all, and lines wait on the event loop's poller, so a read is a
// cancellation point and a stop request ends it (IoErr.Canceled), as
// std/rt's stdin.go. Input read before a reader gave up stays buffered for
// the next read. Readers take turns. When the poller cannot watch the
// descriptor (a regular file, /dev/null on Linux), reads go to read(2)
// directly: they do not block for long.

#ifndef _WIN32
#include <poll.h>
#include <sys/stat.h>
#endif

// The reading itself (the buffer, the turn, lines, read_bytes) is
// std/native/llrt/stdin.xo (the zero-C audit's B24); the C keeps the
// platform's part below: opening the descriptor for the poller, a raw
// read, and the waits.

#ifdef _WIN32
static int sin_sock = -1; // the pumped socket the poller watches (Windows)
#endif

// xo_stdin_stopped is whether the reader was canceled or a stop was
// requested.
int64_t xo_stdin_stopped(void) {
	xo_ctx *c = ctx();
	return ctx_canceled(c) || xo_stop_requested();
}

// xo_stdin_open registers standard input with the poller: its xo_pd, or 0
// when the poller cannot watch it (a regular file, /dev/null on Linux:
// reads go to read(2) directly and do not block for long).
int64_t xo_stdin_open(void) {
#ifdef _WIN32
	// Windows (decision 0197, stage 5): the poller waits on sockets only,
	// so a pipe or the console is read by a thread of its own into a
	// loopback socket (xo_w_pump), whose end the reads use and the poller
	// waits on: reads are cancellation points as on Unix. A file is read
	// directly.
	HANDLE h = GetStdHandle(STD_INPUT_HANDLE);
	DWORD ft = h && h != INVALID_HANDLE_VALUE ? GetFileType(h) : FILE_TYPE_UNKNOWN;
	if (ft == FILE_TYPE_PIPE || ft == FILE_TYPE_CHAR) {
		int s = xo_w_pump(h, 1, 0);
		if (s >= 0) {
			sin_sock = s;
			return (int64_t)(intptr_t)pd_new(s, 0);
		}
	}
	return 0;
#else
	struct stat sb;
	if (fstat(0, &sb) == 0 && !S_ISREG(sb.st_mode)) {
		return (int64_t)(intptr_t)pd_new(0, 0);
	}
	return 0;
#endif
}

// xo_stdin_sysread reads at most n bytes into p: their count, 0 at the
// end of input, -1000000 when the poller's descriptor (pd, not 0) has
// nothing yet (wait with xo_stdin_wait), else minus errno. EINTR and
// EAGAIN on a direct read are retried.
int64_t xo_stdin_sysread(int64_t p, int64_t n, int64_t pd) {
	for (;;) {
		ssize_t r;
#ifdef _WIN32
		if (pd) {
			int k = recv((SOCKET)sin_sock, (char *)(intptr_t)p, (int)n, 0);
			if (k == SOCKET_ERROR && WSAGetLastError() == WSAEWOULDBLOCK) {
				return -1000000;
			}
			if (k == SOCKET_ERROR) {
				return -(int64_t)xo_wsa_errno();
			}
			return k;
		}
		r = read(0, (void *)(intptr_t)p, (size_t)n);
#else
		if (pd) {
			struct pollfd q = {.fd = 0, .events = POLLIN};
			if (poll(&q, 1, 0) == 0) {
				return -1000000;
			}
		}
		r = read(0, (void *)(intptr_t)p, (size_t)n);
#endif
		if (r < 0) {
			if (errno == EINTR || errno == EAGAIN) {
				continue;
			}
			return -(int64_t)errno;
		}
		return (int64_t)r;
	}
}

// xo_stdin_wait waits up to 50 ms on the poller for input on pd: 1 when
// the reader was canceled.
int64_t xo_stdin_wait(int64_t pd) {
	flush_pending();
	return pd_wait_until((xo_pd *)(intptr_t)pd, 0, ctx(), deadline(50000000)) == PD_CANCELED;
}

// xo_stdin_turn_wait waits a millisecond for another reader's turn: 1
// when the reader was canceled or a stop was requested.
int64_t xo_stdin_turn_wait(void) { return clock_wait(ctx(), 1000000) || xo_stop_requested(); }

// Terminals (decision 0162). A test binary's streams are terminals only
// after FakeStdio.set_tty, of the width it gave.

#ifndef _WIN32
#include <sys/ioctl.h>
#include <termios.h>
#endif

static int64_t tw_tty_width;

// xo_tw_set_tty is `stdio.set_tty(width)`.
void xo_tw_set_tty(int64_t w) { __atomic_store_n(&tw_tty_width, w > 0 ? w : 0, __ATOMIC_RELAXED); }

// xo_isatty is whether descriptor fd is a terminal.
int64_t xo_isatty(int64_t fd) {
	if (tw_is_fake(TW_REAL_STDIO)) {
		return __atomic_load_n(&tw_tty_width, __ATOMIC_RELAXED) > 0;
	}
#ifdef _WIN32
	return _isatty((int)fd) ? 1 : 0;
#else
	return isatty((int)fd) ? 1 : 0;
#endif
}

#ifdef _WIN32
// Windows consoles (decision 0197): the window's width and the input
// mode's echo flag.
int64_t xo_term_width(void) {
	if (tw_is_fake(TW_REAL_STDIO)) {
		return __atomic_load_n(&tw_tty_width, __ATOMIC_RELAXED);
	}
	static const DWORD hs[3] = {STD_OUTPUT_HANDLE, STD_ERROR_HANDLE, STD_INPUT_HANDLE};
	for (int i = 0; i < 3; i++) {
		CONSOLE_SCREEN_BUFFER_INFO bi;
		if (GetConsoleScreenBufferInfo(GetStdHandle(hs[i]), &bi)) {
			int w = bi.srWindow.Right - bi.srWindow.Left + 1;
			if (w > 0) {
				return w;
			}
		}
	}
	return 0;
}

static DWORD sin_saved_mode;
static int sin_echo_off;

int64_t xo_stdin_echo(int64_t on) {
	if (tw_is_fake(TW_REAL_STDIO)) {
		return 0;
	}
	HANDLE h = GetStdHandle(STD_INPUT_HANDLE);
	if (on) {
		if (sin_echo_off) {
			SetConsoleMode(h, sin_saved_mode);
			sin_echo_off = 0;
		}
		return 0;
	}
	DWORD m;
	if (!GetConsoleMode(h, &m)) {
		return 0;
	}
	sin_saved_mode = m;
	if (!SetConsoleMode(h, m & ~(DWORD)ENABLE_ECHO_INPUT)) {
		return 0;
	}
	sin_echo_off = 1;
	return 1;
}
#else

// xo_term_width is the columns of the terminal on standard output, error,
// or input (the first that is one), or 0.
int64_t xo_term_width(void) {
	if (tw_is_fake(TW_REAL_STDIO)) {
		return __atomic_load_n(&tw_tty_width, __ATOMIC_RELAXED);
	}
	static const int fds[3] = {1, 2, 0};
	for (int i = 0; i < 3; i++) {
		struct winsize ws;
		if (ioctl(fds[i], TIOCGWINSZ, &ws) == 0 && ws.ws_col > 0) {
			return ws.ws_col;
		}
	}
	return 0;
}

static struct termios sin_saved_tio;
static int sin_echo_off;

// xo_stdin_echo turns standard input's echo off (on false), remembering
// the settings, or restores them (on true). Turning it off returns 1 when
// standard input is a terminal whose echo it turned off.
int64_t xo_stdin_echo(int64_t on) {
	if (tw_is_fake(TW_REAL_STDIO)) {
		return 0;
	}
	if (on) {
		if (sin_echo_off) {
			tcsetattr(0, TCSANOW, &sin_saved_tio);
			sin_echo_off = 0;
		}
		return 0;
	}
	struct termios t;
	if (tcgetattr(0, &t) != 0) {
		return 0;
	}
	sin_saved_tio = t;
	t.c_lflag &= ~(tcflag_t)ECHO;
	if (tcsetattr(0, TCSANOW, &t) != 0) {
		return 0;
	}
	sin_echo_off = 1;
	return 1;
}
#endif
