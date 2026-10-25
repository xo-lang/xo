//go:build unix

package main

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestBuildStagedOutputs covers the hidden staging of final outputs
// (decision 0345's output paths, owner 2026-10-22): two concurrent
// `xo build -o same` both succeed and leave nothing but the program and
// its debug information; no visible file appears in the output
// directory while they build (a poll every 2 ms); a SIGINT during a
// build removes its hidden stage.
func TestBuildStagedOutputs(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
		"out/.keep":   "",
	})
	chdir(t, filepath.Join(dir, "app"))
	setenv(t, "XO_BIN_CACHE", "off")
	outDir := filepath.Join(dir, "out")
	final := filepath.Join(outDir, "prog")
	// visible names other than the outputs, seen while building
	var mu sync.Mutex
	var seen []string
	stop := make(chan struct{})
	polled := make(chan struct{})
	go func() {
		defer close(polled)
		for {
			select {
			case <-stop:
				return
			default:
			}
			ents, _ := os.ReadDir(outDir)
			for _, e := range ents {
				n := e.Name()
				if !strings.HasPrefix(n, ".") && n != "prog" && n != "prog.dSYM" {
					mu.Lock()
					seen = append(seen, n)
					mu.Unlock()
				}
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	var wg sync.WaitGroup
	codes := make([]int, 2)
	errs := make([]string, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out, errOut bytes.Buffer
			codes[i] = run(t, []string{"build", "-o", final, "."}, &out, &errOut)
			errs[i] = errOut.String()
		}()
	}
	wg.Wait()
	close(stop)
	<-polled
	for i := range 2 {
		if codes[i] != 0 {
			t.Errorf("build %d: exit %d\n%s", i, codes[i], errs[i])
		}
	}
	if len(seen) > 0 {
		t.Errorf("visible files during the builds: %v", seen)
	}
	ents, _ := os.ReadDir(outDir)
	for _, e := range ents {
		if n := e.Name(); n != ".keep" && n != "prog" && n != "prog.dSYM" {
			t.Errorf("left in the output directory: %s", n)
		}
	}
	if fi, err := os.Stat(final); err != nil || fi.Mode()&0o111 == 0 {
		t.Errorf("no executable %s: %v", final, err)
	}

	// a SIGINT while a stage exists: the stage goes, xo exits 130
	cli, err := xoCommand()
	if err != nil {
		t.Fatal(err)
	}
	for try := 0; try < 5; try++ {
		cmd := command(t, cli[0], append(append([]string{}, cli[1:]...), "build", "-o", filepath.Join(outDir, "int"), ".")...)
		var errOut bytes.Buffer
		cmd.Stderr = &errOut
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		staged := false
		for i := 0; i < 5000 && !staged; i++ {
			ms, _ := filepath.Glob(filepath.Join(outDir, ".int.tmp-*"))
			staged = len(ms) > 0
			if !staged {
				time.Sleep(time.Millisecond)
			}
		}
		if staged {
			cmd.Process.Signal(syscall.SIGINT) // the process this test started
		}
		cmd.Wait()
		ms, _ := filepath.Glob(filepath.Join(outDir, ".int.tmp-*"))
		if len(ms) > 0 {
			t.Errorf("a SIGINT left %v", ms)
		}
		if staged && cmd.ProcessState.ExitCode() == 130 {
			return
		}
		os.Remove(filepath.Join(outDir, "int"))
		os.RemoveAll(filepath.Join(outDir, "int.dSYM"))
	}
	t.Log("the SIGINT never fell inside a stage in 5 tries (the link was too quick); only the no-leftover checks ran")
}

// TestBuildDevNull: `xo build -o /dev/null` builds and writes the program
// to the device: /dev takes no stage directory, so the program is staged
// in the user cache and its bytes written to the output (dfd47760e's
// stages had broken the idiom: "cannot write the program to /dev/null").
func TestBuildDevNull(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/null")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
	})
	for _, be := range []string{"", "--backend=go"} {
		args := []string{"build", "-o", "/dev/null"}
		if be != "" {
			args = append(args, be)
		}
		var out, errb bytes.Buffer
		if code := run(t, append(args, filepath.Join(dir, "app")), &out, &errb); code != 0 {
			t.Errorf("xo build %s -o /dev/null: exit %d\n%s%s", be, code, out.String(), errb.String())
		}
	}
}

