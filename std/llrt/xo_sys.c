// ---------------------------------------------------------------------------
// The buffers of the Linux system call layer written in Xo
// (std/native/llrt/linux.xo, decision 0206). The system calls themselves
// are rt.sys6, which the emitters lower inline; these are the memory
// primitives the layer needs until raw memory intrinsics exist (decision
// 0195 prerequisite 2): an address of a Bytes, a fresh buffer, a length
// set without a copy, and a per thread scratch buffer for small
// structures (timespec, statx).

// xo_bytes_addr is the address of the first byte of b.
int64_t xo_bytes_addr(const char *p, int64_t n) {
	(void)n;
	return (int64_t)(uintptr_t)p;
}

// xo_bytes_buf is a unique Bytes of n bytes for a system call to fill
// (not zeroed: a 1 MiB read would write every byte twice).
xo_rstr *xo_bytes_buf(int64_t n, int64_t *len) {
	if (n < 0) {
		n = 0;
	}
	xo_rstr *s = rs_new(n);
	*len = n;
	return s;
}

// xo_rs_bytes_len is b with length n (clamped to its capacity),
// consuming b and handing its reference on.
xo_rstr *xo_rs_bytes_len(xo_rstr *s, int64_t sn, int64_t n, int64_t *len) {
	(void)sn;
	if (n < 0) {
		n = 0;
	}
	if (n > s->cap) {
		n = s->cap;
	}
	*len = n;
	return s;
}

// The scratch buffer: an immortal string object per thread (reference
// counting never writes or frees it, decision 0058).
#define XO_SYS_BUF 256
typedef struct {
	int64_t rc, cap;
	char data[XO_SYS_BUF];
} xo_sys_scratch;

static __thread xo_sys_scratch sys_scratch = {XO_RC_IMMORTAL, XO_SYS_BUF, {0}};

xo_rstr *xo_sys_buf(int64_t *len) {
	*len = XO_SYS_BUF;
	return (xo_rstr *)&sys_scratch;
}

// xo_tw_fake reports whether the test world fakes the capability bits
// real (TW_REAL_*): the Xo layer then calls the C runtime's fake.
int64_t xo_tw_fake(int64_t real) { return tw_is_fake((int)real); }

// xo_sys_clock_buf is the address of the calling thread's scratch for
// Clock.now's timespec (Xo calls the vDSO with it), or 0 while the test
// world's virtual clock runs: one call on the clock's path (decision
// 0206 stage 2).
// On every OS since work item B01 (Clock.now is Xo off Linux too).
int64_t xo_sys_clock_buf(void) { return tw.virt ? 0 : (int64_t)(uintptr_t)sys_scratch.data; }

// xo_fs_set_err records the outcome of a file operation the Xo layer
// made (as xo_fs_read and fs_fail do): status st (0 for success, else
// FsErr's tag plus one) and, on an error, the path and message that
// xo_fs_msg reports.
void xo_fs_set_err(int64_t st, const char *p, int64_t pn, const char *m, int64_t mn) {
	n_fs_status = st;
	if (st == 0) {
		return;
	}
	free(n_fs_path);
	free(n_fs_err);
	n_fs_path = malloc((size_t)pn + 1);
	n_fs_err = malloc((size_t)mn + 1);
	if (!n_fs_path || !n_fs_err) {
		xo_oom();
	}
	memcpy(n_fs_path, p, (size_t)pn);
	n_fs_path[pn] = 0;
	memcpy(n_fs_err, m, (size_t)mn);
	n_fs_err[mn] = 0;
}

// xo_auxval is getauxval(k) (decision 0206 stage 2: the vDSO's address,
// the program headers for the TLS image), 0 off Linux; with XO_NOLIBC,
// xo_nolibc.c's getauxval reads the vector _start found.
#if defined(__linux__)
#include <sys/auxv.h>
int64_t xo_auxval(int64_t k) { return (int64_t)getauxval((unsigned long)k); }
#else
int64_t xo_auxval(int64_t k) {
	(void)k;
	return 0;
}
#endif
