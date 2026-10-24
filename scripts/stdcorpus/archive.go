package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// The API this corpus covers, as std/archive.xo declares it: untar and unzip
// over Entry{path, data, mode, is_dir, modified}, with the path rules those
// two state (an absolute path, one leaving the root, and links and devices
// are errors).
//
// The archives are fixtures this generator writes, and the expectation for
// each is Go's own reading of that same archive through archive/tar and
// archive/zip: path, size, permission bits, whether it is a directory, the
// modified time and the SHA-256 of the data, for every entry in order. So the
// writer side of Xo's archive is not what is tested here (that is std's own
// tests and goldens); the reader is, against the formats Go writes and the
// tricky shapes the two readers have to agree on: PAX and GNU long paths,
// the ustar prefix field, a PAX record a reader must ignore, zip data
// descriptors (which Go's zip.Writer always writes for a file), a UTF-8 name,
// a comment, and a zip64 archive, which Go's writer cannot produce for a
// small file and so the generator lays out by hand and then proves Go reads.
// Go's archive tests are Copyright 2009 The Go Authors, BSD-style license.

// archEntry is one entry of a fixture, as the writer and the expectation use
// it.
type archEntry struct {
	path  string
	data  string
	mode  int64
	mtime time.Time
	typ   byte // tar Typeflag; '5' is a directory
	link  string
}

func archTime(sec int64) time.Time { return time.Unix(sec, 0).UTC() }

// tarFixture writes the entries as a tar archive in format.
func tarFixture(format tar.Format, entries []archEntry) []byte {
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, e := range entries {
		h := &tar.Header{
			Name:     e.path,
			Mode:     e.mode,
			ModTime:  e.mtime,
			Typeflag: e.typ,
			Linkname: e.link,
			Format:   format,
		}
		if e.typ == tar.TypeReg {
			h.Size = int64(len(e.data))
		}
		if err := w.WriteHeader(h); err != nil {
			fmt.Fprintln(os.Stderr, "stdcorpus: writing a tar fixture:", err)
			os.Exit(1)
		}
		if e.typ == tar.TypeReg {
			if _, err := w.Write([]byte(e.data)); err != nil {
				fmt.Fprintln(os.Stderr, "stdcorpus: writing a tar fixture:", err)
				os.Exit(1)
			}
		}
	}
	if err := w.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: writing a tar fixture:", err)
		os.Exit(1)
	}
	return b.Bytes()
}

// tarSummary is Go's reading of a tar archive in the corpus's shape.
func tarSummary(data []byte) (string, error) {
	tr := tar.NewReader(bytes.NewReader(data))
	var lines []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return "", err
		}
		// A PAX global header describes the archive, not an entry, and untar
		// skips it (std.md 18); Go's Reader hands it back as an entry named
		// GlobalHead.0.0, so it is dropped here to read like Xo does.
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		// Xo reads a directory's path without the trailing slash (std.md 18:
		// "Directories are written with a trailing / and read without it"),
		// where Go's Reader keeps the slash it wrote. That is the one place
		// the two disagree, and it is the spec's.
		name := strings.TrimSuffix(h.Name, "/")
		lines = append(lines, fmt.Sprintf("%s\t%d\t%o\t%v\t%s\t%s",
			name, len(body), h.Mode&0o7777, h.Typeflag == tar.TypeDir,
			h.ModTime.UTC().Format(time.RFC3339Nano), archSum(body)))
	}
	return strings.Join(lines, "\n"), nil
}

func archSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// zipFixture writes the entries as a zip archive. Go's writer always sets the
// data-descriptor flag for a file, so every one of these exercises it.
func zipFixture(entries []archEntry) []byte {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, e := range entries {
		fh := &zip.FileHeader{Name: e.path, Method: zip.Deflate}
		if e.typ == '5' {
			fh.Name = e.path + "/"
			fh.Method = zip.Store
		}
		fh.SetMode(os.FileMode(e.mode))
		fh.SetModTime(e.mtime)
		fw, err := w.CreateHeader(fh)
		if err != nil {
			fmt.Fprintln(os.Stderr, "stdcorpus: writing a zip fixture:", err)
			os.Exit(1)
		}
		if e.typ == '5' {
			continue
		}
		if _, err := fw.Write([]byte(e.data)); err != nil {
			fmt.Fprintln(os.Stderr, "stdcorpus: writing a zip fixture:", err)
			os.Exit(1)
		}
	}
	if err := w.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: writing a zip fixture:", err)
		os.Exit(1)
	}
	return b.Bytes()
}

