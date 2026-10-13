// errno_probe.c prints the C library's strerror text of each errno from
// 0 to 160 ("E <n> <text>"): TestErrnoTables compares them with the
// tables of std/native/llrt/darwin.xo and windows.xo (decision 0213).
// The Microsoft C runtime's map from Win32 errors to errno (_dosmaperr)
// is not exported (_get_errno_from_oserr did not link with lld-link and
// the Windows SDK, 2026-10-13); TestSysioParity checks it through the
// errors the C path reports.
#ifdef _WIN32
#define _CRT_SECURE_NO_WARNINGS
#endif
#include <stdio.h>
#include <string.h>

int main(void) {
	for (int i = 0; i <= 160; i++) {
		printf("E %d %s\n", i, strerror(i));
	}
	return 0;
}
