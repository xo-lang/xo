package selfhost

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

// The recorded outputs (decision 0178 stage 7 step 2, decision 0215).
// Every TestSelfhost* test compares the output of a self hosted component
// (the lexer's lexdump, the parser's astdump, the formatter, the
// checker's typedump, the SSA builder's ssadump, the arm64 emitter's
// asmdump, the LLVM emitter's llvmdump, the recording's gendump, and
// the definition hashes) with a recording, through recordedPieces, which
// XO_SELFHOST_GOLDENS steers:
//
//   - unset or "replay": the Xo side's pieces are hashed and compared
//     with the recorded hashes. A unit with no recording, and a piece
//     whose hash differs, are named at the end of the run (TestMain) with
//     how to see the full dumps.
//   - "record": a unit recorded for its very input is kept and compared;
//     a unit whose input or std changed takes its slot's recording under
//     its new id (the reference stays, and the comparison says whether
//     it still holds); only a unit with no recording at all (a new
//     input) records the Xo side's output. Every test takes the full tier's inputs with the go
//     backend build of its component (scripts/selfhost-record-goldens.sh).
//   - "rerecord": as record, but every unit the run computes is recorded
//     again: after a change that alters the output on purpose. Read the
//     diff of the recording.
//
// Until decision 0215 (2026-10-13) the recording was the output of the
// Go implementation of the compiler (the differential tests compared the
// two implementations); the last Go recording is of main's tree on
// 2026-10-13 with the Go compiler still in it (decision 0215). Units
// recorded since come from the Xo side.
//
// A unit is the output for one input (a file, a run of a directory) with
// one set of options. Its comparison cuts it into pieces (goldPieces),
// the same way recorded and replayed: the whole dump for the kinds
// compared byte for byte (lexdump, astdump, format, defhash), the dump,
// diagnostics, and notes of typedump, and for the kinds compared function
// by function (ssadump, llvmdump, asmdump, gendump) one member per
// function, helper, file, or declaration in a family, so a mismatch names
// the function. A unit records one line per family (its member count and
// the hash of its sorted member hashes), and each kind records the member
// hashes once, in its pool (pool-<x>.txt, 48 bits each): a unit repeats
// the std functions every other unit has
// (gotchas/selfhost-goldens-per-function-lines.md). Replay resolves a
// family against the Xo side's members (goldPieces.resolved). Sets (the
// Xo side's lines must be among the recorded ones) are families too;
// lists compared in any order are hashed sorted; the few values a
// comparison reads (why there is no output, the verdict of a run) are
// kept as text (meta).
//
// The recording is testdata/gogoldens (or $XO_SELFHOST_GOLDENS_DIR),
// committed: one directory per kind, its units split into goldShards
// files by the first hex digit of their id, each line
//
//	<kind> <unit id> <piece name> <hash, or the text of a meta piece>
//
// sorted (bytewise), so a recording is the same on every machine and a
// change shows as a diff of the units it touches. The hash is the first
// 64 bits of the SHA-256 of the piece, the unit id the first 64 bits of
// the SHA-256 of the kind, the unit's options, and the content of its
// inputs: a file's path and bytes; a run's directory, the working
// directory, every file of its module (the nearest xo.mod at or above
// it, else the directory alone; under a temporary working directory, the
// whole top level directory of a mutation), and std. The native kinds
// (ssadump, asmdump, llvmdump) also key on the host's GOOS/GOARCH. A
// unit also records its slot (goldSlot: the id without the content of
// the inputs); when the input or std changed since the recording, replay
// compares with the unit recorded for the slot (a stale unit, reported
// at the end, not a failure unless a piece differs). The repository root
// and the test's temporary directory are stored as placeholders (in
// keys, piece names, hashed texts, and meta), so a recording replays in
// any checkout. A piece name or a meta text with a blank or a quote is
// written quoted (strconv.Quote).
//
// Full dumps are not recorded: the selfhost/<component> binary of the
// test with the arguments the hint names prints the Xo side's.

const (
	goldensRecord   = "record"
	goldensRerecord = "rerecord"
	goldensReplay   = "replay"
	goldensRelPath  = "testdata/gogoldens"
	// goldSlotPiece is the meta piece of a unit holding its slot
	// (goldSlot)
	goldSlotPiece = "@slot"
	// goldShards is how many files a kind's units are split into: by
	// the first hex digit of the unit id.
	goldShards = 16
)

