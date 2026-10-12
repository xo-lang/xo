package selfhost

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The work of the package runs in units drawn from one pool of
// selfhostJobs slots (XO_SELFHOST_JOBS; default min(GOMAXPROCS, 4), so
// a development Mac does not swap; scripts/remote-full.sh sets the core
// count): a native build of a component (selfhostBinary), a process of
// a self hosted binary on a batch of runs (runXoBatches), and a Go side
// item of parallel (the Go checker, SSA builder, or emitter on one
// run). The backends' subtests run at the same time and the Go side
// results are computed while the Xo processes run (background), all
// drawing from the pool. A heavy unit (a build, or a mergeHeavy run:
// the self hosted compiler's components, from 1 to about 6 GB each)
// also takes a slot of a second pool of selfhostHeavyJobs
// (XO_SELFHOST_HEAVY_JOBS; default one per 8 GiB of memory, 16 GiB in
// the full tier, at most selfhostJobs), which bounds the peak memory where the job count alone
// would not (knowledgebase/infrastructure/toolchain.md, the
// XO_SELFHOST_JOBS line). A unit takes its heavy slot before its job
// slot and never waits for another slot while it holds a job slot.
var (
	jobsOnce sync.Once
	jobsSem  chan struct{}
	heavySem chan struct{}
	jobsErr  error
)

// selfhostJobs is the size of the pool.
func selfhostJobs() int {
	jobsOnce.Do(initJobs)
	return cap(jobsSem)
}

// selfhostHeavyJobs is the size of the pool of heavy units.
func selfhostHeavyJobs() int {
	jobsOnce.Do(initJobs)
	return cap(heavySem)
}

func initJobs() {
	count := func(name string, def int) int {
		v := os.Getenv(name)
		if v == "" {
			return def
		}
		k, err := strconv.Atoi(v)
		if err != nil || k < 1 {
			jobsErr = fmt.Errorf("%s=%q: want a positive count", name, v)
			return def
		}
		return k
	}
	n := count("XO_SELFHOST_JOBS", max(1, min(runtime.GOMAXPROCS(0), 4)))
	h := n
	if mem := totalMemory(); mem > 0 {
		// 8 GiB per heavy unit, 16 GiB in the full tier: there the
		// Xo arm64 emitter on a self hosted component holds 5 to 13 GB
		// (seven at once filled 59 of 63 GB on c8g.8xlarge and the
		// kernel killed one, 2026-10-12)
		shift := 33
		if !testing.Short() && os.Getenv("XO_SELFHOST_TIER") == tierFull {
			shift = 34
		}
		h = max(1, min(n, int(mem>>shift)))
	}
	h = count("XO_SELFHOST_HEAVY_JOBS", h)
	jobsSem = make(chan struct{}, n)
	heavySem = make(chan struct{}, h)
}

// acquireJob takes a slot of the pool (and first one of the heavy pool
// when heavy) and returns its release.
func acquireJob(heavy bool) func() {
	jobsOnce.Do(initJobs)
	if heavy {
		heavySem <- struct{}{}
	}
	jobsSem <- struct{}{}
	return func() {
		<-jobsSem
		if heavy {
			<-heavySem
		}
	}
}

// runUnits runs f(i) for each i of order, each in a slot of the pools
// (heavy(i) says which), starting them in that order as slots free up;
// a heavy unit waiting for its heavy slot lets the light ones after it
// start.
func runUnits(order []int, heavy func(i int) bool, f func(i int)) {
	var wg sync.WaitGroup
	for _, i := range order {
		wg.Add(1)
		if heavy(i) {
			go func() {
				defer wg.Done()
				defer acquireJob(true)()
				f(i)
			}()
			continue
		}
		release := acquireJob(false)
		go func() {
			defer wg.Done()
			defer release()
			f(i)
		}()
	}
	wg.Wait()
}

