// ---------------------------------------------------------------------------
// What the Xo system call layers (std/native/llrt/linux.xo, darwin.xo,
// windows.xo, decision 0206) still take from the C runtime. The Bytes
// primitives and the per thread scratch are Xo since work item B02
// (raw.xo bytes_addr, bytes_buf, bytes_len; obj.xo scratch, in the heap
// record).

// xo_tw_fake reports whether the test world fakes the capability bits
// real (TW_REAL_*): the Xo layer then calls the C runtime's fake.
int64_t xo_tw_fake(int64_t real) { return tw_is_fake((int)real); }

// xo_tw_exit is os.proc.exit in a test binary (std/native/llrt/process.xo).
void xo_tw_exit(int64_t code) { tw_exit(code); }

// xo_sys_clock_buf is the address of the calling thread's scratch for
// Clock.now's timespec (Xo calls the vDSO with it), or 0 while the test
// world's virtual clock runs: one call on the clock's path (decision
// 0206 stage 2).
// Off Linux since work item B01 (Clock.now is Xo there too): on macOS it
// is 1, not an address, since clock_gettime_nsec_np takes no structure
// and a thread local's address costs a call of the TLV getter there
// (bench/results/clocks-2026-10-14.md).
int64_t xo_sys_clock_buf(void) {
#if defined(__APPLE__)
	return tw.virt ? 0 : 1;
#else
	return tw.virt ? 0 : (int64_t)(uintptr_t)heap()->scratch;
#endif
}

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
