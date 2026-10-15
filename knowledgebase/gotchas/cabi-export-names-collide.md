# Two cabi modules with a function of the same name export one symbol twice

Status: Observed (2026-10-14, the zero-C audit's B04 and B19)

Symptom: every XO_NOLIBC=1 linux/arm64 build failed to assemble with
`error: symbol 'xo_x_mutex_lock' is already defined` (and
`xo_x_mutex_unlock`); builds with the C library were fine.

Cause: every top level function of a cabi*.xo module is a C ABI export
named `xo_x_<name>` (decision 0207), with no module prefix. B19 added
cabi.xo's `mutex_lock` (Mutex[T]) while cabi_nolibc.xo, built only for
XO_NOLIBC, already had its own `mutex_lock` (the pthread mutex). Each
check built one of them alone, so neither saw the clash.

Fix: keep a cabi module to the exports the C calls, as one line calls of
a plain module (cabi_nolibc.xo's work moved to libcthr.xo). Check a
change to any cabi module with an XO_NOLIBC linux/arm64 build: `XO_NOLIBC=1
xo build --targets=linux/arm64 --backend=arm64 DIR` prints the .s path;
`clang -target aarch64-linux-gnu -c THAT.s -o /dev/null` reports a
duplicate.
