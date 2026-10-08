package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const waitLimit = 10 * time.Second

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(waitLimit):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func readFlush(t *testing.T, path string) map[string]ServiceSummary {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]ServiceSummary{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var s ServiceSummary
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			t.Fatalf("bad json in %s: %v: %q", path, err, sc.Text())
		}
		out[s.Service] = s
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func totalOf(m map[string]ServiceSummary) int64 {
	var n int64
	for _, s := range m {
		n += s.Total
	}
	return n
}

func flushPath(dir string, seq int) string {
	return filepath.Join(dir, fmt.Sprintf("flush-%06d.jsonl", seq))
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("leftover temp file %s", e.Name())
		}
	}
}

// runAsync starts Run in a goroutine and returns a channel with its result.
type runResult struct {
	st  Stats
	err error
}

func runAsync(ctx context.Context, in io.Reader, cfg Config) <-chan runResult {
	ch := make(chan runResult, 1)
	go func() {
		st, err := Run(ctx, in, cfg)
		ch <- runResult{st, err}
	}()
	return ch
}

func waitResult(t *testing.T, ch <-chan runResult) runResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(waitLimit):
		t.Fatal("Run did not return")
	}
	return runResult{}
}

func TestParseLine(t *testing.T) {
	good := []struct {
		in   string
		want Entry
	}{
		{"1700000000000 INFO api hello world\n", Entry{1700000000000, "INFO", "api", "hello world"}},
		{"0 warn db slow  query\r\n", Entry{0, "WARN", "db", "slow  query"}},
		{"5 ERROR web x", Entry{5, "ERROR", "web", "x"}},
	}
	for _, g := range good {
		e, err := ParseLine(g.in)
		if err != nil || e != g.want {
			t.Errorf("ParseLine(%q) = %+v, %v; want %+v", g.in, e, err, g.want)
		}
	}
	bad := []string{"", "\n", "abc", "123 INFO", "123 INFO svc", "-5 INFO svc msg",
		"12x INFO svc msg", "123  svc msg", "123 INFO  msg", "123 INFO svc   ", " 123 INFO svc msg"}
	for _, b := range bad {
		if _, err := ParseLine(b); err == nil {
			t.Errorf("ParseLine(%q) should fail", b)
		}
	}
}

func TestEveryValidLineCountedOnce(t *testing.T) {
	dir := t.TempDir()
	services := []string{"api", "db", "web"}
	levels := []string{"INFO", "WARN", "ERROR", "DEBUG"}
	const n = 20000
	var buf bytes.Buffer
	wantLevels := map[string]map[string]int64{}
	wantTotal := map[string]int64{}
	for i := 0; i < n; i++ {
		svc := services[i%len(services)]
		lvl := levels[i%len(levels)]
		msg := fmt.Sprintf("msg-%d", i%7)
		fmt.Fprintf(&buf, "%d %s %s %s\n", 1000+i, lvl, svc, msg)
		if wantLevels[svc] == nil {
			wantLevels[svc] = map[string]int64{}
		}
		wantLevels[svc][lvl]++
		wantTotal[svc]++
	}
	st, err := Run(context.Background(), &buf, Config{Workers: 8, OutDir: dir, Ticks: make(chan time.Time)})
	if err != nil {
		t.Fatal(err)
	}
	if st.Read != n || st.Parsed != n || st.Malformed != 0 || st.Flushes != 1 {
		t.Fatalf("stats %+v", st)
	}
	got := readFlush(t, flushPath(dir, 1))
	if len(got) != 3 || totalOf(got) != n {
		t.Fatalf("got %d services total %d", len(got), totalOf(got))
	}
	for svc, s := range got {
		if s.Total != wantTotal[svc] {
			t.Errorf("%s total %d want %d", svc, s.Total, wantTotal[svc])
		}
		for lvl, c := range wantLevels[svc] {
			if s.Levels[lvl] != c {
				t.Errorf("%s %s = %d want %d", svc, lvl, s.Levels[lvl], c)
			}
		}
		if len(s.TopMessages) != TopN {
			t.Errorf("%s top messages len %d", svc, len(s.TopMessages))
		}
	}
	if got["api"].FirstTS != 1000 || got["db"].LastTS != 1000+n-1 {
		t.Errorf("timestamps api.first=%d db.last=%d", got["api"].FirstTS, got["db"].LastTS)
	}
}