// parallel runs f(i) for each of dirs, each call in a slot of the pools
// (mergeHeavy dirs in a heavy one): the runs of the Go checker, SSA
// builder, and emitters (what, in the log line) are independent. f must
// not take a slot itself. It logs the time and the longest item.
func parallel(t testing.TB, what string, dirs []string, f func(i int)) {
	t.Helper()
	order := make([]int, len(dirs))
	for i := range order {
		order[i] = i
	}
	// the heavy runs first: they take longest
	sort.SliceStable(order, func(a, b int) bool { return mergeHeavy(dirs[order[a]]) && !mergeHeavy(dirs[order[b]]) })
	took := make([]time.Duration, len(dirs))
	start := time.Now()
	runUnits(order, func(i int) bool { return mergeHeavy(dirs[i]) }, func(i int) {
		t0 := time.Now()
		f(i)
		took[i] = time.Since(t0)
	})
	if len(dirs) > 0 {
		long := 0
		for i := range took {
			if took[i] > took[long] {
				long = i
			}
		}
		t.Logf("%s: %d items on %d jobs (%d heavy), %.1f s; longest %.1f s (%s)",
			what, len(dirs), selfhostJobs(), selfhostHeavyJobs(), time.Since(start).Seconds(), took[long].Seconds(), dirs[long])
	}
}

// background runs f (the Go side results of a test, which use the
// working directory the test set) while the test goes on to its
// subtests, and returns the wait for it; t's cleanup also waits, before
// the working directory is restored.
func background(t *testing.T, f func()) (wait func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	t.Cleanup(func() { <-done })
	return func() { <-done }
}

// prebuildFull starts the native builds of every component the full
// tier compares (in the order the tests need them, each in a slot of
// the pool), so the slow LLVM and arm64 builds overlap the earlier
// tests instead of starting with the test that needs them. Only for a
// whole full tier run (no -run, no XO_SELFHOST_BACKEND).
func prebuildFull() {
	if testing.Short() || goldensRecording() || os.Getenv("XO_SELFHOST_TIER") != tierFull || os.Getenv("XO_SELFHOST_BACKEND") != "" {
		return
	}
	if f := flag.Lookup("test.run"); f != nil && f.Value.String() != "" {
		return
	}
	backends := []string{"go"}
	for _, b := range []string{"llvm", "arm64"} {
		if nativeSkip(b) == "" {
			backends = append(backends, b)
		}
	}
	// the file order of the tests: arm64, check, gen, lexer, llvm,
	// parser, ssa, wasm; the native backends first within each (the
	// slow builds)
	var keys [][2]string
	for _, c := range []string{"arm64", "check", "gen", "lexer", "llvm", "parser", "ssa"} {
		for i := len(backends) - 1; i >= 0; i-- {
			keys = append(keys, [2]string{c, backends[i]})
		}
	}
	keys = append(keys, [2]string{"wasm", "go"})
	go func() {
		for _, k := range keys {
			e := binEntryFor(k[0], k[1])
			if e.started.Load() {
				continue
			}
			release := acquireJob(true)
			go func() {
				defer release()
				buildBinary(e, k[0], k[1], false)
			}()
		}
	}()
}

// xoBatch is one process of runXoBatches: the paths it was given and
// its standard output.
type xoBatch struct {
	paths []string
	out   string
}

// planBatches splits paths (relative to root) into the batches of
// runXoBatches: each mergeHeavy run alone (they take from seconds to
// minutes where the others take under one), the others in path order in
// batches of at most size, smaller when that leaves slots of the pool
// idle (at least jobs batches when there are enough paths).
func planBatches(paths []string, size, jobs int) [][]string {
	var light []string
	var chunks [][]string
	for _, p := range paths {
		if mergeHeavy(p) {
			chunks = append(chunks, []string{p})
		} else {
			light = append(light, p)
		}
	}
	size = max(1, min(size, (len(light)+jobs-1)/jobs))
	for i := 0; i < len(light); i += size {
		chunks = append(chunks, light[i:min(i+size, len(light))])
	}
	return chunks
}

// batchCost estimates the work of a batch: the bytes of .xo source in
// its directories and in the repository packages they use, at any
// depth (selfhost/xo uses most of the other components).
func batchCost(root string, paths []string) int64 {
	var n int64
	seen := map[string]bool{}
	var walk func(dir string)
	walk = func(dir string) {
		if seen[dir] {
			return
		}
		seen[dir] = true
		files, _ := filepath.Glob(filepath.Join(dir, "*.xo"))
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			n += int64(len(data))
			for _, m := range repoUse.FindAllSubmatch(data, -1) {
				walk(filepath.Join(root, string(m[1])))
			}
		}
	}
	for _, p := range paths {
		walk(filepath.Join(root, p))
	}
	return n
}

