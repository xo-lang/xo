package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testTimeout = 20 * time.Second

func readFlush(t *testing.T, path string) map[string]ServiceSnapshot {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
	out := map[string]ServiceSnapshot{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var s ServiceSnapshot
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		out[s.Service] = s
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func flushFiles(t *testing.T, dir string) (final []string, temps []string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		switch {
		case strings.HasSuffix(e.Name(), ".tmp"):
			temps = append(temps, e.Name())
		case strings.HasPrefix(e.Name(), "flush-") && strings.HasSuffix(e.Name(), ".json"):
			final = append(final, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(final)
	return final, temps
}

func totalOf(m map[string]ServiceSnapshot) int64 {
	var n int64
	for _, s := range m {
		n += s.Total
	}
	return n
}

func wait(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(testTimeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestParseLine(t *testing.T) {
	good := []string{
		"1700000000000 info api hello world",
		"0 ERROR db x\r\n",
		"5 warn svc  leading space message",
	}
	for _, l := range good {
		if _, ok := ParseLine(l); !ok {
			t.Errorf("expected valid: %q", l)
		}
	}
	e, _ := ParseLine("12 info api hello  world")
	if e.TS != 12 || e.Level != "INFO" || e.Service != "api" || e.Message != "hello  world" {
		t.Errorf("parsed %+v", e)
	}
	bad := []string{
		"", "\n", "abc info api msg", "-5 info api msg", "12 info api", "12 info api ",
		"12 info", "12", " 12 info api msg", "12  info api msg", "99999999999999999999 info api m",
	}
	for _, l := range bad {
		if _, ok := ParseLine(l); ok {
			t.Errorf("expected malformed: %q", l)
		}
	}
}

func TestEveryValidLineCountedOnce(t *testing.T) {
	dir := t.TempDir()
	services := []string{"api", "db", "web"}
	levels := []string{"INFO", "WARN", "ERROR"}
	const n = 20000
	want := map[string]map[string]int64{}
	var b strings.Builder
	for i := range n {
		svc := services[i%len(services)]
		lvl := levels[(i/3)%len(levels)]
		if want[svc] == nil {
			want[svc] = map[string]int64{}
		}
		want[svc][lvl]++
		fmt.Fprintf(&b, "%d %s %s msg-%d\n", 1000+i, strings.ToLower(lvl), svc, i%7)
	}
	sum, err := Run(context.Background(), strings.NewReader(b.String()), Config{Workers: 8, QueueSize: 16, OutDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Read != n || sum.Parsed != n || sum.Malformed != 0 || sum.Flushes != 1 {
		t.Fatalf("summary %+v", sum)
	}
	files, temps := flushFiles(t, dir)
	if len(files) != 1 || len(temps) != 0 {
		t.Fatalf("files %v temps %v", files, temps)
	}
	got := readFlush(t, files[0])
	if totalOf(got) != n {
		t.Fatalf("total %d want %d", totalOf(got), n)
	}
	for svc, lv := range want {
		s, ok := got[svc]
		if !ok {
			t.Fatalf("missing service %s", svc)
		}
		for l, c := range lv {
			if s.CountByLevel[l] != c {
				t.Errorf("%s %s = %d want %d", svc, l, s.CountByLevel[l], c)
			}
		}
		if len(s.TopMessages) != TopN {
			t.Errorf("%s top messages %v", svc, s.TopMessages)
		}
		for i := 1; i < len(s.TopMessages); i++ {
			if s.TopMessages[i].Count > s.TopMessages[i-1].Count {
				t.Errorf("%s top messages not sorted: %v", svc, s.TopMessages)
			}
		}
	}
	// i%3 == 0 is api (first line i=0); the last line i=n-1 has n-1 %3 == 1, which is db.
	if got["api"].FirstTS != 1000 || got["db"].LastTS != 1000+n-1 || got["web"].FirstTS != 1002 {
		t.Errorf("timestamps api first %d db last %d web first %d", got["api"].FirstTS, got["db"].LastTS, got["web"].FirstTS)
	}
}

func TestTopMessages(t *testing.T) {
	a := NewAggregator()
	counts := map[string]int{"a": 1, "b": 7, "c": 3, "d": 7, "e": 2, "f": 5, "g": 4}
	for msg, c := range counts {
		for range c {
			a.Add(Entry{TS: 1, Level: "INFO", Service: "s", Message: msg})
		}
	}
	snap := a.Snapshot(1, time.Time{})
	if len(snap) != 1 {
		t.Fatalf("snap %v", snap)
	}
	var got []string
	for _, m := range snap[0].TopMessages {
		got = append(got, fmt.Sprintf("%s=%d", m.Message, m.Count))
	}
	if strings.Join(got, ",") != "b=7,d=7,f=5,g=4,c=3" {
		t.Fatalf("top = %v", got)
	}
}

func TestMalformedLinesCountedAndSkipped(t *testing.T) {
	dir := t.TempDir()
	input := strings.Join([]string{
		"1 info api ok one",
		"garbage",
		"",
		"x info api bad ts",
		"2 info api",
		"-3 info api negative",
		strings.Repeat("z", 200),
		"3 warn api ok two",
		"4 error db ok three",
	}, "\n") // no trailing newline on the last line
	sum, err := Run(context.Background(), strings.NewReader(input), Config{Workers: 3, MaxLine: 64, OutDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Read != 9 || sum.Parsed != 3 || sum.Malformed != 6 {
		t.Fatalf("summary %+v", sum)
	}
	files, _ := flushFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	got := readFlush(t, files[0])
	if got["api"].Total != 2 || got["db"].Total != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestReaderBlocksInsteadOfDropping(t *testing.T) {
	dir := t.TempDir()
	const total = 10
	const queue = 2
	gate := make(chan struct{})
	inWorker := make(chan struct{}, total)
	blocked := make(chan struct{}, 100)
	var accepted atomic.Int64

	var b strings.Builder
	for i := range total {
		fmt.Fprintf(&b, "%d info api m%d\n", i, i)
	}
	cfg := Config{
		Workers:   1,
		QueueSize: queue,
		OutDir:    dir,
		Hooks: Hooks{
			BeforeProcess: func(string) {
				inWorker <- struct{}{}
				<-gate
			},
			ReaderBlocked: func() {
				select {
				case blocked <- struct{}{}:
				default:
				}
			},
			Accepted: func(n int64) { accepted.Store(n) },
		},
	}
	type result struct {
		sum Summary
		err error
	}
	done := make(chan result, 1)
	go func() {
		s, err := Run(context.Background(), strings.NewReader(b.String()), cfg)
		done <- result{s, err}
	}()

	wait(t, inWorker, "worker to take a line")
	// Wait until the reader reports blocking with the queue full and one line
	// held by the worker: exactly queue+1 lines accepted, the rest waiting.
	deadline := time.After(testTimeout)
waitBlocked:
	for {
		select {
		case <-blocked:
			if accepted.Load() == queue+1 {
				break waitBlocked
			}
		case <-deadline:
			t.Fatalf("reader never blocked at capacity; accepted=%d", accepted.Load())
		}
	}
	if got := accepted.Load(); got != queue+1 {
		t.Fatalf("accepted %d while blocked, want %d", got, queue+1)
	}
	select {
	case r := <-done:
		t.Fatalf("Run finished while workers were blocked: %+v", r)
	default:
	}

	close(gate)
	var r result
	select {
	case r = <-done:
	case <-time.After(testTimeout):
		t.Fatal("Run did not finish")
	}
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.sum.Read != total || r.sum.Parsed != total || r.sum.ReaderBlocked == 0 {
		t.Fatalf("summary %+v", r.sum)
	}
}

func TestFlushOnIntervalAtomic(t *testing.T) {
	dir := t.TempDir()
	ticks := make(chan time.Time)
	flushed := make(chan struct{}, 10)
	processed := make(chan struct{}, 100)
	fixed := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	pr, pw := io.Pipe()
	cfg := Config{
		Workers: 2,
		OutDir:  dir,
		Ticks:   ticks,
		Now:     func() time.Time { return fixed },
		Hooks: Hooks{
			AfterProcess: func() { processed <- struct{}{} },
			Flushed:      func(string, int) { flushed <- struct{}{} },
		},
	}
	type result struct {
		sum Summary
		err error
	}
	done := make(chan result, 1)
	go func() {
		s, err := Run(context.Background(), pr, cfg)
		done <- result{s, err}
	}()

	writeLines := func(from, n int) {
		for i := range n {
			if _, err := fmt.Fprintf(pw, "%d info api m\n", from+i); err != nil {
				t.Fatal(err)
			}
		}
		for range n {
			wait(t, processed, "line processed")
		}
	}

	writeLines(0, 5)
	ticks <- fixed
	wait(t, flushed, "first flush")
	files, temps := flushFiles(t, dir)
	if len(files) != 1 || len(temps) != 0 {
		t.Fatalf("after tick 1: files %v temps %v", files, temps)
	}
	f1 := readFlush(t, files[0])
	if f1["api"].Total != 5 || f1["api"].FlushSeq != 1 || !f1["api"].FlushedAt.Equal(fixed) {
		t.Fatalf("flush 1 = %+v", f1)
	}

	writeLines(5, 3)
	ticks <- fixed
	wait(t, flushed, "second flush")

	// A tick with no new data still writes a file.
	ticks <- fixed
	wait(t, flushed, "third flush")

	if err := pw.Close(); err != nil {
		t.Fatal(err)
	}
	var r result
	select {
	case r = <-done:
	case <-time.After(testTimeout):
		t.Fatal("Run did not finish")
	}
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.sum.Flushes != 4 || r.sum.Parsed != 8 {
		t.Fatalf("summary %+v", r.sum)
	}
	files, temps = flushFiles(t, dir)
	if len(files) != 4 || len(temps) != 0 {
		t.Fatalf("files %v temps %v", files, temps)
	}
	wantTotals := []int64{5, 8, 8, 8}
	for i, path := range files {
		if filepath.Base(path) != fmt.Sprintf("flush-%06d.json", i+1) {
			t.Errorf("file %d named %s", i, path)
		}
		if got := readFlush(t, path)["api"].Total; got != wantTotals[i] {
			t.Errorf("file %s total %d want %d", path, got, wantTotals[i])
		}
	}
}

func TestWriteAtomicLeavesNothingOnFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.json")
	boom := errors.New("boom")
	err := writeAtomic(dir, target, func(w io.Writer) error {
		if _, err := io.WriteString(w, "partial"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("leftover files: %v", entries)
	}

	if err := writeAtomic(dir, target, func(w io.Writer) error {
		_, err := io.WriteString(w, "complete")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "complete" {
		t.Fatalf("data %q err %v", data, err)
	}
	entries, err = os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries %v", entries)
	}
}

func TestShutdownMidInputFlushesEverythingRead(t *testing.T) {
	dir := t.TempDir()
	const before = 50
	pr, pw := io.Pipe()
	reached := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := Config{
		Workers:   4,
		QueueSize: 64,
		OutDir:    dir,
		Hooks: Hooks{
			Accepted: func(n int64) {
				if n == before {
					close(reached)
				}
			},
			// Slow workers so lines are still queued when the signal arrives.
			BeforeProcess: func(string) {
				<-reached
			},
		},
	}
	type result struct {
		sum Summary
		err error
	}
	done := make(chan result, 1)
	go func() {
		s, err := Run(ctx, pr, cfg)
		done <- result{s, err}
	}()

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for i := range before {
			if _, err := fmt.Fprintf(pw, "%d info svc%d line %d\n", i, i%2, i); err != nil {
				return
			}
		}
	}()
	wait(t, reached, "50 lines accepted")
	cancel() // the "signal"

	var r result
	select {
	case r = <-done:
	case <-time.After(testTimeout):
		t.Fatal("Run did not stop after cancel")
	}
	// Release the input goroutine still waiting on the pipe.
	if err := pw.Close(); err != nil {
		t.Fatal(err)
	}
	wait(t, writerDone, "writer")
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.sum.Read != before || r.sum.Parsed != before || r.sum.Flushes != 1 {
		t.Fatalf("summary %+v", r.sum)
	}
	files, _ := flushFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	got := readFlush(t, files[0])
	if totalOf(got) != before || got["svc0"].Total != 25 || got["svc1"].Total != 25 {
		t.Fatalf("final flush %+v", got)
	}
}

func TestCLI(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	in := "1 info api a\n2 error api b\nbad\n3 info db c\n"
	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{"--workers", "3", "--flush", "1h", "--out", dir}, strings.NewReader(in), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d stderr %s", code, stderr.String())
	}
	want := "lines read: 4, parsed: 3, malformed: 1, flushes written: 1"
	if strings.TrimSpace(stdout.String()) != want {
		t.Fatalf("stdout %q", stdout.String())
	}
	files, _ := flushFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("files %v", files)
	}

	if code := runCLI(context.Background(), []string{"--workers", "0", "--out", dir}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("bad workers exit %d", code)
	}
	if code := runCLI(context.Background(), nil, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("missing out exit %d", code)
	}
}
