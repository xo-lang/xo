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
- A pooled connection the fake server closed after a mutated case can be
  taken again before its close arrives, in Xo and in go-sql-driver alike
  (both check the socket before reuse since 2026-11-15, but the check
  races the close). The harness runs both with MY_FRESH=1, a pool per
  statement.
- A 1 to 3 byte packet starting 0xfe where a text row is due panics
  go-sql-driver (readStatus(data[3:]) in textRows.readRow).

Upstream (2026-11-15): the column definition panic is open as
go-sql-driver/mysql#1854 (with readStatus and the other unchecked
offsets); the silent truncation reproduces on v1.10.1 only, as master
(f32af30) returns ErrMalformPkt from readColumnBytes. Master also refuses
bytes after a row's last value (96884ae), which Xo follows; v1.10.1, the
harness's reference, takes them, so the fuzz test counts Xo's refusal.
The repository has no AI or LLM contribution policy; SECURITY.md treats a
malformed server's packets as bugs, not vulnerabilities.

## The column collation's high byte (2026-11-15)

Observed: TestMysqlFuzz at 5000 cases (seed 2, case 3470, a `byte`
change) typed a column TEXT in Xo and BLOB in go-sql-driver v1.10.1. The
changed definition's collation was 0x233f.

Cause: go-sql-driver keeps one byte of the column definition's 2-byte
collation (packets.go `columns[i].charSet = data[pos]`, fields.go
`charSet uint8`), so a collation whose low byte is 63 reads as binary.
The protocol's field is 2 bytes and MySQL 8 has collations above 255
(for example 309, utf8mb4_0900_bin), so Xo reads both bytes and keeps
its answer.

Fix: the fuzz comparison counts a case as `go collation byte` when the
answers differ only in the `types` line and a column definition's
collation is above 255 (myfWideCollation, TestMysqlFuzzWideCollation).
