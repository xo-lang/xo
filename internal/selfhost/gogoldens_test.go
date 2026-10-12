package selfhost

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)


// Recorded Go outputs (decision 0178, stage 7 step 2). Every Go side of
// a TestSelfhost* differential (the lexer's lexdump, the parser's
// astdump, the formatter, the checker's typedump, the SSA builder's
// ssadump, the arm64 emitter's asmdump, the LLVM emitter's llvmdump, the
// Go generator's gendump, and internal/defhash) goes through recorded,
// which XO_SELFHOST_GOLDENS steers:
//
//   - unset or "live": the Go side runs, as before;
//   - "record": the Go side runs and its output is written to the
//     recording; the Xo side is skipped (no self hosted binary is built)
//     and every test takes the full tier's inputs, so one run records
//     what every tier compares (scripts/selfhost-record-goldens.sh);
//   - "replay": the Go side does not run; its recorded output is read.
//     A unit with no recording is reported by TestMain, which then fails
//     the run (recordedMissing).
//
// The recording is a directory ($XO_SELFHOST_GOLDENS_DIR, else
// testdata/gogoldens, not committed: 1.2 GB, decision 0178's stage 7
// sizes) holding one gzip file per unit, <kind>/<id[:2]>/<id>.gz: the
// unit's label (a line), then its output. A unit is read only when a
// test asks for it, so replay holds no more than the live Go side does.
//
// A unit's id is the hash of its kind, its options, and the content of
// its inputs: a file's path and bytes; a run's directory, the working
// directory, and every file of its module (the nearest xo.mod at or
// above it, else the directory alone; under a temporary working
// directory, the whole top level directory of a mutation). What a run
// imports from outside that tree (std, other repository packages) is not
// in the key: a change there needs a new recording. The native kinds
// (ssadump, asmdump, llvmdump) also key on the host's GOOS/GOARCH. The
// repository root and the test's temporary directory are stored as
// placeholders, so a recording replays in any checkout.

const (
	goldensLive    = "live"
	goldensRecord  = "record"
	goldensReplay  = "replay"
	goldensRelPath = "testdata/gogoldens"
)

// goldensMode is $XO_SELFHOST_GOLDENS (live when unset).
func goldensMode() string {
	switch v := os.Getenv("XO_SELFHOST_GOLDENS"); v {
	case "", goldensLive:
		return goldensLive
	case goldensRecord, goldensReplay:
		return v
	default:
		panic(fmt.Sprintf("XO_SELFHOST_GOLDENS=%q: want live, record, or replay", v))
	}
}

// goldensRecording reports whether this run records the Go side only.
func goldensRecording() bool { return goldensMode() == goldensRecord }

var goldens struct {
	mu      sync.Mutex
	missing []string
	errs    []error
}

// pkgDir is this package's directory: the working directory when the
// test binary starts, before any test changes it (t.Chdir).
var pkgDir = func() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return wd
}()

// goldensDir is $XO_SELFHOST_GOLDENS_DIR, else testdata/gogoldens of
// this package (a relative directory is taken from the package's).
func goldensDir() string {
	dir := os.Getenv("XO_SELFHOST_GOLDENS_DIR")
	if dir == "" {
		dir = filepath.FromSlash(goldensRelPath)
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(pkgDir, dir)
	}
	return dir
}

// goldenPath is the file of unit id of kind.
func goldenPath(kind, id string) string {
	return filepath.Join(goldensDir(), kind, id[:2], id+".gz")
}

// readGolden reads unit id of kind: its label and output.
func readGolden(kind, id string) (label, out string, err error) {
	f, err := os.Open(goldenPath(kind, id))
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return "", "", err
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		return "", "", err
	}
	label, out, _ = strings.Cut(string(data), "\n")
	return label, out, nil
}

