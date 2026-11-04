# 0479: Large data paths: a row cursor, COPY FROM, and zip by entry

Status: Accepted (owner, 2026-11-03, through the lead): the row cursor,
COPY FROM, and zip by entry. Xo Lang 3 implements it.
Step 1 (the row cursor) implemented 2026-11-03: the driver interface keeps `query` and
`DriverRows` gains an optional `cursor: DriverCursor?` (default None) instead of replacing `query`, so
existing drivers compile unchanged; the Go backend reads the result whole.

## Context

The backend survey of 2026-10-30 (std-coverage.md, "Survey of backend
gaps (2026-10-30)", rank 9, awkward, size M each) found that the import
and export batch job (dataport) holds whole data sets in memory where Go
streams them. The streaming XML reader is done (decision 0463), and so are
`csv.reader` over a File and std/bufio. Three paths remain:

1. **Query results are read whole.** `Db.query` returns `List[Row]`
   (spec/std.md 10.3), the driver interface returns
   `DriverRows{columns, rows: List[List[SqlValue]]}` (10.7), and the
   Postgres driver collects every DataRow before it returns
   (std/native/llrt/pg.xo `collect`, lines 598 to 635). Decision 0278
   said so ("Results are read whole, as the existing API returns
   `List[Row]`") and listed "row streaming (a `query_each` that does not
   buffer)" and COPY as later steps. Go's database/sql returns `*Rows`,
   read with `Next` and `Scan` while the connection is held.
2. **No COPY.** Bulk loading is an `exec` per row or a hand built
   multi-row INSERT. pgwire has no CopyIn messages (pgwire.xo's `Backend`
   enum). pgx's `Conn.CopyFrom` streams rows with the COPY protocol.
3. **Zip is whole archives in memory.** `archive.unzip(data: Bytes) ->
   List[Entry]` inflates every entry at once, and `archive.zip(entries)`
   needs every entry's data before it writes (spec/std.md 18). Go's
   archive/zip opens an archive and its central directory
   (`zip.NewReader(io.ReaderAt, size)`, `Reader.File`, `File.Open`) and
   writes entry by entry (`Writer.Create`, `Writer.Close`). The native
   port already has the pieces: one central directory record
   (`zip_dir_header`, std/native/archive.xo:1299) and one entry's data
   (`zip_read`, :1590).

A File has no positioned read: `read`, `read_line`, `write`, `sync`,
`close` (spec/std.md 3.1); `Fs.stat` gives the size. A zip's directory is
at its end, so reading one from a File needs `read_at`.

## Decision (proposed; options below)

### 1. A row cursor (option C1)

```
struct Rows {}                                   // handle: holds a connection until done
fn (d: Db) rows(q: Str, args: List[SqlValue] = []) -> Rows ! SqlErr uses net.dial
fn (t: Tx) rows(q: Str, args: List[SqlValue] = []) -> Rows ! SqlErr uses net.dial
fn (r: Rows) columns() -> List[Str]
fn (var r: Rows) next() -> Row? ! SqlErr uses net.dial        // None after the last row
fn (var r: Rows) next_as[T: SqlRow]() -> T? ! SqlErr uses net.dial
fn (r: Rows) close() uses net.dial                            // idempotent
fn (d: Db) each(q: Str, args: List[SqlValue], f: fn(Row) -> () ! SqlErr) -> () ! SqlErr uses net.dial
```

```
let rows = db.rows("SELECT id, body FROM notes WHERE owner = $1", [sql.int(owner)])?
defer { rows.close() }
while let Some(n) = rows.next_as[Note]()? {
  out.write(render(n))?
}
```

- `rows` holds a pool permit and one connection from the first row to
  the end or `close`; a `Db.rows` outside a transaction behaves like
  `query` otherwise (same errors, same text values).
- `close` before the end reads and discards the rest of the result so
  the connection can go back to the pool; a canceled task sends the
  CancelRequest 0278 already uses. Whether a `Rows` released without
  `close` returns its connection by itself (a release hook on the
  handle; reference counting would make that point exact) is an
  implementation question for the prototype; until then `defer` or
  `each` is the safe form.
- `each` is the form that cannot leak: it closes in every path.
- `query` and `query_as` stay, defined as `rows` read to the end.
- Driver interface (10.7): `DriverConn.query` becomes a cursor,
  `fn query(self, q, args) -> DriverRows` where `DriverRows` gains
  `next() -> List[SqlValue]? ! SqlErr` and `close()`; the in memory driver
  iterates its stored rows. The only drivers are std's (Postgres, memory),
  so the interface changes in place (pre-1.0).
- Postgres: the extended protocol with Execute's row limit 0, and rows
  read off the socket as `next` asks; the socket's flow control is the
  back pressure (the server blocks when the client stops reading).

### 2. COPY FROM (option P1)

```
fn (d: Db) copy_from(table: Str, columns: List[Str],
                     rows: fn() -> List[SqlValue]? ! SqlErr) -> Int ! SqlErr uses net.dial
fn (t: Tx) copy_from(...) -> Int ! SqlErr uses net.dial       // the same, inside a transaction
```

- `rows` is called until it returns `None`; each row is encoded in
  COPY's text format (tab separated, `\N` for NULL, backslash escapes)
  and sent in CopyData messages of up to 64 KiB, then CopyDone; the
  result is the row count from CommandComplete. An error from `rows`
  sends CopyFail and returns that error; the server rolls the COPY back.
- `table` and `columns` are identifiers, quoted by the driver (the
  survey's "identifier quoting for SQL" gap, rank 13, gets a
  `sql.quote_ident` as a side effect); values are never interpolated.
- The in memory driver runs it as INSERTs; other drivers return
  `Unsupported` until they implement a new optional `DriverConn` method.
- Binary COPY (pgx's format) is later, if the text format measures
  slower than pgx.

### 3. Zip by entry (option Z1)

```
struct ZipReader {}                      // the central directory, read once
struct ZipFile { path: Str, size: Int, compressed: Int, mode: Int, is_dir: Bool, modified: Time }
pub fn zip_reader(data: Bytes) -> ZipReader ! ArchiveErr
pub fn zip_open(f: File, size: Int) -> ZipReader ! ArchiveErr uses fs.read
fn (r: ZipReader) files() -> List[ZipFile]
fn (r: ZipReader) read(zf: ZipFile) -> Bytes ! ArchiveErr uses fs.read      // one entry, checked (CRC)
fn (r: ZipReader) open(zf: ZipFile) -> EntryReader ! ArchiveErr uses fs.read
fn (var e: EntryReader) read(max: Int) -> Bytes? ! ArchiveErr uses fs.read  // inflated chunks

struct ZipWriter {}
pub fn zip_writer(f: File) -> ZipWriter
fn (var w: ZipWriter) add(e: Entry) -> () ! ArchiveErr uses fs.write
fn (var w: ZipWriter) create(path: Str, mode: Int = 0o644) -> EntryWriter ! ArchiveErr uses fs.write
                                         // modified at the Unix epoch, as archive.file
fn (var e: EntryWriter) write(data: Bytes) -> () ! ArchiveErr uses fs.write
fn (var w: ZipWriter) close() -> () ! ArchiveErr uses fs.write            // writes the directory
```

- Reading: `zip_reader` over Bytes keeps the archive in memory but
  inflates one entry at a time; `zip_open` over a File reads only the
  central directory and then each entry's bytes as asked, so an archive
  larger than memory can be walked. Both apply `unzip`'s rules (zip
  slip, links and devices refused) to each `ZipFile` and Go's errors.
- Writing: `add` writes an entry whose data is in hand (sizes and CRC in
  the local header); `create` streams one (a data descriptor after the
  data, as Go's `Writer.Create` does), and the next `create`, `add` or
  `close` ends it. Memory is the directory records, not the data.
- A new prelude method for positioned reads, used by `zip_open`:
  `fn (f: File) read_at(offset: Int, max: Int) -> Bytes? ! FsErr uses
  fs.read` (pread; Go's `os.File.ReadAt`), on every backend.
- `zip` and `unzip` stay, as `zip_writer` and `zip_reader` read whole.
  A streaming tar reader and writer are the same work for a later step.

## Options

Row cursor:

- **C1, `Rows` with `next` and `close`, plus `each` (proposed).** Go's
  shape, with `while let`; `each` for code that wants no `close`.
- **C2, only a callback (`each` / 0278's `query_each`).** Cannot leak,
  but cannot stop a loop early to return a value without an error type
  for it, and does not compose with another stream (a CSV writer that
  pulls rows).
- **C3, paged queries (LIMIT and OFFSET, or keyset) in a helper.** No
  driver change, but each page is a new query, OFFSET is quadratic, and
  results are not one snapshot outside a transaction.
- **C4, a server side cursor (`DECLARE ... CURSOR`, `FETCH n`).** Bounded
  batches without holding the socket mid-result; needs a transaction and
  a round trip per batch. Can be built on C1 by users.

COPY:

- **P1, a row callback in text format (proposed).**
- **P2, `copy_from(table, columns, rows: List[List[SqlValue]])`.** Simple,
  but the rows are in memory, which is the gap.
- **P3, a CopyWriter handle** (`w.write_row(...)`, `w.finish()`), the
  pull form inverted; useful when rows come from another stream's
  callback. Could be added on P1's driver code later.
- **P4, binary format first.** pgx's choice; faster to encode numbers and
  times but needs per-type binary encoders the driver does not have.

Zip:

- **Z1, a reader over Bytes or a File and a writer to a File (proposed).**
- **Z2, Bytes only.** No `read_at`; inflating by entry still cuts the
  `unzip` peak (all entries at once) to one entry, but the archive stays
  in memory.
- **Z3, a generic `ReaderAt` interface** like Go's. More general (an
  HTTP range reader, a memory buffer), but Xo has no std io interfaces
  today; a File and Bytes cover the survey's cases.

## Measurement plan

Peak memory (max RSS) is the claim, wall time the guard. Linux/arm64
host 1, `ab-host.sh` against the parent for Xo, the same machine for Go
twins (Go 1.26.1; database/sql with pgx's stdlib driver, pgx CopyFrom,
archive/zip), 5 rounds each; Postgres 17 as in scripts/pg-diff.sh.

1. **Cursor.** A table of 1,000,000 rows of about 100 bytes (id, two
   text columns, a timestamp), summed while read: `query` (today),
   `rows`/`next`, `rows`/`next_as`, `each`, and Go's `Rows.Next`/`Scan`.
   Expect the cursor's peak flat in the row count (run at 100,000 and
   1,000,000 too) and near Go's; `query`'s grows with it. Also the early
   close: stop after 10 rows of 1,000,000 and time `close`.
2. **COPY.** Load 1,000,000 rows: `exec` per row in one transaction,
   a 1,000-row multi-VALUES INSERT, `copy_from`, and pgx `CopyFrom`.
   Expect `copy_from` within 1.5x of pgx's binary COPY in wall time, at
   flat memory.
3. **Zip.** An archive of 2,000 files totaling about 500 MB (mixed text
   and incompressible data): `unzip`, `zip_reader` over Bytes, `zip_open`
   over a File reading each entry with `read` and with `open`/`read`
   (64 KiB chunks), and Go's `zip.OpenReader` with `File.Open` and
   `io.Copy`; writing the same entries with `zip`, `zip_writer.add`,
   `zip_writer.create` streaming from files, and Go's `zip.Writer`.
   Expect the File paths' peak to track the largest entry (or the chunk
   size for `open`) instead of the archive.
4. **Correctness.** Goldens on every backend: a cursor's rows equal
   `query`'s; an early close leaves the connection usable (the next
   query on the pool works); COPY text escaping round trips tabs,
   newlines, backslashes and NULL against `SELECT`; `zip_open` gives
   `unzip`'s entries and errors on internal/stdtest/archivefuzz_test.go's
   hostile archives; a streamed `create` archive opens with Go's reader
   and `unzip`.

## Consequences

- dataport-style jobs stream rows, bulk loads and archives in bounded
  memory; the survey's rank 9 gaps close except the tar streams.
- New public API in std/sql (`Rows`, `rows`, `each`, `copy_from`,
  `quote_ident`), std/archive (`ZipReader`, `ZipWriter`, entry readers
  and writers) and the prelude (`File.read_at`); spec/std.md 3.1, 10.3,
  10.7, 10.8 and 18; std/changes for sql, archive and prelude.
- The driver interface changes (only std's drivers implement it).
- Implementation deviation (2026-11-03, Lead agreed): `ZipWriter.create(path, mode)`
  returns `()` and `write(data)` feeds the writer's current entry, instead of
  returning an `EntryWriter`; a separate value cannot report its size to the
  writer, which needs it for the directory. `ZipWriter` is move only and
  `close` consumes it. `zip_reader` is built (Bytes); `zip_open` over a File,
  `File.read_at` and a streaming `EntryReader` follow.
- A `Rows` holds a connection: a slow consumer holds a pool permit for
  as long as it reads, as in Go.

## Alternatives

The options above; the proposal is C1, P1, Z1.
