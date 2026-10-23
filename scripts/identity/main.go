// Command identity checks that two xo binaries build the same programs
// (owner, 2026-10-21): for a change that claims no output change, every
// program is built by both, and the outputs are compared, ELF byte for
// byte, PE byte for byte but its PDB record, that record's time stamp
// and CheckSum (they take the output's path: maskPE), Mach-O by disassembly (its code signature and UUID take
// the output's path: gotchas/darwin-binary-identity-output-name.md).
//
//	go run ./scripts/identity [flags] BASE NEW [DIR...]
//
// BASE and NEW are xo binaries or git revisions; a revision is built in
// a temporary worktree (go run ./scripts/xobuild there), removed after.
// The DIRs default to every -step-th golden of internal/llvm/testdata/golden
// and compiler/xo. What two manual runs got wrong on 2026-10-21 is the
// default here (gotchas/identity-runs-std-path-and-caches.md):
//
//   - one std for both (-std, default this checkout's std): the std path
//     is in the debug information, so two stds differ everywhere;
//   - the same output name for both sides, in two directories;
//   - every cache off (XO_BIN_CACHE, XO_BACKEND_CACHE, XO_FE_CACHE,
//     XO_PART_CACHE, XO_CHK_CACHE, XO_CLANG_CACHE), and a private backend
//     cache directory (XO_BACKEND_CACHE_DIR) for every xo started here,
//     revision builds included, removed at the end: an xo older than
//     6738e067 prunes the shared cache with a prune that faulted;
//   - each pair deleted once compared (a cache-on sweep wrote 46 GB), a
//     differing pair kept under OUT/keep/NAME-OS-ARCH/{a,b}.
//
// The exit status is 0 when every program built by NEW matches, 1 when
// one differs or NEW failed to build it, 2 on a usage or setup error.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	targets := flag.String("targets", "darwin/arm64,linux/arm64", "the targets to build for, comma separated")
	backend := flag.String("backend", "arm64", "the backend (--backend)")
	step := flag.Int("step", 4, "every step-th golden (1: every golden)")
	std := flag.String("std", "", "the std both xos build with (default: this checkout's std)")
	out := flag.String("out", "", "the output directory (default: a new temporary directory; kept pairs under OUT/keep)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/identity [-targets T,...] [-backend B] [-step N] [-std DIR] [-out DIR] BASE NEW [DIR...]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(2)
	}
	root, err := xotool.Root()
	if err != nil {
		fatal(err)
	}
	if *std == "" {
		*std = filepath.Join(root, "std")
	}
	if *out == "" {
		*out, err = os.MkdirTemp("", "xo-identity-")
		if err != nil {
			fatal(err)
		}
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}
	cacheDir, err := os.MkdirTemp("", "xo-identity-bcache-")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(cacheDir)
	env := append(os.Environ(),
		"XO_STD="+*std,
		"XO_BIN_CACHE=off", "XO_BACKEND_CACHE=off", "XO_FE_CACHE=off", "XO_PART_CACHE=off",
		"XO_CHK_CACHE=off", "XO_CLANG_CACHE=off",
		"XO_BACKEND_CACHE_DIR="+cacheDir)
	var cleanups []func()
	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()
	xos := make([]string, 2)
	for i, a := range flag.Args()[:2] {
		x, cleanup, err := resolve(root, a, cacheDir)
		if cleanup != nil {
			cleanups = append(cleanups, cleanup)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "identity:", err)
			for _, c := range cleanups {
				c()
			}
			os.Exit(2)
		}
		xos[i] = x
	}
	dirs := flag.Args()[2:]
	if len(dirs) == 0 {
		dirs = defaultDirs(root, *step)
	}
	fmt.Printf("identity: base %s\nidentity: new  %s\nidentity: std %s, out %s, %d programs, targets %s, backend %s\n", xos[0], xos[1], *std, *out, len(dirs), *targets, *backend)
	n, differ, baseFailed, newFailed := 0, 0, 0, 0
	for _, t := range strings.Split(*targets, ",") {
		for _, d := range dirs {
			name := filepath.Base(filepath.Clean(d))
			// xo build names a windows -o NAME without an extension NAME.exe
			prog := "prog"
			if strings.HasPrefix(t, "windows/") {
				prog = "prog.exe"
			}
			a := filepath.Join(*out, "a", prog)
			b := filepath.Join(*out, "b", prog)
			if err := build(xos[0], d, *backend, t, a, env); err != nil {
				baseFailed++
				cleanPair(*out)
				continue
			}
			if err := build(xos[1], d, *backend, t, b, env); err != nil {
				newFailed++
				fmt.Printf("NEW FAILED %s %s: %v\n", t, name, err)
				cleanPair(*out)
				continue
			}
			n++
			same, err := equal(a, b, strings.HasPrefix(t, "darwin/"))
			if err != nil {
				fatal(err)
			}
			if !same {
				differ++
				keep := filepath.Join(*out, "keep", name+"-"+strings.ReplaceAll(t, "/", "-"))
				os.MkdirAll(keep, 0o755)
				os.Rename(a, filepath.Join(keep, "a"))
				os.Rename(b, filepath.Join(keep, "b"))
				fmt.Printf("DIFFERS %s %s (kept in %s)\n", t, name, keep)
			}
			cleanPair(*out)
		}
	}
	fmt.Printf("identity: compared %d, differing %d, new failed %d, base failed %d\n", n, differ, newFailed, baseFailed)
	if differ > 0 || newFailed > 0 {
		for _, c := range cleanups {
			c()
		}
		cleanups = nil
		os.RemoveAll(cacheDir)
		os.Exit(1)
	}
}