// repoUse matches a `use` of a package of this repository.
var repoUse = regexp.MustCompile(`(?m)^use github\.com/xo-lang/xo/(\S+)`)

// runXoBatches runs bin with prefix on each batch of paths
// (planBatches; the largest first, each in a slot of the pools) from
// root and returns the batches in plan order (the first failure in
// that order fails t). It logs the number of processes, the longest,
// and the largest peak resident size.
func runXoBatches(t testing.TB, bin, root string, prefix, paths []string, size int) []xoBatch {
	t.Helper()
	chunks := planBatches(paths, size, selfhostJobs())
	costs := make([]int64, len(chunks))
	order := make([]int, len(chunks))
	for i, c := range chunks {
		costs[i] = batchCost(root, c)
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return costs[order[a]] > costs[order[b]] })
	outs := make([]xoBatch, len(chunks))
	errs := make([]error, len(chunks))
	took := make([]time.Duration, len(chunks))
	rss := make([]int64, len(chunks))
	start := time.Now()
	heavy := func(i int) bool { return len(chunks[i]) == 1 && mergeHeavy(chunks[i][0]) }
	runUnits(order, heavy, func(i int) {
		cmd := exec.Command(bin, append(append([]string(nil), prefix...), chunks[i]...)...)
		cmd.Dir = root
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		t0 := time.Now()
		if err := cmd.Run(); err != nil {
			errs[i] = fmt.Errorf("%s on %d paths (%s): %v\n%s", filepath.Base(bin), len(chunks[i]), chunks[i][0], err, stderr.String())
		}
		took[i] = time.Since(t0)
		rss[i] = maxRSS(cmd.ProcessState)
		outs[i] = xoBatch{paths: chunks[i], out: stdout.String()}
	})
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(chunks) > 0 {
		long, big := 0, 0
		for i := range chunks {
			if took[i] > took[long] {
				long = i
			}
			if rss[i] > rss[big] {
				big = i
			}
		}
		name := func(i int) string {
			if len(chunks[i]) == 1 {
				return chunks[i][0]
			}
			return fmt.Sprintf("%s and %d more", chunks[i][0], len(chunks[i])-1)
		}
		peak := "n/a"
		if rss[big] > 0 {
			peak = fmt.Sprintf("%d MB (%s)", rss[big]>>20, name(big))
		}
		t.Logf("%s %s: %d paths in %d processes on %d jobs (%d heavy), %.1f s; longest %.1f s (%s); peak RSS %s",
			filepath.Base(bin), strings.Join(prefix, " "), len(paths), len(chunks), selfhostJobs(), selfhostHeavyJobs(),
			time.Since(start).Seconds(), took[long].Seconds(), name(long), peak)
	}
	return outs
}

// compareAll runs cmp on every dir at the same time, each in a slot of
// the pools (comparing the output of a large run takes seconds to
// minutes: run by run, the comparisons were most of TestSelfhostSSA in
// the full tier, 2026-10-12), each with its own counts, then adds the
// counts and joins the failures in the order of dirs.
func compareAll[S any](t testing.TB, what string, dirs []string, cmp func(i int, st *S) []string, add func(dst *S, src S)) (S, []string) {
	t.Helper()
	sts := make([]S, len(dirs))
	fails := make([][]string, len(dirs))
	parallel(t, what, dirs, func(i int) { fails[i] = cmp(i, &sts[i]) })
	var st S
	var all []string
	for i := range dirs {
		add(&st, sts[i])
		all = append(all, fails[i]...)
	}
	return st, all
}

// addCounts adds the counts of src to *dst.
func addCounts(dst *map[string]int, src map[string]int) {
	if src == nil {
		return
	}
	if *dst == nil {
		*dst = map[string]int{}
	}
	for k, v := range src {
		(*dst)[k] += v
	}
}
