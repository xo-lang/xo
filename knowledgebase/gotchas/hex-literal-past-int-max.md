# A 64 bit hex constant past 2^63 does not fit Int

Status: Observed (2026-10-29)

Symptom: porting a Go uint64 constant (`0xd2b51da312547e1b`, fiat-crypto's
Montgomery constant) as `const MINV: Int = 0xd2b51da312547e1b` fails the
check with XO0421 "the integer literal ... does not fit `Int`"; xobuild
then reports that no seed could check compiler/xo, which reads like a seed
problem.

Cause: Xo has no unsigned 64 bit literal; a hex literal is a value, not a
bit pattern, and Int is signed.

Fix: write the two's complement value negative, `-0x2d4ae25cedab81e5`
(2^64 - v; python3 -c "print('-0x%x' % (2**64 - 0xd2b5...))"), and keep the
unsigned value in a trailing comment so it still greps against Go's source.
Wrapping arithmetic (wrapping_mul, mul_hi_unsigned) treats it as the same
word. Check every constant of a port with the high bit set before the first
build.
