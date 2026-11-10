package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// peExports reads a PE image's export directory: the DLL's name and its
// exported names, sorted.
func peExports(t *testing.T, path string) (string, []string, *pe.File) {
	t.Helper()
	f, err := pe.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	oh, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok || len(oh.DataDirectory) == 0 {
		t.Fatalf("%s: no PE32+ optional header", path)
	}
	dd := oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_EXPORT]
	if dd.VirtualAddress == 0 {
		t.Fatalf("%s: no export directory", path)
	}
	read := func(rva, n uint32) []byte {
		for _, s := range f.Sections {
			if rva >= s.VirtualAddress && rva+n <= s.VirtualAddress+s.VirtualSize {
				b, err := s.Data()
				if err != nil {
					t.Fatal(err)
				}
				return b[rva-s.VirtualAddress : rva-s.VirtualAddress+n]
			}
		}
		t.Fatalf("%s: rva %#x is in no section", path, rva)
		return nil
	}
	cstr := func(rva uint32) string {
		b := read(rva, 1)
		var s []byte
		for i := uint32(0); b[0] != 0; i++ {
			s = append(s, b[0])
			b = read(rva+i+1, 1)
		}
		return string(s)
	}
	ed := read(dd.VirtualAddress, 40)
	name := cstr(binary.LittleEndian.Uint32(ed[12:]))
	nn := binary.LittleEndian.Uint32(ed[24:])
	names := binary.LittleEndian.Uint32(ed[32:])
	var out []string
	for i := uint32(0); i < nn; i++ {
		out = append(out, cstr(binary.LittleEndian.Uint32(read(names+4*i, 4))))
	}
	if !sort.StringsAreSorted(out) {
		t.Errorf("%s: export names not sorted (the loader binary searches them): %v", path, out)
	}
	return name, out, f
}

// TestCLibWindowsDLL cross-builds the C library as a windows DLL
// (decision 0574, `--lib=shared` for windows/amd64 and windows/arm64):
// a DLL image with its export directory of the runtime's exports and
// the library's, and the header geo.h beside it. Running it needs
// Windows (the offload hosts; decision 0574's Windows notes).
func TestCLibWindowsDLL(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ target, backend string }{{"windows/amd64", ""}, {"windows/arm64", "--backend=arm64"}} {
		t.Run(strings.ReplaceAll(c.target, "/", "_"), func(t *testing.T) {
			t.Parallel()
			dir := writeFiles(t, map[string]string{
				"xo.mod": "module example.com/geo\nedition preview\n",
				"geo.xo": clibSrc,
			})
			dll := filepath.Join(dir, "geo.dll")
			args := []string{"build", "--lib=shared", "--targets=" + c.target, "-o", dll, dir}
			if c.backend != "" {
				args = append([]string{"build", c.backend}, args[1:]...)
			}
			var out, errOut bytes.Buffer
			if code := run(t, args, &out, &errOut); code != 0 {
				t.Fatalf("xo build: exit %d: %s\n%s%s", code, clibTail(out.String()+errOut.String()), out.String(), errOut.String())
			}
			name, exports, f := peExports(t, dll)
			if f.Characteristics&pe.IMAGE_FILE_DLL == 0 {
				t.Errorf("Characteristics %#x: no IMAGE_FILE_DLL", f.Characteristics)
			}
			if oh := f.OptionalHeader.(*pe.OptionalHeader64); oh.DllCharacteristics&pe.IMAGE_DLLCHARACTERISTICS_DYNAMIC_BASE == 0 || oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_BASERELOC].Size == 0 {
				t.Errorf("not relocatable: DllCharacteristics %#x, base relocations %d bytes", oh.DllCharacteristics, oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_BASERELOC].Size)
			}
			if name != "geo.dll" {
				t.Errorf("the export directory's name: %q", name)
			}
			if got := strings.Join(exports, " "); got != "geo_add geo_byte_sum geo_list_sum geo_norm geo_str_len xo_free xo_init xo_last_error xo_release" {
				t.Errorf("exports: %s", got)
			}
			h, err := os.ReadFile(filepath.Join(dir, "geo.h"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(h), "int64_t geo_byte_sum(XoBytes b);") {
				t.Errorf("geo.h:\n%s", h)
			}
		})
	}
}