// writeGolden writes unit id of kind (through a temporary file, so a
// unit is whole or absent).
func writeGolden(kind, id, label, out string) error {
	p := goldenPath(kind, id)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	zw.Write([]byte(strings.ReplaceAll(label, "\n", " ") + "\n" + out))
	if err := zw.Close(); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", p, os.Getpid())
	if err := os.WriteFile(tmp, b.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// flushGoldens is the first error writing the recording (TestMain).
func flushGoldens() error {
	goldens.mu.Lock()
	defer goldens.mu.Unlock()
	if len(goldens.errs) > 0 {
		return fmt.Errorf("%d units not written, the first: %v", len(goldens.errs), goldens.errs[0])
	}
	return nil
}

// recordedMissing lists the units replay found no recording for.
func recordedMissing() []string {
	goldens.mu.Lock()
	defer goldens.mu.Unlock()
	return append([]string(nil), goldens.missing...)
}

// missingGolden is what replay returns for a unit with no recording
// (TestMain fails the run; the comparison shows it too).
const missingGolden = "== no recorded Go output: "

// recorded is the Go side compute of unit label of kind, live, recorded,
// or replayed by goldensMode; key holds the unit's options and input
// fingerprints (goldenCtx.fileKey, goldenCtx.dirKey).
func recorded(kind, label string, key []string, cx goldenCtx, compute func() string) string {
	mode := goldensMode()
	if mode == goldensLive {
		return compute()
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00", kind)
	for _, k := range key {
		fmt.Fprintf(h, "%d:%s\x00", len(k), k)
	}
	id := hex.EncodeToString(h.Sum(nil)[:16])
	if v := os.Getenv("XO_SELFHOST_GOLDENS_DEBUG"); v != "" && strings.Contains(label, v) {
		// the key of the units whose label holds the text (why a unit
		// is missing)
		fmt.Fprintf(os.Stderr, "gogoldens %s %s %s: %q\n", mode, kind, id, key)
	}
	if mode == goldensReplay {
		_, out, err := readGolden(kind, id)
		if err != nil {
			goldens.mu.Lock()
			goldens.missing = append(goldens.missing, kind+" "+cx.norm(label))
			goldens.mu.Unlock()
			return missingGolden + kind + " " + label
		}
		return cx.denorm(out)
	}
	out := compute()
	if err := writeGolden(kind, id, cx.norm(label), cx.norm(out)); err != nil {
		goldens.mu.Lock()
		goldens.errs = append(goldens.errs, err)
		goldens.mu.Unlock()
	}
	if recordDrops[kind] {
		return ""
	}
	return out
}

// recordDrops are the kinds whose output a recording run does not keep
// once written: no test reads them when it records (the Xo side is
// skipped), and the full tier's SSA stages, IR, and assembly of every run
// held at once took the run to 15 GB (2026-10-12). The typedump kind is
// kept: mutationSources picks its sources by the Go checker's verdicts.
var recordDrops = map[string]bool{"ssadump": true, "llvmdump": true, "asmdump": true, "gendump": true}


// recordedJSON is recorded for a Go side whose result is a struct: enc
// and dec convert it to and from JSON text; dec of a missing unit gets
// the missingGolden text.
func recordedJSON[T any](kind, label string, key []string, cx goldenCtx, compute func() T, enc func(T) any, dec func(text string) T) T {
	if goldensMode() == goldensLive {
		return compute()
	}
	var live T
	have := false
	text := recorded(kind, label, key, cx, func() string {
		live, have = compute(), true
		b, err := json.Marshal(enc(live))
		if err != nil {
			panic(err)
		}
		return string(b)
	})
	if have && !recordDrops[kind] {
		return live
	}
	return dec(text)
}

// goldenCtx holds the forms of the repository root and of the test's
// temporary directory that a recording stores as placeholders.
type goldenCtx struct {
	root, tmp string
}

// tempRE matches a directory of t.TempDir: <os.TempDir>/<test><n>/<NNN>.
var tempRE = func() *regexp.Regexp {
	var alts []string
	for _, d := range pathForms(os.TempDir()) {
		alts = append(alts, regexp.QuoteMeta(strings.TrimSuffix(d, "/")))
	}
	return regexp.MustCompile(`(?:` + strings.Join(alts, "|") + `)/[^/]+/\d{3}`)
}()

// newGoldenCtx is the context of a unit whose paths (the working
// directory, the inputs) are hints: the first temporary directory one of
// them lies in is the test's.
func newGoldenCtx(hints ...string) goldenCtx {
	// the repository root from the package's directory, not from the
	// working directory (tests change it)
	cx := goldenCtx{root: filepath.Dir(filepath.Dir(pkgDir))}
	for _, h := range hints {
		for _, f := range pathForms(h) {
			if m := tempRE.FindString(f); m != "" {
				cx.tmp = m
				return cx
			}
		}
	}
	return cx
}

// pathForms is p and p with its symbolic links resolved (on macOS
// /var is /private/var), longest first.
func pathForms(p string) []string {
	forms := []string{p, p}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		forms[1] = r
	} else if strings.HasPrefix(p, "/private/") {
		forms[1] = strings.TrimPrefix(p, "/private")
	}
	return forms
}

// subst is the placeholder of each form of the root and the temporary
// directory, the longest form first so that one form is not replaced
// inside another.
func (cx goldenCtx) subst() [][2]string {
	var out [][2]string
	add := func(p, name string) {
		if p == "" {
			return
		}
		f := pathForms(p)
		out = append(out, [2]string{f[0], "@" + name + "0@"}, [2]string{f[1], "@" + name + "1@"})
	}
	add(cx.tmp, "TMP")
	add(cx.root, "ROOT")
	sort.SliceStable(out, func(i, j int) bool { return len(out[i][0]) > len(out[j][0]) })
	return out
}

func (cx goldenCtx) norm(s string) string {
	for _, p := range cx.subst() {
		s = strings.ReplaceAll(s, p[0], p[1])
	}
	return s
}

func (cx goldenCtx) denorm(s string) string {
	if !strings.Contains(s, "@") {
		return s
	}
	for _, p := range cx.subst() {
		s = strings.ReplaceAll(s, p[1], p[0])
	}
	return s
}

// fileKey is the key part of one input file: its path as given (with
// placeholders) and its bytes.
func (cx goldenCtx) fileKey(path string, src []byte) string {
	sum := sha256.Sum256(src)
	return cx.norm(path) + "\x00" + hex.EncodeToString(sum[:])
}

// dirKey is the key part of run dir checked from wd: wd and dir (with
// placeholders), the host for the native kinds, and the fingerprint of
// the files the run reads (goldenTree).
func (cx goldenCtx) dirKey(wd, dir string, native bool) string {
	full := dir
	if !filepath.IsAbs(dir) {
		full = filepath.Join(wd, dir)
	}
	k := cx.norm(wd) + "\x00" + cx.norm(dir) + "\x00" + treeSum(goldenTree(cx, wd, full))
	if native {
		k += "\x00" + runtime.GOOS + "/" + runtime.GOARCH
	}
	return k
}

// goldenTree is the directory whose files key a run in full: under a
// temporary working directory the mutation's top level directory; else
// the nearest directory at or above full (below wd's root) holding an
// xo.mod; else full alone (not its subdirectories, "!" suffix).
func goldenTree(cx goldenCtx, wd, full string) string {
	if cx.tmp != "" && strings.HasPrefix(full, wd+string(filepath.Separator)) {
		rel, _ := filepath.Rel(wd, full)
		top, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
		return filepath.Join(wd, top)
	}
	for d := full; len(d) > len(cx.root); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "xo.mod")); err == nil {
			return d
		}
	}
	return full + "!"
}

