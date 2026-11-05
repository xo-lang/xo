// A fuzz harness for std/archive's untar and unzip (2026-11-02, decision
// 0461: a parser of untrusted input is Early on testing until it is
// fuzzed).
//
// The Go backend's untar and unzip (std/rt/archive.go) are Go's
// archive/tar and archive/zip readers with std's entry rules (a path that
// is empty, absolute, holds a zero byte or leaves the root through `..` is
// an error, and so is an entry that is not a file or a directory; a PAX
// global header is skipped). The native reader (std/native/archive.xo) is
// a port that must give the same entries and the same errors, so the
// answers here come from calling std/rt's functions directly.
//
// Tar cases are archives Go's tar.Writer writes (USTAR, PAX and GNU
// formats; files, directories, symlinks, hard links, devices, FIFOs, PAX
// global headers; long names, PAX records, paths with `..`, `/`, `\`, a
// drive letter, non-ASCII) and headers built by hand (octal fields with
// spaces, zeros, no terminator, base-256 numbers, a sign bit, every magic,
// random type flags); then cut short, given a header byte changed with the
// checksum recomputed (so the field parsers see it) or not, a size field
// rewritten, trailing bytes or a missing end. Zip cases are archives Go's
// zip.Writer writes (stored and deflated files, directories, symlink and
// device modes, comments, the same hostile names), then cut short, given a
// byte changed anywhere or in a local header, central directory or end
// record field (method, sizes, CRC, name length, offset, entry count),
// or a prefix (a self-extracting stub).
//
// Each archive is in the generated test gzip-compressed (std/compress's
// fuzz covers gunzip), which keeps tar's 512 byte blocks small; the test
// unpacks it with archive.gunzip and compares untar or unzip's rendering
// (each entry's path, mode, directory flag, time in milliseconds and data,
// or the error's text) with std/rt's.
//
// The cases from seed 1, and the later seeds' cases in afRegress, are the
// committed corpus
// testdata/corpus/archivefuzz/archivefuzz_test.xo, which TestStdCorpora
// runs on every corpus backend; TestArchiveFuzzCorpusCurrent checks it is
// what this generator writes (XO_ARCHIVEFUZZ_WRITE=1 rewrites it).
// TestArchiveFuzz runs a fresh seed (XO_ARCHIVEFUZZ_SEED, default 2;
// XO_ARCHIVEFUZZ_N cases, default 2000) on XO_ARCHIVEFUZZ_BACKENDS
// (default arm64 and llvm).
package stdtest

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/std/rt"
)

// afCase is one archive and std/rt's answer for it.
type afCase struct {
	zip  bool
	data []byte
	want string
}

var afNames = []string{"a.txt", "dir/b.txt", "dir/", "x", "é/ü.txt", "a/../b", "./c", `back\slash`, "sp ace", strings.Repeat("long/", 30) + "f", strings.Repeat("n", 120), "a//b", "trail/"}
var afBadNames = []string{"../up", "/abs", "C:/win", "dir/../../out", ".", ""}

func afPick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

// afName is an entry name; one in twelve leaves the archive.
func afName(r *rand.Rand) string {
	if r.Intn(12) == 0 {
		return afPick(r, afBadNames)
	}
	return afPick(r, afNames)
}

func afData(r *rand.Rand) []byte {
	switch r.Intn(4) {
	case 0:
		return nil
	case 1:
		return bytes.Repeat([]byte("ab"), r.Intn(300))
	default:
		b := make([]byte, r.Intn(60))
		for i := range b {
			b[i] = byte('a' + r.Intn(26))
		}
		return b
	}
}

// afTime is in UTC: zip writes a header's Modified as an MS-DOS date and
// time in Modified's location, so a local time made the corpus differ by
// time zone (the Mac's EST against the linux hosts' UTC, 2026-11-05).
var afTime = time.Unix(1700000000, 0).UTC()

