// Standard input (std.md 3.6, decisions 0088 and 0102) is
// std/native/llrt/stdin.xo (the zero-C audit's B24): the buffer, the
// turn, lines, the waits, and the terminal calls. What stays C is the
// Windows registration and read (decision 0197, stage 5): the poller
// waits on sockets only, so a pipe or the console is read by a thread of
// its own into a loopback socket (xo_w_pump, xo_win.c), whose end the
// reads use and the poller waits on.
//
// End pass, two kinds: the Windows part is the Windows pump (stays C
// until xo_win.c's pump is Xo); the stubs below the #else are seed
// compatibility: this compiler lowers stdin.xo's rt.stdin_* calls to a
// fault off Windows (selfhost/ssa call.xo), the seed (0.11.0) names them
// (gotchas/dead-target-arm-still-links.md). Delete the stubs when the
// seed has this change.

#ifdef _WIN32
static int sin_sock = -1; // the pumped socket the poller watches

// xo_stdin_open registers standard input with the poller: its xo_pd, or 0
// when it is read directly (a file).
int64_t xo_stdin_open(void) {
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
}

// xo_stdin_sysread reads at most n bytes into p: their count, 0 at the
// end of input, -1000000 when the pumped socket (pd, not 0) has nothing
// yet, else minus errno. EINTR and EAGAIN on a direct read are retried.
int64_t xo_stdin_sysread(int64_t p, int64_t n, int64_t pd) {
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
	for (;;) {
		int r = read(0, (void *)(intptr_t)p, (unsigned)n);
		if (r < 0) {
			if (errno == EINTR || errno == EAGAIN) {
				continue;
			}
			return -(int64_t)errno;
		}
		return (int64_t)r;
	}
}
#else
XO_NORETURN static void stdin_dead(void) {
	static const char m[] = "the C standard input off Windows";
	xo_fault_raise(m, sizeof m - 1, "", 0);
}
int64_t xo_stdin_open(void) { stdin_dead(); }
int64_t xo_stdin_sysread(int64_t p, int64_t n, int64_t pd) {
	(void)p;
	(void)n;
	(void)pd;
	stdin_dead();
}
#endif
