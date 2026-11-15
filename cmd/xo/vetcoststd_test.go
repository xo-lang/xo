package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

// vetCostStdCodes are the xo vet --cost notes std is kept free of (the
// std timing audit's lessons, knowledgebase/std-performance-guide.md).
var vetCostStdCodes = map[string]bool{"XO0913": true, "XO0914": true, "XO0915": true}

// TestVetCostStd runs xo vet --cost --cost-std over every program under
// bench/perf (the std benches and their Go twins, which reach the std code
// the timing audit measures; cost notes need a lowered program, so std is
// vetted through programs that use it) and fails on a note of
// vetCostStdCodes in a std file that testdata/vetcost-std-allow.txt does
// not list. An allowed note is keyed by its file, its code and its
// source line's text (not its line number, so edits above it do not move
// it), each with the reason it stays: "FILE<TAB>CODE<TAB>TEXT<TAB>REASON".
// An allow line that no note matches fails too, so the list keeps no
// stale entries. It lives here, not in internal/repotest beside
// TestRawAddrKeepsBuffer, because it needs the xo binary.
func TestVetCostStd(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds every bench/perf program")
	}
	allow := readVetCostAllow(t)
	used := map[string]bool{}
	std := xoStd()
	mains, _ := filepath.Glob(filepath.Join("..", "..", "bench", "perf", "*", "main.xo"))
	var dirs []string
	for _, m := range mains {
		dirs = append(dirs, filepath.Dir(m))
	}
	if len(dirs) == 0 {
		t.Fatal("no bench/perf programs")
	}
	// the vets on a pool (each lowers its program whole: one after another
	// the 176 programs took 936 s on host 2, 2026-11-15), read in order
	type vetOut struct {
		out, errOut bytes.Buffer
		code        int
	}
	outs := make([]vetOut, len(dirs))
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < vetCostWorkers(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				outs[i].code = run(t, []string{"vet", "--json", "--cost", "--cost-std", dirs[i]}, &outs[i].out, &outs[i].errOut)
			}
		}()
	}
	for i := range dirs {
		next <- i
	}
	close(next)
	wg.Wait()
	seen := map[string]bool{}
	var bad []string
	for i, dir := range dirs {
		out, errOut := &outs[i].out, &outs[i].errOut
		if code := outs[i].code; code != 0 {
			t.Errorf("vet --cost %s: exit %d: %s", dir, code, errOut.String())
			continue
		}
		var res struct {
			Diagnostics []struct {
				Code string `json:"code"`
				Span struct {
					File string `json:"file"`
					Line int    `json:"line"`
				} `json:"span"`
			} `json:"diagnostics"`
		}
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			t.Fatalf("vet --cost %s: bad JSON: %v\n%s", dir, err, out.String())
		}
		for _, d := range res.Diagnostics {
			if !vetCostStdCodes[d.Code] {
				continue
			}
			// a std file is named relative to the std root or, as
			// "std/...", to the repository
			file := d.Span.File
			if !filepath.IsAbs(file) {
				file = filepath.Join(std, strings.TrimPrefix(filepath.ToSlash(file), "std/"))
			}
			rel, err := filepath.Rel(std, file)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			rel = "std/" + filepath.ToSlash(rel)
			key := rel + "\t" + d.Code + "\t" + sourceLine(file, d.Span.Line)
			if seen[key] {
				continue
			}
			seen[key] = true
			if _, ok := allow[key]; ok {
				used[key] = true
				continue
			}
			bad = append(bad, rel+":"+itoa(d.Span.Line)+": "+d.Code+" (to allow it: "+key+"\t<reason>)")
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Errorf("%s", b)
	}
	if t.Failed() {
		t.Log("fix the code (knowledgebase/std-performance-guide.md), or add the line with a measured reason to cmd/xo/testdata/vetcost-std-allow.txt")
	}
	for k := range allow {
		if !used[k] {
			t.Errorf("vetcost-std-allow.txt: no note matches %q any more; remove it", k)
		}
	}
}

// vetCostWorkers is how many vets TestVetCostStd runs at once: a quarter
// of the CPUs, 2 to 8 (a vet of a large bench peaks near 1 GB).
func vetCostWorkers() int {
	return min(max(runtime.NumCPU()/4, 2), 8)
}

func readVetCostAllow(t *testing.T) map[string]string {
	f, err := os.Open(filepath.Join("testdata", "vetcost-std-allow.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	allow := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := sc.Text()
		if strings.TrimSpace(l) == "" || strings.HasPrefix(l, "#") {
			continue
		}
		parts := strings.Split(l, "\t")
		if len(parts) != 4 || strings.TrimSpace(parts[3]) == "" {
			t.Fatalf("vetcost-std-allow.txt: %q: want FILE<TAB>CODE<TAB>TEXT<TAB>REASON", l)
		}
		allow[strings.Join(parts[:3], "\t")] = parts[3]
	}
	return allow
}

// sourceLine is line n of file, trimmed.
func sourceLine(file string, n int) string {
	b, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	ls := strings.Split(string(b), "\n")
	if n < 1 || n > len(ls) {
		return ""
	}
	return strings.TrimSpace(ls[n-1])
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