// TestBuildWorkDirsPrivate: the files of a build (assembly, part objects)
// are in a directory of that build, so concurrent builds of one program
// never write the file another links (two concurrent builds failed once
// with "prog-darwin.1.o: not a 64 bit Mach-O file", 2026-10-24); the
// directory is gone after the link, and XO_KEEP_ASM=1 keeps the assembly
// beside the build directory's other files.
func TestBuildWorkDirsPrivate(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
		"out/.keep":   "",
	})
	chdir(t, filepath.Join(dir, "app"))
	setenv(t, "XO_BIN_CACHE", "off")
	setenv(t, "XO_WHOLE_BUILD", "off")
	bdir := filepath.Join(dir, "build")
	setenv(t, "XO_BUILD_DIR", bdir)
	for round := 0; round < 4; round++ {
		var wg sync.WaitGroup
		codes := make([]int, 4)
		errs := make([]string, 4)
		for i := range codes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var out, errOut bytes.Buffer
				codes[i] = run(t, []string{"build", "-o", filepath.Join(dir, "out", "p"+strconv.Itoa(i)), "."}, &out, &errOut)
				errs[i] = errOut.String()
			}()
		}
		wg.Wait()
		for i, c := range codes {
			if c != 0 {
				t.Errorf("round %d build %d: exit %d\n%s", round, i, c, errs[i])
			}
		}
	}
	if left, _ := filepath.Glob(filepath.Join(bdir, "*", "*", "w-*")); len(left) > 0 {
		t.Errorf("work directories left after the links: %v", left)
	}
	if kept, _ := filepath.Glob(filepath.Join(bdir, "*", "*", "prog-*.o")); len(kept) > 0 && runtime.GOARCH == "arm64" {
		t.Errorf("part objects left in the build directory: %v", kept)
	}

	setenv(t, "XO_KEEP_ASM", "1")
	var out, errOut bytes.Buffer
	if c := run(t, []string{"build", "-o", filepath.Join(dir, "out", "keep"), "."}, &out, &errOut); c != 0 {
		t.Fatalf("XO_KEEP_ASM build: exit %d\n%s", c, errOut.String())
	}
	if kept, _ := filepath.Glob(filepath.Join(bdir, "*", "*", "prog-*")); len(kept) == 0 {
		t.Errorf("XO_KEEP_ASM=1 kept no prog-* file in the build directory")
	}
}