// zipSummary is Go's reading of a zip archive in the corpus's shape.
func zipSummary(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var lines []string
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", err
		}
		// as in tarSummary: a directory path has no trailing slash
		lines = append(lines, fmt.Sprintf("%s\t%d\t%o\t%v\t%s\t%s",
			strings.TrimSuffix(f.Name, "/"), len(body), f.Mode().Perm(), f.FileInfo().IsDir(),
			f.Modified.UTC().Format(time.RFC3339Nano), archSum(body)))
	}
	return strings.Join(lines, "\n"), nil
}

// zip64Fixture lays out a zip archive with one stored file whose sizes and
// offsets are all in zip64 extra fields, which Go's zip.Writer writes only
// for values that do not fit 4 bytes and so cannot produce for a small file.
// The generator proves Go's reader takes it (zipSummary must succeed).
func zip64Fixture(name, data string, mtime time.Time) []byte {
	var b bytes.Buffer
	put16 := func(v int) { b.Write([]byte{byte(v), byte(v >> 8)}) }
	put32 := func(v uint32) { b.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}) }
	put64 := func(v uint64) {
		for i := 0; i < 8; i++ {
			b.WriteByte(byte(v >> (8 * i)))
		}
	}
	dos := func(t time.Time) (uint16, uint16) {
		t = t.UTC()
		d := uint16(t.Day()) | uint16(t.Month())<<5 | uint16(t.Year()-1980)<<9
		s := uint16(t.Second()/2) | uint16(t.Minute())<<5 | uint16(t.Hour())<<11
		return s, d
	}
	crc := crc32.ChecksumIEEE([]byte(data))
	dosTime, dosDate := dos(mtime)
	// local file header, with a zip64 extra for both sizes
	loc := b.Len()
	put32(0x04034b50)
	put16(45) // version needed: zip64
	put16(0)  // flags
	put16(0)  // method: store
	put16(int(dosTime))
	put16(int(dosDate))
	put32(crc)
	put32(0xFFFFFFFF) // compressed size: in the extra
	put32(0xFFFFFFFF) // uncompressed size: in the extra
	put16(len(name))
	put16(20) // extra: header 4 + 8 + 8
	b.WriteString(name)
	put16(0x0001)
	put16(16)
	put64(uint64(len(data)))
	put64(uint64(len(data)))
	b.WriteString(data)
	// central directory, with a zip64 extra for both sizes and the offset
	cd := b.Len()
	put32(0x02014b50)
	put16(45) // version made by
	put16(45) // version needed
	put16(0)
	put16(0)
	put16(int(dosTime))
	put16(int(dosDate))
	put32(crc)
	put32(0xFFFFFFFF)
	put32(0xFFFFFFFF)
	put16(len(name))
	put16(28) // extra: header 4 + 8 + 8 + 8
	put16(0)  // comment
	put16(0)  // disk
	put16(0)  // internal attributes
	put32(0)  // external attributes
	put32(0xFFFFFFFF)
	b.WriteString(name)
	put16(0x0001)
	put16(24)
	put64(uint64(len(data)))
	put64(uint64(len(data)))
	put64(uint64(loc))
	cdSize := b.Len() - cd
	cdOff := cd
	// zip64 end of central directory record, its locator, and the record
	z64 := b.Len()
	put32(0x06064b50)
	put64(44)
	put16(45)
	put16(45)
	put32(0)
	put32(0)
	put64(1)
	put64(1)
	put64(uint64(cdSize))
	put64(uint64(cdOff))
	put32(0x07064b50)
	put32(0)
	put64(uint64(z64))
	put32(1)
	put32(0x06054b50)
	put16(0)
	put16(0)
	put16(0xFFFF)
	put16(0xFFFF)
	put32(0xFFFFFFFF)
	put32(0xFFFFFFFF)
	put16(0)
	return b.Bytes()
}

