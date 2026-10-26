// Package lz4test checks the native LZ4 block codec (std/internal's
// lz4_compress and lz4_decompress, std/native/llrt/lz4.xo; decision 0365)
// against std/rt's Go copy (internal_lz4.go) and, when it is installed,
// the lz4 command line tool, the reference implementation.
package lz4test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
	"github.com/xo-lang/xo/std/rt"
)

// tool builds testdata/tool with backend.
func tool(t *testing.T, backend string) string {
	t.Helper()
	if backend == xotool.BackendLLVM {
		if _, err := xotool.Clang(); err != nil {
			t.Skipf("no clang: %v", err)
		}
	}
	r, err := xotool.Build([]string{filepath.Join("testdata", "tool")}, xotool.Options{Backend: backend, Release: true})
	if err != nil {
		t.Skipf("no xo: %v", err)
	}
	if r.Failed {
		t.Fatalf("building testdata/tool:\n%s", r.Output)
	}
	if r.Dir != "" {
		t.Cleanup(func() { os.RemoveAll(r.Dir) })
	}
	return r.Binaries[xotool.Host()]
}

// corpus is the inputs: the format's edge lengths (0 to 13 bytes, where
// a block holds literals only), runs, text, random bytes (incompressible:
// a block a little longer than its input), long matches and long
// literal runs (the 255 continuation bytes), offsets near 65535, and the
// repository's own sources.
func corpus(t *testing.T) map[string][]byte {
	r := rand.New(rand.NewSource(1))
	c := map[string][]byte{}
	for n := 0; n <= 20; n++ {
		c[fmt.Sprintf("len%02d", n)] = bytes.Repeat([]byte{'a'}, n)
		b := make([]byte, n)
		r.Read(b)
		c[fmt.Sprintf("rnd%02d", n)] = b
	}
	c["zeros1m"] = make([]byte, 1<<20)
	c["ab300"] = bytes.Repeat([]byte("ab"), 150)
	c["abc70k"] = bytes.Repeat([]byte("abcdefg"), 10000)
	rnd := make([]byte, 300000)
	r.Read(rnd)
	c["random300k"] = rnd
	// a long literal run, then a match of it
	lit := make([]byte, 5000)
	r.Read(lit)
	c["litmatch"] = append(append([]byte{}, lit...), lit...)
	// matches at offsets 65535 and 65536 (past the format's reach)
	far := make([]byte, 70000)
	r.Read(far)
	copy(far[65535+100:], far[100:200])
	copy(far[65536+300:], far[300:400])
	c["far"] = far
	// text of words drawn from a small vocabulary
	words := strings.Fields("the cache stores every entry once by key and reads it back with a check of its version before use")
	var sb strings.Builder
	for sb.Len() < 200000 {
		sb.WriteString(words[r.Intn(len(words))])
		sb.WriteByte(" \n"[r.Intn(2)])
	}
	c["words"] = []byte(sb.String())
	for _, glob := range []string{"../../compiler/check/*.xo", "../../std/native/llrt/flate.xo", "../../std/rt/*.go"} {
		ms, _ := filepath.Glob(glob)
		sort.Strings(ms)
		var all []byte
		for _, m := range ms {
			b, err := os.ReadFile(m)
			if err != nil {
				t.Fatal(err)
			}
			all = append(all, b...)
		}
		if len(all) > 0 {
			c["src_"+strings.NewReplacer("/", "_", ".", "_", "*", "x").Replace(strings.TrimPrefix(glob, "../../"))] = all
		}
	}
	return c
}

func writeList(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, bin string, args ...string) {
	t.Helper()
	if out, err := exec.Command(bin, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", bin, args, err, out)
	}
}

// TestNativeMatchesGo: the native compressor writes the Go copy's bytes,
// the Go decompressor reads them back, and the native decompressor reads
// the Go copy's blocks; on damaged blocks (cut short, a byte changed)
// both decompressors agree on each: the same bytes, or both None. On the
// native backend (xo's own), LLVM, and the Go backend (std/rt's copy
// through the generated code).
func TestNativeMatchesGo(t *testing.T) {
	for _, b := range []string{xotool.NativeBackend(), xotool.BackendLLVM, xotool.BackendGo} {
		t.Run(b, func(t *testing.T) { t.Parallel(); matchesGo(t, tool(t, b)) })
	}
}