// afTar is an archive from Go's tar.Writer; entries the writer refuses are
// left out.
func afTar(r *rand.Rand) []byte {
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	formats := []tar.Format{tar.FormatUnknown, tar.FormatUSTAR, tar.FormatPAX, tar.FormatGNU}
	for i, n := 0, r.Intn(6); i < n; i++ {
		h := &tar.Header{Name: afName(r), Mode: int64(r.Intn(0o10000)), ModTime: afTime.Add(time.Duration(r.Intn(100000)) * time.Second), Format: formats[r.Intn(4)]}
		var data []byte
		switch k := r.Intn(30); {
		case k < 20:
			h.Typeflag = tar.TypeReg
			data = afData(r)
			h.Size = int64(len(data))
		case k < 26:
			h.Typeflag = tar.TypeDir
		case k == 26:
			h.Typeflag, h.Linkname = tar.TypeSymlink, "target"
		case k == 27:
			h.Typeflag, h.Linkname = tar.TypeLink, "a.txt"
		case k == 28:
			h.Typeflag = []byte{tar.TypeChar, tar.TypeBlock, tar.TypeFifo}[r.Intn(3)]
		default:
			h.Typeflag = tar.TypeXGlobalHeader
			h.PAXRecords = map[string]string{"comment": "git"}
			h.Format = tar.FormatPAX
		}
		if r.Intn(6) == 0 {
			h.ModTime = time.Unix(int64(r.Intn(1<<20)), int64(r.Intn(1e9))).UTC()
			h.Format = tar.FormatPAX
		}
		if r.Intn(10) == 0 && h.Typeflag != tar.TypeXGlobalHeader {
			h.PAXRecords = map[string]string{"path": afName(r)}
			h.Format = tar.FormatPAX
		}
		if err := w.WriteHeader(h); err != nil {
			continue
		}
		if len(data) > 0 {
			w.Write(data)
		}
	}
	w.Close()
	return b.Bytes()
}

// afOctal is a numeric field of width n as hostile writers fill it.
func afOctal(r *rand.Rand, v int64, n int) []byte {
	f := make([]byte, n)
	switch r.Intn(8) {
	case 0: // spaces before, NUL after
		s := fmt.Sprintf("%*o", n-1, v)
		copy(f, s)
	case 1: // no terminator
		copy(f, fmt.Sprintf("%0*o", n, v%(1<<(3*uint(n)))))
	case 2: // base-256
		f[0] = 0x80
		binary.BigEndian.PutUint64(f[n-8:], uint64(v))
	case 3: // a negative base-256 number
		for i := range f {
			f[i] = 0xff
		}
	case 4: // junk
		for i := range f {
			f[i] = byte(r.Intn(256))
		}
	case 5: // empty
	default:
		copy(f, fmt.Sprintf("%0*o", n-1, v))
	}
	return f
}

// afSum writes block's header checksum.
func afSum(block []byte) {
	for i := 148; i < 156; i++ {
		block[i] = ' '
	}
	sum := 0
	for _, c := range block[:512] {
		sum += int(c)
	}
	copy(block[148:], fmt.Sprintf("%06o\x00 ", sum))
}

// afRawTar is a hand-built header and its data, then the end blocks.
func afRawTar(r *rand.Rand) []byte {
	var out []byte
	for i, n := 0, 1+r.Intn(2); i < n; i++ {
		h := make([]byte, 512)
		copy(h, afName(r))
		copy(h[100:], afOctal(r, 0o644, 8))
		copy(h[108:], afOctal(r, 0, 8))
		copy(h[116:], afOctal(r, 0, 8))
		data := afData(r)
		size := int64(len(data))
		if r.Intn(8) == 0 {
			size += int64(r.Intn(1000))
		}
		copy(h[124:], afOctal(r, size, 12))
		copy(h[136:], afOctal(r, afTime.Unix(), 12))
		h[156] = "00000\x0025xgKLAS7"[r.Intn(15)]
		switch r.Intn(4) {
		case 0:
			copy(h[257:], "ustar\x0000")
		case 1:
			copy(h[257:], "ustar  \x00")
		case 2:
			copy(h[257:], "ustar\x00xx")
		}
		if r.Intn(5) == 0 {
			copy(h[345:], "prefix/dir")
		}
		afSum(h)
		if r.Intn(10) == 0 {
			h[148] ^= 1
		}
		out = append(out, h...)
		out = append(out, data...)
		if pad := len(data) % 512; pad != 0 {
			out = append(out, make([]byte, 512-pad)...)
		}
	}
	return append(out, make([]byte, 1024)...)
}