// skipDumpReplay skips a test whose only check is the recorded replay of
// a back end or mutation dump (SSA, arm64, LLVM, Go generator, defhash,
// checker mutations and hatches) unless XO_SELFHOST_DUMPS=all or the run
// records (decision 0217): an intended compiler change would otherwise
// need a 25 to 40 minute re-recording. Program output goldens, the fixed
// point, and the front end replays (lexer, parser, format, checker) stay
// on.
func skipDumpReplay(t *testing.T) {
	t.Helper()
	if os.Getenv("XO_SELFHOST_DUMPS") == "all" || goldensRecording() {
		return
	}
	t.Skip("recorded back end dump replay is off by default (decision 0217); XO_SELFHOST_DUMPS=all runs it")
}

// goldensMode is $XO_SELFHOST_GOLDENS (replay when unset).
func goldensMode() string {
	switch v := os.Getenv("XO_SELFHOST_GOLDENS"); v {
	case "", goldensReplay:
		return goldensReplay
	case goldensRecord, goldensRerecord:
		return v
	case "live", "index":
		panic(fmt.Sprintf("XO_SELFHOST_GOLDENS=%s: the Go implementation it ran was deleted (decision 0215); want replay, record, or rerecord", v))
	default:
		panic(fmt.Sprintf("XO_SELFHOST_GOLDENS=%q: want replay or record", v))
	}
}

// goldensRecording reports whether this run records the Xo side as the
// reference.
func goldensRecording() bool {
	m := goldensMode()
	return m == goldensRecord || m == goldensRerecord
}

// goldLine is one line of the recording (without its kind).
type goldLine struct{ id, name, value string }

