package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// SplitConfig is how an LLVM build divides its IR (decision 0168).
type SplitConfig struct {
	// Modules is the number of modules (llvm.Split); 1 compiles the IR
	// as one module with full LTO, the default.
	Modules int
	// Jobs bounds the clang processes compiling modules at once, across
	// every build of the process.
	Jobs int
	// LTO is the link time optimization of split builds: "thin" or
	// "none".
	LTO string
}

// LLVMSplitConfig reads the split of LLVM builds from the environment:
// XO_LLVM_SPLIT (the module count; unset, "", "0", "1", or "off": one
// module), XO_LLVM_JOBS (default min(cores, 6): each clang at -O2 on a
// part of a large program holds a few hundred MB), XO_LLVM_LTO (thin,
// the default, or none).
func LLVMSplitConfig() SplitConfig {
	c := SplitConfig{Modules: 1, Jobs: min(runtime.NumCPU(), 6), LTO: "thin"}
	if n, err := strconv.Atoi(os.Getenv("XO_LLVM_SPLIT")); err == nil && n > 1 {
		c.Modules = n
	}
	if n, err := strconv.Atoi(os.Getenv("XO_LLVM_JOBS")); err == nil && n > 0 {
		c.Jobs = n
	}
	if v := os.Getenv("XO_LLVM_LTO"); v == "none" || v == "thin" {
		c.LTO = v
	}
	return c
}

var (
	splitSemMu sync.Mutex
	splitSem   chan struct{}
)

// splitSlots is the process-wide bound on module compiles.
func splitSlots(jobs int) chan struct{} {
	splitSemMu.Lock()
	defer splitSemMu.Unlock()
	if splitSem == nil || cap(splitSem) != jobs {
		splitSem = make(chan struct{}, jobs)
	}
	return splitSem
}

// ltoFlags are clang's flags for an LTO mode of split builds.
func ltoFlags(lto string) []string {
	if lto == "thin" {
		return []string{"-flto=thin"}
	}
	return nil
}

// splitBuild compiles the modules lls of a split LLVM build in parallel
// (at most c.Jobs clang processes), then links their objects with the
// other sources (the C runtime, zone data, `use c` sources) into bin.
func splitBuild(clang string, lls, others []string, bin string, c SplitConfig, flags []string) error {
	objs := make([]string, len(lls))
	errs := make([]error, len(lls))
	sem := splitSlots(c.Jobs)
	var wg sync.WaitGroup
	for i, ll := range lls {
		objs[i] = strings.TrimSuffix(ll, ".ll") + ".o"
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			args := append([]string{LLVMOptLevel}, ltoFlags(c.LTO)...)
			args = append(args, "-Wno-override-module", "-fexceptions", "-funwind-tables", "-c", ll, "-o", objs[i])
			cmd := exec.Command(clang, args...)
			var b bytes.Buffer
			cmd.Stdout, cmd.Stderr = &b, &b
			if err := cmd.Run(); err != nil {
				errs[i] = &ClangError{Output: fmt.Sprintf("%s: %v\n%s", ll, err, b.String())}
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	lf := append(append([]string{LLVMOptLevel}, ltoFlags(c.LTO)...), "-Wno-override-module")
	return ccBuild(clang, append(lf, flags...), append(objs, others...), bin)
}

// fileExists reports whether path names an existing file.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Split builds of tests (decision 0168): the module count and job bound
// EnableTestLLVMSplit sets when the environment names none.
const (
	testSplitModules = "12"
	testSplitJobs    = "4"
)

// EnableTestLLVMSplit makes the LLVM builds of a test process split
// (XO_LLVM_SPLIT, XO_LLVM_JOBS) unless the environment already sets
// them: the self hosted compiler's tests build programs of hundreds of
// MB of IR whose single module compile takes over half an hour.
func EnableTestLLVMSplit() {
	if _, ok := os.LookupEnv("XO_LLVM_SPLIT"); !ok {
		os.Setenv("XO_LLVM_SPLIT", testSplitModules)
	}
	if _, ok := os.LookupEnv("XO_LLVM_JOBS"); !ok {
		os.Setenv("XO_LLVM_JOBS", testSplitJobs)
	}
}