// afTarMutate breaks a tar archive the ways a hostile one is broken.
func afTarMutate(r *rand.Rand, b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	switch r.Intn(8) {
	case 0:
		return b[:r.Intn(len(b))]
	case 1, 2: // a header byte changed, the checksum recomputed
		blk := 512 * r.Intn((len(b)+511)/512)
		if blk+512 <= len(b) {
			i := blk + []int{r.Intn(100), 100 + r.Intn(56), 156, 257 + r.Intn(8), 345 + r.Intn(155), r.Intn(512)}[r.Intn(6)]
			b[i] = "0789 \x00\xff/.a"[r.Intn(10)]
			afSum(b[blk : blk+512])
		}
	case 3:
		b[r.Intn(len(b))] ^= byte(1 << r.Intn(8))
	case 4: // a size field rewritten
		if len(b) >= 512 {
			copy(b[124:], fmt.Sprintf("%011o\x00", r.Intn(4096)))
			afSum(b[:512])
		}
	case 5:
		b = append(b, afData(r)...)
	case 6: // no end blocks
		if len(b) >= 1024 {
			b = b[:len(b)-1024]
		}
	}
	return b
}

// afZip is an archive from Go's zip.Writer.
func afZip(r *rand.Rand) []byte {
	var b bytes.Buffer
	if r.Intn(10) == 0 {
		b.WriteString("#!/bin/sh stub\n")
	}
	w := zip.NewWriter(&b)
	if b.Len() > 0 {
		w.SetOffset(int64(b.Len()))
	}
	for i, n := 0, r.Intn(6); i < n; i++ {
		h := &zip.FileHeader{Name: afName(r), Method: []uint16{zip.Store, zip.Deflate}[r.Intn(2)], Modified: afTime.Add(time.Duration(r.Intn(100000)) * time.Second)}
		var data []byte
		switch k := r.Intn(25); {
		case k < 17:
			h.SetMode(fs.FileMode(r.Intn(0o1000)))
			data = afData(r)
		case k < 23:
			h.Name = strings.TrimSuffix(h.Name, "/") + "/"
			h.SetMode(fs.ModeDir | 0o755)
		case k == 23:
			h.SetMode(fs.ModeSymlink | 0o777)
			data = []byte("target")
		default:
			h.SetMode(fs.ModeDevice | 0o600)
		}
		if r.Intn(5) == 0 {
			h.Comment = "c"
		}
		fw, err := w.CreateHeader(h)
		if err != nil {
			continue
		}
		fw.Write(data)
	}
	if r.Intn(5) == 0 {
		w.SetComment("archive comment")
	}
	w.Close()
	return b.Bytes()
}

