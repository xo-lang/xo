package arm64

import (
	"bytes"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestWarmBuildSequence builds std-using programs through caches that
// earlier builds of other programs filled, with xo's default cache settings
// (front end, check, back-end and binary caches on, as a user's builds run):
//   - pairs: every ordered pair of programs sharing a std module, A then B,
//     each pair on its own fresh cache;
//   - sequences: all the programs one after another through one cache, in
//     name order, in reverse and in a shuffled order.
//
// Each build must be the same file as its cold build (every cache off,
// XO_SEPCOMP=0, the same output name: the name is the only other byte, in
// the code signature), its run must match want.txt, and no front end cache
// entry may fail to replay (xo warns and falls back).
//
// The 0.51.0 respin's bug (2026-11-13, fixed by 80638063b): building one
// std/log program and then another failed with XO1201, since a replayed
// front end cache entry's interface type lacked its method tables. Every
// release gate built one program per fresh cache or ran with the caches
// off, so only a user's second build met it (release-0.51.0 retro). The
// pairs find it; a long sequence alone can miss it, since a later std/log
// build repaired the entry first.
//
// XO_WARMSEQ picks the programs: gate (the default, for scripts/gate.sh:
// the std/log goldens, all their ordered pairs, and one golden of each of
// ten common modules in the sequences), modules (the 2-hourly main check:
// one golden per std module in the sequences, and pairs among the first 3
// goldens of each module) or all (every golden that uses std; pairs among
// the first 4 of each module). XO_WARMSEQ_SEED sets the shuffle (default:
// the day number, so each day checks another order; the seed is logged).
func TestWarmBuildSequence(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	progs, pairs := warmSeqPrograms(t, os.Getenv("XO_WARMSEQ"))
	seed := time.Now().Unix() / 86400
	if s := os.Getenv("XO_WARMSEQ_SEED"); s != "" {
		if seed, err = strconv.ParseInt(s, 10, 64); err != nil {
			t.Fatalf("XO_WARMSEQ_SEED: %v", err)
		}
	}
	t.Logf("%d programs, %d pairs, shuffle seed %d (XO_WARMSEQ_SEED reruns it)", len(progs), len(pairs), seed)
	work, err := testtmp.Dir("xo-warmseq-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(work) })

	build := func(dir, out string, env []string) (string, error) {
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "-o", out, "."))
		cmd.Dir = dir
		cmd.Env = env
		log, err := cmd.CombinedOutput()
		if err == nil {
			for _, m := range noReplayRE.FindAllStringSubmatch(string(log), -1) {
				if why, ok := knownNoReplay[m[1]]; ok {
					t.Logf("%s: known: the front end cache entry of %s does not replay (%s)", filepath.Base(dir), m[1], why)
					continue
				}
				err = errDidNotReplay
			}
			if err == nil && strings.Contains(string(log), "did not replay") && !noReplayRE.MatchString(string(log)) {
				err = errDidNotReplay
			}
		}
		return string(log), err
	}

	// the cold builds, four at a time
	coldEnv := append(warmSeqEnv(), "XO_BACKEND_CACHE=off", "XO_FE_CACHE=off", "XO_CHK_CACHE=off", "XO_BIN_CACHE=off",
		"XO_SEPCOMP=0", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cold-cache"))
	cold := map[string][]byte{}
	var mu sync.Mutex
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, dir := range progs {
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out := filepath.Join(work, "cold", filepath.Base(dir), progExe)
			if log, err := build(dir, out, coldEnv); err != nil {
				t.Errorf("%s: cold build: %v\n%s", filepath.Base(dir), err, log)
				return
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			cold[dir] = data
			mu.Unlock()
		}(dir)
	}
	wg.Wait()
	if t.Failed() {
		return
	}

	// check builds dir through env's cache and compares it with its cold
	// build and want.txt; after says what the cache held.
	check := func(t *testing.T, dir, out, after string, env []string) {
		name := filepath.Base(dir)
		if log, err := build(dir, out, env); err != nil {
			t.Errorf("%s: build through the shared cache, %s: %v\n%s", name, after, err, log)
			return
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Error(err)
			return
		}
		if !bytes.Equal(data, cold[dir]) {
			t.Errorf("%s: the build through the shared cache, %s, differs from its cold build (every cache off)", name, after)
			return
		}
		want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
		if err != nil {
			t.Error(err)
			return
		}
		if got := runBinary(t, dir, out); got.String() != string(want) {
			t.Errorf("%s: output differs from want.txt\n--- want\n%s\n--- got\n%s", name, want, got)
		}
	}

	// every pair of programs sharing a std module, each on its own fresh
	// cache: in one long sequence a later build can repair the entry an
	// earlier one broke (the respin's bug hid behind log_handlers that way)
	t.Run("pairs", func(t *testing.T) {
		t.Parallel()
		sem := make(chan struct{}, 4)
		var wg sync.WaitGroup
		for i, pr := range pairs {
			wg.Add(1)
			go func(i int, a, b string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				d := filepath.Join(work, "pair"+strconv.Itoa(i))
				env := append(warmSeqEnv(), "XO_BACKEND_CACHE_DIR="+filepath.Join(d, "cache"), "XO_FE_KEEP_DIR="+filepath.Join(d, "fe-bad"))
				if log, err := build(a, filepath.Join(d, "a", progExe), env); err != nil {
					t.Errorf("%s: build on a fresh cache: %v\n%s", filepath.Base(a), err, log)
					return
				}
				check(t, b, filepath.Join(d, "b", progExe), "after "+filepath.Base(a)+" alone", env)
				os.RemoveAll(d)
			}(i, pr[0], pr[1])
		}
		wg.Wait()
	})

	reversed := make([]string, len(progs))
	for i, p := range progs {
		reversed[len(progs)-1-i] = p
	}
	shuffled := append([]string(nil), progs...)
	rand.New(rand.NewSource(seed)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	for _, order := range []struct {
		name  string
		progs []string
	}{{"fixed", progs}, {"reversed", reversed}, {"shuffled", shuffled}} {
		order := order
		t.Run(order.name, func(t *testing.T) {
			t.Parallel()
			cache := filepath.Join(work, "cache-"+order.name)
			env := append(warmSeqEnv(), "XO_BACKEND_CACHE_DIR="+cache, "XO_FE_KEEP_DIR="+filepath.Join(work, "fe-bad-"+order.name))
			for i, dir := range order.progs {
				name := filepath.Base(dir)
				after := "a fresh cache"
				if i > 0 {
					after = "after " + filepath.Base(order.progs[i-1]) + " (build " + strconv.Itoa(i+1) + " of the sequence)"
				}
				check(t, dir, filepath.Join(work, order.name, name, progExe), after, env)
			}
		})
	}
}

type warmSeqError string

func (e warmSeqError) Error() string { return string(e) }

const errDidNotReplay = warmSeqError("a front end cache entry did not replay (the log says which)")

// noReplayRE finds the function of xo's warning about a front end cache
// entry that does not replay.
var noReplayRE = regexp.MustCompile(`front end cache entry [0-9a-f]+ \(([^,]+), recorded by`)

// knownNoReplay are the functions whose entries are known not to replay,
// with who is fixing it; the build falls back (no wrong code) and the test
// logs them instead of failing. Remove an entry in its fix's commit.
var knownNoReplay = map[string]string{}

// warmSeqEnv is the environment with every cache setting removed, so xo
// uses its defaults, as a user's build does (a gate may export
// XO_BACKEND_CACHE=off and the like).
func warmSeqEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "XO_") && (strings.Contains(k, "CACHE") || k == "XO_SEPCOMP" || k == "XO_FE_KEEP_DIR") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

var useStdRE = regexp.MustCompile(`(?m)^use std/([a-z0-9]+)`)

// warmSeqGate: the modules of the gate subset, besides std/log's goldens.
var warmSeqGate = []string{"json", "http", "time", "csv", "url", "regexp", "template", "crypto", "x509", "sql"}

// warmSeqPrograms returns the golden directories of mode (gate, modules or
// all), sorted, and the ordered pairs to build one after the other on a
// fresh cache: every two programs of a group that share a std module (gate:
// the std/log goldens; modules: the first 3 goldens of each module; all:
// the first 4).
func warmSeqPrograms(t *testing.T, mode string) ([]string, [][2]string) {
	t.Helper()
	if mode == "" {
		mode = "gate"
	}
	users := map[string][]string{} // module -> the goldens (by name) that use it
	var all []string
	for _, dir := range goldenDirs(t) { // sorted by name
		data, err := os.ReadFile(filepath.Join(dir, "main.xo"))
		if err != nil {
			continue
		}
		ms := useStdRE.FindAllStringSubmatch(string(data), -1)
		if len(ms) == 0 {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "want.txt")); err != nil {
			continue
		}
		all = append(all, dir)
		seen := map[string]bool{}
		for _, m := range ms {
			if !seen[m[1]] {
				seen[m[1]] = true
				users[m[1]] = append(users[m[1]], dir)
			}
		}
	}
	pick := map[string]bool{}
	var groups [][]string
	var logs []string
	for _, dir := range all {
		if strings.HasPrefix(filepath.Base(dir), "log_") {
			pick[dir] = true
			logs = append(logs, dir)
		}
	}
	per := 0
	switch mode {
	case "all":
		for _, dir := range all {
			pick[dir] = true
		}
		per = 4
	case "modules":
		for _, us := range users {
			pick[us[0]] = true
		}
		per = 3
	case "gate":
		for _, m := range warmSeqGate {
			if us := users[m]; len(us) > 0 {
				pick[us[0]] = true
			}
		}
		groups = append(groups, logs)
	default:
		t.Fatalf("XO_WARMSEQ=%s: want gate, modules or all", mode)
	}
	if per > 0 {
		mods := make([]string, 0, len(users))
		for m := range users {
			mods = append(mods, m)
		}
		sort.Strings(mods)
		for _, m := range mods {
			g := users[m]
			if len(g) > per {
				g = g[:per]
			}
			groups = append(groups, g)
		}
	}
	var pairs [][2]string
	seenPair := map[[2]string]bool{}
	for _, g := range groups {
		for _, a := range g {
			for _, b := range g {
				if a != b && !seenPair[[2]string{a, b}] {
					seenPair[[2]string{a, b}] = true
					pairs = append(pairs, [2]string{a, b})
					pick[a], pick[b] = true, true
				}
			}
		}
	}
	var out []string
	for dir := range pick {
		out = append(out, dir)
	}
	sort.Strings(out)
	return out, pairs
}