// TestBuildCCLinkOSOPaths: a program linked by cc on darwin (a `use c`
// source) records its objects' paths relative to the build's private
// directory, so two builds are equal byte for byte (TestModuleCheckAuto
// failed on the random directory's name, 2026-10-24), and its line
// tables are still in the program after the directory is gone (dsymutil
// finds the objects by --oso-prepend-path).
func TestBuildCCLinkOSOPaths(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("darwin/arm64: the Mach-O debug map")
	}
	if _, err := exec.LookPath("dwarfdump"); err != nil {
		t.Skip("no dwarfdump")
	}
	src, err := filepath.Abs(filepath.Join("..", "..", "internal", "llvm", "testdata", "golden", "use_c_add"))
	if err != nil {
		t.Fatal(err)
	}
	chdir(t, src)
	setenv(t, "XO_BIN_CACHE", "off")
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	out := filepath.Join(t.TempDir(), "p")
	var prog [2][]byte
	for i := range prog {
		os.Remove(out)
		var o, e bytes.Buffer
		if c := run(t, []string{"build", "--backend=arm64", "--debug-info=on", "-o", out, "."}, &o, &e); c != 0 {
			t.Fatalf("build %d: exit %d\n%s", i, c, e.String())
		}
		prog[i], err = os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(prog[0], prog[1]) {
		t.Errorf("two builds of one program differ byte for byte")
	}
	lines, err := exec.Command("dwarfdump", "--debug-line", out).Output()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(lines, []byte("main.xo")) {
		t.Errorf("no line table for main.xo in the program")
	}

	// the same for the LLVM backend (its objects are an LTO object)
	for i := range prog {
		os.Remove(out)
		var o, e bytes.Buffer
		if c := run(t, []string{"build", "--backend=llvm", "--debug-info=on", "-o", out, "."}, &o, &e); c != 0 {
			t.Fatalf("llvm build %d: exit %d\n%s", i, c, e.String())
		}
		prog[i], err = os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(prog[0], prog[1]) {
		t.Errorf("two LLVM builds of one program differ byte for byte")
	}
	lines, err = exec.Command("dwarfdump", "--debug-line", out).Output()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(lines, []byte("main.xo")) {
		t.Errorf("no line table for main.xo in the LLVM program")
	}
}

// TestBuildNativeReproducible: a native program built twice, each time
// with a private backend cache, plain and --release, is equal byte for
// byte when the output has one name (2026-10-24: two builds named p1 and
// p2 differed in one byte, the ad hoc signature's identifier, which is
// the output's base name; gotchas/build-output-name-in-signature.md).
func TestBuildNativeReproducible(t *testing.T) {
	if testing.Short() {
		t.Skip("builds native programs")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("darwin/arm64: the ad hoc signature")
	}
	src, err := filepath.Abs(filepath.Join("..", "..", "internal", "llvm", "testdata", "golden", "arith"))
	if err != nil {
		t.Fatal(err)
	}
	chdir(t, src)
	setenv(t, "XO_BIN_CACHE", "off")
	for _, mode := range []string{"", "--release"} {
		var prog [2][]byte
		for i := range prog {
			setenv(t, "XO_BACKEND_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
			out := filepath.Join(t.TempDir(), "prog")
			args := []string{"build"}
			if mode != "" {
				args = append(args, mode)
			}
			args = append(args, "-o", out, ".")
			var o, e bytes.Buffer
			if c := run(t, args, &o, &e); c != 0 {
				t.Fatalf("build %d %q: exit %d\n%s", i, mode, c, e.String())
			}
			prog[i], err = os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
		}
		if !bytes.Equal(prog[0], prog[1]) {
			t.Errorf("two native builds %q of one program differ byte for byte", mode)
		}
	}
}

// TestReleaseTracebackDebugInfo: a --release build has no debug info, so
// the DWARF readers are neither built nor linked (dwline.built() folds
// to false, decision 0354's addendum) and its traceback names the
// functions only; --release --debug-info=on keeps them and the traceback
// has file:line.
func TestReleaseTracebackDebugInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("darwin/arm64: the dSYM")
	}
	src, err := filepath.Abs(filepath.Join("..", "..", "internal", "arm64", "testdata", "traceback"))
	if err != nil {
		t.Fatal(err)
	}
	chdir(t, src)
	setenv(t, "XO_BIN_CACHE", "off")
	build := func(extra ...string) string {
		out := filepath.Join(t.TempDir(), "prog")
		args := append([]string{"build", "--release"}, extra...)
		args = append(args, "-o", out, ".")
		var o, e bytes.Buffer
		if c := run(t, args, &o, &e); c != 0 {
			t.Fatalf("build %v: exit %d\n%s", extra, c, e.String())
		}
		return out
	}
	trace := func(bin string) string {
		cmd := exec.Command(bin)
		var eb bytes.Buffer
		cmd.Stderr = &eb
		if err := cmd.Run(); err == nil {
			t.Fatalf("%s: no fault", bin)
		}
		return eb.String()
	}
	const head = "fault: index 7 out of range for List of length 3 at main.xo:5:12\ntraceback:\n"
	rel := build()
	if got, want := trace(rel), head+"  down\n  down\n  down\n  down\n  main\n"; got != want {
		t.Errorf("release traceback:\n%s\nwant:\n%s", got, want)
	}
	di := build("--debug-info=on")
	want := head + "  down (main.xo:5)\n  down (main.xo:7)\n  down (main.xo:7)\n  down (main.xo:7)\n  main (main.xo:11)\n"
	if got := trace(di); got != want {
		t.Errorf("--debug-info=on traceback:\n%s\nwant:\n%s", got, want)
	}
	if _, err := exec.LookPath("nm"); err == nil {
		syms := func(bin string) string {
			b, _ := exec.Command("nm", bin).Output()
			return string(b)
		}
		// parse_info reads .debug_info: the readers, not the small
		// accessors (find, inline_chain) that stay linked
		if strings.Contains(syms(rel), "dwline_2eparse") {
			t.Errorf("the release program links the DWARF readers")
		}
		if !strings.Contains(syms(di), "dwline_2eparse") {
			t.Errorf("the --debug-info=on program has no DWARF readers")
		}
	}
}

// TestBuildCarriesNoBuildPaths: a program built in a temporary directory,
// plain and --release, holds neither that directory nor the home directory
// (nor the checkout it was built with) anywhere in its bytes: the build
// information has no directory (dir is empty), and the DWARF compile and
// std directories are relative to the working directory (--trimpath, on by
// default; decision 0354's addendum, Go's -trimpath).
func TestBuildCarriesNoBuildPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	home, _ := os.UserHomeDir()
	dir := writeFiles(t, map[string]string{"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"})
	setenv(t, "XO_BUILD_DIR", filepath.Join(dir, "build"))
	app := filepath.Join(dir, "app")
	chdir(t, dir)
	for _, c := range []struct {
		name  string
		flags []string
	}{{"plain", nil}, {"release", []string{"--release"}}, {"release-debug", []string{"--release", "--debug-info=on"}}} {
		bin := filepath.Join(dir, c.name)
		var out, errOut bytes.Buffer
		if code := run(t, append(append([]string{"build"}, c.flags...), "-o", bin, app), &out, &errOut); code != 0 {
			t.Fatalf("%s: build exit %d\n%s", c.name, code, errOut.String())
		}
		data, err := os.ReadFile(bin)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{dir, home} {
			if p != "" && p != "/" && bytes.Contains(data, []byte(p)) {
				t.Errorf("%s: the binary holds %s", c.name, p)
			}
		}
	}
	// --trimpath=off keeps the absolute directories
	bin := filepath.Join(dir, "abs")
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "--trimpath=off", "-o", bin, app}, &out, &errOut); code != 0 {
		t.Fatalf("--trimpath=off: build exit %d\n%s", code, errOut.String())
	}
	if data, err := os.ReadFile(bin); err != nil || !bytes.Contains(data, []byte(app)) {
		t.Errorf("--trimpath=off: the binary lacks the program's directory %s (%v)", app, err)
	}
}