func TestTopMessages(t *testing.T) {
	a := NewAggregator()
	counts := map[string]int{"a": 1, "b": 6, "c": 3, "d": 3, "e": 5, "f": 2, "g": 4}
	ts := int64(100)
	for msg, c := range counts {
		for i := 0; i < c; i++ {
			a.Add(Entry{TS: ts, Level: "INFO", Service: "s", Message: msg})
			ts--
		}
	}
	snap := a.Snapshot()
	if len(snap) != 1 {
		t.Fatal(snap)
	}
	want := []MessageCount{{"b", 6}, {"e", 5}, {"g", 4}, {"c", 3}, {"d", 3}}
	got := snap[0].TopMessages
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("top[%d] = %v want %v (all %v)", i, got[i], want[i], got)
		}
	}
	if snap[0].FirstTS != ts+1 || snap[0].LastTS != 100 {
		t.Fatalf("first %d last %d", snap[0].FirstTS, snap[0].LastTS)
	}
}

func TestMalformedCountedAndSkipped(t *testing.T) {
	dir := t.TempDir()
	input := strings.Join([]string{
		"100 INFO api ok",
		"garbage",
		"",
		"101 INFO",
		"notanumber INFO api x",
		"102 ERROR api failed hard",
		"-3 INFO api negative",
		"103 INFO api   ",
		"\x00\x01\x02",
		"104 WARN db last line without newline",
	}, "\n")
	st, err := Run(context.Background(), strings.NewReader(input), Config{Workers: 3, OutDir: dir, Ticks: make(chan time.Time)})
	if err != nil {
		t.Fatal(err)
	}
	if st.Read != 10 || st.Parsed != 3 || st.Malformed != 7 {
		t.Fatalf("stats %+v", st)
	}
	got := readFlush(t, flushPath(dir, 1))
	if got["api"].Total != 2 || got["api"].Levels["ERROR"] != 1 || got["db"].Total != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestReaderBlocksInsteadOfDropping(t *testing.T) {
	dir := t.TempDir()
	const queueSize, workers, total = 10, 1, 200
	gate := make(chan struct{})
	full := make(chan struct{}, 1)
	var lastRead, parsed atomic.Int64
	cfg := Config{
		Workers: workers, QueueSize: queueSize, OutDir: dir, Ticks: make(chan time.Time),
		hooks: hooks{
			onRead:      func(n int64) { lastRead.Store(n) },
			beforeParse: func() { <-gate },
			afterParse:  func() { parsed.Add(1) },
			onQueueFull: func() {
				select {
				case full <- struct{}{}:
				default:
				}
			},
		},
	}
	pr, pw := io.Pipe()
	defer pw.Close()
	go func() {
		for i := 0; i < total; i++ {
			if _, err := fmt.Fprintf(pw, "%d INFO svc m%d\n", i, i%3); err != nil {
				return
			}
		}
		pw.Close()
	}()
	res := runAsync(context.Background(), pr, cfg)

	waitFor(t, full, "queue full")
	// Reader is blocked: it has accepted at most queue + in-worker + the one it holds.
	if n := lastRead.Load(); n > queueSize+workers+1 || n >= total {
		t.Fatalf("reader accepted %d lines while workers were stuck", n)
	}
	if parsed.Load() != 0 {
		t.Fatalf("parsed %d while gated", parsed.Load())
	}
	close(gate)
	r := waitResult(t, res)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.st.Read != total || r.st.Parsed != total || r.st.Malformed != 0 {
		t.Fatalf("stats %+v", r.st)
	}
	if got := totalOf(readFlush(t, flushPath(dir, 1))); got != total {
		t.Fatalf("flushed total %d", got)
	}
}

func TestDefaultQueueSize(t *testing.T) {
	dir := t.TempDir()
	gate := make(chan struct{})
	full := make(chan struct{}, 1)
	var lastRead atomic.Int64
	cfg := Config{
		Workers: 1, OutDir: dir, Ticks: make(chan time.Time),
		hooks: hooks{
			onRead:      func(n int64) { lastRead.Store(n) },
			beforeParse: func() { <-gate },
			onQueueFull: func() {
				select {
				case full <- struct{}{}:
				default:
				}
			},
		},
	}
	var buf bytes.Buffer
	for i := 0; i < 1500; i++ {
		fmt.Fprintf(&buf, "%d INFO svc m\n", i)
	}
	res := runAsync(context.Background(), &buf, cfg)
	waitFor(t, full, "queue full")
	if n := lastRead.Load(); n < DefaultQueueSize || n > DefaultQueueSize+2 {
		t.Fatalf("queue filled at %d reads, want about %d", n, DefaultQueueSize)
	}
	close(gate)
	r := waitResult(t, res)
	if r.err != nil || r.st.Parsed != 1500 {
		t.Fatalf("%+v %v", r.st, r.err)
	}
}

func TestFlushOnIntervalAtomic(t *testing.T) {
	dir := t.TempDir()
	ticks := make(chan time.Time)
	processed := make(chan struct{}, 1000)
	flushed := make(chan string, 10)
	cfg := Config{
		Workers: 3, OutDir: dir, Ticks: ticks,
		hooks: hooks{
			afterParse: func() { processed <- struct{}{} },
			onFlush:    func(p string) { flushed <- p },
		},
	}
	pr, pw := io.Pipe()
	defer pw.Close()
	res := runAsync(context.Background(), pr, cfg)

	write := func(from, to int) {
		go func() {
			for i := from; i < to; i++ {
				fmt.Fprintf(pw, "%d INFO api m\n", i)
			}
		}()
		for i := from; i < to; i++ {
			waitFor(t, processed, "line processed")
		}
	}
	tick := func(want string) string {
		t.Helper()
		ticks <- time.Unix(0, 0)
		select {
		case p := <-flushed:
			if p != want {
				t.Fatalf("flush path %s want %s", p, want)
			}
			return p
		case <-time.After(waitLimit):
			t.Fatal("no flush after tick")
		}
		return ""
	}

	// No flush happens without a tick.
	write(0, 10)
	select {
	case p := <-flushed:
		t.Fatalf("unexpected flush %s", p)
	default:
	}
	p1 := tick(flushPath(dir, 1))
	if got := totalOf(readFlush(t, p1)); got != 10 {
		t.Fatalf("flush 1 total %d", got)
	}
	assertNoTempFiles(t, dir)

	write(10, 25)
	p2 := tick(flushPath(dir, 2))
	if got := totalOf(readFlush(t, p2)); got != 25 {
		t.Fatalf("flush 2 total %d", got)
	}
	// Earlier flush file is untouched.
	if got := totalOf(readFlush(t, p1)); got != 10 {
		t.Fatalf("flush 1 changed: %d", got)
	}
	// An empty interval still produces a file.
	tick(flushPath(dir, 3))

	pw.Close()
	r := waitResult(t, res)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.st.Flushes != 4 || r.st.Parsed != 25 {
		t.Fatalf("stats %+v", r.st)
	}
	if got := totalOf(readFlush(t, flushPath(dir, 4))); got != 25 {
		t.Fatalf("final flush total %d", got)
	}
	assertNoTempFiles(t, dir)
	ents, _ := os.ReadDir(dir)
	if len(ents) != 4 {
		t.Fatalf("want 4 files, got %d", len(ents))
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := WriteFileAtomic(path, []byte("one\n")); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("two\n")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "two\n" {
		t.Fatalf("content %q err %v", b, err)
	}
	assertNoTempFiles(t, dir)

	// Rename onto a directory fails; the temp file must be cleaned up and
	// the target left as it was.
	target := filepath.Join(dir, "adir")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(target, []byte("x")); err == nil {
		t.Fatal("expected error renaming onto non empty dir")
	}
	assertNoTempFiles(t, dir)
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		t.Fatal("target dir damaged")
	}
}

func TestShutdownMidInputFlushesEverythingRead(t *testing.T) {
	dir := t.TempDir()
	gate := make(chan struct{})
	readAll := make(chan struct{})
	const n = 50
	var parsed atomic.Int64
	cfg := Config{
		Workers: 4, OutDir: dir, Ticks: make(chan time.Time),
		hooks: hooks{
			onRead: func(c int64) {
				if c == n {
					close(readAll)
				}
			},
			beforeParse: func() { <-gate },
			afterParse:  func() { parsed.Add(1) },
		},
	}
	pr, pw := io.Pipe()
	defer pw.Close() // input never ends on its own: shutdown is mid input
	go func() {
		for i := 0; i < n; i++ {
			fmt.Fprintf(pw, "%d INFO svc%d m\n", i, i%2)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res := runAsync(ctx, pr, cfg)

	waitFor(t, readAll, "all lines read")
	if parsed.Load() != 0 {
		t.Fatal("workers should be gated")
	}
	cancel() // the signal
	close(gate)
	r := waitResult(t, res)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.st.Read != n || r.st.Parsed != n || r.st.Flushes != 1 {
		t.Fatalf("stats %+v", r.st)
	}
	got := readFlush(t, flushPath(dir, 1))
	if totalOf(got) != n || got["svc0"].Total != n/2 {
		t.Fatalf("final flush %+v", got)
	}
}

func TestShutdownWhileReaderBlockedKeepsHeldLine(t *testing.T) {
	dir := t.TempDir()
	gate := make(chan struct{})
	full := make(chan struct{}, 1)
	var lastRead atomic.Int64
	cfg := Config{
		Workers: 1, QueueSize: 5, OutDir: dir, Ticks: make(chan time.Time),
		hooks: hooks{
			onRead:      func(c int64) { lastRead.Store(c) },
			beforeParse: func() { <-gate },
			onQueueFull: func() {
				select {
				case full <- struct{}{}:
				default:
				}
			},
		},
	}
	pr, pw := io.Pipe()
	defer pw.Close()
	go func() {
		for i := 0; i < 100; i++ {
			if _, err := fmt.Fprintf(pw, "%d INFO svc m\n", i); err != nil {
				return
			}
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res := runAsync(ctx, pr, cfg)
	waitFor(t, full, "queue full")
	cancel()
	close(gate)
	r := waitResult(t, res)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.st.Read < lastRead.Load() || r.st.Read >= 100 {
		t.Fatalf("read %d (last seen %d)", r.st.Read, lastRead.Load())
	}
	if r.st.Parsed != r.st.Read {
		t.Fatalf("lost lines: read %d parsed %d", r.st.Read, r.st.Parsed)
	}
	if got := totalOf(readFlush(t, flushPath(dir, 1))); got != r.st.Read {
		t.Fatalf("final flush total %d read %d", got, r.st.Read)
	}
}

func TestMainRun(t *testing.T) {
	dir := t.TempDir()
	in := strings.NewReader("1 INFO a x\nbad\n2 ERROR b y\n")
	var errOut bytes.Buffer
	code := run([]string{"--workers", "2", "--flush", "1h", "--out", dir}, in, &errOut)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	want := "lines read: 3, parsed: 2, malformed: 1, flushes written: 1"
	if !strings.Contains(errOut.String(), want) {
		t.Fatalf("summary %q", errOut.String())
	}
	if code := run([]string{"--workers", "0", "--out", dir}, strings.NewReader(""), &errOut); code != 2 {
		t.Fatalf("bad flags exit %d", code)
	}
}

