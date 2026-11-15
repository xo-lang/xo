// Command stdcov writes the website's standard library coverage page
// (content/docs/std-coverage.md of the site) from this repository's committed
// files (decision 0616 section 5; it replaces the site's gen-stdcov.py, which
// now only calls it):
//
//	go run ./scripts/stdcov [-o FILE] [-today YYYY-MM-DD]
//
// Run it in a checkout of the commit to describe (it needs go, git and the
// checkout's tags). The page goes to stdout, or to FILE with -o; a summary of
// the counts goes to stderr. -today fixes the date the evidence staleness rule
// (decision 0616 section 3) reads, for reproducible output.
//
// The marks: features from knowledgebase/infrastructure/std-coverage.md and
// the open gaps of scripts/stdaudit/map.txt;
// testing from the corpus of Go's tests, Go differentials, fuzz harnesses, and
// Proven from std/evidence/MODULE/*.md; performance from bench/results files
// with a Go twin, and Proven also from std/evidence/MODULE/perf.md.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const gh = "https://github.com/xo-lang/xo/blob/main/"

var (
	repo  string
	today time.Time
)

func die(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "stdcov: "+f+"\n", a...)
	os.Exit(1)
}

// sh runs a command in the repository and returns its trimmed output.
func sh(name string, args ...string) string {
	c := exec.Command(name, args...)
	c.Dir = repo
	c.Stderr = os.Stderr
	out, err := c.Output()
	if err != nil {
		die("%s %s: %v", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}

// ---- the coverage table

type coverRow struct{ status, xo, note string }

var (
	cover      = map[string]coverRow{}
	coverOrder []string // first insertion order, as the lookup's prefix scan reads it
	rowsByMod  = map[string][]string{}
	// pkgsByMod: the Go packages of the rows that name the module (the last
	// element of each import path, as scripts/stdaudit/map.txt keys them)
	pkgsByMod = map[string][]string{}
	// gapsByPkg: the open `gap:` symbols of scripts/stdaudit/map.txt by package
	gapsByPkg = map[string][]string{}
)

var (
	reRow     = regexp.MustCompile(`^\| (.+?) \| (.+?) \| (done|partial|in progress|planned|not planned)[^|]*\|(.*)\|`)
	reRowNP   = regexp.MustCompile(`^\| (.+?) \| (not planned) \| (.*) \|`)
	reStdMod  = regexp.MustCompile(`std/([a-z0-9]+)`)
	reSplitCo = regexp.MustCompile(`,\s*`)
	reSideTag = regexp.MustCompile(`\s*\((server|client)\)`)
)

func readCoverage() {
	f, err := os.Open(filepath.Join(repo, "knowledgebase/infrastructure/std-coverage.md"))
	if err != nil {
		die("%v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	outside := false
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, "## ") {
			outside = l == "## Outside Go's standard library"
		}
		var first, xo, st, note string
		if m := reRow.FindStringSubmatch(l); m != nil {
			first, xo, st, note = m[1], m[2], m[3], m[4]
			if st == "in progress" { // the outside rows' word for partial
				st = "partial"
			}
		} else if m2 := reRowNP.FindStringSubmatch(l); m2 != nil && (strings.Contains(m2[1], "go/*") ||
			strings.Contains(m2[1], "debug/*") || strings.Contains(m2[1], "runtime") ||
			strings.Contains(m2[1], "plugin") || strings.Contains(m2[1], "reflect") || strings.Contains(m2[1], "image")) {
			first, xo, st, note = m2[1], "none", "not planned", m2[3]
		} else {
			continue
		}
		var names []string
		for _, part := range reSplitCo.Split(first, -1) {
			part = strings.Trim(part, "` ")
			part = reSideTag.ReplaceAllString(part, "")
			if strings.Contains(part, "(") { // go/* (ast, ...): expand
				var pre string
				if strings.Contains(part, "/*") {
					pre = strings.Split(part, "/*")[0]
				} else {
					pre = strings.TrimRight(strings.Split(part, " ")[0], "/")
				}
				names = append(names, pre+"/*")
			} else {
				names = append(names, part)
			}
		}
		for _, n := range names {
			if _, ok := cover[n]; !ok {
				coverOrder = append(coverOrder, n)
			}
			cover[n] = coverRow{st, strings.Trim(xo, "` "), strings.TrimSpace(note)}
		}
		seen := map[string]bool{}
		var mods []string
		for _, mm := range reStdMod.FindAllStringSubmatch(xo, -1) {
			if !seen[mm[1]] {
				seen[mm[1]] = true
				mods = append(mods, mm[1])
			}
		}
		if strings.Contains(xo, "prelude") {
			mods = append(mods, "prelude")
		}
		// a done row outside Go's standard library counts only with its
		// symbol audit (decision 0655): its reference module pinned in
		// scripts/stdaudit/outside.txt, whose packages' gaps then apply
		modSt := st
		pkgs := names
		if outside {
			pkgs = nil
			if pin, ok := outsidePins[strings.TrimSpace(first)]; ok {
				pkgs = pin
			} else if st == "done" {
				modSt = "unaudited"
			}
		}
		for _, m := range mods {
			rowsByMod[m] = append(rowsByMod[m], modSt)
			for _, n := range pkgs {
				if !strings.Contains(n, "*") {
					pkgsByMod[m] = append(pkgsByMod[m], shortPkg(n))
				}
			}
		}
	}
}

// outsidePins maps an outside row's Need cell to the Go packages
// scripts/stdaudit/outside.txt pins for it.
var outsidePins = map[string][]string{}

// readOutsidePins reads scripts/stdaudit/outside.txt (decision 0655).
func readOutsidePins() {
	data, err := os.ReadFile(filepath.Join(repo, "scripts/stdaudit/outside.txt"))
	if err != nil {
		die("%v", err)
	}
	for _, l := range strings.Split(string(data), "\n") {
		if l = strings.TrimSpace(l); l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if f := strings.Split(l, "|"); len(f) == 4 {
			outsidePins[strings.TrimSpace(f[0])] = strings.Fields(f[3])
		}
	}
}

// shortPkg is map.txt's key prefix for a Go package: its last path
// element, or the one before a /vN suffix (scripts/stdaudit shortName).
func shortPkg(p string) string {
	parts := strings.Split(p, "/")
	last := parts[len(parts)-1]
	if len(parts) > 1 && len(last) > 1 && last[0] == 'v' && strings.Trim(last[1:], "0123456789") == "" {
		return parts[len(parts)-2]
	}
	if i := strings.LastIndex(last, ".v"); i > 0 && strings.Trim(last[i+2:], "0123456789") == "" && i+2 < len(last) {
		return last[:i] // gopkg.in/yaml.v3: yaml
	}
	return last
}

// readAuditGaps fills gapsByPkg from scripts/stdaudit/map.txt (decision
// 0276's audit): a row marked done in std-coverage.md does not make a module
// Proven while its packages have an open gap there.
func readAuditGaps() {
	data, err := os.ReadFile(filepath.Join(repo, "scripts/stdaudit/map.txt"))
	if err != nil {
		die("%v", err)
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "#") {
			continue
		}
		key, val, ok := strings.Cut(l, " = ")
		if !ok || !strings.HasPrefix(strings.TrimSpace(val), "gap:") {
			continue
		}
		key = strings.TrimSpace(key)
		// a key with an import path (github.com/x/jwt/v5.Parse): the package
		// is the path's short name; else the key's first element
		var pkg, sym string
		if i := strings.LastIndex(key, "/"); i >= 0 {
			j := strings.Index(key[i:], ".")
			if j < 0 {
				continue
			}
			pkg, sym = shortPkg(key[:i+j]), key[i+j+1:]
		} else {
			var ok bool
			if pkg, sym, ok = strings.Cut(key, "."); !ok {
				continue
			}
		}
		gapsByPkg[pkg] = append(gapsByPkg[pkg], pkg+"."+sym)
	}
}

// auditGaps are the open stdaudit gaps of module m's Go packages, sorted.
func auditGaps(m string) []string {
	seen := map[string]bool{}
	var gs []string
	for _, p := range pkgsByMod[m] {
		if seen[p] {
			continue
		}
		seen[p] = true
		gs = append(gs, gapsByPkg[p]...)
	}
	sort.Strings(gs)
	return gs
}

func lookup(p string) (coverRow, bool) {
	if c, ok := cover[p]; ok {
		return c, true
	}
	top := strings.Split(p, "/")[0]
	if c, ok := cover[top+"/*"]; ok {
		return c, true
	}
	for _, k := range coverOrder {
		if strings.HasSuffix(k, "/*") && strings.HasPrefix(p, k[:len(k)-1]) {
			return cover[k], true
		}
	}
	if strings.HasPrefix(p, "image") {
		if c, ok := cover["image"]; ok {
			return c, true
		}
	}
	if strings.HasPrefix(p, "runtime") {
		if c, ok := cover["runtime"]; ok {
			return c, true
		}
	}
	return coverRow{}, false
}

// ---- hand-kept tables (performance rows wait for their std/evidence/MODULE/perf.md)

var parsers = set("http", "url", "mime", "multipart", "mail", "smtp", "tls", "x509", "websocket", "json", "xml", "xmlcodec",
	"toml", "csv", "archive", "compress", "dns", "regexp", "template", "html", "netip", "time", "png", "jpeg", "cbor")

type evText struct{ path, text string }

// fuzzHarnesses maps each module to its fuzz harnesses, from their paths
// (decision 0616 section 5: no typed table): internal/stdtest/STEMfuzz_test.go
// belongs to the longest module name STEM starts with (httpheadfuzz to http,
// jpegencfuzz to jpeg) and to a module of three letters or more STEM ends with
// (httpurlfuzz also to url).
func fuzzHarnesses(mods []string) map[string][]string {
	ents, err := os.ReadDir(filepath.Join(repo, "internal/stdtest"))
	if err != nil {
		die("%v", err)
	}
	out := map[string][]string{}
	for _, e := range ents {
		stem, ok := strings.CutSuffix(e.Name(), "fuzz_test.go")
		if !ok || stem == "" {
			continue
		}
		path := "internal/stdtest/" + e.Name()
		best := ""
		for _, m := range mods {
			if strings.HasPrefix(stem, m) && len(m) > len(best) {
				best = m
			}
		}
		if best != "" {
			out[best] = append(out[best], path)
		}
		for _, m := range mods {
			if m != best && len(m) >= 3 && strings.HasSuffix(stem, m) {
				out[m] = append(out[m], path)
			}
		}
	}
	return out
}

var fuzz map[string][]string

var godiff = map[string]evText{
	"tls":  {"internal/llvm/tls13_diff_test.go", "TLS 1.3 client and server differential against Go"},
	"http": {"internal/llvm/http1_diff_test.go", "HTTP/1 differential against Go"},
}

var beyondGo = map[string]string{
	"sql": "database/sql plus a PostgreSQL driver", "websocket": "Go has no websocket in std", "toml": "Go has no TOML in std",
	"uuid": "Go has no UUID in std", "rate": "x/time/rate is outside std", "backoff": "no Go std package",
	"metrics": "expvar plus a Prometheus registry", "trace": "no Go std tracing API of this kind",
	"dns": "net.Resolver covers part", "udp": "net.UDPConn",
}

type perfRow struct{ level, ev, fact string }

var perfRows = map[string]perfRow{
	"json":      {"Proven", "bench/results/json-vs-go-2026-10-21.md", "0.33 to 0.49 of Go's instructions on native (2026-10-21)"},
	"csv":       {"Proven", "bench/results/std-twin-rows-2026-11-03.md", "parse 0.92, format 1.01 of Go's wall (2026-11-03)"},
	"xml":       {"Solid", "bench/results/std-twin-rows-2026-11-03.md", "tokens 1.15, tree 0.95 of Go's wall (2026-11-03)"},
	"png":       {"Proven", "bench/results/std-twin-rows5-2026-11-03.md", "decode 1.04, encode 0.94 of Go's wall (2026-11-03)"},
	"cbor":      {"Proven", "bench/results/std-twin-rows7-2026-11-03.md", "decode 0.43, encode 0.26, deterministic encode 0.62 of fxamacker/cbor's wall (2026-11-03)"},
	"compress":  {"Proven", "bench/results/std-twin-rows-2026-11-03.md", "gzip 0.96, gunzip 0.92, deflate 0.96, inflate 0.91 of Go's wall (2026-11-03)"},
	"archive":   {"Solid", "bench/results/std-twin-rows-2026-11-03.md", "zip 1.20, tar 1.04, unzip 0.99, untar 0.27 of Go's wall (2026-11-03)"},
	"mime":      {"Solid", "bench/results/std-twin-rows-2026-11-03.md", "format 1.28, parse 1.06, detect 0.90 of Go's wall (2026-11-03)"},
	"mail":      {"Solid", "bench/results/std-twin-rows-2026-11-03.md", "display 1.15, address 0.62, header block 0.45 of Go's wall (2026-11-03)"},
	"prelude":   {"Early", "bench/results/std-twin-rows2-2026-11-03.md", "Str.trim_chars 1.62 of Go's wall (was 2.24); fs walk_dir_entries 0.91 of Go's wall (decision 0483; walk_dir with a FileInfo each 1.49, 0.94 with Go reading each entry's info); itoa 1.35 (was 1.52), atoi 1.46, sort of strings 1.42; split, search, case and predicate workloads 0.4 to 1.24 (2026-11-03)"},
	"base64":    {"Proven", "bench/results/std-twin-rows2-2026-11-03.md", "encode 0.57, decode 0.88 of Go's wall (2026-11-03)"},
	"base32":    {"Proven", "bench/results/std-twin-rows3-2026-11-03.md", "encode 0.98, decode 0.47 of Go's wall (2026-11-03)"},
	"path":      {"Solid", "bench/results/std-twin-rows2-2026-11-03.md", "clean 1.08, join 1.13, parts 1.40 of Go's wall (2026-11-03)"},
	"template":  {"Solid", "bench/results/std-twin-rows2-2026-11-03.md", "render 1.36, parse 0.90 of Go's wall (2026-11-03)"},
	"big":       {"Solid", "bench/results/std-twin-rows2-2026-11-03.md", "mul 1.23, parse 0.49 of Go's wall (2026-11-03)"},
	"crc32":     {"Proven", "bench/results/std-twin-rows2-2026-11-03.md", "crc32 0.96, crc32c 0.37 of Go's wall (2026-11-03)"},
	"deque":     {"Proven", "bench/results/std-twin-rows2-2026-11-03.md", "queue and both-ends workloads 0.13 to 0.17 of Go's wall (2026-11-03)"},
	"dns":       {"Proven", "bench/results/dns-netpoll-2026-11-03.md", "MX lookup over UDP 0.93 of Go's wall (was 4.66), a lookup-less run 0.69 (2026-11-03)"},
	"x509":      {"Proven", "bench/results/std-twin-rows2-2026-11-03.md", "parse, verify, create and CRL at most 0.98 of Go's wall (2026-11-03)"},
	"cookie":    {"Proven", "bench/results/std-twin-rows3-2026-11-03.md", "parse cookie 0.76, parse set-cookie 1.02 of Go's wall (2026-11-03)"},
	"toml":      {"Solid", "bench/results/std-twin-rows3-2026-11-03.md", "decode 1.17, encode 1.32 of Go's wall (2026-11-03)"},
	"rate":      {"Solid", "bench/results/std-twin-rows3-2026-11-03.md", "allow 1.29 of Go's wall (2026-11-03)"},
	"aead":      {"Solid", "bench/results/std-twin-rows3-2026-11-03.md", "gcm 0.39 to 1.31, chacha 0.48 to 1.16, hkdf 1.19 of Go's wall (2026-11-03)"},
	"websocket": {"Solid", "bench/results/std-twin-rows3-2026-11-03.md", "small messages 1.12, big 0.40, dial 0.77 of Go's wall (2026-11-03)"},
	"xmlcodec":  {"Solid", "bench/results/std-twin-rows-2026-11-03.md", "encode 0.57, decode 1.13 of Go's wall (2026-11-03)"},
	"otp":       {"Solid", "bench/results/std-twin-otp-2026-11-03.md", "sha1 0.94, sha512 0.94, sha256 1.32 of Go's wall (2026-11-03)"},
	"netip":     {"Early", "bench/results/std-twin-netip-2026-11-03.md", "1.79 of Go's wall (instructions 1.58 on 2026-10-22); decision 0485 moves the address to a 128-bit value (2026-11-03)"},
	"regexp":    {"Solid", "bench/results/std-twin-rows2-2026-11-03.md", "match 1.25, find_all 0.89 of Go's wall (2026-11-03)"},
	"multipart": {"Solid", "bench/results/json-vs-go-2026-10-21.md", "parse 1.15 of Go's instructions (2026-10-21)"},
	"crypto":    {"Solid", "bench/results/aesgcm-2026-10-22.md", "AES-GCM bulk loop 1.12, ChaCha20-Poly1305 1.2 of Go (2026-10-22)"},
	"tls":       {"Solid", "bench/results/tls-h2-vs-go-2026-10-21.md", "HTTP/2 14 us a request against Go's 32; handshakes mixed (2026-10-21)"},
	"http":      {"Solid", "bench/results/tls-h2-vs-go-2026-10-21.md", "HTTP/2 14 us a request against Go's 32; /alloc 0.97 of v0.35.0 (2026-11-02)"},
	"time":      {"Early", "bench/results/std-twin-rows2-2026-11-03.md", "format 3.7 and parse 3.6 of Go's wall with string layouts (5.4 and 4.7 before the one-scan format and parse), 2.03 and 1.90 with a const Layout; civil 0.46 (2026-11-03)"},
}

// extraResults names other results files a module's newest row may come from.
var extraResults = map[string][]string{"dns": {"dns-netpoll"}}

func set(xs ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// ---- performance rows from the newest per-module results file

var reDateMd = regexp.MustCompile(`(\d{4}-\d\d-\d\d)\.md$`)

func newestResults(m string, files []string) (date, file string) {
	for _, n := range append([]string{m}, extraResults[m]...) {
		re := regexp.MustCompile(`^(?:std-twin-)?` + regexp.QuoteMeta(n) + `-(\d{4}-\d\d-\d\d)\.md$`)
		for _, f := range files {
			g := re.FindStringSubmatch(f)
			if g != nil && (file == "" || g[1] > date || (g[1] == date && f > file)) {
				date, file = g[1], f
			}
		}
	}
	return
}

type ratio struct {
	work string
	r    float64
}

// tableFacts reads the last table with a ratio column (Xo over Go) of a
// results file. A before/after table (one change's --ab) is skipped: it is not
// a row against Go (std-twin-time-2026-11-03.md's "ratio" was after/before and
// read as Xo/Go, so time showed Proven at 5.4x Go's format).
func tableFacts(path string) []ratio {
	data, err := os.ReadFile(path)
	if err != nil {
		die("%v", err)
	}
	const none, skip = -1, -2
	var last, rows []ratio
	col := none
	for _, l := range strings.SplitAfter(string(data), "\n") {
		if l == "" {
			continue
		}
		if !strings.HasPrefix(l, "|") {
			if len(rows) > 0 {
				last = rows
			}
			rows, col = nil, none
			continue
		}
		parts := strings.Split(strings.Trim(strings.TrimSpace(l), "|"), "|")
		cells := make([]string, len(parts))
		for i, c := range parts {
			cells[i] = strings.TrimSpace(c)
		}
		if col == none {
			isAB := false
			for _, c := range cells {
				lc := strings.ToLower(c)
				if strings.Contains(lc, "before") || strings.Contains(lc, "after") {
					isAB = true
				}
			}
			if isAB {
				col, rows = skip, nil
				continue
			}
			for i, c := range cells {
				switch strings.ToLower(c) {
				case "ratio", "xo / go", "xo/go":
					col = i
				}
				if col != none {
					break
				}
			}
			rows = nil
			continue
		}
		if col == skip || strings.Trim(strings.Join(cells, ""), "-: ") == "" {
			continue
		}
		if col >= len(cells) {
			continue
		}
		if v, err := strconv.ParseFloat(cells[col], 64); err == nil {
			rows = append(rows, ratio{cells[0], v})
		}
	}
	if len(rows) > 0 {
		last = rows
	}
	return last
}

// pyG formats a float as Python's "{:g}" does.
func pyG(v float64) string {
	s := strconv.FormatFloat(v, 'g', 6, 64)
	if i := strings.Index(s, "e"); i >= 0 { // Go writes e+06 as Python does, and e-05; keep
		return s
	}
	return s
}

func applyResults() {
	ents, err := os.ReadDir(filepath.Join(repo, "bench/results"))
	if err != nil {
		die("%v", err)
	}
	var files []string
	for _, e := range ents {
		files = append(files, e.Name())
	}
	for m, pr := range perfRows {
		date, f := newestResults(m, files)
		if f == "" {
			continue
		}
		if m0 := reDateMd.FindStringSubmatch(pr.ev); m0 != nil && date < m0[1] {
			continue
		}
		rows := tableFacts(filepath.Join(repo, "bench/results", f))
		if len(rows) == 0 {
			continue
		}
		allNamed := true
		for _, r := range rows {
			if r.work == "" {
				allNamed = false
			}
		}
		if !allNamed {
			continue
		}
		var kept []ratio
		for _, r := range rows {
			if r.work != "none" && r.work != "setup" {
				kept = append(kept, r)
			}
		}
		if len(kept) == 0 {
			continue
		}
		worst := math.Inf(-1)
		var txt []string
		for _, r := range kept {
			worst = math.Max(worst, r.r)
			txt = append(txt, r.work+" "+pyG(r.r))
		}
		lvl := "Early"
		if worst <= 1.05 {
			lvl = "Proven"
		} else if worst <= 1.5 {
			lvl = "Solid"
		}
		perfRows[m] = perfRow{lvl, "bench/results/" + f, strings.Join(txt, ", ") + " of Go's wall (" + date + ")"}
	}
}

// ---- marks

type mark struct{ level, text, ev string }

var lv = map[string]int{"Early": 0, "Solid": 1, "Proven": 2}
var nm = []string{"Early", "Solid", "Proven"}

func pill(level, link, key, name string) string {
	if key != "" {
		return fmt.Sprintf(`{{< level "%s" "%s" "%s" "%s" >}}`, level, link, key, name)
	}
	if link != "" {
		return fmt.Sprintf(`{{< level "%s" "%s" >}}`, level, link)
	}
	return fmt.Sprintf(`{{< level "%s" >}}`, level)
}

func link(ev string) string {
	if ev == "" {
		return ""
	}
	return gh + ev
}

func count(xs []string) map[string]int {
	c := map[string]int{}
	for _, x := range xs {
		c[x]++
	}
	return c
}

func feat(m string) mark {
	st := rowsByMod[m]
	if b, ok := beyondGo[m]; ok && len(st) == 0 {
		return mark{"Solid", "beyond Go: " + b, ""}
	}
	if len(st) == 0 {
		return mark{"Solid", "no Go package maps to it; covers its own domain's common uses", ""}
	}
	c := count(st)
	if c["unaudited"] > 0 && c["partial"] == 0 && c["planned"] == 0 {
		return mark{"Solid", fmt.Sprintf("all %d mapped rows done, %d outside Go's standard library without a symbol audit yet (decision 0655)", len(st), c["unaudited"]),
			"knowledgebase/infrastructure/std-coverage.md"}
	}
	if c["partial"] > 0 || c["planned"] > 0 {
		text := fmt.Sprintf("%d of %d mapped Go rows done, %d partial", c["done"]+c["unaudited"], len(st), c["partial"])
		if gs := auditGaps(m); len(gs) > 0 {
			text += fmt.Sprintf(", %d open stdaudit gaps", len(gs))
		}
		return mark{"Solid", text,
			"knowledgebase/infrastructure/std-coverage.md"}
	}
	if gs := auditGaps(m); len(gs) > 0 {
		shown := gs
		if len(shown) > 4 {
			shown = append(append([]string(nil), gs[:4]...), fmt.Sprintf("%d more", len(gs)-4))
		}
		return mark{"Solid", fmt.Sprintf("all %d mapped Go rows done, %d open stdaudit gaps (%s)", len(st), len(gs), strings.Join(shown, ", ")),
			"scripts/stdaudit/map.txt"}
	}
	return mark{"Proven", fmt.Sprintf("all %d mapped Go rows done, no open stdaudit gap", len(st)), "knowledgebase/infrastructure/std-coverage.md"}
}

// ---- evidence (decision 0616)

type evFile struct {
	hdr  map[string]string
	path string
}

var evidenceReview = set("crypto", "tls", "http", "x509")
var reHdr = regexp.MustCompile(`^([a-z]+): (.*)$`)

// evidence returns the module's evidence files that count: `result: pass`, and
// fresh by decision 0616 section 3 (fresh below).
func evidence(m string) map[string]evFile {
	d := filepath.Join(repo, "std/evidence", m)
	out := map[string]evFile{}
	if st, err := os.Stat(d); err != nil || !st.IsDir() {
		return out
	}
	cur, have := "", false
	for _, k := range []string{"race", "stress", "fuzz", "review", "perf"} {
		data, err := os.ReadFile(filepath.Join(d, k+".md"))
		if err != nil {
			continue
		}
		hdr := map[string]string{}
		for _, l := range strings.Split(string(data), "\n") {
			mm := reHdr.FindStringSubmatch(l)
			if mm == nil {
				break
			}
			hdr[mm[1]] = strings.TrimSpace(strings.SplitN(mm[2], "  #", 2)[0])
		}
		if hdr["result"] != "pass" {
			continue
		}
		if !have {
			cur = strings.SplitN(sh("go", "run", "./scripts/stdproven", "hash", m), "\n", 2)[0]
			cur, have = strings.TrimSpace(cur), true
		}
		if fresh(hdr, cur) {
			out[k] = evFile{hdr, "std/evidence/" + m + "/" + k + ".md"}
		}
	}
	return out
}

// fresh: decision 0616 section 3. Evidence counts while its source hash
// matches the module's now, or for 90 days and one minor release after the
// run, whichever is later: a run with a stale hash goes stale only once it is
// over 90 days old and a minor release (a vX.Y.0 tag dated on or before today)
// has been tagged after its date.
func fresh(hdr map[string]string, cur string) bool {
	if hdr["source"] == cur {
		return true
	}
	date := hdr["date"]
	if date == "" {
		date = "1970-01-01"
	}
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false
	}
	if int(today.Sub(d).Hours()/24) <= 90 {
		return true
	}
	for _, t := range minorReleases() {
		if t.After(d) && !t.After(today) {
			return false
		}
	}
	return true
}

var minors []time.Time
var minorsRead bool

var reMinorTag = regexp.MustCompile(`^v\d+\.\d+\.0$`)

// minorReleases returns the dates of the vX.Y.0 tags.
func minorReleases() []time.Time {
	if minorsRead {
		return minors
	}
	minorsRead = true
	for _, l := range strings.Split(sh("git", "for-each-ref", "refs/tags", "--format=%(refname:short) %(creatordate:short)"), "\n") {
		f := strings.Fields(l)
		if len(f) != 2 || !reMinorTag.MatchString(f[0]) {
			continue
		}
		if t, err := time.Parse("2006-01-02", f[1]); err == nil {
			minors = append(minors, t)
		}
	}
	return minors
}

var evCache = map[string]map[string]evFile{}

func ev(m string) map[string]evFile {
	if e, ok := evCache[m]; ok {
		return e
	}
	e := evidence(m)
	evCache[m] = e
	return e
}

var corpus = map[string]bool{}

func test(m string) mark {
	e := ev(m)
	need := []string{"race", "stress", "fuzz"}
	if _, err := os.Stat(filepath.Join(repo, "std/evidence", m, "review.md")); evidenceReview[m] || err == nil {
		need = append(need, "review")
	}
	all := true
	for _, k := range need {
		if _, ok := e[k]; !ok {
			all = false
		}
	}
	if all {
		return mark{"Proven", fmt.Sprintf("Proven evidence (%s; decision 0616): %s", strings.Join(need, ", "), e["fuzz"].hdr["measure"]),
			"std/evidence/" + m}
	}
	hasCorpus := corpus[m]
	gd, diff := godiff[m]
	if !hasCorpus && !diff {
		return mark{"Early", "own tests only", ""}
	}
	evp, what := gd.path, gd.text
	if hasCorpus {
		evp, what = "internal/stdtest/testdata/corpus/"+m, "Go's tests ported"
	}
	if parsers[m] {
		if fz := fuzz[m]; len(fz) > 0 {
			names := make([]string, len(fz))
			for i, f := range fz {
				names[i] = "`" + filepath.Base(f) + "`"
			}
			return mark{"Solid", what + "; fuzz tested (" + strings.Join(names, ", ") + ")", fz[0]}
		}
		return mark{"Early", what + "; parser with no fuzz harness", evp}
	}
	return mark{"Solid", what, evp}
}

var reWorst = regexp.MustCompile(`worst Xo / Go (\d+\.\d+)`)

// perfHelpers: internal modules (tables and helpers other std modules
// use) whose performance is their users' (lead, 2026-11-15): a helper
// takes the lowest perf mark among its users, so it is Proven only when
// every user's twin is. No twin of its own.
var perfHelpers = map[string][]string{
	"htmlent":   {"html", "htmltok"},
	"uniclass":  {"regexp"},
	"protowire": {"protodesc", "protojson", "protowkt"},
}

func perf(m string) mark {
	if users, ok := perfHelpers[m]; ok {
		var low mark
		for i, u := range users {
			if p := perf(u); i == 0 || lv[p.level] < lv[low.level] {
				low = p
			}
		}
		return mark{low.level, "the lowest of its users' marks (" + strings.Join(users, ", ") + ")", low.ev}
	}
	e := ev(m)
	if p, ok := e["perf"]; ok {
		meas := p.hdr["measure"]
		if mm := reWorst.FindStringSubmatch(meas); mm != nil {
			if v, _ := strconv.ParseFloat(mm[1], 64); v <= 1.05 {
				return mark{"Proven", fmt.Sprintf("worst case %s of Go's wall over %s (%s)", mm[1], strings.Split(meas, ",")[0], p.hdr["date"]), p.path}
			}
		}
	}
	if pr, ok := perfRows[m]; ok {
		return mark{pr.level, pr.fact, pr.ev}
	}
	return mark{"Early", "no committed row against Go", ""}
}

// ---- short texts of the marks table

var (
	reAllRows  = regexp.MustCompile(`^all (\d+) mapped Go rows done`)
	reSomeRows = regexp.MustCompile(`^(\d+) of (\d+) mapped Go rows done`)
	reEndDate  = regexp.MustCompile(`\s*\(\d{4}-\d{2}-\d{2}\)\s*$`)
	reParens   = regexp.MustCompile(`\([^)]*\)`)
)

func shortFeat(f mark) string {
	t := f.text
	if m := reAllRows.FindStringSubmatch(t); m != nil {
		return m[1] + "/" + m[1]
	}
	if m := reSomeRows.FindStringSubmatch(t); m != nil {
		return m[1] + "/" + m[2]
	}
	if strings.HasPrefix(t, "beyond Go") {
		return "beyond Go"
	}
	if strings.HasPrefix(t, "no Go package") {
		return "own domain"
	}
	return ""
}

func shortTest(t mark) string {
	x := t.text
	if strings.HasPrefix(x, "own tests") {
		return "own tests"
	}
	base := "ported"
	if strings.Contains(x, "differential") && !strings.Contains(x, "ported") {
		base = "differential"
	}
	switch {
	case strings.Contains(x, "no fuzz harness"):
		return base + ", no fuzz"
	case strings.HasPrefix(x, "Proven evidence"):
		s := "race, stress, fuzz 1M"
		if strings.Contains(x, "review") {
			s += ", review"
		}
		return s
	case strings.Contains(x, "hostile"):
		return base + " + hostile"
	case strings.Contains(x, "fuzz"):
		return base + " + fuzz"
	}
	return base
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// decimals finds the numbers d+.d+ not preceded by a digit, '.', '/' or '-'
// and not followed by a digit (Python's (?<![\d./-])\d+\.\d+(?![\d])).
func decimals(s string) []float64 {
	var out []float64
	for i := 0; i < len(s); {
		if !isDigit(s[i]) || (i > 0 && (isDigit(s[i-1]) || strings.IndexByte("./-", s[i-1]) >= 0)) {
			i++
			continue
		}
		j := i
		for j < len(s) && isDigit(s[j]) {
			j++
		}
		if j < len(s) && s[j] == '.' && j+1 < len(s) && isDigit(s[j+1]) {
			k := j + 1
			for k < len(s) && isDigit(s[k]) {
				k++
			}
			v, _ := strconv.ParseFloat(s[i:k], 64)
			out = append(out, v)
			i = k
			continue
		}
		i++
	}
	return out
}

func shortPerf(p mark) string {
	x := p.text
	if strings.HasPrefix(x, "no committed row") {
		return "no row"
	}
	x = reEndDate.ReplaceAllString(x, "")
	x = reParens.ReplaceAllString(x, "")
	var nums []float64
	for _, c := range strings.Split(x, ";") {
		if strings.Contains(c, "of Go") && !strings.Contains(c, " us ") {
			nums = append(nums, decimals(c)...)
		}
	}
	if len(nums) == 0 {
		return "see notes"
	}
	unit := "x"
	if strings.Contains(x, "instructions") {
		unit = "x instr."
	}
	lo, hi := nums[0], nums[0]
	for _, n := range nums {
		lo, hi = math.Min(lo, n), math.Max(hi, n)
	}
	if lo == hi {
		return pyG(lo) + unit
	}
	return pyG(lo) + " to " + pyG(hi) + unit
}

// ---- module history

func added(m string) (date, rel string) {
	lines := strings.Split(sh("git", "log", "--diff-filter=A", "--format=%h %cs", "--", "std/"+m+".xo"), "\n")
	f := strings.Fields(lines[len(lines)-1])
	if len(f) < 2 {
		die("no history for std/%s.xo", m)
	}
	for _, t := range strings.Fields(sh("git", "tag", "--contains", f[0], "--sort=v:refname")) {
		if strings.HasPrefix(t, "v0.") {
			return f[1], t
		}
	}
	return f[1], "unreleased"
}

// ---- the -json snapshot

// modMarks: one module's mark on each axis and its overall level.
type modMarks struct {
	Module      string `json:"module"`
	Features    string `json:"features"`
	Testing     string `json:"testing"`
	Performance string `json:"performance"`
	Overall     string `json:"overall"`
}

// marksJSON: the snapshot `xw std` ingests, the commit it describes (sha
// and commit time) and every module's marks.
func marksJSON(sha, at string, ms []modMarks) ([]byte, error) {
	return json.MarshalIndent(struct {
		Sha     string     `json:"sha"`
		At      string     `json:"at"`
		Modules []modMarks `json:"modules"`
	}{sha, at, ms}, "", " ")
}

// ---- main

func main() {
	out := flag.String("o", "", "write the page to `FILE` (default stdout)")
	day := flag.String("today", "", "the date staleness is judged on, `YYYY-MM-DD` (default today)")
	js := flag.String("json", "", "also write each module's marks to `FILE` (xw std reads these snapshots; ADR 0658 stage 3)")
	flag.Parse()
	var err error
	if repo, err = os.Getwd(); err != nil {
		die("%v", err)
	}
	today = time.Now().UTC().Truncate(24 * time.Hour)
	if *day != "" {
		if today, err = time.Parse("2006-01-02", *day); err != nil {
			die("-today: %v", err)
		}
	}

	head := sh("git", "log", "-1", "--format=%h")
	headDate := sh("git", "log", "-1", "--format=%cs")
	vb, err := os.ReadFile(filepath.Join(repo, "std/VERSION"))
	if err != nil {
		die("%v", err)
	}
	version := strings.TrimSpace(string(vb))

	var gopk []string
	for _, p := range strings.Split(sh("go", "list", "std"), "\n") {
		if p == "" || strings.Contains(p, "/internal") || strings.HasPrefix(p, "internal") || strings.Contains(p, "vendor") {
			continue
		}
		gopk = append(gopk, p)
	}
	gover := strings.Fields(sh("go", "version"))[2]

	readOutsidePins()
	readCoverage()
	readAuditGaps()
	status := map[string]string{}
	for _, p := range gopk {
		if c, ok := lookup(p); ok {
			status[p] = c.status
		} else {
			status[p] = "unmapped"
		}
	}
	// the owner's list of potential gaps (2026-11-02) overrides the table's "not planned" for these
	// (the image packages left it on 2026-11-15: std/image, std/png, std/jpeg and std/gif map them)
	for _, g := range []string{"encoding/gob", "net/rpc", "net/rpc/jsonrpc",
		"compress/bzip2", "compress/lzw", "hash/crc64", "hash/adler32", "os/user", "log/syslog", "text/scanner", "container/ring",
		"unicode/utf16", "index/suffixarray", "crypto/des", "crypto/dsa", "crypto/rc4", "weak", "unique"} {
		if _, ok := status[g]; ok {
			status[g] = "planned"
		}
	}
	// a test helper for a package xo does not export; the parse tree is internal to std/template
	for _, g := range []string{"crypto/mlkem/mlkemtest", "text/template/parse"} {
		status[g] = "not planned"
	}
	cnt := map[string]int{}
	for _, s := range status {
		cnt[s]++
	}
	var unmapped []string
	for _, p := range gopk {
		if status[p] == "unmapped" {
			unmapped = append(unmapped, p)
		}
	}
	if len(unmapped) > 0 {
		fmt.Fprintf(os.Stderr, "unmapped: %q\n", unmapped)
	}

	ents, err := os.ReadDir(filepath.Join(repo, "std"))
	if err != nil {
		die("%v", err)
	}
	var mods []string
	for _, e := range ents {
		n := e.Name()
		if strings.HasSuffix(n, ".xo") && !strings.Contains(n, "_test") && n != "internal.xo" && n != "prelude.xo" {
			mods = append(mods, strings.TrimSuffix(n, ".xo"))
		}
	}
	sort.Strings(mods)
	nmods := len(mods)
	cents, err := os.ReadDir(filepath.Join(repo, "internal/stdtest/testdata/corpus"))
	if err != nil {
		die("%v", err)
	}
	for _, e := range cents {
		corpus[e.Name()] = true
	}
	fuzz = fuzzHarnesses(mods)
	applyResults()

	type row struct {
		m          string
		f, t, p    mark
		lv, d, rel string
	}
	var rows []row
	for _, m := range append([]string{"prelude"}, mods...) {
		f, t, p := feat(m), test(m), perf(m)
		l := nm[min(lv[f.level], lv[t.level], lv[p.level])]
		d, rel := "2026-10-08", "go-compiler-final"
		if m != "prelude" {
			d, rel = added(m)
		}
		rows = append(rows, row{m, f, t, p, l, d, rel})
	}
	if *js != "" {
		var ms []modMarks
		for _, r := range rows {
			ms = append(ms, modMarks{r.m, r.f.level, r.t.level, r.p.level, r.lv})
		}
		data, err := marksJSON(sh("git", "rev-parse", "HEAD"), sh("git", "log", "-1", "--format=%cI"), ms)
		if err != nil {
			die("-json: %v", err)
		}
		if err := os.WriteFile(*js, data, 0o644); err != nil {
			die("-json: %v", err)
		}
	}
	lvc := map[string]int{}
	ax := map[string]map[string]int{"features": {}, "testing": {}, "performance": {}}
	for _, r := range rows {
		lvc[r.lv]++
		ax["features"][r.f.level]++
		ax["testing"][r.t.level]++
		ax["performance"][r.p.level]++
	}

	var b strings.Builder
	w := func(f string, a ...any) {
		if len(a) == 0 {
			b.WriteString(f)
		} else {
			fmt.Fprintf(&b, f, a...)
		}
		b.WriteString("\n")
	}
	w("---")
	w(`title: "Standard library coverage"`)
	w(`linkTitle: "Std coverage"`)
	w(`description: "Which of Go's %d standard packages Xo covers, what is beyond Go, what is not applicable by design, what is a gap with its priority, and a maturity level for each std module with its evidence."`, len(gopk))
	w("weight: 7")
	w("toc: true")
	w("---")
	w("")
	w("As of Xo %s (development, main %s of %s); Go %s. Every count below comes from a command run on that commit: `go list std` without internal and vendor packages, and the table in the repository's [std-coverage.md](%sknowledgebase/infrastructure/std-coverage.md) (decision 0276: Go's standard library is std's reference scope).", version, head, headDate, gover[2:], gh)
	w("")
	w("## Summary")
	w("")
	w("Go has **%d** public standard library packages. Xo has **%d** `std` modules besides the prelude (the builtin types and capabilities every program sees).", len(gopk), nmods)
	w("")
	w("| Status | Go packages |")
	w("|---|---|")
	w("| Covered (done) | %d |", cnt["done"])
	w("| Partial | %d |", cnt["partial"])
	w("| Not applicable by design | %d |", cnt["not planned"])
	w("| Gap, potentially planned | %d |", cnt["planned"])
	if cnt["unmapped"] > 0 {
		w("| Not yet mapped | %d |", cnt["unmapped"])
	}
	w("")
	w("Beyond Go: modules with no Go standard library counterpart, or going past it, are listed under [Beyond Go](#beyond-go).")
	w("")
	w("## Maturity levels")
	w("")
	w("Each `std` module gets three marks, one per axis, and an overall level that is the weakest of the three (owner, 2026-11-02, decision 0461).")
	w("")
	w("%s", "| Axis | "+pill("Early", "", "", "")+" | "+pill("Solid", "", "", "")+" | "+pill("Proven", "", "", "")+" |")
	w("|---|---|---|---|")
	w("| Features, against Go's package or the domain | core uses | common uses, gaps listed | near full parity, or beyond Go |")
	w("| Testing | own tests | Go's tests ported or a Go differential; parsers fuzzed | also `--race` and load stress clean, long fuzz runs, security reviewed where it applies (crypto, tls, http, x509) |")
	w("| Performance | not measured, or measured and above 2x Go | measured against Go, within 2x | at or better than Go on its benchmark |")
	w("")
	w("How a mark is read here: *Features* from the rows of the coverage table that name the module (all done and no open function level gap of their Go packages in the audit, `scripts/stdaudit/map.txt`, is Proven; any partial row or open gap is Solid; a module with no Go package is Solid and says so). *Testing* from the committed corpus of Go's tests under `internal/stdtest/testdata/corpus`, Go differential tests, and fuzz tests for modules that parse untrusted input. *Performance* from a committed result file with a Go twin; an internal helper module (htmlent, uniclass, protowire) takes the lowest performance mark among the modules that use it. A mark with no committed evidence is Early, so a missing result is the gap to close, not a hidden number. The dates are those of the result files, which can be older than this page; a re-run can only move a mark if the file is replaced. Proven testing evidence (`std/evidence/MODULE`) counts while the module's source hash matches the one it ran at, or for 90 days and one minor release: the hash covers the module's own files (`std/MODULE.xo`, `std/native/MODULE.xo`, `std/native/MODULE_native.xo` and the module specific `std/native/llrt` files they reach), the same files of every std module it uses, and `std/internal.xo`, but not the shared runtime (the allocator, scheduler, I/O and OS layer and the prelude), which its own tests, the fixed point and the full tier cover (decision 0616 section 3).")
	w("")
	w("Today: overall %d Proven, %d Solid, %d Early of %d (prelude and %d modules). Per axis, features %d/%d/%d, testing %d/%d/%d, performance %d/%d/%d (Proven/Solid/Early). No module has the committed `--race`, load stress, long fuzz and security review evidence that Proven testing needs yet.",
		lvc["Proven"], lvc["Solid"], lvc["Early"], len(rows), nmods,
		ax["features"]["Proven"], ax["features"]["Solid"], ax["features"]["Early"],
		ax["testing"]["Proven"], ax["testing"]["Solid"], ax["testing"]["Early"],
		ax["performance"]["Proven"], ax["performance"]["Solid"], ax["performance"]["Early"])
	w("")
	w("### Per module")
	w("")
	w("Marks only. Each module has its three marks (F features, T testing, P performance) and the short figures behind them; times are multiples of Go's time, so lower is faster and 0.47x is better than Go. The prose behind each mark (which rows are partial, the timings, what is missing, the evidence link) is under [Module notes](#module-notes); each module name links to its note.")
	w("")
	w("| Module | Marks: F features, T testing, P performance |")
	w("|---|---|")
	name := func(m string) string {
		if m == "prelude" {
			return "prelude"
		}
		return "std/" + m
	}
	for _, r := range rows {
		pills := pill(r.f.level, link(r.f.ev), "F", "Features") + " " + pill(r.t.level, link(r.t.ev), "T", "Testing") + " " +
			pill(r.p.level, link(r.p.ev), "P", "Performance")
		desc := "Features " + shortFeat(r.f) + " · Testing " + shortTest(r.t) + " · Performance " + shortPerf(r.p)
		w("| [`%s`](#mod-%s) | %s {{< covdesc \"%s\" >}} |", name(r.m), r.m, pills, desc)
	}
	w("")
	w("### Module notes")
	w("")
	w("One entry per module: the three marks with their evidence, in words, and when the module was added. The marks are the table's; nothing here changes one.")
	w("")
	for _, r := range rows {
		w("#### `%s` {#mod-%s}", name(r.m), r.m)
		w("")
		w("Overall %s. Added %s (%s).", pill(r.lv, "", "", ""), r.rel, r.d)
		w("")
		w("- Features %s: %s.", pill(r.f.level, link(r.f.ev), "", ""), r.f.text)
		w("- Testing %s: %s.", pill(r.t.level, link(r.t.ev), "", ""), r.t.text)
		w("- Performance %s: %s.", pill(r.p.level, link(r.p.ev), "", ""), r.p.text)
		w("")
	}
	w("## Go packages")
	w("")
	w("All %d packages of `go list std`, by status. A package that maps to a module names it; the repository's coverage table has the notes per row.", len(gopk))
	w("")
	byst := func(st string) []string {
		var ps []string
		for _, p := range gopk {
			if status[p] == st {
				ps = append(ps, p)
			}
		}
		sort.Strings(ps)
		return ps
	}
	xoname := func(p string) string {
		c, ok := lookup(p)
		if !ok || c.xo == "" || c.xo == "none" {
			return "none"
		}
		return c.xo
	}
	for _, sec := range [][3]string{
		{"Covered", "done", "The module covers what a backend uses of the package, on the native backends and the Go backend."},
		{"Partial", "partial", "Some of it; the notes in the coverage table name what is missing."},
		{"Not applicable by design", "not planned", "Reflection, Go source tooling, cgo and the runtime's internals are replaced by language features or the toolchain, or the algorithm is broken or legacy."},
		{"Gaps, potentially planned", "planned", "Nothing yet, wanted. See [Roadmap of gaps](#roadmap-of-gaps)."},
	} {
		ps := byst(sec[1])
		w("### %s (%d)", sec[0], len(ps))
		w("")
		w("%s", sec[2])
		w("")
		if sec[1] == "not planned" {
			q := make([]string, len(ps))
			for i, p := range ps {
				q[i] = "`" + p + "`"
			}
			w("%s", strings.Join(q, ", "))
		} else {
			w("| Go package | Xo |")
			w("|---|---|")
			for _, p := range ps {
				w("| `%s` | %s |", p, xoname(p))
			}
		}
		w("")
	}
	w("### Beyond Go")
	w("")
	var bg []string
	for m := range beyondGo {
		bg = append(bg, m)
	}
	sort.Strings(bg)
	for i, m := range bg {
		bg[i] = "`std/" + m + "` (" + beyondGo[m] + ")"
	}
	w("%s", "Modules with no Go standard library counterpart, or that go past it: "+strings.Join(bg, ", ")+". Also in `std/crypto`: Argon2id password hashing, AES-GCM and ChaCha20-Poly1305 as public AEAD, HKDF.")
	w("")
	w("## Function level checks")
	w("")
	w("The coverage table is checked against the code, not only the package names. Examples from this commit (`xo symbols --module=... --pub`):")
	w("")
	w("- `crypto/md5`, `crypto/sha1`: `Hash.Md5` and `Hash.Sha1` in `std/crypto`, one-shot and streaming (`Hasher`), also through `hmac`. Covered.")
	w("- `encoding/hex`: `Bytes.to_hex` and `Bytes.from_hex` in the prelude. Covered; Go's `Dump` is not offered.")
	w("- `hash/crc32`: `std/crc32` with `Poly.Ieee` and `Poly.Castagnoli`, `checksum` and `update`. Covered. `hash/crc64` and `hash/adler32` have no module: gaps.")
	w("- `net/http/cookiejar`: `std/http` reads and writes cookies on a `Request` (`cookie`, `cookies`, `with_cookie`) but the client has no jar. Gap, planned with client cookies.")
	w("")
	w("## Roadmap of gaps")
	w("")
	w("The gaps are *potentially planned*: a priority and a realistic timeframe, not a commitment. The ordering is the std owner's estimate (2026-11-02) from port effort and demand, and the owner decides.")
	w("")
	w("| Priority | Gap | Timeframe |")
	w("|---|---|---|")
	w("| 1 | `encoding/gob`, `net/rpc`; `compress/bzip2`, `compress/lzw`; small hashes (`hash/crc64`, `hash/adler32`); `os/user`; `log/syslog`; `expvar`; `text/scanner`; `container/ring`; `unicode/utf16`; `index/suffixarray` | Q1 2027 |")
	w("| 2 | legacy crypto (`crypto/des`, `crypto/dsa`, `crypto/rc4`); `weak`, `unique` | last, or possibly never |")
	w("")
	w("The repository's coverage table still lists some of these as not planned (for example `encoding/gob`, `net/rpc`, `text/scanner`, `log/syslog`, `container/ring`, `index/suffixarray`); this page follows the owner's list, and the table moves with the decision. CBOR is not a Go package; it is listed because it is first on the list.")
	w("")
	w("Sources: [std-coverage.md](" + gh + "knowledgebase/infrastructure/std-coverage.md), [std/changes](" + gh + "std/changes/README.md) (every module's change log), [decision 0276](" + gh + "knowledgebase/decisions/0276-std-follows-go-std.md) and [decision 0461](" + gh + "knowledgebase/decisions/0461-std-module-maturity.md).")

	if *out == "" {
		os.Stdout.WriteString(b.String())
	} else if err := os.WriteFile(*out, []byte(b.String()), 0o644); err != nil {
		die("%v", err)
	}
	sum, _ := json.Marshal(map[string]any{"counts": cnt, "levels": lvc, "axes": ax, "unmapped": unmapped})
	fmt.Fprintln(os.Stderr, string(sum))
}