// resolve returns the xo binary a names: the file itself, or the xo built
// from git revision a in a temporary worktree (removed by the cleanup).
func resolve(root, a, cacheDir string) (string, func(), error) {
	if st, err := os.Stat(a); err == nil && !st.IsDir() {
		p, err := filepath.Abs(a)
		return p, nil, err
	}
	rev, err := exec.Command("git", "-C", root, "rev-parse", "--verify", a+"^{commit}").Output()
	if err != nil {
		return "", nil, fmt.Errorf("%s is neither an xo binary nor a git revision", a)
	}
	commit := strings.TrimSpace(string(rev))
	tmp, err := os.MkdirTemp("", "xo-identity-tree-")
	if err != nil {
		return "", nil, err
	}
	tree := filepath.Join(tmp, "xo")
	cleanup := func() {
		exec.Command("git", "-C", root, "worktree", "remove", "--force", tree).Run()
		os.RemoveAll(tmp)
	}
	if o, err := exec.Command("git", "-C", root, "worktree", "add", "-q", "--detach", tree, commit).CombinedOutput(); err != nil {
		return "", cleanup, fmt.Errorf("worktree for %s: %v\n%s", a, err, o)
	}
	fmt.Fprintf(os.Stderr, "identity: building the xo of %s (%.12s)\n", a, commit)
	cmd := exec.Command("go", "run", "./scripts/xobuild")
	cmd.Dir = tree
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE_DIR="+cacheDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	o, err := cmd.Output()
	if err != nil {
		return "", cleanup, fmt.Errorf("xobuild of %s: %v\n%s", a, err, tail(stderr.String(), 5))
	}
	x := strings.TrimSpace(string(o))
	if i := strings.LastIndex(x, "\n"); i >= 0 {
		x = x[i+1:]
	}
	// the binary outlives the tree: copy it out
	dst := filepath.Join(tmp, "xo-bin")
	data, err := os.ReadFile(x)
	if err != nil {
		return "", cleanup, err
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return "", cleanup, err
	}
	// it finds std beside it (bin/../std) when XO_STD is unset; every
	// build here sets XO_STD
	return dst, cleanup, nil
}

// defaultDirs: every step-th golden of internal/llvm/testdata/golden, and
// compiler/xo.
func defaultDirs(root string, step int) []string {
	ms, _ := filepath.Glob(filepath.Join(root, "internal", "llvm", "testdata", "golden", "*", "want.txt"))
	sort.Strings(ms)
	var dirs []string
	for i, m := range ms {
		if step <= 1 || i%step == 0 {
			dirs = append(dirs, filepath.Dir(m))
		}
	}
	return append(dirs, filepath.Join(root, "compiler", "xo"))
}

// build builds the program in dir with xo for target t into out.
func build(xo, dir, backend, t, out string, env []string) error {
	os.MkdirAll(filepath.Dir(out), 0o755)
	cmd := exec.Command(xo, "build", "--backend="+backend, "--targets="+t, "-o", out, ".")
	cmd.Dir = dir
	cmd.Env = env
	o, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, tail(string(o), 1))
	}
	return nil
}

