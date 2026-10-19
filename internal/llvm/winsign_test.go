package llvm

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestWindowsCertTableAppends: a windows/amd64 program, with a
// signature-shaped certificate table appended as an Authenticode signer
// (jsign, signtool) appends one, still parses: the table starts on an
// 8 byte boundary at the end of the image, the security directory
// (data directory 4) names it, the checksum is recomputed, and every
// section's data lies before it. scripts/sign-windows.sh signs this way
// (knowledgebase/infrastructure/code-signing.md "Windows"); no signing
// service is called. The program is cross built with the LLVM backend
// and the Windows cross toolchain (as TestWinConsts finds it), so the
// test skips where that is not set up.
func TestWindowsCertTableAppends(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	clang, ar, sysroot := os.Getenv("XO_CLANG"), os.Getenv("XO_AR"), os.Getenv("XO_WINSYSROOT")
	if sysroot == "" {
		if cache, err := os.UserCacheDir(); err == nil {
			x := filepath.Join(cache, "xo", "winxc")
			sysroot = filepath.Join(x, "winsysroot")
			if clang == "" {
				clang = filepath.Join(x, "LLVM-23.1.3-macOS-ARM64", "bin", "clang")
				ar = filepath.Join(x, "LLVM-23.1.3-macOS-ARM64", "bin", "llvm-ar")
			}
		}
	}
	if _, err := os.Stat(sysroot); err != nil {
		t.Skip("no Windows sysroot (XO_WINSYSROOT): the Windows cross toolchain is not set up")
	}
	if _, err := exec.LookPath(clang); err != nil {
		t.Skipf("no clang for the Windows cross build: %v", err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	src := filepath.Join(work, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hello, signed\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(work, "hello.exe")
	cmd := exec.Command(xo, "build", "--backend=llvm", "--target=windows/amd64", "-o", exe, ".")
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "XO_CLANG="+clang, "XO_AR="+ar, "XO_WINSYSROOT="+sysroot)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	img, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	// a PKCS#7 SignedData's shape: a DER SEQUENCE, an odd length so the
	// table needs padding
	blob := append([]byte{0x30, 0x82, 0x06, 0x63}, bytes.Repeat([]byte{0xa5}, 0x663)...)
	signed, off, size := appendCertTable(t, img, blob)

	f, err := pe.NewFile(bytes.NewReader(signed))
	if err != nil {
		t.Fatalf("the signed image does not parse: %v", err)
	}
	oh, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		t.Fatalf("not PE32+: %T", f.OptionalHeader)
	}
	dir := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_SECURITY]
	if dir.VirtualAddress != off || dir.Size != size {
		t.Errorf("security directory %#x+%#x, want %#x+%#x", dir.VirtualAddress, dir.Size, off, size)
	}
	if off%8 != 0 || int(off+size) != len(signed) {
		t.Errorf("certificate table at %#x+%#x in %#x bytes: want 8 byte aligned and last", off, size, len(signed))
	}
	for _, s := range f.Sections {
		if s.Offset+s.Size > off {
			t.Errorf("section %s's data (%#x+%#x) runs into the certificate table at %#x", s.Name, s.Offset, s.Size, off)
		}
	}
	if got, want := oh.CheckSum, peChecksum(signed, checksumAt(t, signed)); got != want {
		t.Errorf("checksum %#x, want %#x", got, want)
	}
	if len(f.Sections) == 0 {
		t.Error("no sections")
	}
}

// appendCertTable returns img with blob appended as a WIN_CERTIFICATE
// (revision 2.0, PKCS_SIGNED_DATA) the way Authenticode signers append
// one, and the table's offset and size.
func appendCertTable(t *testing.T, img, blob []byte) ([]byte, uint32, uint32) {
	t.Helper()
	out := append([]byte(nil), img...)
	for len(out)%8 != 0 {
		out = append(out, 0)
	}
	off := uint32(len(out))
	n := 8 + len(blob)
	var h [8]byte
	binary.LittleEndian.PutUint32(h[0:], uint32(n))
	binary.LittleEndian.PutUint16(h[4:], 0x0200)
	binary.LittleEndian.PutUint16(h[6:], 0x0002)
	out = append(append(out, h[:]...), blob...)
	for len(out)%8 != 0 {
		out = append(out, 0)
	}
	size := uint32(len(out)) - off
	ck := checksumAt(t, out)
	// the data directories follow the checksum's optional header fields
	dd := ck - 64 + 112
	binary.LittleEndian.PutUint32(out[dd+8*4:], off)
	binary.LittleEndian.PutUint32(out[dd+8*4+4:], size)
	binary.LittleEndian.PutUint32(out[ck:], peChecksum(out, ck))
	return out, off, size
}

// checksumAt is the offset of the optional header's CheckSum in a PE32+
// image.
func checksumAt(t *testing.T, img []byte) int {
	t.Helper()
	if len(img) < 0x40 {
		t.Fatal("not a PE image")
	}
	pe := int(binary.LittleEndian.Uint32(img[0x3c:]))
	opt := pe + 24
	if binary.LittleEndian.Uint16(img[opt:]) != 0x20b {
		t.Fatal("not PE32+")
	}
	return opt + 64
}

// peChecksum is the image checksum (imagehlp's CheckSumMappedFile): the
// 16 bit one's complement sum of the image with the CheckSum field taken
// as zero, plus the file's length.
func peChecksum(img []byte, at int) uint32 {
	var s uint32
	for i := 0; i+1 < len(img); i += 2 {
		if i == at || i == at+2 {
			continue
		}
		s += uint32(binary.LittleEndian.Uint16(img[i:]))
		s = (s & 0xffff) + (s >> 16)
	}
	if len(img)%2 == 1 {
		s += uint32(img[len(img)-1])
		s = (s & 0xffff) + (s >> 16)
	}
	return (s & 0xffff) + uint32(len(img))
}
