package xotool

// Golden pass keys (decision 0737 step 2, task #610): a tier golden whose
// binary is identical to the one that passed in an earlier tier, with the
// same want.txt and run inputs, is not run again. The key is the binary's
// SHA-256 with the build-info id masked (decision 0657's rule, as
// scripts/golden-bin-select.py masks it), every file of the golden's
// directory other than its .xo sources (want.txt, args.txt, env.txt,
// stdin.txt, data), the host's OS and architecture, the test's name and
// the caller's run settings. Builds are path independent (2026-11-22: three
// goldens built from two directories gave identical binaries on the native
// and LLVM backends), so a tier's new checkout path does not change a key.
//
// XO_GOLDEN_PASS_DIR=DIR turns it on: each subtest appends its key and
// result to DIR/run-PID.jsonl. XO_GOLDEN_PASS_SKIP=1 also skips a subtest
// whose key is the one DIR/pass.json holds for it. MergeGoldenPass (go run
// ./scripts/goldenpass DIR, after the step) folds the run files into
// pass.json: a passing or skipped subtest keeps its key, a failing one
// loses it, so only a binary that passed last time is ever skipped. The
// 8-hourly full tier records without skipping (XO_GOLDEN_PASS_SKIP unset).

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

var buildInfoID = regexp.MustCompile(`(\{"id":"|, id )[0-9a-f]{24}`)