// paxRecord is one PAX record: "<len> key=value\n", where len counts the
// whole record including its own digits (Go's formatPAXRecord).
func paxRecord(k, v string) string {
	body := " " + k + "=" + v + "\n"
	n := len(body)
	for d := 1; ; d++ {
		if len(strconv.Itoa(n+d))+len(body) == n+d {
			return strconv.Itoa(n+d) + body
		}
	}
}

// tarBlock is one ustar header block, with the checksum the format wants.
func tarBlock(name string, mode, size int64, mtime time.Time, typ byte) []byte {
	b := make([]byte, 512)
	copy(b[0:100], name)
	putOctal := func(off, n int, v int64) {
		s := fmt.Sprintf("%0*o", n-1, v)
		if len(s) > n-1 {
			s = s[len(s)-(n-1):]
		}
		copy(b[off:off+n-1], s)
		b[off+n-1] = 0
	}
	putOctal(100, 8, mode)
	putOctal(108, 8, 0)
	putOctal(116, 8, 0)
	putOctal(124, 12, size)
	putOctal(136, 12, mtime.Unix())
	b[156] = typ
	copy(b[257:263], "ustar\x00")
	copy(b[263:265], "00")
	copy(b[265:297], "root")
	copy(b[297:329], "root")
	for i := 148; i < 156; i++ {
		b[i] = ' '
	}
	sum := 0
	for _, c := range b {
		sum += int(c)
	}
	putOctal(148, 7, int64(sum))
	b[155] = ' '
	return b
}

// pad512 rounds a payload up to a block.
func pad512(b []byte) []byte {
	out := make([]byte, (len(b)+511)/512*512)
	copy(out, b)
	return out
}

// sparseArchive is the logical content of both sparse fixtures: 8 bytes at 0,
// 4 at 1024, and a logical size of 2048, so 2036 bytes of holes are zeros.
func sparseContent() []byte {
	want := make([]byte, 2048)
	copy(want[0:], "ABCDEFGH")
	copy(want[1024:], "WXYZ")
	return want
}

// sparse01Fixture is a GNU PAX sparse archive in format 0.1: the map lives in
// GNU.sparse.offset and GNU.sparse.numbytes records (the shape Go converts
// from the 0.0 headers it lists), and the data area holds only the two
// blocks, so the header's size is the physical 12 bytes while the entry reads
// as 2048.
func sparse01Fixture() []byte {
	phys := "ABCDEFGH" + "WXYZ"
	pax := paxRecord("GNU.sparse.size", "2048") +
		paxRecord("GNU.sparse.numblocks", "2") +
		paxRecord("GNU.sparse.offset", "0") +
		paxRecord("GNU.sparse.numbytes", "8") +
		paxRecord("GNU.sparse.offset", "1024") +
		paxRecord("GNU.sparse.numbytes", "4") +
		paxRecord("GNU.sparse.name", "sparse.bin")
	var b bytes.Buffer
	b.Write(tarBlock("./PaxHeaders.0/sparse.bin", 0o644, int64(len(pax)), archTime(1700000000), 'x'))
	b.Write(pad512([]byte(pax)))
	b.Write(tarBlock("GNUSparseFile.0/sparse.bin", 0o644, int64(len(phys)), archTime(1700000000), '0'))
	b.Write(pad512([]byte(phys)))
	b.Write(make([]byte, 1024))
	return b.Bytes()
}