// afZipMutate breaks a zip archive in a field Go's reader checks.
func afZipMutate(r *rand.Rand, b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	at := func(sig string) []int {
		var o []int
		for i := 0; i+4 <= len(b); i++ {
			if string(b[i:i+4]) == sig {
				o = append(o, i)
			}
		}
		return o
	}
	put16 := func(i int, v uint16) {
		if i+2 <= len(b) {
			binary.LittleEndian.PutUint16(b[i:], v)
		}
	}
	put32 := func(i int, v uint32) {
		if i+4 <= len(b) {
			binary.LittleEndian.PutUint32(b[i:], v)
		}
	}
	switch r.Intn(9) {
	case 0:
		return b[:r.Intn(len(b))]
	case 1, 2:
		b[r.Intn(len(b))] ^= byte(1 << r.Intn(8))
	case 3: // a central directory field
		if cds := at("PK\x01\x02"); len(cds) > 0 {
			c := cds[r.Intn(len(cds))]
			switch r.Intn(5) {
			case 0:
				put16(c+10, uint16(r.Intn(100))) // method
			case 1:
				put32(c+16, r.Uint32()) // CRC
			case 2:
				put32(c+20+4*r.Intn(2), uint32(r.Intn(1<<16))) // sizes
			case 3:
				put16(c+28, uint16(r.Intn(300))) // name length
			default:
				put32(c+42, uint32(r.Intn(len(b)+100))) // local header offset
			}
		}
	case 4: // a local header field
		if ls := at("PK\x03\x04"); len(ls) > 0 {
			l := ls[r.Intn(len(ls))]
			switch r.Intn(3) {
			case 0:
				put16(l+26, uint16(r.Intn(300))) // name length
			case 1:
				put16(l+28, uint16(r.Intn(300))) // extra length
			default:
				put16(l+8, uint16(r.Intn(100))) // method
			}
		}
	case 5: // the end record
		if es := at("PK\x05\x06"); len(es) > 0 {
			e := es[len(es)-1]
			switch r.Intn(3) {
			case 0:
				put16(e+10, uint16(r.Intn(10))) // entries
			case 1:
				put32(e+16, uint32(r.Intn(len(b)+100))) // directory offset
			default:
				put16(e+20, uint16(r.Intn(100))) // comment length
			}
		}
	case 6:
		b = append(afData(r), b...)
	case 7:
		b = append(b, afData(r)...)
	}
	return b
}

// afRender is the answer as the generated test renders it.
func afRender(res rt.Result[rt.List[rt.Archive_Entry], rt.Archive_ArchiveErr]) string {
	if !res.Ok {
		return "err " + res.E.M_display(nil)
	}
	var o strings.Builder
	o.WriteString("ok ")
	for _, e := range rt.List_Slice(res.V) {
		ns := int64(e.F_modified)
		ms := ns / 1e6
		if ns%1e6 < 0 {
			ms--
		}
		fmt.Fprintf(&o, "%x:%d:%t:%d:%x;", e.F_path, e.F_mode, e.F_is_dir, ms, string(e.F_data))
	}
	return o.String()
}

func afCases(r *rand.Rand, n int) []afCase {
	cases := make([]afCase, 0, n)
	for len(cases) < n {
		c := afCase{zip: r.Intn(2) == 0}
		if c.zip {
			c.data = afZip(r)
			if r.Intn(3) != 0 {
				c.data = afZipMutate(r, c.data)
			}
			c.want = afRender(rt.Archive_unzip(nil, rt.Bytes(c.data)))
		} else {
			if r.Intn(4) == 0 {
				c.data = afRawTar(r)
			} else {
				c.data = afTar(r)
			}
			if r.Intn(3) != 0 {
				c.data = afTarMutate(r, c.data)
			}
			c.want = afRender(rt.Archive_untar(nil, rt.Bytes(c.data)))
		}
		cases = append(cases, c)
	}
	return cases
}

const afRenderXo = `use std/archive

fn af_list(es: List[archive.Entry]) -> Str {
  var out = "ok "
  for e in es {
    out = out + "${e.path.bytes().to_hex()}:${e.mode}:${e.is_dir}:${e.modified.unix_millis()}:${e.data.to_hex()};"
  }
  out
}

fn af_untar(gz: Bytes) -> Str {
  match archive.gunzip(gz) {
    Ok(b) => match archive.untar(b) {
      Ok(es) => af_list(es)
      Err(e) => "err " + e.display()
    }
    Err(e) => "gunzip " + e.display()
  }
}

fn af_unzip(gz: Bytes) -> Str {
  match archive.gunzip(gz) {
    Ok(b) => match archive.unzip(b) {
      Ok(es) => af_list(es)
      Err(e) => "err " + e.display()
    }
    Err(e) => "gunzip " + e.display()
  }
}

`

func afGzip(b []byte) []byte {
	var o bytes.Buffer
	w, _ := gzip.NewWriterLevel(&o, gzip.BestCompression)
	w.Write(b)
	w.Close()
	return o.Bytes()
}