// TestBuildLLVMWorkDirsPrivate: concurrent LLVM builds of different
// programs each write their IR and split modules into a directory of
// their own, removed after the link, so no prog*.ll is left in the build
// directory and no build reads another's IR; XO_KEEP_ASM=1 keeps them
// (2026-10-24: the shared llvm directory raced like the native one, and
// its IR stayed on disk until the prune).
func TestBuildLLVMWorkDirsPrivate(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("no clang")
	}
	files := map[string]string{"out/.keep": ""}
	for i := 0; i < 3; i++ {
		n := strconv.Itoa(i)
		files["app"+n+"/xo.mod"] = "module app\n"
		files["app"+n+"/main.xo"] = "fn main(os: Os) {\n  os.stdio.println(\"hi " + n + "\")\n}\n"
	}
	dir := writeFiles(t, files)
	setenv(t, "XO_BIN_CACHE", "off")
	setenv(t, "XO_WHOLE_BUILD", "off")
	bdir := filepath.Join(dir, "build")
	setenv(t, "XO_BUILD_DIR", bdir)
	leftover := func() []string {
		var left []string
		filepath.WalkDir(bdir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, ".ll") {
				left = append(left, p)
			}
			return nil
		})
		return left
	}
	for _, split := range []string{"1", "3"} {
		setenv(t, "XO_LLVM_SPLIT", split)
		var wg sync.WaitGroup
		codes := make([]int, 3)
		errs := make([]string, 3)
		for i := range codes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				n := strconv.Itoa(i)
				var out, errOut bytes.Buffer
				codes[i] = run(t, []string{"build", "--backend=llvm", "-o", filepath.Join(dir, "out", "p"+split+n), filepath.Join(dir, "app"+n)}, &out, &errOut)
				errs[i] = errOut.String()
			}()
		}
		wg.Wait()
		for i, c := range codes {
			if c != 0 {
				t.Errorf("split %s build %d: exit %d\n%s", split, i, c, errs[i])
				continue
			}
			got, err := exec.Command(filepath.Join(dir, "out", "p"+split+strconv.Itoa(i))).Output()
			if err != nil || string(got) != "hi "+strconv.Itoa(i)+"\n" {
				t.Errorf("split %s program %d printed %q (%v)", split, i, got, err)
			}
		}
		if left := leftover(); len(left) > 0 {
			t.Errorf("split %s: IR left in the build directory: %v", split, left)
		}
	}

	setenv(t, "XO_KEEP_ASM", "1")
	var out, errOut bytes.Buffer
	if c := run(t, []string{"build", "--backend=llvm", "-o", filepath.Join(dir, "out", "keep"), filepath.Join(dir, "app0")}, &out, &errOut); c != 0 {
		t.Fatalf("XO_KEEP_ASM build: exit %d\n%s", c, errOut.String())
	}
	if len(leftover()) == 0 {
		t.Errorf("XO_KEEP_ASM=1 kept no .ll file in the build directory")
	}
}

// TestBuildStopsAtAssemblyKeepsIt: a cross build that cannot link (cc is
// not run for another target) stops at the assembly it names, and that
// file is there to open (the build's private work directory would have
// been removed; 2026-10-24).
func TestBuildStopsAtAssemblyKeepsIt(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("linux/arm64 is another target only on an arm64 Mac")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n",
	})
	chdir(t, filepath.Join(dir, "app"))
	setenv(t, "XO_BUILD_DIR", filepath.Join(dir, "build"))
	var out, errOut bytes.Buffer
	code := run(t, []string{"build", "--backend=arm64", "--ld=cc", "--targets=linux/arm64", "-o", filepath.Join(dir, "p"), "."}, &out, &errOut)
	if code == 0 {
		t.Fatalf("exit 0, want the stop at the assembly\n%s", errOut.String())
	}
	m := regexp.MustCompile(`its assembly: (\S+prog-linux\.s)\)`).FindStringSubmatch(errOut.String())
	if m == nil {
		t.Fatalf("no assembly path in the message:\n%s", errOut.String())
	}
	if fi, err := os.Stat(m[1]); err != nil || fi.Size() == 0 {
		t.Errorf("the assembly the message names is not there: %v", err)
	}
}