var treeSums sync.Map // directory -> fingerprint, per test binary

// treeSum fingerprints the files of dir (with a "!" suffix, only its own
// files; else every file below it but dot directories): names and bytes.
func treeSum(dir string) string {
	if goldensMode() == goldensRecord {
		// a recording run writes no input while it runs: cache
		if v, ok := treeSums.Load(dir); ok {
			return v.(string)
		}
	}
	key := dir // "!" kept: a flat and a whole tree sum of one directory differ
	flat := strings.HasSuffix(dir, "!")
	dir = strings.TrimSuffix(dir, "!")
	h := sha256.New()
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != dir && (flat || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		sum := sha256.Sum256(data)
		fmt.Fprintf(h, "%s %x\n", filepath.ToSlash(rel), sum)
		return nil
	})
	s := hex.EncodeToString(h.Sum(nil))
	if goldensMode() == goldensRecord {
		treeSums.Store(key, s)
	}
	return s
}

// goldensWD is the working directory for a key (os.Getwd).
func goldensWD() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return wd
}

// recordedDir is recorded for the Go side of run dir checked from the
// working directory, with opts in the key.
func recordedDir(kind, dir string, native bool, opts string, compute func() string) string {
	if goldensMode() == goldensLive {
		return compute()
	}
	wd := goldensWD()
	cx := newGoldenCtx(wd, dir)
	return recorded(kind, dir+" "+opts, []string{opts, cx.dirKey(wd, dir, native)}, cx, compute)
}