// equal compares two outputs: Mach-O by disassembly, others byte for byte.
// maskPE zeroes what a PE file takes from its output path: the CodeView
// record (RSDS, the PDB's GUID, age and path, which names the a or b
// directory), its debug directory entry's TimeDateStamp (from the GUID)
// and the optional header's CheckSum. Other files are returned as they
// are.
func maskPE(x []byte) []byte {
	if len(x) < 0x40 || x[0] != 'M' || x[1] != 'Z' {
		return x
	}
	x = bytes.Clone(x)
	pe := int(x[0x3c]) | int(x[0x3d])<<8 | int(x[0x3e])<<16 | int(x[0x3f])<<24
	// the signature (4 bytes), the file header (20), CheckSum at 64
	if c := pe + 4 + 20 + 64; c+4 <= len(x) {
		copy(x[c:c+4], make([]byte, 4))
	}
	// every RSDS followed by a path ending in .pdb: a program can hold
	// the four letters elsewhere (xo's own PE writer does)
	for at := 0; ; {
		n := bytes.Index(x[at:], []byte("RSDS"))
		if n < 0 {
			break
		}
		i := at + n
		at = i + 4
		j := i + 4 + 16 + 4
		for j < len(x) && x[j] != 0 {
			j++
		}
		if j > len(x) || i+24 > j || !bytes.HasSuffix(x[i+24:j], []byte(".pdb")) {
			continue
		}
		copy(x[i+4:j], make([]byte, j-i-4))
	}
	// the debug directory's CodeView entries' TimeDateStamp (taken from
	// the GUID): data directory 6 of a PE32+ optional header, its RVA
	// mapped to the file through the section table
	le32 := func(o int) int {
		if o < 0 || o+4 > len(x) {
			return -1
		}
		return int(x[o]) | int(x[o+1])<<8 | int(x[o+2])<<16 | int(x[o+3])<<24
	}
	opt := pe + 4 + 20
	if opt+112+7*8 > len(x) || x[opt] != 0x0b || x[opt+1] != 0x02 {
		return x
	}
	rva, size := le32(opt+112+6*8), le32(opt+112+6*8+4)
	nsec := int(x[pe+6]) | int(x[pe+7])<<8
	sec := opt + (int(x[pe+20]) | int(x[pe+21])<<8)
	for k := 0; k < nsec; k++ {
		h := sec + 40*k
		va, vsz, raw := le32(h+12), le32(h+8), le32(h+20)
		if va < 0 || rva < va || rva >= va+vsz {
			continue
		}
		for e := raw + rva - va; e+28 <= raw+rva-va+size && e+28 <= len(x); e += 28 {
			if le32(e+12) == 2 { // IMAGE_DEBUG_TYPE_CODEVIEW
				copy(x[e+4:e+8], make([]byte, 4))
			}
		}
	}
	return x
}

func equal(a, b string, macho bool) (bool, error) {
	if !macho {
		x, err := os.ReadFile(a)
		if err != nil {
			return false, err
		}
		y, err := os.ReadFile(b)
		if err != nil {
			return false, err
		}
		return bytes.Equal(maskPE(x), maskPE(y)), nil
	}
	dis := func(p string) ([]byte, error) {
		o, err := exec.Command("objdump", "-d", p).Output()
		if err != nil {
			return nil, fmt.Errorf("objdump %s: %v", p, err)
		}
		// the first lines name the file
		if i := bytes.Index(o, []byte("\n\n")); i >= 0 {
			o = o[i:]
		}
		return o, nil
	}
	x, err := dis(a)
	if err != nil {
		return false, err
	}
	y, err := dis(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(x, y), nil
}

// cleanPair removes a compared pair and its debug bundles.
func cleanPair(out string) {
	for _, s := range []string{"a", "b"} {
		os.RemoveAll(filepath.Join(out, s, "prog"))
		os.RemoveAll(filepath.Join(out, s, "prog.dSYM"))
		os.RemoveAll(filepath.Join(out, s, "prog.exe"))
		os.RemoveAll(filepath.Join(out, s, "prog.pdb"))
	}
}

func tail(s string, n int) string {
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return strings.Join(ls, "\n")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "identity:", err)
	os.Exit(2)
}