// sparse10Fixture is a GNU PAX sparse archive in format 1.0: GNU.sparse.major
// 1 and minor 0, and the map as the first block of the data area ("<n>" then
// pairs of offset and length), then the blocks themselves.
func sparse10Fixture() []byte {
	phys := "ABCDEFGH" + "WXYZ"
	sparseMap := "2\n0\n8\n1024\n4\n"
	body := pad512([]byte(sparseMap))
	body = append(body, phys...)
	pax := paxRecord("GNU.sparse.major", "1") +
		paxRecord("GNU.sparse.minor", "0") +
		paxRecord("GNU.sparse.name", "sparse.bin") +
		paxRecord("GNU.sparse.realsize", "2048")
	var b bytes.Buffer
	b.Write(tarBlock("./PaxHeaders.0/sparse.bin", 0o644, int64(len(pax)), archTime(1700000000), 'x'))
	b.Write(pad512([]byte(pax)))
	b.Write(tarBlock("GNUSparseFile.0/sparse.bin", 0o644, int64(len(body)), archTime(1700000000), '0'))
	b.Write(pad512(body))
	b.Write(make([]byte, 1024))
	return b.Bytes()
}

// checkSparse reads a sparse fixture the way Go does and requires one entry
// named sparse.bin whose data is the 2048 bytes with the two blocks, so a
// fixture whose holes Go does not fill (or whose map it does not read) never
// reaches the corpus.
func checkSparse(name string, data []byte) {
	tr := tar.NewReader(bytes.NewReader(data))
	h, err := tr.Next()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdcorpus: Go cannot read the sparse fixture %s: %v\n", name, err)
		os.Exit(1)
	}
	body, err := io.ReadAll(tr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdcorpus: Go cannot read the sparse fixture %s: %v\n", name, err)
		os.Exit(1)
	}
	want := sparseContent()
	if h.Name != "sparse.bin" || !bytes.Equal(body, want) {
		fmt.Fprintf(os.Stderr, "stdcorpus: Go reads the sparse fixture %s as %q with %d bytes (want sparse.bin with %d)\n",
			name, h.Name, len(body), len(want))
		os.Exit(1)
	}
	if _, err := tr.Next(); err != io.EOF {
		fmt.Fprintf(os.Stderr, "stdcorpus: the sparse fixture %s holds more than one entry\n", name)
		os.Exit(1)
	}
}