// The JSON forms of the structured Go sides.
type goRunJSON struct {
	Err        string   `json:"err,omitempty"`
	Dump       string   `json:"dump,omitempty"`
	Accepted   bool     `json:"accepted,omitempty"`
	OnlyStaged bool     `json:"onlyStaged,omitempty"`
	Staged     []string `json:"staged,omitempty"`
	Full       []string `json:"full,omitempty"`
	Other      []string `json:"other,omitempty"`
}

func encGoRun(r *goRun) any {
	j := goRunJSON{Dump: r.dump, Accepted: r.accepted, OnlyStaged: r.onlyStaged, Staged: r.staged, Full: r.full, Other: r.other}
	if r.err != nil {
		j.Err = r.err.Error()
	}
	return j
}

func decGoRun(text string) *goRun {
	var j goRunJSON
	if strings.HasPrefix(text, missingGolden) {
		j.Err = text
	} else if err := json.Unmarshal([]byte(text), &j); err != nil {
		j.Err = "recorded typedump: " + err.Error()
	}
	r := &goRun{dump: j.Dump, accepted: j.Accepted, onlyStaged: j.OnlyStaged, staged: j.Staged, full: j.Full, other: j.Other}
	if j.Err != "" {
		r.err = errors.New(j.Err)
	}
	return r
}

type goSSAJSON struct {
	Text   string            `json:"text,omitempty"`
	None   string            `json:"none,omitempty"`
	Stages map[string]string `json:"stages,omitempty"`
}

func encGoSSA(g goSSA) any { return goSSAJSON{g.text, g.none, g.stages} }

func decGoSSA(text string) goSSA {
	var j goSSAJSON
	if strings.HasPrefix(text, missingGolden) {
		return goSSA{none: text}
	}
	if err := json.Unmarshal([]byte(text), &j); err != nil {
		return goSSA{none: "recorded ssadump: " + err.Error()}
	}
	g := goSSA{text: j.Text, none: j.None, stages: j.Stages}
	if g.none == "" && g.stages == nil {
		g.stages = map[string]string{}
	}
	return g
}

type goIRJSON struct {
	IR   string `json:"ir,omitempty"`
	None string `json:"none,omitempty"`
}

func encGoIR(g goIR) any { return goIRJSON{g.ir, g.none} }

func decGoIR(text string) goIR {
	var j goIRJSON
	if strings.HasPrefix(text, missingGolden) {
		return goIR{none: text}
	}
	if err := json.Unmarshal([]byte(text), &j); err != nil {
		return goIR{none: "recorded llvmdump: " + err.Error()}
	}
	return goIR{ir: j.IR, none: j.None}
}

// skipRecording ends t after its Go side when the run records it (the
// Xo side is not built).
func skipRecording(t testing.TB) {
	t.Helper()
	if goldensRecording() {
		t.Skip("XO_SELFHOST_GOLDENS=record: the Go side is recorded")
	}
}