// afXoFile is the generated test file: the helpers and one test per case.
func afXoFile(seed int64, cases []afCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/archivefuzz_test.go (seed %d, %d cases) from the Go\n", seed, len(cases))
	b.WriteString("// backend's untar and unzip (Go's archive/tar and archive/zip); do not edit:\n")
	b.WriteString("// XO_ARCHIVEFUZZ_WRITE=1 go test ./internal/stdtest -run TestArchiveFuzzCorpusCurrent rewrites it.\n")
	b.WriteString(afRenderXo)
	for i, c := range cases {
		fn := "af_untar"
		if c.zip {
			fn = "af_unzip"
		}
		fmt.Fprintf(&b, "test \"archive fuzz %d\" {\n  let got = %s(%s)\n  if got != %s {\n    fault(\"got \" + got)\n  }\n}\n\n", i, fn, ufBytes(afGzip(c.data)), xoStr(c.want))
	}
	return b.String()
}

// afCoverage fails when the cases stop reaching the readers' parts.
func afCoverage(t *testing.T, cases []afCase) {
	t.Helper()
	ok := map[bool]int{}
	entries := map[bool]int{}
	msgs := map[string]int{}
	for _, c := range cases {
		if strings.HasPrefix(c.want, "ok ") {
			ok[c.zip]++
			entries[c.zip] += strings.Count(c.want, ";")
		} else {
			m := c.want
			if i := strings.Index(m, `"`); i > 0 {
				m = m[:i]
			}
			msgs[m]++
		}
	}
	t.Logf("tar: %d ok (%d entries); zip: %d ok (%d entries); %d distinct errors", ok[false], entries[false], ok[true], entries[true], len(msgs))
	for m, n := range msgs {
		t.Logf("  %4d %s", n, m)
	}
	if ok[false] < len(cases)/10 || ok[true] < len(cases)/10 || entries[false] < len(cases)/5 || entries[true] < len(cases)/5 || len(msgs) < 10 {
		t.Fatalf("the cases stopped covering the readers")
	}
}

const afCorpus = "testdata/corpus/archivefuzz/archivefuzz_test.xo"

// afRegress are cases of later seeds that found bugs seed 1 did not
// reach, kept in the corpus after seed 1's: (seed, case index).
var afRegress = [][2]int{
	{2, 1625}, // a zip name that is not UTF-8, quoted as Go quotes its bytes
	{3, 435},  // a PAX header claiming 14 MB, cut short: unexpected EOF, not too long
	{3, 611},  // the same quoting in "is not a file or a directory"
	{4, 828},
	{5, 1230}, // a PAX global header's own path, checked before it is skipped
	{6, 72},   // a zip local header cut short: strings.Reader's EOF
	{7, 1},    // a global header named "."
	{7, 1175}, // a global header with an empty name
	{8, 826},
	{10, 1401}, // a zip entry with an unknown Unix file type: a regular file in Go
}

// afCorpusCases are seed 1's cases and afRegress's.
func afCorpusCases() []afCase {
	cases := afCases(rand.New(rand.NewSource(1)), 2000)
	for _, rg := range afRegress {
		cases = append(cases, afCases(rand.New(rand.NewSource(int64(rg[0]))), rg[1]+1)[rg[1]])
	}
	return cases
}

// TestArchiveFuzzCorpusCurrent: the committed corpus is what seed 1
// writes.
func TestArchiveFuzzCorpusCurrent(t *testing.T) {
	cases := afCorpusCases()
	afCoverage(t, cases)
	text := afXoFile(1, cases)
	if os.Getenv("XO_ARCHIVEFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(afCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(afCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(afCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's archive packages, std/rt or the generator changed): XO_ARCHIVEFUZZ_WRITE=1 go test ./internal/stdtest -run TestArchiveFuzzCorpusCurrent", afCorpus)
	}
}

// TestArchiveFuzz runs a fresh seed's cases.
func TestArchiveFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the archive fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_ARCHIVEFUZZ_SEED", 2))
	n := envInt(t, "XO_ARCHIVEFUZZ_N", 2000)
	cases := afCases(rand.New(rand.NewSource(seed)), n)
	for i, c := range cases {
		t.Logf("case %d zip=%t %x", i, c.zip, c.data)
	}
	runFuzzDir(t, "archivefuzz_test.xo", afXoFile(seed, cases), envBackends("XO_ARCHIVEFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