// GoldenKey is the pass key of binary bin, run for the golden in dir by
// test with the settings in extra.
func GoldenKey(test, bin, dir string, extra ...string) (string, error) {
	b, err := os.ReadFile(bin)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "xo-golden-pass 1\x00%s\x00%s/%s\x00", test, runtime.GOOS, runtime.GOARCH)
	for _, e := range extra {
		fmt.Fprintf(h, "%s\x00", e)
	}
	h.Write(buildInfoID.ReplaceAll(b, []byte("${1}000000000000000000000000")))
	h.Write([]byte{0})
	var files []string
	err = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.Mode().IsRegular() && !strings.HasSuffix(p, ".xo") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	for _, p := range files {
		rel, _ := filepath.Rel(dir, p)
		f, err := os.Open(p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00", filepath.ToSlash(rel))
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return "", err
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// PassEntry is one subtest's last passing key and the run time it took.
type PassEntry struct {
	Key   string `json:"key"`
	RunMS int64  `json:"run_ms"`
}

// PassRecord is one line of a run file.
type PassRecord struct {
	Test   string `json:"test"`
	Key    string `json:"key"`
	Result string `json:"result"` // pass, fail or skip
	RunMS  int64  `json:"run_ms"`
}

// GoldenPass is one test binary's view of XO_GOLDEN_PASS_DIR.
type GoldenPass struct {
	dir  string
	skip bool
	prev map[string]PassEntry
	mu   sync.Mutex
	f    *os.File
	// unlock releases the run file's lock (close; a test process keeps it
	// until it exits)
	unlock func()
}

var (
	passOnce sync.Once
	passOpen *GoldenPass
)

// OpenGoldenPass is the process's GoldenPass, or nil when
// XO_GOLDEN_PASS_DIR is unset or its pass.json cannot be read (then
// every golden runs).
func OpenGoldenPass() *GoldenPass {
	passOnce.Do(func() {
		d := os.Getenv("XO_GOLDEN_PASS_DIR")
		if d == "" {
			return
		}
		p, err := openGoldenPass(d, os.Getenv("XO_GOLDEN_PASS_SKIP") == "1")
		if err != nil {
			fmt.Fprintf(os.Stderr, "golden pass: %v (every golden runs)\n", err)
			return
		}
		passOpen = p
	})
	return passOpen
}

func openGoldenPass(dir string, skip bool) (*GoldenPass, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	prev, err := readPass(dir)
	if err != nil {
		return nil, err
	}
	return &GoldenPass{dir: dir, skip: skip, prev: prev}, nil
}

func readPass(dir string) (map[string]PassEntry, error) {
	prev := map[string]PassEntry{}
	b, err := os.ReadFile(filepath.Join(dir, "pass.json"))
	if os.IsNotExist(err) {
		return prev, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &prev); err != nil {
		return nil, fmt.Errorf("%s: %v", filepath.Join(dir, "pass.json"), err)
	}
	return prev, nil
}

func (p *GoldenPass) record(r PassRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.f == nil {
		run := filepath.Join(p.dir, fmt.Sprintf("run-%d.jsonl", os.Getpid()))
		// held until the process ends: a merge by a step running beside
		// this one (gate.sh's llvmpre) leaves the file alone
		unlock, err := lockFile(run + ".lock")
		if err != nil {
			fmt.Fprintf(os.Stderr, "golden pass: %v\n", err)
			return
		}
		f, err := os.OpenFile(run, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			unlock()
			fmt.Fprintf(os.Stderr, "golden pass: %v\n", err)
			return
		}
		p.f, p.unlock = f, unlock
	}
	b, _ := json.Marshal(r)
	p.f.Write(append(b, '\n'))
}

// close ends p's run file (unit tests; a test binary's run file closes
// when it exits).
func (p *GoldenPass) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.f != nil {
		p.f.Close()
		p.unlock()
		p.f, p.unlock = nil, nil
	}
}

// Check is called by a golden subtest after its build and before its run.
// It reports true when the run is to be skipped (the subtest then calls
// t.Skip); otherwise the subtest's result is recorded with its key when it
// ends. A nil GoldenPass, or a key that cannot be made, runs the golden.
func (p *GoldenPass) Check(t testing.TB, bin, dir string, extra ...string) bool {
	if p == nil {
		return false
	}
	key, err := GoldenKey(t.Name(), bin, dir, extra...)
	if err != nil {
		t.Logf("golden pass: no key (%v): it runs", err)
		return false
	}
	if e, ok := p.prev[t.Name()]; ok && p.skip && e.Key == key {
		p.record(PassRecord{Test: t.Name(), Key: key, Result: "skip", RunMS: e.RunMS})
		return true
	}
	start := time.Now()
	t.Cleanup(func() {
		res := "pass"
		if t.Failed() {
			res = "fail"
		} else if t.Skipped() {
			return
		}
		p.record(PassRecord{Test: t.Name(), Key: key, Result: res, RunMS: time.Since(start).Milliseconds()})
	})
	return false
}

// PassSummary counts one merge.
type PassSummary struct {
	Passed, Skipped, Failed int
	SavedMS                 int64
}

// MergeGoldenPass folds dir's run files into its pass.json and removes
// them: a pass sets the subtest's key, a skip keeps it, a fail removes it.
// A subtest with two records in one merge (a rerun) keeps the key only
// when none of them failed.
func MergeGoldenPass(dir string) (PassSummary, error) {
	var s PassSummary
	// a step whose goldens all failed to build wrote no run file
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return s, err
	}
	// two shards of one step may merge into one directory at once
	unlock, err := lockFile(filepath.Join(dir, ".merge.lock"))
	if err != nil {
		return s, err
	}
	defer unlock()
	prev, err := readPass(dir)
	if err != nil {
		return s, err
	}
	runs, err := filepath.Glob(filepath.Join(dir, "run-*.jsonl"))
	if err != nil {
		return s, err
	}
	sort.Strings(runs)
	// a run file whose process still runs is merged by its own step
	done := runs[:0]
	for _, r := range runs {
		if !lockHeld(r + ".lock") {
			done = append(done, r)
		}
	}
	runs = done
	failed := map[string]bool{}
	for _, r := range runs {
		b, err := os.ReadFile(r)
		if err != nil {
			return s, err
		}
		for _, l := range bytes.Split(b, []byte("\n")) {
			if len(bytes.TrimSpace(l)) == 0 {
				continue
			}
			var rec PassRecord
			if err := json.Unmarshal(l, &rec); err != nil {
				// a line cut by a killed run: that run's other lines stand
				continue
			}
			switch rec.Result {
			case "pass":
				s.Passed++
				if !failed[rec.Test] {
					prev[rec.Test] = PassEntry{Key: rec.Key, RunMS: rec.RunMS}
				}
			case "skip":
				s.Skipped++
				s.SavedMS += rec.RunMS
			default:
				s.Failed++
				failed[rec.Test] = true
				delete(prev, rec.Test)
			}
		}
	}
	b, err := json.MarshalIndent(prev, "", " ")
	if err != nil {
		return s, err
	}
	tmp := filepath.Join(dir, fmt.Sprintf(".pass.json.%d", os.Getpid()))
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return s, err
	}
	if err := os.Rename(tmp, filepath.Join(dir, "pass.json")); err != nil {
		return s, err
	}
	for _, r := range runs {
		os.Remove(r)
		os.Remove(r + ".lock")
	}
	return s, nil
}