var goldens struct {
	mu       sync.Mutex
	recorded map[string]map[string][]goldLine // record: kind -> unit id -> lines
	loaded   map[string]map[string][]goldLine // replay: kind -> unit id -> lines
	loadErrs []error
	missing  []string // replay: the units with no recording
	stale    []string // replay: the units compared with their slot's recording
	differ   []string // replay: the pieces whose hash differs, with the hint
	// slots: kind -> slot -> the unit id recorded for it (replay)
	slots map[string]map[string]string
	// pool: kind -> the member hashes of every family of every unit
	// (record: what this run recorded; replay: read)
	pool map[string]map[string]bool
	errs []error
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

// goldSum is the recorded hash of a piece: 64 bits of its SHA-256, hex.
func goldSum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

// goldID is the id of the unit of kind keyed by key.
func goldID(kind string, key []string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00", kind)
	for _, k := range key {
		fmt.Fprintf(h, "%d:%s\x00", len(k), k)
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// goldSlot is the slot of the unit of kind keyed by key: its id without
// std's sum and, for an input of the repository, without the content of
// its files (the 64 hex digit sums of fileKey and dirKey), so the units
// of one input (a path, a run with its options and host) recorded at
// different times share it. A replay whose unit is not recorded (its
// input or std changed) compares with the unit recorded for its slot.
// An input in the test's temporary directory (a mutation, a spec block,
// a fuzz file) keeps its content in the slot: its name says nothing of
// what it holds (a mutation m0027 of another source is another program),
// so only a change of std falls back for it.
func goldSlot(kind string, key []string) string {
	stripped := make([]string, len(key))
	for i, k := range key {
		tmp := strings.Contains(k, "@TMP")
		var keep []string
		for _, f := range strings.Split(k, "\x00") {
			if strings.HasPrefix(f, "std:") || !tmp && hex64.MatchString(f) {
				continue
			}
			keep = append(keep, f)
		}
		stripped[i] = strings.Join(keep, "\x00")
	}
	return goldID(kind+" slot", stripped)
}

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// goldQuote quotes a piece name or a meta text that a line could not
// hold as it is.
func goldQuote(s string, name bool) string {
	if s == "" || strings.HasPrefix(s, `"`) || strings.HasSuffix(s, " ") || !utf8.ValidString(s) ||
		strings.ContainsAny(s, "\n\r\t\\\x00") || name && strings.Contains(s, " ") {
		return strconv.Quote(s)
	}
	return s
}

func formatGoldLine(kind string, l goldLine) string {
	return kind + " " + l.id + " " + goldQuote(l.name, true) + " " + goldQuote(l.value, false) + "\n"
}

func parseGoldLine(s string) (kind string, l goldLine, err error) {
	kind, rest, ok := strings.Cut(s, " ")
	if ok {
		l.id, rest, ok = strings.Cut(rest, " ")
	}
	if !ok {
		return "", l, fmt.Errorf("a line without a name: %q", s)
	}
	if strings.HasPrefix(rest, `"`) {
		q, err := strconv.QuotedPrefix(rest)
		if err != nil {
			return "", l, fmt.Errorf("%q: %v", s, err)
		}
		l.name, _ = strconv.Unquote(q)
		rest, ok = strings.CutPrefix(rest[len(q):], " ")
	} else {
		l.name, rest, ok = strings.Cut(rest, " ")
	}
	if !ok {
		return "", l, fmt.Errorf("a line without a value: %q", s)
	}
	l.value = rest
	if strings.HasPrefix(rest, `"`) {
		if l.value, err = strconv.Unquote(rest); err != nil {
			return "", l, fmt.Errorf("%q: %v", s, err)
		}
	}
	return kind, l, nil
}

// readGoldKind reads every unit of kind in dir: unit id -> lines.
func readGoldKind(dir, kind string) (map[string][]goldLine, error) {
	units := map[string][]goldLine{}
	files, _ := filepath.Glob(filepath.Join(dir, kind, "[0-9a-f].txt"))
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 1<<26)
		for sc.Scan() {
			k, l, err := parseGoldLine(sc.Text())
			if err == nil && k != kind {
				err = fmt.Errorf("a line of kind %s", k)
			}
			if err != nil {
				fh.Close()
				return nil, fmt.Errorf("%s: %v", f, err)
			}
			units[l.id] = append(units[l.id], l)
		}
		err = sc.Err()
		fh.Close()
		if err != nil {
			return nil, err
		}
	}
	return units, nil
}

// writeGoldKind writes the units of kind into dir, one sorted file per
// shard (through a temporary file, so a shard is whole or absent).
func writeGoldKind(dir, kind string, units map[string][]goldLine) error {
	shards := map[string][]string{}
	for _, ls := range units {
		for _, l := range ls {
			s := l.id[:1]
			shards[s] = append(shards[s], formatGoldLine(kind, l))
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, kind), 0o755); err != nil {
		return err
	}
	for s, lines := range shards {
		sort.Strings(lines)
		p := filepath.Join(dir, kind, s+".txt")
		tmp := fmt.Sprintf("%s.%d.tmp", p, os.Getpid())
		if err := os.WriteFile(tmp, []byte(strings.Join(lines, "")), 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, p); err != nil {
			return err
		}
	}
	return nil
}

// readGoldPool reads the member pool of kind in dir.
func readGoldPool(dir, kind string) (map[string]bool, error) {
	pool := map[string]bool{}
	files, _ := filepath.Glob(filepath.Join(dir, kind, "pool-[0-9a-f].txt"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, h := range strings.Fields(string(data)) {
			pool[h] = true
		}
	}
	return pool, nil
}

// writeGoldPool writes the member pool of kind into dir, one sorted file
// per first hex digit.
func writeGoldPool(dir, kind string, pool map[string]bool) error {
	shards := map[string][]string{}
	for h := range pool {
		shards[h[:1]] = append(shards[h[:1]], h+"\n")
	}
	for s, lines := range shards {
		sort.Strings(lines)
		p := filepath.Join(dir, kind, "pool-"+s+".txt")
		tmp := fmt.Sprintf("%s.%d.tmp", p, os.Getpid())
		if err := os.WriteFile(tmp, []byte(strings.Join(lines, "")), 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, p); err != nil {
			return err
		}
	}
	return nil
}

// goldPool is the member pool of kind (replay).
func goldPool(kind string) map[string]bool {
	goldens.mu.Lock()
	defer goldens.mu.Unlock()
	if goldens.pool == nil {
		goldens.pool = map[string]map[string]bool{}
	}
	pool, ok := goldens.pool[kind]
	if !ok {
		var err error
		pool, err = readGoldPool(goldensDir(), kind)
		if err != nil {
			goldens.loadErrs = append(goldens.loadErrs, err)
		}
		goldens.pool[kind] = pool
	}
	return pool
}

// flushGoldens writes what a recording run recorded, merged into the
// recording on disk (a unit recorded again replaces its lines; the pool
// keeps every member hash), and returns the first error (TestMain).
func flushGoldens() error {
	goldens.mu.Lock()
	defer goldens.mu.Unlock()
	if len(goldens.errs) > 0 {
		return fmt.Errorf("%d units not recorded, the first: %v", len(goldens.errs), goldens.errs[0])
	}
	dir := goldensDir()
	var kinds []string
	for k := range goldens.recorded {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		units, err := readGoldKind(dir, kind)
		if err != nil {
			return err
		}
		// a unit recorded again for its slot replaces the slot's
		// earlier unit
		slotOf := map[string]string{}
		for id, ls := range goldens.recorded[kind] {
			if s := lineSlot(ls); s != "" {
				slotOf[s] = id
			}
		}
		for id, ls := range units {
			if nid, ok := slotOf[lineSlot(ls)]; ok && nid != id {
				delete(units, id)
			}
		}
		for id, ls := range goldens.recorded[kind] {
			units[id] = ls
		}
		if err := writeGoldKind(dir, kind, units); err != nil {
			return err
		}
		pool, err := readGoldPool(dir, kind)
		if err != nil {
			return err
		}
		for h := range goldens.pool[kind] {
			pool[h] = true
		}
		if err := writeGoldPool(dir, kind, pool); err != nil {
			return err
		}
	}
	return nil
}

// lineSlot is the slot of a unit's lines ("" when it has none).
func lineSlot(ls []goldLine) string {
	for _, l := range ls {
		if l.name == goldMetaMark+goldSlotPiece {
			return l.value
		}
	}
	return ""
}

// goldensReport is what a replay run reports at its end (TestMain): the
// units with no recording and the pieces that differ, and whether the
// run fails for them.
func goldensReport() (string, bool) {
	goldens.mu.Lock()
	defer goldens.mu.Unlock()
	var b strings.Builder
	for _, err := range goldens.loadErrs {
		fmt.Fprintf(&b, "XO_SELFHOST_GOLDENS=replay: %v\n", err)
	}
	if n := len(goldens.missing); n > 0 {
		fmt.Fprintf(&b, "XO_SELFHOST_GOLDENS=replay: %d units have no recording, not even of an earlier input (%s), the first: %s\n",
			n, recordAdvice, strings.Join(goldens.missing[:min(10, n)], "; "))
	}
	if n := len(goldens.stale); n > 0 {
		fmt.Fprintf(&b, "XO_SELFHOST_GOLDENS=replay: %d units were compared with the recording of an earlier input of theirs (the input or std changed since; not a failure), the first: %s\n",
			n, strings.Join(goldens.stale[:min(5, n)], "; "))
	}
	if n := len(goldens.differ); n > 0 {
		show := 20
		if v := os.Getenv("XO_SELFHOST_FAILS"); v != "" {
			fmt.Sscan(v, &show)
		}
		fmt.Fprintf(&b, "XO_SELFHOST_GOLDENS=replay: %d pieces differ from the recording (if the change is intended, %s):\n", n, recordAdvice)
		for _, d := range goldens.differ[:min(show, n)] {
			b.WriteString("  " + d + "\n")
		}
	}
	return b.String(), len(goldens.loadErrs)+len(goldens.missing) > 0
}

// recordAdvice is how to record a missing unit.
const recordAdvice = "check the Xo output and record it with scripts/selfhost-record-goldens.sh (RUN=<test> for one test)"

// goldSlotUnit is the unit recorded for slot of kind (replay), from the
// slot lines of the recording.
func goldSlotUnit(kind, slot string) (string, bool) {
	goldUnitLines(kind, "") // loads the kind
	goldens.mu.Lock()
	defer goldens.mu.Unlock()
	if goldens.slots == nil {
		goldens.slots = map[string]map[string]string{}
	}
	m, ok := goldens.slots[kind]
	if !ok {
		m = map[string]string{}
		for id, ls := range goldens.loaded[kind] {
			if s := lineSlot(ls); s != "" {
				m[s] = id
			}
		}
		goldens.slots[kind] = m
	}
	id, ok := m[slot]
	return id, ok
}

// goldUnitLines is the recorded lines of unit id of kind (replay).
func goldUnitLines(kind, id string) ([]goldLine, bool) {
	goldens.mu.Lock()
	defer goldens.mu.Unlock()
	if goldens.loaded == nil {
		goldens.loaded = map[string]map[string][]goldLine{}
	}
	units, ok := goldens.loaded[kind]
	if !ok {
		var err error
		units, err = readGoldKind(goldensDir(), kind)
		if err != nil {
			goldens.loadErrs = append(goldens.loadErrs, err)
		}
		goldens.loaded[kind] = units
	}
	ls, ok := units[id]
	return ls, ok
}

// goldPieces is the recorded side of one unit cut into the pieces its
// comparison takes. Built live from the recorded side's output (the texts),
// or read back from the recording (the hashes); the comparisons read it
// through its methods only, which give the same verdicts both ways.
type goldPieces struct {
	kind, id, label string
	// hint tells how to see the unit's full dumps (goldHint)
	hint    string
	cx      goldenCtx
	hashed  bool // replay: parts, lists, and sets hold hashes
	missing bool // replay: the unit is not recorded
	stale   bool // replay: the recording is its slot's, of an earlier input
	meta    map[string]string
	parts   map[string]string   // name -> text, or hash; a member of family f is "<f>:<member>"
	lists   map[string][]string // live: name -> lines (compared sorted)
	listSum map[string]string   // replay: name -> hash of the sorted list
	// fams: live, family -> member -> text (also in parts); roots:
	// replayed, family -> "<members> <root>" until resolved
	// (goldPieces.resolved)
	fams  map[string]map[string]string
	roots map[string]string
}

func newPieces() *goldPieces {
	return &goldPieces{meta: map[string]string{}, parts: map[string]string{}, lists: map[string][]string{},
		listSum: map[string]string{}, fams: map[string]map[string]string{}, roots: map[string]string{}}
}

// setMeta, add, addList, addMember, and addSet build the live pieces.
func (p *goldPieces) setMeta(name, v string) { p.meta[name] = v }
func (p *goldPieces) add(name, text string)  { p.parts[name] = text }
func (p *goldPieces) addList(name string, lines []string) {
	p.lists[name] = append([]string(nil), lines...)
}

// addMember adds member name (a function, helper, file, or declaration)
// of family fam with its text; a family may be empty (addMember with no
// name: addFamily).
func (p *goldPieces) addMember(fam, name, text string) {
	p.addFamily(fam)
	p.fams[fam][name] = text
	p.parts[fam+":"+name] = text
}

func (p *goldPieces) addFamily(fam string) {
	if p.fams[fam] == nil {
		p.fams[fam] = map[string]string{}
	}
}

// addSet adds elements of set name: a family whose members are the
// elements, with no text (inSet).
func (p *goldPieces) addSet(name string, elems ...string) {
	p.addFamily(name)
	for _, e := range elems {
		p.addMember(name, e, "")
	}
}

// Piece names of the recording: a meta piece is "=<name>", a sorted
// list "~<name>", a family "#<name>" (its value "<members> <root>").
const (
	goldMetaMark = "="
	goldListMark = "~"
	goldFamMark  = "#"
)

// memberSum is the pool hash of member name of family fam with text: 48
// bits of the SHA-256 of the three with the placeholders.
func (cx goldenCtx) memberSum(fam, name, text string) string {
	h := sha256.Sum256([]byte(cx.norm(fam) + "\x00" + cx.norm(name) + "\x00" + cx.norm(text)))
	return hex.EncodeToString(h[:6])
}

// famRoot is the recorded value of a family: its number of members and
// the hash of their sorted member hashes.
func famRoot(sums []string) string {
	s := append([]string(nil), sums...)
	sort.Strings(s)
	return strconv.Itoa(len(s)) + " " + goldSum(strings.Join(s, "\n"))
}

// normSortJoin is a list as replay hashes it: each line with the
// placeholders, sorted, joined.
func (cx goldenCtx) normSortJoin(lines []string) string {
	n := make([]string, len(lines))
	for i, l := range lines {
		n[i] = cx.norm(l)
	}
	sort.Strings(n)
	return strings.Join(n, "\n")
}

// recordLines is the live pieces as recorded lines, and the member
// hashes of its families (the pool).
func (p *goldPieces) recordLines() (lines []goldLine, pool []string) {
	cx := p.cx
	add := func(name, value string) { lines = append(lines, goldLine{p.id, name, value}) }
	for k, v := range p.meta {
		add(goldMetaMark+cx.norm(k), cx.norm(v))
	}
	member := map[string]bool{}
	for fam, ms := range p.fams {
		var sums []string
		for name, text := range ms {
			member[fam+":"+name] = true
			sums = append(sums, cx.memberSum(fam, name, text))
		}
		pool = append(pool, sums...)
		add(goldFamMark+cx.norm(fam), famRoot(sums))
	}
	for k, v := range p.parts {
		if !member[k] {
			add(cx.norm(k), goldSum(cx.norm(v)))
		}
	}
	for k, v := range p.lists {
		add(goldListMark+cx.norm(k), goldSum(cx.normSortJoin(v)))
	}
	if len(lines) == 0 {
		add(goldMetaMark, "")
	}
	return lines, pool
}

// piecesFromLines is the replayed pieces of a unit.
func piecesFromLines(p *goldPieces, lines []goldLine) {
	p.hashed = true
	for _, l := range lines {
		switch {
		case l.name == goldMetaMark, l.name == goldMetaMark+goldSlotPiece:
		case strings.HasPrefix(l.name, goldMetaMark):
			p.meta[p.cx.denorm(strings.TrimPrefix(l.name, goldMetaMark))] = p.cx.denorm(l.value)
		case strings.HasPrefix(l.name, goldListMark):
			p.listSum[p.cx.denorm(strings.TrimPrefix(l.name, goldListMark))] = l.value
		case strings.HasPrefix(l.name, goldFamMark):
			p.roots[p.cx.denorm(strings.TrimPrefix(l.name, goldFamMark))] = l.value
		default:
			p.parts[p.cx.denorm(l.name)] = l.value
		}
	}
}

// resolved is p with family fam resolved against the Xo side's members
// (member -> text), for a comparison that reads the family's members
// (names, has, eq, inSet). Live it is p. Replayed, when the Xo side's
// members hash to the recorded root, they are the recorded side's members (the
// same names, and texts equal to the Xo side's); else each Xo member is
// the recorded side's when its hash is in the kind's pool (a member of some
// recorded unit: the Xo side differs elsewhere, or skips a member), and
// differs when not (named by its comparison); the names of unknown (the
// members the Xo side skips) and anonymous ones up to the recorded count
// stand for the recorded side's members it lacks. p itself is not changed (the
// backends compare one Go side in parallel).
func (p *goldPieces) resolved(fam string, xo map[string]string, unknown []string) *goldPieces {
	root, ok := p.roots[fam]
	if !p.hashed || !ok {
		return p
	}
	q := *p
	q.parts = make(map[string]string, len(p.parts)+len(xo))
	for k, v := range p.parts {
		q.parts[k] = v
	}
	q.roots = make(map[string]string, len(p.roots))
	for k, v := range p.roots {
		if k != fam {
			q.roots[k] = v
		}
	}
	cx := p.cx
	sums := make([]string, 0, len(xo))
	for name, text := range xo {
		sums = append(sums, cx.memberSum(fam, name, text))
	}
	same := famRoot(sums) == root
	pool := map[string]bool{}
	if !same {
		pool = goldPool(p.kind)
	}
	for name, text := range xo {
		if same || pool[cx.memberSum(fam, name, text)] {
			q.parts[fam+":"+name] = goldSum(cx.norm(text))
		} else {
			q.parts[fam+":"+name] = "!" // never a hash: differs
		}
	}
	if same {
		return &q
	}
	n, _ := strconv.Atoi(strings.Fields(root)[0])
	have := len(xo)
	for _, name := range unknown {
		if _, ok := xo[name]; !ok && have < n {
			q.parts[fam+":"+name] = "?"
			have++
		}
	}
	for i := 0; have < n; i++ {
		q.parts[fmt.Sprintf("%s:?%d", fam, i)] = "?"
		have++
	}
	return &q
}

// resolvedSet is resolved for set name and the Xo side's elements.
func (p *goldPieces) resolvedSet(name string, xo []string) *goldPieces {
	m := make(map[string]string, len(xo))
	for _, e := range xo {
		m[e] = ""
	}
	return p.resolved(name, m, nil)
}

// recordedPieces is the recorded reference of unit label of kind,
// replayed or (XO_SELFHOST_GOLDENS=record, rerecord) recorded from the
// Xo side:
// key holds the unit's options and input fingerprints (goldenCtx.fileKey,
// goldenCtx.dirKey); compute cuts the Xo side's output into pieces as
// the reference (nil when the test has none to record); hint tells how
// to see the full dump (goldHint).
func recordedPieces(kind, label, hint string, key []string, cx goldenCtx, compute func() *goldPieces) *goldPieces {
	mode := goldensMode()
	id := goldID(kind, key)
	slot := goldSlot(kind, key)
	if v := os.Getenv("XO_SELFHOST_GOLDENS_DEBUG"); v != "" && strings.Contains(label, v) {
		// the key of the units whose label holds the text (why a unit
		// is missing)
		fmt.Fprintf(os.Stderr, "gogoldens %s %s %s (slot %s): %q\n", mode, kind, id, slot, key)
	}
	out := newPieces()
	out.kind, out.id, out.label, out.hint, out.cx = kind, id, label, hint, cx
	if mode == goldensReplay {
		lines, ok := goldUnitLines(kind, id)
		if !ok {
			// the input (or std) changed since the recording: compare
			// with the recording of the slot's earlier input
			if sid, found := goldSlotUnit(kind, slot); found {
				lines, ok = goldUnitLines(kind, sid)
				out.stale = ok
				if ok {
					goldens.mu.Lock()
					goldens.stale = append(goldens.stale, kind+" "+cx.norm(label))
					goldens.mu.Unlock()
				}
			}
		}
		if !ok {
			out.hashed, out.missing = true, true
			goldens.mu.Lock()
			goldens.missing = append(goldens.missing, kind+" "+cx.norm(label))
			goldens.mu.Unlock()
			return out
		}
		piecesFromLines(out, lines)
		return out
	}
	if mode == goldensRecord {
		// record keeps a unit recorded for this very input (the Go
		// recording among them); rerecord replaces it too
		if lines, ok := goldUnitLines(kind, id); ok {
			piecesFromLines(out, lines)
			return out
		}
		// a unit recorded for the slot (the input or std changed):
		// its lines move to the new id, so the reference stays what it
		// was (the Go side's) and the comparison says whether it still
		// holds; rerecord takes the Xo side instead
		if sid, found := goldSlotUnit(kind, slot); found {
			if old, ok := goldUnitLines(kind, sid); ok {
				var lines []goldLine
				for _, l := range old {
					if l.name != goldMetaMark+goldSlotPiece {
						lines = append(lines, goldLine{id, l.name, l.value})
					}
				}
				lines = append(lines, goldLine{id, goldMetaMark + goldSlotPiece, slot})
				goldens.mu.Lock()
				if goldens.recorded == nil {
					goldens.recorded = map[string]map[string][]goldLine{}
				}
				if goldens.recorded[kind] == nil {
					goldens.recorded[kind] = map[string][]goldLine{}
				}
				goldens.recorded[kind][id] = lines
				goldens.mu.Unlock()
				piecesFromLines(out, lines)
				return out
			}
		}
	}
	if compute == nil {
		goldens.mu.Lock()
		goldens.errs = append(goldens.errs, fmt.Errorf("%s %s: the test gives no Xo side to record", kind, cx.norm(label)))
		goldens.mu.Unlock()
		out.hashed, out.missing = true, true
		return out
	}
	live := compute()
	live.id, live.cx = id, cx
	lines, pool := live.recordLines()
	lines = append(lines, goldLine{id, goldMetaMark + goldSlotPiece, slot})
	goldens.mu.Lock()
	if goldens.pool == nil {
		goldens.pool = map[string]map[string]bool{}
	}
	if goldens.pool[kind] == nil {
		goldens.pool[kind] = map[string]bool{}
	}
	for _, h := range pool {
		goldens.pool[kind][h] = true
	}
	if goldens.recorded == nil {
		goldens.recorded = map[string]map[string][]goldLine{}
	}
	if goldens.recorded[kind] == nil {
		goldens.recorded[kind] = map[string][]goldLine{}
	}
	goldens.recorded[kind][id] = lines
	goldens.mu.Unlock()
	// the hashed form, as replayed: the comparison then checks the
	// recording against the output it was made from (and the full tier's
	// SSA stages, IR, and assembly of every run held at once took a
	// recording run to 15 GB, 2026-10-12)
	piecesFromLines(out, lines)
	return out
}

// m is meta piece name ("" when there is none).
func (p *goldPieces) m(name string) string { return p.meta[name] }

// has reports whether the recorded side has piece name.
func (p *goldPieces) has(name string) bool {
	_, ok := p.parts[name]
	return ok
}

// text is piece name's text, when the pieces are live.
func (p *goldPieces) text(name string) (string, bool) {
	if p.hashed {
		return "", false
	}
	t, ok := p.parts[name]
	return t, ok
}

// resolve is the recorded side's piece name for a comparison that reads it
// beyond equality: its text when live; replayed, the Xo side's x when
// the hashes agree, and ok false when they do not.
func (p *goldPieces) resolve(name, x string) (text string, ok bool) {
	if t, live := p.text(name); live {
		return t, true
	}
	if p.eq(name, x) {
		return x, true
	}
	return "", false
}

// eq reports whether the Xo side's x is the recorded side's piece name (a
// piece the recorded side does not have is "").
func (p *goldPieces) eq(name, x string) bool {
	if !p.hashed {
		return p.parts[name] == x
	}
	h, ok := p.parts[name]
	if !ok {
		return x == "" && !p.missing
	}
	return h == goldSum(p.cx.norm(x))
}

// eqList reports whether the Xo side's lines are the recorded side's list name
// in some order.
func (p *goldPieces) eqList(name string, x []string) bool {
	if !p.hashed {
		g, xs := append([]string(nil), p.lists[name]...), append([]string(nil), x...)
		sort.Strings(g)
		sort.Strings(xs)
		return strings.Join(g, "\n") == strings.Join(xs, "\n")
	}
	return !p.missing && p.listSum[name] == goldSum(p.cx.normSortJoin(x))
}

// inSet reports whether x is an element of the recorded side's set name.
func (p *goldPieces) inSet(name, x string) bool {
	v, ok := p.parts[name+":"+x]
	return ok && v != "!"
}

// names is the sorted names of the pieces that start with prefix,
// without it.
func (p *goldPieces) names(prefix string) []string {
	if _, open := p.roots[strings.TrimSuffix(prefix, ":")]; open && p.hashed {
		panic("gogoldens: family " + prefix + " read before it is resolved (goldPieces.resolved)")
	}
	var out []string
	for n := range p.parts {
		if rest, ok := strings.CutPrefix(n, prefix); ok {
			out = append(out, rest)
		}
	}
	sort.Strings(out)
	return out
}

// diff describes how the Xo side's x differs from piece name: the first
// differing line, live; replayed, that the hash differs and how to see
// the dumps (also listed at the end of the run).
func (p *goldPieces) diff(name, x string) string {
	if t, ok := p.text(name); ok || !p.hashed {
		return firstDiff(x, t)
	}
	return p.mismatch(name)
}

// diffList is diff for list name.
func (p *goldPieces) diffList(name string, x []string) string {
	if !p.hashed {
		g, xs := append([]string(nil), p.lists[name]...), append([]string(nil), x...)
		sort.Strings(g)
		sort.Strings(xs)
		return firstDiff(strings.Join(xs, "\n"), strings.Join(g, "\n"))
	}
	return p.mismatch(name)
}

// mismatch records that piece name of a replayed unit differs and says
// so.
func (p *goldPieces) mismatch(name string) string {
	what := fmt.Sprintf("%s %s (unit %s), %s", p.kind, p.label, p.id, name)
	if p.missing {
		return "no recording for " + what
	}
	stale := ""
	if p.stale {
		stale = " of an earlier input (the input or std changed since it was recorded)"
	}
	goldens.mu.Lock()
	goldens.differ = append(goldens.differ, what+stale+": "+p.hint)
	goldens.mu.Unlock()
	return "the hash differs from the recorded output's" + stale + " (" + name + "); the dumps: " + p.hint
}

// missingText is what a comparison reports for a unit with no recording
// (TestMain fails the run for it too).
func (p *goldPieces) missingText() string {
	return fmt.Sprintf("no recording for %s %s (unit %s), not even of an earlier input: %s", p.kind, p.label, p.id, recordAdvice)
}

// goldHint tells how to see the Xo side's full dump of a unit: the
// test's self hosted component with its arguments, from wd.
func goldHint(test, component, xoArgs, wd string) string {
	return fmt.Sprintf("selfhost/%s (`xo build selfhost/%s`, or the binary %s builds) with %s from %s", component, component, test, xoArgs, wd)
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
	k := cx.norm(wd) + "\x00" + cx.norm(dir) + "\x00" + treeSum(goldenTree(cx, wd, full)) + "\x00std:" + stdSum()
	if native {
		k += "\x00" + runtime.GOOS + "/" + runtime.GOARCH
	}
	return k
}

// stdSum fingerprints the repository's std tree (every file), once per
// test binary: every run reads it, so a change there (2026-10-12: the
// runtime port rewrote std text code) makes the runs' units missing,
// to record again, rather than differing.
var stdSum = sync.OnceValue(func() string {
	return treeSum(filepath.Join(filepath.Dir(filepath.Dir(pkgDir)), "std"))
})

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
	if goldensRecording() {
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
	if goldensRecording() {
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
