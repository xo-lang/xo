# bcrypt: "" and "\x00" are one key

Status: Observed 2026-11-17 (task #152, bcrypt stress on linux/arm64 host 2).

Symptom: the bcrypt stress program failed after 24 minutes with "task 1
password 694: another password matches". std/bcrypt was right.

Cause: bcrypt's key is the password followed by a NUL, repeated to fill
72 bytes. The empty password gives "\x00" repeated, and the one byte
password "\x00" gives "\x00\x00" repeated, which is the same key. Go's
x/crypto/bcrypt matches them too (CompareHashAndPassword of the empty
password's hash and "\x00" returns nil). The stress program drew another
password of n+1 seeded bytes and expected no match. For an empty password
that is one byte, which is zero 1 time in 256, and 1 password in 74 is
empty, so a 30-minute run of 64 tasks meets it.

Fix: another password of an empty one is never all zero
(std/evidence/bcrypt/stress/main.xo). Quick mode (`main 0`) now runs 700
passwords per task, which reaches task 1 password 694: it fails without
the fix. A check that two inputs never match must allow the inputs the
algorithm defines as equal.
