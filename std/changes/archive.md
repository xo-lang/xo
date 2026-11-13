# std/archive changes

## 2026-11-13: security: an old GNU sparse map is read to 1 MiB at most (untar, native)

- Kind: security
- Commit: this commit
- Found by: Go 1.26.11's archive/tar changes (Lang 2's toolchain bump list), reviewed against the port
- Go reference: archive/tar reader.go readOldGNUSparseMap (go1.26.11: maxSpecialFileSize, errSparseTooLong)
- Affected: Xo 0.50.1 and earlier, native backends: untar
- Test: std/native/archive_test.xo: 2047 extended blocks read, 2048 are "sparse map too long", as go1.27.4 (fails without the change)
- Timing: none
- Shipped in: not yet released

The extended blocks of an old GNU sparse header were read to the end of the chain however long; past 1 MiB of map (the header's 97 bytes and 512 a block) untar now stops with `archive/tar: sparse map too long`, as Go. No list was built, so this bounds work, not memory.

## 2026-11-10: refactor: integer bounds as `Int.max` and `Int.min` (decision 0587 point 4)

- Kind: refactor
- Commit: this commit
- Found by: decision 0587 point 4 (std's literal bounds become the type constants)
- Go reference: none (no behavior change; Go's math.MaxInt64 and MinInt64)
- Test: the std module tests and goldens of the gate; the self build

`9223372036854775807` and `-9223372036854775807 - 1` in std/archive's code are
`Int.max` and `Int.min`; masks, Float literals, comments and texts keep their
digits. No behavior change.

## 2026-11-04: feature: zip and zip_writer write zip64 as Go does

- Kind: feature
- Commit: this push
- Found by: the zip differential for the owner's v0.38.1 "not tested yet" list; decision 0499 (owner accepted 2026-11-04)
- Go reference: archive/zip Writer.Close (the zip64 end record and locator past 65,534 entries or 4 GB, the zip64 extra field for an entry past 4 GB or at an offset past it), fileWriter.close and writeDataDescriptor (reader version 45, 64-bit sizes in the data descriptor)
- Test: golden zip_entry_limit (65,534 to 65,537 entries, native bytes equal to the Go backend's), the zip differential harness against Go's reader
- Shipped in: not yet released

`zip` and `zip_writer` no longer refuse a 65,536th entry or an archive, entry or offset past 4 GB: they write zip64 records where Go's writer does, and the archive is byte for byte the Go backend's (Go's writer). An archive under the limits is unchanged.

## 2026-11-04: fix: zip refuses a 65,536th entry instead of writing a wrong count

- Kind: fix
- Commit: this push
- Found by: the zip differential for the owner's v0.38.1 "not tested yet" list (Xo Lang 2): native `zip` of 65,537 entries wrote 1 as the end record's count
- Go reference: archive/zip writes a zip64 end record past 65,534 entries or 4 GB; Xo writes no zip64 yet (std.md 18)
- Test: golden zip_entry_limit (65,535 entries, then the 65,536th, on every backend)
- Shipped in: not yet released

`zip` wrote the end record's 16-bit entry count modulo 65,536, so an archive of 65,537 entries said it held 1; Go's reader still read it (it compares the counts modulo 65,536), but a stricter reader does not. `zip` now refuses the 65,536th entry with `zip: more than 65535 entries (zip64 is not supported)` and an archive past 4 GB with `zip: archive larger than 4 GB (zip64 is not supported)`, the limits `zip_writer` already had natively. The Go backend's `zip` and `zip_writer`, which let Go write zip64, refuse at the same counts, so every backend gives the same result.

## 2026-11-04: fix: a NUL byte in an entry name is named, not "leaves the archive"

- Kind: fix
- Commit: this push
- Found by: the owner's v0.36.0 evaluation (darwin/amd64)
- Go reference: archive/zip and archive/tar take the name as it is; Xo's path rules (std.md 18) refuse a name with a NUL byte
- Test: archive_test.xo "unzip names a NUL byte, not the archive's root"
- Shipped in: not yet released

`unzip`, `untar` and `zip_reader` refused a name containing a NUL byte with `entry path "..." leaves the archive`. It now says `entry path "..." contains a NUL byte`; every other refused name keeps the old text.

## 2026-11-04: docs: a directory entry's data is empty whatever the archive stores

- Kind: docs
- Commit: this push
- Found by: the owner's v0.36.0 evaluation (the fuzz test: a zip entry whose Unix mode says directory but which has data)
- Go reference: archive/zip's Reader hands the entry's data to the caller
- Test: archive_test.xo "unzip keeps a directory's contract"
- Shipped in: not yet released

Decided: `Entry`'s contract stays (a directory's `data` is empty), so `unzip` of such an entry returns a directory with no data where Go would let the caller read the data. An error would refuse archives Go reads, and keeping the data would give a directory a body the writers ignore. The declaration's doc comment says so.

## 2026-11-04: perf: a ZipReader read no longer builds the fixed Huffman decoder

- Kind: perf
- Commit: this push
- Found by: the host 1 row zip_reader 1.147 of Go against unzip 0.995 for the same entries, and the Mac XO_PROF (flate.hdec_init 21 percent)
- Go reference: compress/flate's fixedHuffmanDecoder (built once, in a sync.Once)
- Test: corpora archive, compress, png on arm64 and the Go backend; flate_test.xo compares the constant with hdec_init
- Timing: host 1 A/B zip_reader 0.857 of before (0.848-0.879, n=6), about 0.95 of Go's wall
- Shipped in: not yet released

`flate.fixed_decoder()` is a constant 512 entry chunk table (no links: the fixed code has no code longer than 9 bits) in place of 288 pushes and an `hdec_init` on every call. `ZipReader.read` called it for each entry, and a small zip entry is often one fixed block; every gzip, zlib and zip reader call benefits.

## 2026-11-03: feature: zip_open (a zip archive read from a File)

- Kind: feature
- Commit: this commit
- Go reference: archive/zip (go1.26.1): `NewReader(io.ReaderAt, size)`
- Test: the archive corpus `zip_open reads a File as zip_reader reads the bytes` (every ZIP_CASES row) and `zip_open walks a large archive from the File`; a real file on macOS (2,000 entries) in a scratch program
- Timing: none yet

`zip_open(f, size)` reads the last 66 KiB, finds the end record and, if the directory lies before it, reads from the directory's start (the zip64 and prepended-data rules of `zip_directory_at` unchanged); `ZipReader.read` then reads one entry's local header and bytes with `File.read_at`. `ZipReader.read` `uses fs.read`.

## 2026-11-03: feature: zip_writer

- Kind: feature
- Commit: this commit
- Go reference: archive/zip (go1.26.1): `Writer.CreateHeader`, `Writer.Close`
- Test: the archive corpus `zip_writer writes what unzip and zip_reader read` and `zip_writer refuses a bad path and a write without create` (arm64, LLVM, Go backend)
- Timing: none yet (the writer is bound by the File)

`zip_writer(f)` writes local header, deflated data and data descriptor per entry to the File and the central directory at `close`. `zip` shares the header, descriptor and directory builders with it (`zip_new_header`, `zip_local_header`, `zip_descriptor`, `zip_central`), with the same bytes as before. The streamed entry is the writer's current one (`create` then `write`), not a separate `EntryWriter` as the decision sketched.

## 2026-11-03: feature: zip_reader, ZipReader.files and ZipReader.read

- Kind: feature
- Commit: this commit
- Go reference: archive/zip (go1.26.1): `NewReader`, `Reader.File`, `File.Open`
- Test: the archive corpus: `zip_reader reads one entry at a time what unzip reads` (every ZIP_CASES row and every ZIP_REFUSED row against `unzip`) and `zip_reader lists sizes and kinds before inflating`, on arm64, LLVM and the Go backend
- Timing: host row follows in std-twin-ziprd-2026-11-03.md (bench/perf/archive, workload zip_reader)

`zip_reader(data)` reads the central directory and applies `unzip`'s path and kind rules to every entry without inflating any; `files()` lists them (path, sizes, mode, time) and `read(f)` inflates one and checks its CRC. `unzip` is now the same two steps in a loop (`zip_directory`, `zip_meta`, `zip_read`), with unchanged results and errors. The native module's own header struct is `ZipHdr` (it was `ZipFile`, the public type's name).

## 2026-11-03: perf: zip writes every file with one flate stream

- Kind: perf
- Commit: this commit
- Found by: bench/perf/archive zip 1.35 of Go's wall on host 1 (bench/results/std-twin-rows-2026-11-03.md)
- Go reference: archive/zip writer.go (its compressor Writers come from a sync.Pool), compress/flate deflate.go reset (go1.26.1)
- Test: internal/stdtest/testdata/corpus/archive and archivefuzz (go, arm64, LLVM), TestArchiveFuzz

`archive.zip` deflates all files with one flate Stream, reset between them, instead of a new Stream
(640 KB of hash tables) for each. The archive bytes are the same. Quiet `xo bench --ab` on host 1, 15
rounds, Xo / Go wall of zip: 1.352 to 1.203 (with the compress change above; the unused-tables change
alone gave 1.229).

## 2026-11-02: fix: untar and unzip of hostile archives no longer fault, and read as Go does

- Kind: fix, security
- Commit: this commit
- Found by: the archive fuzz harness (internal/stdtest/archivefuzz_test.go: archives from Go's tar and zip writers and hand-built tar headers, cut short or with fields changed, against the Go backend's untar and unzip), seeds 1 to 16
- Go reference: archive/zip reader.go (Reader.init, readDirectoryHeader, findBodyOffset, the NTFS extra time, unixModeToFileMode), archive/tar reader.go (Reader.next's global header, tryReadFull, readSpecialFile, readOldGNUSparseMap, validateSparseEntries), strings.Reader.ReadAt, time.Time.UnixNano (go1.26.1)
- Test: internal/stdtest/testdata/corpus/archivefuzz (2,010 cases: seed 1's and the later seeds' cases that found these) on arm64, LLVM and the Go backend; TestArchiveFuzz seeds 2 to 16; internal/llvm and internal/arm64 archive goldens; std/native archive module tests
- Timing: none (a correctness and security fix; the extra checks run once per header or directory entry)
- Shipped in: v0.11.0 (dbd0ec952, the native port: the zip local header read with no check for a negative offset, the tar, PAX and NTFS time conversions, the zip file type rule, the global header skip, the padding and special-header reads, the raw-name quoting) through v0.35.0; the zip directory count's `unexpected EOF` reply from v0.24.0 (004dd2d45) through v0.35.0; fixed in v0.36.0 (943e349c5)

On native builds an archive could crash the program instead of returning
an error: a zip whose central directory, with the base offset Go
computes, put a local header before the start of the data (an index out
of range; Go: `strings.Reader.ReadAt: negative offset`), a tar or PAX
modification time past the year 2262 or a zip NTFS time with its top bit
set (an integer overflow; Go wraps, as Time.UnixNano does). Each now
gives Go's answer, and a 64-bit field is read as Go's int64(uint64) with
a wrapping shift. The rest gave a
different answer from Go:

- a tar archive that ends inside an entry's padding, or without its two
  zero blocks after a header's padding, was `unexpected EOF`; Go reads
  the padding with tryReadFull, whose short read is the end of the
  archive, so the entries before it are returned;
- a PAX global header (`g`) was skipped before its name was checked; Go
  returns it from Next under its own name (or its own `path` record), so
  a global header named `.`, empty, or `../up` is the path error, and the
  records and long name read before it no longer apply to the next entry;
- an old GNU sparse header (`S`) in a non-GNU archive, or with an
  invalid sparse map, is `invalid tar header` before the type check;
- a PAX or long-name header claiming more than 1 MiB with fewer bytes
  left is `unexpected EOF`, not `header field too long`;
- a zip whose entry count disagrees with its directory now gives the
  reason the directory stopped (`zip: not a valid zip file` for a bad
  signature, `EOF` when nothing is left), not always `unexpected EOF`; a
  local header cut short is `EOF`;
- a zip entry name that is not UTF-8 is quoted in errors as Go quotes its
  bytes (`\xNN`);
- a zip entry with an unknown Unix file type in its attributes is a
  regular file, as in Go.

## 2026-10-28: perf: CRC-32 a word a step with arm64's crc32x

- Kind: perf
- Commit: this commit
- Found by: bench/perf/archive unzip at 1.41x Go and compress gunzip at 1.23x (measured on main 55af0adc5); unzip's profile: flate.crc32 13%
- Go reference: hash/crc32 crc32_arm64.go (ieeeUpdate with the CRC32X instructions) (go1.26.1)
- Test: std/native/llrt/flate_test.xo (zlib's IEEE values, whole and split at every offset, crc32_list) and cpu_test.xo (crc32x against its body) on arm64 and llvm; internal/llvm and internal/arm64 archive_formats and http_gzip with Go parity; linux/arm64 in Docker (the AT_HWCAP path); fixed point; test/codegen/intrinsics flate.crc32

std/compress, std/archive and std/http's gzip share flate.crc32: where
the CPU has arm64's CRC32 instructions (every Apple arm64 CPU; Linux's
AT_HWCAP bit), it takes eight bytes a step with one crc32x (cpu.crc32x,
decision 0231's addendum of 2026-10-28), and keeps slicing-by-8
elsewhere (amd64, whose crc32 instruction is the Castagnoli polynomial,
and wasm). Results unchanged. Quiet --ab, 15 rounds, the parent's xo and
std as A: compress gunzip 0.949 (0.941 to 0.953), instructions 0.956;
archive unzip 0.893 (0.888 to 0.902), instructions 0.915. Xo / Go about
1.17 for gunzip (estimate: 1.228 x 0.949) and 1.26 for unzip (1.406 x
0.893).

## 2026-10-21: fix: untar and unzip read as Go's archive/tar and archive/zip

- Kind: fix
- Commit: 004dd2d4
- Found by: differential test against Go
- Go reference: archive/tar Reader (parsePAX, validPAXRecord, readHeader's pre-Go 1.8 prefix, readGNUSparsePAXHeaders, readGNUSparseMap1x0 and 0x1, the sparse reader), archive/zip readDirectoryEnd, readDirectory64End, readDirectoryHeader's zip64 extra field (go1.26.1)
- Test: std/native/archive_test.xo (395 cases: gzip of generated data at 84 sizes and alphabets, gunzip of 150 streams of every level with corrupted, concatenated, and trailing-junk ones, tar and zip of 100 entry lists, and untar or unzip of Go's 61 small test archives)

Native builds read some archives differently from Go: a PAX path with
a zero byte, and a PAX header whose records were malformed at the end
of the archive, were accepted (Go: `archive/tar: invalid tar header`);
a header that Go 1.7 wrote with a USTAR prefix in a GNU header lost
the prefix; GNU sparse files described by PAX records (formats 0.0,
0.1, and 1.0) came out as their raw data under the `GNUSparseFile.0/`
name; and zip64 archives were an error. Each now gives Go's entries
or Go's error. gzip, gunzip, tar, and zip had no other difference.

## 2026-10-24: fix: a zero byte in an entry path is refused

- Kind: fix
- Found by: differential fuzzing against the Go backend
- Go reference: archive/tar Writer (validPAXRecord refuses a NUL in a path record)
- Test: internal/llvm/testdata/golden/archive_formats (nul path); unzip of a NUL name is by the shared archive_path

`archive.tar` wrote an entry path with a zero byte (`"a\0b"`) into the
header on native builds, and failed on the Go backend; `zip` and `unzip`
accepted it on every backend (Go's own zip does too). A zero byte is now
part of the entry-path rule, beside empty, absolute, and `../` paths
(a C API truncates the name at it): tar and zip return `archive: bad
entry path`, and unzip returns `entry path ... leaves the archive`, on
every backend. This is stricter than Go's zip, as the path rule already is.
