# std/archive changes

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
