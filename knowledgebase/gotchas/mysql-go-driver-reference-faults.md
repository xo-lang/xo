# go-sql-driver as a MySQL reference: panics and silent truncation

Status: Observed (2026-11-15, TestMysqlFuzz against go-sql-driver v1.10.1).

Symptoms and causes:

- A differential test against go-sql-driver exits with "index out of range"
  in skipLengthEncodedString (packets.go): the driver panics on some
  malformed column definitions. scripts/mysqldiff/gomysql recovers the
  panic, prints "panic go-sql-driver" and reopens its pool; the fuzz test
  treats that case as having no reference answer.
- go-sql-driver returns fewer rows than the server sent, with no error,
  when a value's length encoding runs past the packet: readRow returns
  io.EOF, which database/sql takes as the end of the rows. Xo errors there.
  A differential test must accept an Xo error where the reference took a
  changed (mutated) packet, and require equality only for the rows Xo
  returns.
- A pooled connection the fake server closed after a mutated case makes
  the next clean case fail in Xo (no liveness check yet; go-sql-driver
  checks before reuse). Run the reference and Xo with MY_FRESH=1, a pool
  per statement, until std/sql gains the check.
