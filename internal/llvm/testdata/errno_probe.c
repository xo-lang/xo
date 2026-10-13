// errno_probe.c prints the C library's strerror text of each errno from
// 0 to 160 ("E <n> <text>") and, on Windows, the Microsoft C runtime's
// errno of each Win32 error below 20000 that is not EINVAL ("M <win32>
// <errno>"): TestErrnoTables compares them with the tables of
// std/native/llrt/darwin.xo and windows.xo (decision 0213).
#include <stdio.h>
#include <string.h>

#ifdef _WIN32
// exported by the Universal CRT (its _dosmaperr table)
int __cdecl _get_errno_from_oserr(unsigned long oserr);
#endif

int main(void) {
	for (int i = 0; i <= 160; i++) {
		printf("E %d %s\n", i, strerror(i));
	}
#ifdef _WIN32
	for (unsigned long e = 0; e < 20000; e++) {
		int m = _get_errno_from_oserr(e);
		if (m != 22) {
			printf("M %lu %d\n", e, m);
		}
	}
#endif
	return 0;
}