func matchesGo(t *testing.T, bin string) {
	dir := t.TempDir()
	c := corpus(t)
	var names []string
	for name, b := range c {
		names = append(names, name)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(names)
	writeList(t, filepath.Join(dir, "list.txt"), names)
	run(t, bin, "c", dir)
	r := rand.New(rand.NewSource(2))
	var dlist []string
	want := map[string][]byte{}
	addCase := func(name string, block []byte, n int) {
		if err := os.WriteFile(filepath.Join(dir, name), block, 0o644); err != nil {
			t.Fatal(err)
		}
		dlist = append(dlist, fmt.Sprintf("%s %d", name, n))
		d, ok := rt.LZ4DecompressBlock(block, n)
		if ok {
			want[name] = d
		}
	}
	for _, name := range names {
		src := c[name]
		got, err := os.ReadFile(filepath.Join(dir, name+".lz4"))
		if err != nil {
			t.Fatal(err)
		}
		gob := rt.LZ4CompressBlock(src)
		if !bytes.Equal(got, gob) {
			t.Errorf("%s (%d bytes): native block of %d bytes differs from Go's of %d", name, len(src), len(got), len(gob))
			continue
		}
		if len(got) > len(src)+len(src)/255+16 {
			t.Errorf("%s: block of %d bytes over the bound for %d", name, len(got), len(src))
		}
		if d, ok := rt.LZ4DecompressBlock(got, len(src)); !ok || !bytes.Equal(d, src) {
			t.Errorf("%s: Go cannot read the native block back", name)
		}
		addCase(name+".lz4", got, len(src))
		// the wrong length either way is not a block of that length
		addCase(name+".short", got, len(src)-1)
		addCase(name+".long", got, len(src)+1)
		for k := 0; k < 6 && len(got) > 1; k++ {
			cut := got[:r.Intn(len(got))]
			addCase(fmt.Sprintf("%s.cut%d", name, k), cut, len(src))
			bad := append([]byte{}, got...)
			bad[r.Intn(len(bad))] ^= byte(1 + r.Intn(255))
			addCase(fmt.Sprintf("%s.flip%d", name, k), bad, len(src))
		}
	}
	writeList(t, filepath.Join(dir, "dlist.txt"), dlist)
	run(t, bin, "d", dir)
	for _, line := range dlist {
		f := strings.Fields(line)
		name := f[0]
		w, ok := want[name]
		got, err := os.ReadFile(filepath.Join(dir, name+".out"))
		_, noneErr := os.Stat(filepath.Join(dir, name+".none"))
		switch {
		case ok && (err != nil || !bytes.Equal(got, w)):
			t.Errorf("%s: Go reads %d bytes, the native decompressor %v", name, len(w), describe(got, err))
		case !ok && noneErr != nil:
			t.Errorf("%s: Go finds no block of %s bytes, the native decompressor %v", name, f[1], describe(got, err))
		}
	}
}

func describe(b []byte, err error) string {
	if err != nil {
		return "returned None"
	}
	return strconv.Itoa(len(b)) + " different bytes"
}

// xxh32 is XXH32 of b with seed 0 (for the frame header's checksum).
func xxh32(b []byte) uint32 {
	const p1, p2, p3, p4, p5 uint32 = 2654435761, 2246822519, 3266489917, 668265263, 374761393
	rotl := func(x uint32, r uint) uint32 { return x<<r | x>>(32-r) }
	n := len(b)
	var h uint32
	i := 0
	if n >= 16 {
		p1v := p1
		v1, v2, v3, v4 := p1v+p2, p2, uint32(0), -p1v
		for ; i+16 <= n; i += 16 {
			round := func(v uint32, j int) uint32 {
				return rotl(v+binary.LittleEndian.Uint32(b[j:])*p2, 13) * p1
			}
			v1, v2, v3, v4 = round(v1, i), round(v2, i+4), round(v3, i+8), round(v4, i+12)
		}
		h = rotl(v1, 1) + rotl(v2, 7) + rotl(v3, 12) + rotl(v4, 18)
	} else {
		h = p5
	}
	h += uint32(n)
	for ; i+4 <= n; i += 4 {
		h = rotl(h+binary.LittleEndian.Uint32(b[i:])*p3, 17) * p4
	}
	for ; i < n; i++ {
		h = rotl(h+uint32(b[i])*p5, 11) * p1
	}
	h ^= h >> 15
	h *= p2
	h ^= h >> 13
	h *= p3
	h ^= h >> 16
	return h
}

// frame wraps one block in an LZ4 frame (independent blocks, 4 MB
// maximum, no checksums) for the lz4 tool.
func frame(block []byte) []byte {
	desc := []byte{0x60, 0x70}
	f := []byte{0x04, 0x22, 0x4d, 0x18}
	f = append(f, desc...)
	f = append(f, byte(xxh32(desc)>>8))
	if len(block) > 0 {
		f = binary.LittleEndian.AppendUint32(f, uint32(len(block)))
		f = append(f, block...)
	}
	return binary.LittleEndian.AppendUint32(f, 0)
}

// blocks is the blocks of an LZ4 frame the lz4 tool wrote: (data,
// compressed) each.
func blocks(t *testing.T, f []byte) ([][]byte, []bool) {
	t.Helper()
	if len(f) < 7 || binary.LittleEndian.Uint32(f) != 0x184d2204 {
		t.Fatalf("not an LZ4 frame")
	}
	flg := f[4]
	i := 7
	if flg&0x08 != 0 {
		i += 8
	}
	var bs [][]byte
	var cs []bool
	for {
		n := binary.LittleEndian.Uint32(f[i:])
		i += 4
		if n == 0 {
			break
		}
		size := int(n & 0x7fffffff)
		bs = append(bs, f[i:i+size])
		cs = append(cs, n&0x80000000 == 0)
		i += size
		if flg&0x10 != 0 {
			i += 4
		}
	}
	return bs, cs
}

// TestNativeMatchesLZ4Tool: the lz4 tool reads the native blocks back,
// and the native decompressor reads the tool's blocks (lz4 -1 and -9).
func TestNativeMatchesLZ4Tool(t *testing.T) {
	lz4, err := exec.LookPath("lz4")
	if err != nil {
		t.Skip("no lz4 tool")
	}
	bin := tool(t, xotool.NativeBackend())
	dir := t.TempDir()
	c := corpus(t)
	var names []string
	for name, b := range c {
		if len(b) > 4<<20 {
			continue
		}
		names = append(names, name)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(names)
	writeList(t, filepath.Join(dir, "list.txt"), names)
	run(t, bin, "c", dir)
	var dlist []string
	for _, name := range names {
		src := c[name]
		block, err := os.ReadFile(filepath.Join(dir, name+".lz4"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(lz4, "-d", "-c")
		cmd.Stdin = bytes.NewReader(frame(block))
		out, err := cmd.Output()
		if err != nil || !bytes.Equal(out, src) {
			t.Errorf("%s: lz4 -d of the native block: %v, %d bytes for %d", name, err, len(out), len(src))
		}
		for _, level := range []string{"-1", "-9"} {
			cmd := exec.Command(lz4, level, "-B7", "--no-frame-crc", "-c")
			cmd.Stdin = bytes.NewReader(src)
			f, err := cmd.Output()
			if err != nil {
				t.Fatalf("lz4 %s: %v", level, err)
			}
			bs, cs := blocks(t, f)
			if len(bs) == 1 && cs[0] {
				bn := name + ".tool" + level
				if err := os.WriteFile(filepath.Join(dir, bn), bs[0], 0o644); err != nil {
					t.Fatal(err)
				}
				dlist = append(dlist, fmt.Sprintf("%s %d", bn, len(src)))
			}
		}
	}
	writeList(t, filepath.Join(dir, "dlist.txt"), dlist)
	run(t, bin, "d", dir)
	// the tool stores a block it cannot shrink as it is (short and random
	// inputs): those are not LZ4 blocks
	if len(dlist) < len(names)/2 {
		t.Errorf("only %d compressed tool blocks for %d inputs", len(dlist), len(names))
	}
	for _, line := range dlist {
		f := strings.Fields(line)
		got, err := os.ReadFile(filepath.Join(dir, f[0]+".out"))
		src := c[strings.Split(f[0], ".tool")[0]]
		if err != nil || !bytes.Equal(got, src) {
			t.Errorf("%s: the native decompressor %v", f[0], describe(got, err))
		}
	}
}