func archiveCorpus() string {
	var o strings.Builder
	o.WriteString(header("archive/tar and archive/zip reading the fixtures this generator writes with Go's writers (and, for zip64, lays out by hand), compared entry by entry: path, size, permission bits, whether a directory, the modified time and the SHA-256 of the data, plus the archives Xo's path rules refuse (an absolute path, one leaving the root, a symbolic link, a hard link). Go's archive tests are Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/archive\nuse std/crypto\n")

	type fixture struct {
		name  string
		data  []byte
		kind  string // "tar" or "zip"
		ok    bool
		why   string // for a refused archive
		entry string // how it was made, for the comment
	}
	t1 := archTime(1700000000) // 2023-11-14T22:13:20Z, even seconds for zip
	fixtures := []fixture{
		{name: "TAR_PLAIN", kind: "tar", ok: true, entry: "USTAR: a directory, two files and an empty one",
			data: tarFixture(tar.FormatUSTAR, []archEntry{
				{path: "dir/", mode: 0o755, mtime: t1, typ: tar.TypeDir},
				{path: "dir/a.txt", data: "hello\n", mode: 0o644, mtime: t1, typ: tar.TypeReg},
				{path: "dir/b.bin", data: "\x00\x01\x02\xff", mode: 0o600, mtime: t1, typ: tar.TypeReg},
				{path: "empty", data: "", mode: 0o644, mtime: t1, typ: tar.TypeReg},
			})},
		{name: "TAR_LONG_PAX", kind: "tar", ok: true, entry: "PAX: a path of 150 bytes and a nanosecond time",
			data: tarFixture(tar.FormatPAX, []archEntry{
				{path: "deep/" + strings.Repeat("d", 60) + "/" + strings.Repeat("e", 80) + "/file.txt", data: "long\n", mode: 0o644, mtime: t1.Add(123456789 * time.Nanosecond), typ: tar.TypeReg},
			})},
		{name: "TAR_USTAR_PREFIX", kind: "tar", ok: true, entry: "USTAR: a 150 byte path split over the prefix field",
			data: tarFixture(tar.FormatUSTAR, []archEntry{
				{path: strings.Repeat("p", 60) + "/" + strings.Repeat("q", 80) + "/f", data: "prefix\n", mode: 0o644, mtime: t1, typ: tar.TypeReg},
			})},
		{name: "TAR_GNU_LONG", kind: "tar", ok: true, entry: "GNU: a 140 byte path in a long name entry",
			data: tarFixture(tar.FormatGNU, []archEntry{
				{path: "gnu/" + strings.Repeat("g", 130), data: "gnu\n", mode: 0o644, mtime: t1, typ: tar.TypeReg},
			})},
		{name: "TAR_PAX_EXTRA", kind: "tar", ok: true, entry: "PAX: a record a reader must ignore next to a long path",
			data: func() []byte {
				var b bytes.Buffer
				w := tar.NewWriter(&b)
				h := &tar.Header{Name: strings.Repeat("x", 120) + "/f", Mode: 0o644, ModTime: t1,
					Typeflag: tar.TypeReg, Size: 4, Format: tar.FormatPAX,
					PAXRecords: map[string]string{"SCHILY.xattr.user.k": "v"}}
				if err := w.WriteHeader(h); err != nil {
					fmt.Fprintln(os.Stderr, "stdcorpus: writing a pax fixture:", err)
					os.Exit(1)
				}
				w.Write([]byte("pax\n"))
				if err := w.Close(); err != nil {
					fmt.Fprintln(os.Stderr, "stdcorpus: writing a pax fixture:", err)
					os.Exit(1)
				}
				return b.Bytes()
			}()},
		{name: "TAR_PAX_GLOBAL", kind: "tar", ok: true, entry: "PAX: a global header holding a comment, then a file (git archive's shape); the header describes the archive, so untar skips it",
			data: func() []byte {
				var b bytes.Buffer
				w := tar.NewWriter(&b)
				g := &tar.Header{Typeflag: tar.TypeXGlobalHeader, Format: tar.FormatPAX,
					PAXRecords: map[string]string{"comment": "a git archive"}}
				if err := w.WriteHeader(g); err != nil {
					fmt.Fprintln(os.Stderr, "stdcorpus: writing a pax global header:", err)
					os.Exit(1)
				}
				h := &tar.Header{Name: "f.txt", Mode: 0o644, ModTime: t1, Typeflag: tar.TypeReg,
					Size: 3, Format: tar.FormatPAX}
				if err := w.WriteHeader(h); err != nil {
					fmt.Fprintln(os.Stderr, "stdcorpus: writing a pax global header:", err)
					os.Exit(1)
				}
				w.Write([]byte("hi\n"))
				if err := w.Close(); err != nil {
					fmt.Fprintln(os.Stderr, "stdcorpus: writing a pax global header:", err)
					os.Exit(1)
				}
				return b.Bytes()
			}()},
		{name: "TAR_SPARSE_01", kind: "tar", ok: true, entry: "GNU PAX sparse 0.1: two blocks in a 2048 byte file, so 2036 bytes of holes; the map is in GNU.sparse.offset and GNU.sparse.numbytes records and the header's size is the physical 12 bytes",
			data: sparse01Fixture()},
		{name: "TAR_SPARSE_10", kind: "tar", ok: true, entry: "GNU PAX sparse 1.0: the same two blocks, with the map as the first block of the data area and GNU.sparse.realsize for the logical size",
			data: sparse10Fixture()},
		{name: "TAR_EMPTY", kind: "tar", ok: true, entry: "USTAR: no entries",
			data: tarFixture(tar.FormatUSTAR, nil)},
		{name: "TAR_SYMLINK", kind: "tar", ok: false, entry: "USTAR: a symbolic link", why: "links are an error (std/archive.xo)",
			data: tarFixture(tar.FormatUSTAR, []archEntry{
				{path: "link", mode: 0o777, mtime: t1, typ: tar.TypeSymlink, link: "dir/a.txt"},
			})},
		{name: "TAR_HARDLINK", kind: "tar", ok: false, entry: "USTAR: a hard link", why: "links are an error (std/archive.xo)",
			data: tarFixture(tar.FormatUSTAR, []archEntry{
				{path: "hard", mode: 0o644, mtime: t1, typ: tar.TypeLink, link: "dir/a.txt"},
			})},
		{name: "TAR_ABSOLUTE", kind: "tar", ok: false, entry: "USTAR: /etc/passwd", why: "an absolute path is an error (std/archive.xo)",
			data: tarFixture(tar.FormatUSTAR, []archEntry{
				{path: "/etc/passwd", data: "x", mode: 0o644, mtime: t1, typ: tar.TypeReg},
			})},
		{name: "TAR_DOTDOT", kind: "tar", ok: false, entry: "USTAR: ../out", why: "a path leaving the root is an error (std/archive.xo)",
			data: tarFixture(tar.FormatUSTAR, []archEntry{
				{path: "../out", data: "x", mode: 0o644, mtime: t1, typ: tar.TypeReg},
			})},
		{name: "ZIP_PLAIN", kind: "zip", ok: true, entry: "a directory, a deflated file, a UTF-8 name and an empty file",
			data: zipFixture([]archEntry{
				{path: "dir", mode: 0o755, mtime: t1, typ: '5'},
				{path: "dir/a.txt", data: "hello\n", mode: 0o644, mtime: t1, typ: '0'},
				{path: "naïve/日本語.txt", data: "unicode\n", mode: 0o644, mtime: t1, typ: '0'},
				{path: "empty", data: "", mode: 0o644, mtime: t1, typ: '0'},
			})},
		{name: "ZIP_BIGGER", kind: "zip", ok: true, entry: "a file long enough that deflate does real work",
			data: zipFixture([]archEntry{
				{path: "text.txt", data: strings.Repeat("the quick brown fox jumps over the lazy dog\n", 40), mode: 0o644, mtime: t1, typ: '0'},
			})},
		{name: "ZIP64", kind: "zip", ok: true, entry: "a stored file with zip64 sizes and offset",
			data: zip64Fixture("big/name.txt", "zip64\n", t1)},
		{name: "ZIP_EMPTY", kind: "zip", ok: true, entry: "no entries", data: zipFixture(nil)},
		{name: "ZIP_ABSOLUTE", kind: "zip", ok: false, entry: "/etc/passwd", why: "an absolute path is an error (std/archive.xo)",
			data: zipFixture([]archEntry{{path: "/etc/passwd", data: "x", mode: 0o644, mtime: t1, typ: '0'}})},
		{name: "ZIP_DOTDOT", kind: "zip", ok: false, entry: "../out", why: "a path leaving the root is an error (std/archive.xo)",
			data: zipFixture([]archEntry{{path: "../out", data: "x", mode: 0o644, mtime: t1, typ: '0'}})},
	}

	var okTar, okZip, badTar, badZip []fixture
	for _, f := range fixtures {
		var summary string
		var err error
		if f.kind == "tar" {
			summary, err = tarSummary(f.data)
		} else {
			summary, err = zipSummary(f.data)
		}
		if f.name == "TAR_SPARSE_01" || f.name == "TAR_SPARSE_10" {
			checkSparse(f.name, f.data)
		}
		if err != nil {
			// every fixture must be readable by Go; the refusals are Xo's
			// path rules over an archive Go reads fine
			fmt.Fprintf(os.Stderr, "stdcorpus: Go cannot read the %s fixture %s: %v\n", f.kind, f.name, err)
			os.Exit(1)
		}
		if f.ok && summary == "" && f.name != "TAR_EMPTY" && f.name != "ZIP_EMPTY" {
			fmt.Fprintf(os.Stderr, "stdcorpus: the %s fixture %s reads empty\n", f.kind, f.name)
			os.Exit(1)
		}
		if f.ok {
			if f.kind == "tar" {
				okTar = append(okTar, f)
			} else {
				okZip = append(okZip, f)
			}
		} else if f.kind == "tar" {
			badTar = append(badTar, f)
		} else {
			badZip = append(badZip, f)
		}
		fmt.Fprintf(&o, "\n/// %s\nconst %s: Str = %s\n", f.entry, f.name, xq(hex.EncodeToString(f.data)))
	}

	// The expected summaries, computed by Go's reader on the same bytes.
	o.WriteString("\n/// (fixture, Go's reading of it: path, size, octal mode, is_dir, modified, SHA-256 of the data; entries by newline)\nconst TAR_CASES: List[(Str, Str)] = [\n")
	for _, f := range okTar {
		s, _ := tarSummary(f.data)
		fmt.Fprintf(&o, "  (%s, %s),\n", f.name, xq(s))
	}
	o.WriteString("]\n\n/// (fixture, Go's reading of it, the same way as TAR_CASES)\nconst ZIP_CASES: List[(Str, Str)] = [\n")
	for _, f := range okZip {
		s, _ := zipSummary(f.data)
		fmt.Fprintf(&o, "  (%s, %s),\n", f.name, xq(s))
	}
	o.WriteString("]\n\n/// (fixture, why Xo refuses it: Go reads all of these, and std/archive.xo's path rules refuse them)\nconst TAR_REFUSED: List[(Str, Str)] = [\n")
	for _, f := range badTar {
		fmt.Fprintf(&o, "  (%s, %s),\n", f.name, xq(f.why))
	}
	o.WriteString("]\n\n/// (fixture, why Xo refuses it, as TAR_REFUSED)\nconst ZIP_REFUSED: List[(Str, Str)] = [\n")
	for _, f := range badZip {
		fmt.Fprintf(&o, "  (%s, %s),\n", f.name, xq(f.why))
	}
	o.WriteString("]\n")

	o.WriteString(`
fn show3(bad: List[(Str, Str, Str)]) -> List[(Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

/// The corpus's shape of a list of entries, the same shape the generator's Go
/// side takes: path, size, octal mode, is_dir, modified, SHA-256 of the data.
fn summarize(entries: List[archive.Entry]) -> Str {
  var out = ""
  for e in entries {
    if out != "" {
      out = out + "\n"
    }
    out = out + "${e.path}\t${e.data.len()}\t${e.mode.to_str_base(8)}\t${e.is_dir}\t${e.modified}\t${crypto.sha256(e.data).to_hex()}"
  }
  out
}

test "untar reads what Go's archive/tar reads" {
  var bad: List[(Str, Str, Str)] = []
  for (hex_text, want) in TAR_CASES {
    let data = Bytes.from_hex(hex_text) ?? b""
    let got = match archive.untar(data) {
      Ok(entries) => summarize(entries)
      Err(e) => "!error ${e.display()}"
    }
    if got != want {
      bad.push((hex_text[0..16], got, want))
    }
  }
  expect(show3(bad)) == []
}

test "unzip reads what Go's archive/zip reads" {
  var bad: List[(Str, Str, Str)] = []
  for (hex_text, want) in ZIP_CASES {
    let data = Bytes.from_hex(hex_text) ?? b""
    let got = match archive.unzip(data) {
      Ok(entries) => summarize(entries)
      Err(e) => "!error ${e.display()}"
    }
    if got != want {
      bad.push((hex_text[0..16], got, want))
    }
  }
  expect(show3(bad)) == []
}

test "untar refuses what std/archive.xo says it refuses" {
  var bad: List[(Str, Str, Str)] = []
  for (hex_text, why) in TAR_REFUSED {
    let data = Bytes.from_hex(hex_text) ?? b""
    match archive.untar(data) {
      Ok(_) => bad.push((why, "read", "refused"))
      Err(_) => {}
    }
  }
  expect(show3(bad)) == []
}

test "unzip refuses what std/archive.xo says it refuses" {
  var bad: List[(Str, Str, Str)] = []
  for (hex_text, why) in ZIP_REFUSED {
    let data = Bytes.from_hex(hex_text) ?? b""
    match archive.unzip(data) {
      Ok(_) => bad.push((why, "read", "refused"))
      Err(_) => {}
    }
  }
  expect(show3(bad)) == []
}
`)
	return o.String()
}
