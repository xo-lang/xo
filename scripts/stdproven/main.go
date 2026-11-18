// Command stdproven runs the testing evidence of decision 0616 for one std
// module and writes it to std/evidence/MODULE/KIND.md:
//
//	go run ./scripts/stdproven hash MODULE
//	go run ./scripts/stdproven fuzz [-cases N] [-hours H] [-batch N] [-backends B,...] MODULE
//	go run ./scripts/stdproven timing [-n N] [-backends B,...] -pairs A:B,... MODULE NAME
//	go run ./scripts/stdproven race [-backend B] [-host LABEL] MODULE
//
// race (decision 0616 section 2) runs the module's own tests
// (std/native/MODULE_test.xo) and every corpus directory of
// internal/stdtest/testdata/corpus named MODULE or starting with it, each
// with xo test --race, and writes std/evidence/MODULE/race.md when none
// fails or reports a data race. (The stress program under --race joins it
// when the module has one.)
//
// timing (decision 0616 checklist item 3) builds the dudect program
// std/evidence/MODULE/timing/NAME/ on each backend and runs it once per
// pair of input classes with -n measurements (default 1,000,000); the
// program prints a last line "A vs B: N measurements, max |t| T, pass".
// It writes std/evidence/MODULE/timing-NAME.md when every |t| is below
// 4.5, and fails with the program's output otherwise.
//
// hash prints the module's source hash (decision 0616 section 3) and the
// files it covers. fuzz runs every fuzz harness that names the module with
// fresh seeds, a batch of cases a seed, until each backend has run -cases
// cases (default 1,000,000) or -hours have passed (default 8), whichever
// comes first, and stops at the first failing batch. A passing run writes
// std/evidence/MODULE/fuzz.md (its header first, then the batches); a
// failing one writes nothing there and prints the seed and the failing
// output, for a fix with a regression case. A later run starts past the
// seeds the module's fuzz.md records.
//
// It runs where it is started (an offload host for the long runs: decision
// 0616 section 4; scripts/lead/stdproven-offload.sh sends it to the
// linux/arm64 pass/fail host outside the lane, as soaks run). It uses the
// checkout's xo as the Go tests do (internal/xotool).
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	"time"
)

// harness is one fuzz test of internal/stdtest: its test name and the
// prefix of its XO_<NAME>_SEED, _N and _BACKENDS variables.
type harness struct {
	test, env string
}

// harnesses lists, per module, the fuzz tests that name it. A module with
// none has no fuzz evidence to run (decision 0461: a parser of untrusted
// input gets a harness first, its Solid requirement).
var harnesses = map[string][]harness{
	"compress":     {{"TestCompressFuzz", "XO_COMPRESSFUZZ"}, {"TestLzwFuzz", "XO_LZWFUZZ"}},
	"crypto":       {{"TestRsaFuzz", "XO_RSAFUZZ"}},
	"hpke":         {{"TestHpkeFuzz", "XO_HPKEFUZZ"}},
	"httpproxy":    {{"TestHttpproxyFuzz", "XO_HTTPPROXYFUZZ"}},
	"mail":         {{"TestMailFuzz", "XO_MAILFUZZ"}},
	"mime":         {{"TestMimeFuzz", "XO_MIMEFUZZ"}},
	"protojson":    {{"TestProtojsonFuzz", "XO_PROTOJSONFUZZ"}},
	"otlp":         {{"TestOtlpFuzz", "XO_OTLPFUZZ"}},
	"jwt":          {{"TestJwtFuzz", "XO_JWTFUZZ"}},
	"bcrypt":       {{"TestBcryptFuzz", "XO_BCRYPTFUZZ"}},
	"asn1":         {{"TestAsn1Fuzz", "XO_ASN1FUZZ"}},
	"cookiejar":    {{"TestCookiejarFuzz", "XO_COOKIEJARFUZZ"}},
	"sql":          {{"TestMysqlFuzz", "XO_MYSQLFUZZ"}, {"TestPgFuzz", "XO_PGFUZZ"}},
	"websocket":    {{"TestWsFuzz", "XO_WSFUZZ"}},
	"dns":          {{"TestDnsFuzz", "XO_DNSFUZZ"}},
	"tls":          {{"TestTlsFuzz", "XO_TLSFUZZ"}, {"TestPemFuzz", "XO_PEMFUZZ"}},
	"sse":          {{"TestSseFuzz", "XO_SSEFUZZ"}},
	"json":         {{"TestJsonFuzz", "XO_JSONFUZZ"}},
	"otp":          {{"TestOtpFuzz", "XO_OTPFUZZ"}},
	"jsonrpc":      {{"TestJsonrpcFuzz", "XO_JSONRPCFUZZ"}},
	"grpc":         {{"TestGrpcFuzz", "XO_GRPCFUZZ"}},
	"amqp":         {{"TestAmqpFuzz", "XO_AMQPFUZZ"}},
	"time":         {{"TestTimeFuzz", "XO_TIMEFUZZ"}},
	"multipart":    {{"TestMultipartFuzz", "XO_MULTIPARTFUZZ"}},
	"http":         {{"TestHttpHeadFuzz", "XO_HEADFUZZ"}, {"TestHttpResponseFuzz", "XO_RESPFUZZ"}, {"TestHttpUrlFuzz", "XO_URLFUZZ"}},
	"x509":         {{"TestX509Fuzz", "XO_X509FUZZ"}},
	"url":          {{"TestUrlFuzz", "XO_URLQFUZZ"}, {"TestUrlTypeFuzz", "XO_URLTFUZZ"}},
	"html":         {{"TestHtmlFuzz", "XO_HTMLFUZZ"}},
	"grpcreflect":  {{"TestGrpcReflectFuzz", "XO_GRPCREFLECTFUZZ"}},
	"fnv":          {{"TestFnvFuzz", "XO_FNVFUZZ"}},
	"uuid":         {{"TestUuidFuzz", "XO_UUIDFUZZ"}},
	"norm":         {{"TestNormFuzz", "XO_NORMFUZZ"}},
	"idna":         {{"TestIdnaFuzz", "XO_IDNAFUZZ"}},
	"yaml":         {{"TestYamlFuzz", "XO_YAMLFUZZ"}},
	"jpeg":         {{"TestJpegFuzz", "XO_JPEGFUZZ"}, {"TestJpegEncFuzz", "XO_JPEGENC"}},
	"base32":       {{"TestBase32Fuzz", "XO_BASE32FUZZ"}},
	"base64":       {{"TestBase64Fuzz", "XO_BASE64FUZZ"}},
	"crc32":        {{"TestCrc32Fuzz", "XO_CRC32FUZZ"}},
	"adler32":      {{"TestAdler32Fuzz", "XO_ADLER32FUZZ"}},
	"crc64":        {{"TestCrc64Fuzz", "XO_CRC64FUZZ"}},
	"csv":          {{"TestCsvFuzz", "XO_CSVFUZZ"}},
	"png":          {{"TestPngFuzz", "XO_PNGFUZZ"}},
	"gif":          {{"TestGifFuzz", "XO_GIFFUZZ"}},
	"binary":       {{"TestBinaryFuzz", "XO_BINARYFUZZ"}},
	"path":         {{"TestPathFuzz", "XO_PATHFUZZ"}},
	"htmlent":      {{"TestHtmlentFuzz", "XO_HTMLENTFUZZ"}},
	"uniclass":     {{"TestUniclassFuzz", "XO_UNICLASSFUZZ"}},
	"cron":         {{"TestCronFuzz", "XO_CRONFUZZ"}},
	"heap":         {{"TestHeapFuzz", "XO_HEAPFUZZ"}},
	"deque":        {{"TestDequeFuzz", "XO_DEQUEFUZZ"}},
	"bufio":        {{"TestBufioFuzz", "XO_BUFIOFUZZ"}},
	"rate":         {{"TestRateFuzz", "XO_RATEFUZZ"}},
	"cli":          {{"TestCliFuzz", "XO_CLIFUZZ"}},
	"toml":         {{"TestTomlFuzz", "XO_TOMLFUZZ"}},
	"regexp":       {{"TestRegexpFuzz", "XO_REGEXPFUZZ"}},
	"template":     {{"TestTemplateFuzz", "XO_TEMPLATEFUZZ"}},
	"xmlcodec":     {{"TestXmlcodecFuzz", "XO_XMLCODECFUZZ"}},
	"log":          {{"TestLogFuzz", "XO_LOGFUZZ"}},
	"tabwriter":    {{"TestTabwriterFuzz", "XO_TABWRITERFUZZ"}},
	"textproto":    {{"TestTextprotoFuzz", "XO_TEXTPROTOFUZZ"}},
	"archive":      {{"TestArchiveFuzz", "XO_ARCHIVEFUZZ"}},
	"timego":       {{"TestTimegoFuzz", "XO_TIMEGOFUZZ"}},
	"xml":          {{"TestXmlFuzz", "XO_XMLFUZZ"}},
	"big":          {{"TestBigFuzz", "XO_BIGFUZZ"}},
	"image":        {{"TestImageFuzz", "XO_IMAGEFUZZ"}},
	"brotli":       {{"TestBrotliFuzz", "XO_BROTLIFUZZ"}},
	"cbor":         {{"TestCborFuzz", "XO_CBORFUZZ"}},
	"charset":      {{"TestCharsetFuzz", "XO_CHARSETFUZZ"}},
	"htmlparse":    {{"TestHtmlparseFuzz", "XO_HTMLPARSEFUZZ"}},
	"htmltok":      {{"TestHtmltokFuzz", "XO_HTMLTOKFUZZ"}},
	"publicsuffix": {{"TestPublicsuffixFuzz", "XO_PUBLICSUFFIXFUZZ"}},
	"random":       {{"TestRandomFuzz", "XO_RANDOMFUZZ"}},
	"protodesc":    {{"TestProtodescFuzz", "XO_PROTODESCFUZZ"}},
	"protoparse":   {{"TestProtoparseFuzz", "XO_PROTOPARSEFUZZ"}},
	"migrate":      {{"TestMigrateFuzz", "XO_MIGRATEFUZZ"}},
	"testing":      {{"TestFakenetFuzz", "XO_FAKENETFUZZ"}},
	"protowkt":     {{"TestProtowktFuzz", "XO_PROTOWKTFUZZ"}},
	"prototext":    {{"TestPrototextFuzz", "XO_PROTOTEXTFUZZ"}},
	"metrics":      {{"TestMetricsFuzz", "XO_METRICSFUZZ"}},
	"embed":        {{"TestEmbedFuzz", "XO_EMBEDFUZZ"}},
	"backoff":      {{"TestBackoffFuzz", "XO_BACKOFFFUZZ"}},
	"protowire":    {{"TestProtowireFuzz", "XO_PROTOWIREFUZZ"}},
	"smtp":         {{"TestSmtpFuzz", "XO_SMTPFUZZ"}},
	"trace":        {{"TestTraceFuzz", "XO_TRACEFUZZ"}},
	"netip":        {{"TestNetipFuzz", "XO_NETIPFUZZ"}},
	"amqptest":     {{"TestAmqptestFuzz", "XO_AMQPTFUZZ"}},
	"protobuf":     {{"TestProtobufFuzz", "XO_PROTOFUZZ"}},
	"udp":          {{"TestUdpFuzz", "XO_UDPFUZZ"}},
	"term":         {{"TestTermFuzz", "XO_TERMFUZZ"}},
	"sqlite":       {{"TestSqliteFuzz", "XO_SQLITEFUZZ"}},
}

// fixedCases lists the harnesses whose seed makes a fixed number of cases
// and reads no _N knob: a seed of one counts these, not -batch, so the
// evidence records the cases that ran.
var fixedCases = map[string]int{
	// scripts/stdcorpus/protobuf.go: ten generators of their own sizes
	"TestProtobufFuzz": 8942,
}

// seedCases is the number of cases one seed of h runs at -batch batch.
func seedCases(h harness, batch int) int {
	if n := fixedCases[h.test]; n > 0 {
		return n
	}
	return batch
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "hash":
		if len(os.Args) != 3 {
			usage()
		}
		var h string
		var files []string
		h, files, err = sourceHash(os.Args[2])
		if err == nil {
			fmt.Println(h)
			for _, f := range files {
				fmt.Println("  " + f)
			}
		}
	case "fuzz":
		err = fuzz(os.Args[2:])
	case "timing":
		err = timing(os.Args[2:])
	case "race":
		err = race(os.Args[2:])
	case "stress":
		err = stress(os.Args[2:])
	case "perf":
		err = perf(os.Args[2:])
	case "serverperf":
		err = serverPerf(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdproven:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: stdproven hash MODULE | stdproven fuzz [-cases N] [-hours H] [-batch N] [-backends B,...] MODULE | stdproven timing [-n N] [-backends B,...] -pairs A:B,... MODULE NAME | stdproven race [-backend B] [-stress-minutes M] MODULE | stdproven stress [-minutes M] [-tasks N] [-backends B,...] [-check-minutes M] [-check-only] MODULE | stdproven perf [-rounds N] MODULE | stdproven serverperf [-reps N] MODULE")
	os.Exit(2)
}

var useLine = regexp.MustCompile(`^use\s+([a-z0-9_/]+)`)

// sourceHash is the SHA-256 over the paths and contents, sorted by path, of
// the module's std/MODULE.xo, std/native/MODULE.xo and
// std/native/MODULE_native.xo (the native bodies of std/MODULE.xo's
// bodyless functions, as url's escape) and of every std file they reach
// through use lines (std/X, native/X, native/llrt/X, each with its
// _native.xo), tests left out.
// runtimeEntries and runtimeCore are where the source hash stops
// (decision 0616 section 3, amended 2026-11-14): the runtime every module
// runs on (the entries, the core std/native/llrt files and every file they
// reach, and the OS layer's llrt sys_* files). Its changes are covered by
// its own tests, the fixed point and the full tier, not by every module's
// evidence. Module specific llrt files (tzrules, jsontext, flate, sha256)
// are still followed.
var (
	runtimeEntries = []string{"std/native/rt.xo", "std/native/internal.xo", "std/prelude.xo", "std/native/prelude.xo"}
	runtimeCore    = []string{"alloc", "obj", "rc", "pool", "sched", "tasks", "poll", "sysio", "layout", "raw"}
)

// useFiles is the files a `use` path names that exist.
func useFiles(u string) []string {
	var c []string
	switch {
	case strings.HasPrefix(u, "std/"):
		c = []string{filepath.Join("std", u[4:]+".xo"), filepath.Join("std", "native", u[4:]+".xo"), filepath.Join("std", "native", u[4:]+"_native.xo")}
	case strings.HasPrefix(u, "native/"):
		c = []string{filepath.Join("std", u+".xo")}
	}
	return c
}

// usesOf is the use paths a file names.
func usesOf(p string) ([]string, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var us []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if m := useLine.FindStringSubmatch(sc.Text()); m != nil {
			us = append(us, m[1])
		}
	}
	return us, sc.Err()
}

// closure walks use lines from seeds, skipping tests, missing files and
// any file stop reports.
func closure(seeds []string, stop func(string) bool) (map[string]bool, error) {
	seen := map[string]bool{}
	var queue []string
	add := func(p string) {
		if seen[p] || strings.HasSuffix(p, "_test.xo") || stop(p) {
			return
		}
		if _, err := os.Stat(p); err != nil {
			return
		}
		seen[p] = true
		queue = append(queue, p)
	}
	for _, s := range seeds {
		add(s)
	}
	for i := 0; i < len(queue); i++ {
		us, err := usesOf(queue[i])
		if err != nil {
			return nil, err
		}
		for _, u := range us {
			for _, f := range useFiles(u) {
				add(f)
			}
		}
	}
	return seen, nil
}

// runtimeFiles is the set the source hash stops at.
func runtimeFiles() (map[string]bool, error) {
	var seeds []string
	for _, c := range runtimeCore {
		seeds = append(seeds, filepath.Join("std", "native", "llrt", c+".xo"))
	}
	rt, err := closure(seeds, func(string) bool { return false })
	if err != nil {
		return nil, err
	}
	for _, e := range runtimeEntries {
		rt[filepath.FromSlash(e)] = true
	}
	return rt, nil
}

func sourceHash(module string) (string, []string, error) {
	rt, err := runtimeFiles()
	if err != nil {
		return "", nil, err
	}
	stop := func(p string) bool {
		return rt[p] || (filepath.Dir(p) == filepath.Join("std", "native", "llrt") && strings.HasPrefix(filepath.Base(p), "sys_"))
	}
	seen, err := closure(useFiles("std/"+module), stop)
	if err != nil {
		return "", nil, err
	}
	if module == "prelude" {
		// the prelude is the runtime's entry: its hash is the runtime the
		// others stop at (std/prelude.xo, std/native/prelude.xo, rt.xo and
		// the llrt core), so any runtime change dates its evidence
		for p := range rt {
			if _, err := os.Stat(p); err == nil {
				seen[p] = true
			}
		}
	}
	if len(seen) == 0 {
		return "", nil, fmt.Errorf("no std/%s.xo or std/native/%s.xo (run from the repository root)", module, module)
	}
	files := make([]string, 0, len(seen))
	for p := range seen {
		files = append(files, p)
	}
	sort.Strings(files)
	h := sha256.New()
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", nil, err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(p), len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), files, nil
}

// header is an evidence file's parsed first lines (key: value).
type header map[string]string

func readHeader(path string) (header, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	h := header{}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) == "" {
			break
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			break
		}
		h[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return h, nil
}

// fuzz runs the long fuzz of decision 0616 section 2 for one module.
// killedUnchecked reports a fuzz seed's go test output whose xo test was
// killed by a signal with no case passed or failed (runFuzzDir's line
// "xo test --backend=B: signal: killed; 0 passed, 0 failed").
func killedUnchecked(out string) bool {
	return strings.Contains(out, ": signal: killed; 0 passed, 0 failed")
}

// notRun names why a harness's go test -v output that exited 0 checked no
// case, or is "": a skipped harness (yaml's, when its oracle cannot run) or
// a -run that matched nothing passes as ok and was counted as cases
// (2026-11-16).
func notRun(test, out string) string {
	switch {
	case strings.Contains(out, "--- SKIP: "+test):
		return "skipped"
	case strings.Contains(out, "no tests to run") || !strings.Contains(out, "--- PASS: "+test):
		return "did not run"
	}
	return ""
}

func fuzz(args []string) error {
	fs := flag.NewFlagSet("fuzz", flag.ExitOnError)
	cases := fs.Int("cases", 1000000, "cases per backend to reach")
	hours := fs.Float64("hours", 8, "time limit, over every shard")
	minutes := fs.Float64("minutes", 0, "this shard's time limit (0: none): stop at a seed boundary and write result: partial, which the next run resumes")
	batch := fs.Int("batch", 5000, "cases a seed")
	backends := fs.String("backends", defaultBackends(runtime.GOARCH), "backends, comma separated (default: this host's native backend and llvm)")
	hostLabel := fs.String("host", "", "the host's name in the evidence (no network names: e.g. \"linux/arm64 host 2\")")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	module := fs.Arg(0)
	hs := harnesses[module]
	if len(hs) == 0 {
		return fmt.Errorf("no fuzz harness names %s (scripts/stdproven harnesses)", module)
	}
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	if dirty, _ := gitOut("status", "--porcelain", "--", "std", ":(exclude)std/evidence", "compiler", "internal/stdtest"); dirty != "" {
		return fmt.Errorf("std, compiler or internal/stdtest has uncommitted changes: evidence names a commit:\n%s", dirty)
	}
	out := filepath.Join("std", "evidence", module, "fuzz.md")
	prev, _ := os.ReadFile(out)
	st := fuzzResume(string(prev), src)
	if st.shards > 0 {
		fmt.Printf("stdproven fuzz: resuming %s from seed %d after %d shard(s), %s\n", module, st.first, st.shards, st.prior.Round(time.Second))
	}
	f := fuzzRun{hs: hs, cases: *cases, batch: *batch, backends: *backends,
		limit: time.Duration(*hours * float64(time.Hour)), shard: time.Duration(*minutes * float64(time.Minute)), run: goTestSeed}
	if err := f.loop(&st); err != nil {
		return err
	}
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, []byte(f.evidence(st, module, commit, src, host, *hours, *hostLabel, time.Now())), 0o644); err != nil {
		return err
	}
	if !st.pass {
		fmt.Printf("stdproven fuzz: %s partial at seed %d (%s of %s): run again to resume\n", module, st.seed-1, st.prior.Round(time.Second), f.limit)
	}
	return nil
}

// fuzzState is a fuzz's progress: what earlier shards did (fuzzResume) and
// what this one adds (fuzzRun.loop).
type fuzzState struct {
	first, seed int            // the first seed of the whole run, the next to run
	ran         map[string]int // cases per backend by harness test
	prior       time.Duration  // the run's time so far, every shard
	log         []string       // a line per seed
	shards      int            // shards before this one
	pass        bool           // every harness reached its cases, or the run its time
}

// fuzzResume reads an earlier fuzz.md. A `result: partial` file of the
// same source resumes: its counts, time, seed lines and shards carry on. Any
// other file only moves the first seed past its last (seeds stay fresh);
// none starts at 2, seed 1 being each harness's committed corpus.
func fuzzResume(prev, src string) fuzzState {
	st := fuzzState{first: 2, ran: map[string]int{}}
	h := header{}
	body := ""
	for i, l := range strings.Split(prev, "\n") {
		k, v, ok := strings.Cut(l, ":")
		if strings.TrimSpace(l) == "" || !ok {
			body = strings.Join(strings.Split(prev, "\n")[i:], "\n")
			break
		}
		h[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	a, b, ok := strings.Cut(h["seeds"], " to ")
	if !ok {
		st.seed = st.first
		return st
	}
	last, err := strconv.Atoi(b)
	if err != nil {
		st.seed = st.first
		return st
	}
	st.first, st.seed = last+1, last+1
	if h["result"] != "partial" || h["source"] != src {
		return st
	}
	if n, err := strconv.Atoi(a); err == nil {
		st.first = n
	}
	for _, kv := range strings.Split(h["ran"], ",") {
		if t, n, ok := strings.Cut(strings.TrimSpace(kv), "="); ok {
			if c, err := strconv.Atoi(n); err == nil {
				st.ran[t] = c
			}
		}
	}
	if s, err := strconv.Atoi(h["elapsed"]); err == nil {
		st.prior = time.Duration(s) * time.Second
	}
	st.shards, _ = strconv.Atoi(h["shards"])
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, "- ") {
			st.log = append(st.log, l)
		}
	}
	return st
}

// fuzzRun runs seeds of every harness that has not reached its cases.
type fuzzRun struct {
	hs           []harness
	cases, batch int
	backends     string
	limit, shard time.Duration // the whole run's time, this shard's (0: none)
	run          func(h harness, seed, batch int, backends string) ([]byte, error)
	now          func() time.Time
}

func goTestSeed(h harness, seed, batch int, backends string) ([]byte, error) {
	cmd := exec.Command("go", "test", "./internal/stdtest", "-run", "^"+h.test+"$", "-count=1", "-v", "-timeout", "2h")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s_SEED=%d", h.env, seed),
		fmt.Sprintf("%s_N=%d", h.env, batch),
		fmt.Sprintf("%s_BACKENDS=%s", h.env, backends))
	return cmd.CombinedOutput()
}

// loop runs seeds from st.seed until every harness has its cases (pass), the
// whole run its time (pass, as decision 0616 counts 8 hours), or this shard
// its time (partial); st.prior ends as the run's time over every shard.
func (f fuzzRun) loop(st *fuzzState) error {
	now := f.now
	if now == nil {
		now = time.Now
	}
	start := now()
	// the longest seed round so far: a shard does not start a round that
	// would end past its minutes (#320: a jsonrpc shard of 55 minutes began
	// a round near its end and ran 1h25m, past its lease's -max)
	var longest time.Duration
	for {
		done := true
		for _, h := range f.hs {
			if st.ran[h.test] < f.cases {
				done = false
			}
		}
		el := now().Sub(start)
		if done || st.prior+el >= f.limit {
			st.pass = true
			break
		}
		if f.shard > 0 && (el >= f.shard || el+longest > f.shard) {
			break
		}
		r0 := now()
		for _, h := range f.hs {
			if st.ran[h.test] >= f.cases {
				continue
			}
			t0 := now()
			b, err := f.run(h, st.seed, f.batch, f.backends)
			if err != nil && killedUnchecked(string(b)) {
				// the host killed the seed's test program before it checked a
				// case (idna seeds 40 and 33 on host 2, 2026-11-14 and 15: each
				// passed alone at 3 GB): run the seed once more, and say so in
				// the evidence; a mismatch or a second kill still fails
				line := fmt.Sprintf("- %s seed %d: killed before any case was checked (the host); run again", h.test, st.seed)
				st.log = append(st.log, line)
				fmt.Println(line)
				b, err = f.run(h, st.seed, f.batch, f.backends)
			}
			if why := notRun(h.test, string(b)); err == nil && why != "" {
				err = fmt.Errorf("%s %s", h.test, why)
			}
			if err != nil {
				tail := string(b)
				if len(tail) > 20000 {
					tail = tail[len(tail)-20000:]
				}
				return fmt.Errorf("%s seed %d (%d cases on %s) failed after %d cases: %v\n%s", h.test, st.seed, seedCases(h, f.batch), f.backends, st.ran[h.test], err, tail)
			}
			st.ran[h.test] += seedCases(h, f.batch)
			line := fmt.Sprintf("- %s seed %d: %d cases on %s, %s", h.test, st.seed, seedCases(h, f.batch), f.backends, now().Sub(t0).Round(time.Second))
			st.log = append(st.log, line)
			fmt.Println(line)
		}
		if d := now().Sub(r0); d > longest {
			longest = d
		}
		st.seed++
	}
	st.prior += now().Sub(start)
	return nil
}

// evidence is the fuzz.md text: result pass, or partial with what the next
// shard resumes (ran, elapsed, shards).
func (f fuzzRun) evidence(st fuzzState, module, commit, src, host string, hours float64, hostLabel string, at time.Time) string {
	nb := len(strings.Split(f.backends, ","))
	var meas, ran []string
	for _, h := range f.hs {
		meas = append(meas, fmt.Sprintf("%s %d cases per backend", h.test, st.ran[h.test]))
		ran = append(ran, fmt.Sprintf("%s=%d", h.test, st.ran[h.test]))
	}
	result := "pass"
	if !st.pass {
		result = "partial"
	}
	shards := ""
	if st.shards > 0 || !st.pass {
		shards = fmt.Sprintf(" in %d shards", st.shards+1)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kind: fuzz\nmodule: %s\nresult: %s\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, result, commit, src, at.UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven fuzz -cases %d -hours %g -batch %d -backends %s -host %q %s\n", f.cases, hours, f.batch, f.backends, hostLabel, module)
	fmt.Fprintf(&b, "measure: %s, %d backends (%s), %s%s, 0 mismatches\n", strings.Join(meas, "; "), nb, f.backends, st.prior.Round(time.Second), shards)
	fmt.Fprintf(&b, "seeds: %d to %d\n", st.first, st.seed-1)
	if !st.pass || st.shards > 0 {
		fmt.Fprintf(&b, "ran: %s\nelapsed: %d\nshards: %d\n", strings.Join(ran, ","), int(st.prior.Seconds()), st.shards+1)
	}
	fmt.Fprintf(&b, "\nLong fuzz evidence (decision 0616): every harness that names std/%s, against Go's package, with fresh seeds of %d cases each.\n\n", module, f.batch)
	b.WriteString(strings.Join(st.log, "\n") + "\n")
	return b.String()
}

func gitOut(args ...string) (string, error) {
	b, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(b)), err
}

// timing runs a dudect program of decision 0616's checklist item 3.
func timing(args []string) error {
	fs := flag.NewFlagSet("timing", flag.ExitOnError)
	n := fs.Int("n", 1000000, "measurements per pair")
	backends := fs.String("backends", defaultBackends(runtime.GOARCH), "backends, comma separated (default: this host's native backend and llvm)")
	pairs := fs.String("pairs", "", "class pairs A:B, comma separated")
	hostLabel := fs.String("host", "", "the host's name in the evidence")
	fs.Parse(args)
	if fs.NArg() != 2 || *pairs == "" {
		usage()
	}
	module, name := fs.Arg(0), fs.Arg(1)
	dir := filepath.Join("std", "evidence", module, "timing", name)
	if _, err := os.Stat(filepath.Join(dir, "main.xo")); err != nil {
		return err
	}
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	if dirty, _ := gitOut("status", "--porcelain", "--", "std", ":(exclude)std/evidence", "compiler"); dirty != "" {
		return fmt.Errorf("std or compiler has uncommitted changes: evidence names a commit:\n%s", dirty)
	}
	tmp, err := os.MkdirTemp("", "stdproven-timing-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	start := time.Now()
	var lines []string
	worst := 0.0
	for _, b := range strings.Split(*backends, ",") {
		bin := filepath.Join(tmp, "t-"+b)
		if out, err := exec.Command(filepath.Join("bin", "xo"), "build", "--backend="+b, "-o", bin, dir).CombinedOutput(); err != nil {
			return fmt.Errorf("xo build --backend=%s %s: %v\n%s", b, dir, err, out)
		}
		for _, p := range strings.Split(*pairs, ",") {
			a, c, ok := strings.Cut(p, ":")
			if !ok {
				usage()
			}
			out, err := exec.Command(bin, a, c, strconv.Itoa(*n)).CombinedOutput()
			text := strings.TrimSpace(string(out))
			last := text[strings.LastIndex(text, "\n")+1:]
			fmt.Printf("%s: %s\n", b, last)
			if err != nil || !strings.HasSuffix(last, ", pass") {
				return fmt.Errorf("%s %s: %v\n%s", b, p, err, text)
			}
			if i := strings.Index(last, "max |t| "); i >= 0 {
				f := strings.Fields(last[i+len("max |t| "):])
				if len(f) > 0 {
					if t, err := strconv.ParseFloat(strings.TrimSuffix(f[0], ","), 64); err == nil && t > worst {
						worst = t
					}
				}
			}
			lines = append(lines, "- "+b+": "+last)
		}
	}
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kind: timing\nmodule: %s\nresult: pass\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, commit, src, time.Now().UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven timing -n %d -backends %s -pairs %s -host %q %s %s\n", *n, *backends, *pairs, host, module, name)
	fmt.Fprintf(&b, "measure: %d pairs on %s, %d measurements each, max |t| %.3f (pass below 4.5), %s\n\n", len(lines), *backends, *n, worst, time.Since(start).Round(time.Second))
	fmt.Fprintf(&b, "Timing evidence (decision 0616 checklist item 3): the dudect program %s, Welch's t over the times of two input classes, uncropped and cropped at several percentiles.\n\n", dir)
	b.WriteString(strings.Join(lines, "\n") + "\n")
	return os.WriteFile(filepath.Join("std", "evidence", module, "timing-"+name+".md"), []byte(b.String()), 0o644)
}

// raceTestArgs: xo test's arguments for one race run. A race build runs
// several times slower, so a test gets timeout (xo test --timeout) instead
// of the 60 s default.
func raceTestArgs(backend, timeout string, r raceRun) []string {
	args := []string{"test", "--race", "--backend=" + backend, "--timeout=" + timeout}
	if r.real {
		args = append(args, "--real")
	}
	return append(args, r.target)
}

// race runs decision 0616's race kind for one module.
func race(args []string) error {
	fs := flag.NewFlagSet("race", flag.ExitOnError)
	// llvm: no ThreadSanitizer runtime ships for the native backend on
	// linux/arm64 (xo: "--race: no ThreadSanitizer runtime for linux/arm64")
	backend := fs.String("backend", "llvm", "the backend to run --race on")
	stressMin := fs.Int("stress-minutes", 10, "minutes the stress program runs under --race (0 skips it)")
	churnRounds := fs.Int("churn-rounds", 50, "rounds of 200000 tasks of the task churn against Go's race build (0 skips it; decision 0616 section 2a)")
	hostLabel := fs.String("host", "", "the host's name in the evidence")
	stockRT := fs.Bool("stock-runtime", false, "allow clang's own ThreadSanitizer runtime when lib/xo/race has none (not the runtime users get; race.md says so)")
	testTimeout := fs.String("test-timeout", "10m", "the real time each module or corpus test may take under --race (xo test --timeout; its 60s default stopped brotli's quality 0 to 11 round trip on host 4, 2026-11-17)")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	module := fs.Arg(0)
	rtLabel, err := raceRuntimeLabel(*stockRT)
	if err != nil {
		return err
	}
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	if dirty, _ := gitOut("status", "--porcelain", "--", "std", ":(exclude)std/evidence", "compiler", "internal/stdtest"); dirty != "" {
		return fmt.Errorf("std, compiler or internal/stdtest has uncommitted changes: evidence names a commit:\n%s", dirty)
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	xo := filepath.Join(root, "bin", "xo")
	runs := raceRuns(root, module)
	stressOnly, err := raceStressOnly(root, module, len(runs), *stressMin)
	if err != nil {
		return err
	}
	start := time.Now()
	var lines []string
	for _, r := range runs {
		t0 := time.Now()
		cmd := exec.Command(xo, raceTestArgs(*backend, *testTimeout, r)...)
		cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		out, err := cmd.CombinedOutput()
		text := string(out)
		if err != nil || strings.Contains(text, "DATA RACE") {
			if len(text) > 20000 {
				text = text[len(text)-20000:]
			}
			return fmt.Errorf("%s: xo test --race: %v\n%s", r.name, err, text)
		}
		sum := strings.TrimSpace(text)
		sum = sum[strings.LastIndex(sum, "\n")+1:]
		line := fmt.Sprintf("- %s: %s, %s", r.name, sum, time.Since(t0).Round(time.Second))
		fmt.Println(line)
		lines = append(lines, line)
	}
	// decision 0616: the stress program of the stress kind, also under
	// --race, for -stress-minutes
	if dir := filepath.Join("std", "evidence", module, "stress"); *stressMin > 0 {
		if _, err := os.Stat(filepath.Join(dir, "main.xo")); err == nil {
			tmp, err := os.MkdirTemp("", "stdproven-race-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmp)
			bin := filepath.Join(tmp, "s-race")
			bc := exec.Command(xo, "build", "--race", "--backend="+*backend, "-o", bin, dir)
			bc.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
			if out, err := bc.CombinedOutput(); err != nil {
				return fmt.Errorf("xo build --race --backend=%s %s: %v\n%s", *backend, dir, err, out)
			}
			t0 := time.Now()
			out, err := exec.Command(bin, strconv.Itoa(*stressMin), "64").CombinedOutput()
			text := strings.TrimSpace(string(out))
			last := summaryLine(text)
			// decision 0616 section 2a: under --race the resident set
			// grows with every finished task (ThreadSanitizer keeps each
			// task's context, as Go's race runtime keeps each
			// goroutine's), so the program's own memory check is not this
			// kind's: memory is the stress kind's, and the task churn
			// below compares the growth per task with Go's
			memOnly := err != nil && raceMemOnly(last)
			// only with the task churn in this run to judge memory per
			// task instead (lead, 2026-11-13: else a race-build leak
			// would pass unseen)
			if memOnly && *churnRounds == 0 {
				return fmt.Errorf("stress under --race grew in memory and -churn-rounds 0 skips the task churn that would judge it: run the churn\n%s", last)
			}
			if memOnly {
				last = strings.TrimSuffix(last, ", FAIL (memory grew)") + ", memory not judged under --race"
			}
			if (err != nil && !memOnly) || strings.Contains(text, "DATA RACE") || !(memOnly || strings.HasSuffix(last, ", pass")) {
				if len(text) > 20000 {
					text = text[len(text)-20000:]
				}
				return fmt.Errorf("stress under --race: %v\n%s", err, text)
			}
			line := fmt.Sprintf("- stress program under --race, 64 tasks: %s, %s", last, time.Since(t0).Round(time.Second))
			fmt.Println(line)
			lines = append(lines, line)
			runs = append(runs, raceRun{"stress", dir, false})
		}
	}
	result := "pass"
	if *churnRounds > 0 {
		line, ok, err := raceChurn(xo, *backend, root, *churnRounds)
		if err != nil {
			return err
		}
		fmt.Println(line)
		lines = append(lines, line)
		if !ok {
			// a measured miss, as perf's: the evidence records it
			result = "fail"
		}
	}
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kind: race\nmodule: %s\nresult: %s\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, result, commit, src, time.Now().UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "race runtime: %s\n", rtLabel)
	if *churnRounds > 0 {
		gv, err := exec.Command("go", "env", "GOVERSION").Output()
		if err != nil {
			return fmt.Errorf("go env GOVERSION: %v", err)
		}
		fmt.Fprintf(&b, "go: %s (the task churn's Go twin)\n", strings.TrimSpace(string(gv)))
	}
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven race -backend %s -stress-minutes %d -churn-rounds %d -host %q %s\n", *backend, *stressMin, *churnRounds, host, module)
	churnNote := ""
	if result == "fail" {
		churnNote = ", the task churn over 1.25 of Go's memory per task"
	}
	fmt.Fprintf(&b, "measure: %d runs under --race on %s, 0 data races, 0 failures%s, %s\n\n", len(runs), *backend, churnNote, time.Since(start).Round(time.Second))
	fmt.Fprintf(&b, "Race evidence (decision 0616): std/%s's tests, corpora and stress program under --race (ThreadSanitizer).\n\n", module)
	if stressOnly {
		fmt.Fprintf(&b, "%s\n\n", stressOnlyNote(module))
	}
	b.WriteString(strings.Join(lines, "\n") + "\n")
	out := filepath.Join("std", "evidence", module, "race.md")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, []byte(b.String()), 0o644)
}

// raceStressOnly says whether the module's race evidence is its stress
// program alone (lead, 2026-11-16): a module with no module tests and no
// corpus, as std/websocket and std/dns, whose servers the test world
// cannot host (its http.server serves requests, not raw connections:
// #120), runs its stress program under --race on real loopback instead.
// With neither, or with -stress-minutes 0, there is nothing to run.
func raceStressOnly(root, module string, runs, stressMin int) (bool, error) {
	if runs > 0 {
		return false, nil
	}
	if _, err := os.Stat(filepath.Join(root, "std", "evidence", module, "stress", "main.xo")); err != nil {
		return false, fmt.Errorf("%s has no module tests, no corpus and no stress program", module)
	}
	if stressMin <= 0 {
		return false, fmt.Errorf("%s has no module tests and no corpus: its race evidence is the stress program under --race, which -stress-minutes 0 skips", module)
	}
	return true, nil
}

// stressOnlyNote is the race evidence's line for a module raced by its
// stress program alone.
func stressOnlyNote(module string) string {
	return fmt.Sprintf("std/%s has no module tests and no corpus (the test world's http.server serves requests, not the raw connections its tests would need: #120), so its race evidence is the stress program under --race on real loopback alone (lead, 2026-11-16).", module)
}

// raceRuntimeLabel names the ThreadSanitizer runtime the race kind's
// builds link (lead, 2026-11-13: evidence measures the runtime users
// get): xo's own in lib/xo/race (built by scripts/race-runtime.sh, whose
// hash and trace part size it gives; scripts/lead/race-rt-host.sh
// installs it on an offload host), or $XO_RACE_RUNTIME. Without either it
// refuses, unless stock allows clang's own runtime (256 KB trace parts).
// raceRun is one xo test --race target of the race kind.
type raceRun struct {
	name, target string
	real         bool // a test uses net: xo test --real (the fake network has no UDP)
}

// realNetTest matches a test header whose uses list names net; such a test
// runs only under xo test --real (std/udp's tests, 2026-11-16).
var realNetTest = regexp.MustCompile(`(?m)^test "[^"\n]*" uses \([^)]*\bnet\b`)

func usesRealNet(path string) bool {
	b, err := os.ReadFile(path)
	return err == nil && realNetTest.Match(b)
}

// raceRuns lists the module's tests (std/native/MODULE_test.xo, and
// std/MODULE_test.xo, which the race kind left out until 2026-11-16:
// htmlent's only tests are there) and its corpus directories under root.
func raceRuns(root, module string) []raceRun {
	var runs []raceRun
	if _, err := os.Stat(filepath.Join(root, "std", "native", module+"_test.xo")); err == nil {
		runs = append(runs, raceRun{"module tests (std/native/" + module + "_test.xo)", "--std-module=native/" + module, usesRealNet(filepath.Join(root, "std", "native", module+"_test.xo"))})
	}
	if _, err := os.Stat(filepath.Join(root, "std", module+"_test.xo")); err == nil {
		// std/MODULE, not a bare MODULE: with std/native/MODULE_test.xo too,
		// a bare name is ambiguous to xo test (#152: cbor)
		runs = append(runs, raceRun{"module tests (std/" + module + "_test.xo)", "--std-module=std/" + module, usesRealNet(filepath.Join(root, "std", module+"_test.xo"))})
	}
	dirs, _ := filepath.Glob(filepath.Join(root, "internal", "stdtest", "testdata", "corpus", module+"*"))
	for _, d := range dirs {
		base := filepath.Base(d)
		if _, err := os.Stat(filepath.Join(root, "std", base+".xo")); err == nil && base != module {
			continue // another module's corpus (jsonrpc for json)
		}
		runs = append(runs, raceRun{"corpus " + base, d, false})
	}
	return runs
}

func raceRuntimeLabel(stock bool) (string, error) {
	if env := os.Getenv("XO_RACE_RUNTIME"); env != "" {
		return "XO_RACE_RUNTIME " + env, nil
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		dir := filepath.Join("lib", "xo", "race", runtime.GOOS+"-"+runtime.GOARCH)
		if ents, err := os.ReadDir(dir); err == nil && len(ents) > 0 {
			b, err := os.ReadFile(filepath.Join("scripts", "race-runtime.sh"))
			if err != nil {
				return "", err
			}
			sum := sha256.Sum256(b)
			kb := 256
			if bytes.Contains(b, []byte("kByteSize = 128 << 10;")) {
				kb = 128
			}
			return fmt.Sprintf("xo's (lib/xo/race/%s-%s from scripts/race-runtime.sh %x, TracePart %d KB)", runtime.GOOS, runtime.GOARCH, sum[:8], kb), nil
		}
	}
	if !stock {
		return "", fmt.Errorf("race: no xo ThreadSanitizer runtime in lib/xo/race/%s-%s (scripts/race-runtime.sh builds it; on an offload host scripts/lead/race-rt-host.sh installs it): refusing to measure clang's (-stock-runtime allows it)", runtime.GOOS, runtime.GOARCH)
	}
	return "clang's stock runtime (TracePart 256 KB; not the runtime xo ships)", nil
}

// churnPer is the tasks of one round of the race kind's task churn.
const churnPer = 200000

// raceChurn runs decision 0616 section 2a's task churn: rounds rounds of
// churnPer tasks spawned and awaited (scripts/stdproven/racechurn/xo)
// under --race, and the same goroutines under Go's -race
// (racechurn/gotwin). Each finished task leaves ThreadSanitizer state
// behind in both (a thread context, about 250 B), and Xo's C flavor fills
// a trace part pool first (256 KB parts against Go's 128 KB; about 1.8 GB
// on linux/arm64, full after about 6 million tasks). The judged figure is
// the growth per task over the last two fifths of the rounds, after that
// pool is full: Xo's within 1.25 times Go's. The resident set at three
// fifths is shown, not judged.
// It returns the line, whether the figure is within 1.25, and an error
// for a run that could not be measured (a build, a race, a fault).
func raceChurn(xo, backend, root string, rounds int) (string, bool, error) {
	if rounds < 5 {
		return "", false, fmt.Errorf("-churn-rounds %d: at least 5", rounds)
	}
	tmp, err := os.MkdirTemp("", "stdproven-churn-")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(tmp)
	xbin, gbin := filepath.Join(tmp, "churn-xo"), filepath.Join(tmp, "churn-go")
	bc := exec.Command(xo, "build", "--race", "--backend="+backend, "-o", xbin, filepath.Join(root, "scripts", "stdproven", "racechurn", "xo"))
	bc.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if out, err := bc.CombinedOutput(); err != nil {
		return "", false, fmt.Errorf("xo build --race the task churn: %v\n%s", err, out)
	}
	gc := exec.Command("go", "build", "-race", "-o", gbin, ".")
	gc.Dir = filepath.Join(root, "scripts", "stdproven", "racechurn", "gotwin")
	if out, err := gc.CombinedOutput(); err != nil {
		return "", false, fmt.Errorf("go build -race the task churn: %v\n%s", err, out)
	}
	t0 := time.Now()
	from := rounds * 3 / 5
	run := func(bin string) ([]int, error) {
		out, err := exec.Command(bin, strconv.Itoa(rounds), strconv.Itoa(churnPer)).CombinedOutput()
		text := string(out)
		if err != nil || strings.Contains(text, "DATA RACE") {
			return nil, fmt.Errorf("%s: %v\n%s", filepath.Base(bin), err, text)
		}
		var rss []int
		for _, l := range strings.Split(strings.TrimSpace(text), "\n") {
			f := strings.Fields(l)
			if len(f) == 4 && f[0] == "round" && f[2] == "rss" {
				n, _ := strconv.Atoi(f[3])
				rss = append(rss, n)
			}
		}
		if len(rss) != rounds || rss[rounds-1] == 0 {
			return nil, fmt.Errorf("%s printed %d rounds with a resident set, want %d (Linux /proc)", filepath.Base(bin), len(rss), rounds)
		}
		return rss, nil
	}
	xr, err := run(xbin)
	if err != nil {
		return "", false, err
	}
	gr, err := run(gbin)
	if err != nil {
		return "", false, err
	}
	tasks := float64((rounds - 1 - from) * churnPer)
	per := func(r []int) float64 { return float64(r[rounds-1]-r[from]) * 1024 / tasks }
	xp, gp := per(xr), per(gr)
	if gp <= 0 {
		return "", false, fmt.Errorf("task churn: Go's race build did not grow (%d to %d KiB): no figure to compare with", gr[from], gr[rounds-1])
	}
	ratio := xp / gp
	line := fmt.Sprintf("- task churn under --race, %d million tasks: %.0f B per finished task after %d million (Go's race build: %.0f B), %.2f of Go's (at most 1.25); resident set at %d million %d MB (Go's %d MB, shown), %s",
		rounds*churnPer/1000000, xp, from*churnPer/1000000, gp, ratio, from*churnPer/1000000, xr[from]/1024, gr[from]/1024, time.Since(t0).Round(time.Second))
	if ratio > 1.25 {
		return line + ", FAIL", false, nil
	}
	return line + ", pass", true, nil
}

// stress runs decision 0616's stress kind: the module's stress program
// std/evidence/MODULE/stress/main.xo, built on each backend and run for
// -minutes with -tasks tasks. The program checks its own results and
// memory and prints one summary line ending in ", pass".
// runSteps runs each step in turn, or all at once with parallel, and
// gives their lines in the steps' order or the first step's error (in
// that order too).
func runSteps(steps []string, parallel bool, run func(string) (string, error)) ([]string, error) {
	lines := make([]string, len(steps))
	errs := make([]error, len(steps))
	if !parallel {
		for i, st := range steps {
			if lines[i], errs[i] = run(st); errs[i] != nil {
				return nil, errs[i]
			}
		}
		return lines, nil
	}
	var wg sync.WaitGroup
	for i, st := range steps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lines[i], errs[i] = run(st)
		}()
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return nil, e
		}
	}
	return lines, nil
}

func stress(args []string) error {
	fs := flag.NewFlagSet("stress", flag.ExitOnError)
	minutes := fs.Int("minutes", 30, "run time per backend")
	tasks := fs.Int("tasks", 64, "concurrent tasks")
	backends := fs.String("backends", defaultBackends(runtime.GOARCH), "backends, comma separated (default: this host's native backend and llvm)")
	checkMin := fs.Int("check-minutes", 5, "minutes of a further native backend run built with --check=all (decision 0638: use after free and overruns fault, a leak fails the run; 0 skips it)")
	checkOnly := fs.Bool("check-only", false, "run only the --check=all run and add it to the module's stress.md, which must pass at the current source hash")
	hostLabel := fs.String("host", "", "the host's name in the evidence")
	parallel := fs.Bool("parallel", false, "run the backends and the --check=all run at once rather than in turn (#152: about 35 minutes a module rather than 65; each still runs its full -minutes)")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	module := fs.Arg(0)
	dir := filepath.Join("std", "evidence", module, "stress")
	if _, err := os.Stat(filepath.Join(dir, "main.xo")); err != nil {
		return err
	}
	if *checkOnly {
		host := *hostLabel
		if host == "" {
			host = runtime.GOOS + "/" + runtime.GOARCH
		}
		if *checkMin <= 0 {
			return errors.New("-check-only needs -check-minutes above 0")
		}
		return stressCheckOnly(module, dir, *checkMin, *tasks, host)
	}
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	if dirty, _ := gitOut("status", "--porcelain", "--", "std", ":(exclude)std/evidence", "compiler"); dirty != "" {
		return fmt.Errorf("std or compiler has uncommitted changes: evidence names a commit:\n%s", dirty)
	}
	tmp, err := os.MkdirTemp("", "stdproven-stress-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	start := time.Now()
	runOne := func(b string) (string, error) {
		bin := filepath.Join(tmp, "s-"+b)
		if out, err := exec.Command(filepath.Join("bin", "xo"), "build", "--backend="+b, "-o", bin, dir).CombinedOutput(); err != nil {
			return "", fmt.Errorf("xo build --backend=%s %s: %v\n%s", b, dir, err, out)
		}
		lines, err := twoOfThree(func(i int) (string, bool, error) {
			t0 := time.Now()
			out, err := exec.Command(bin, strconv.Itoa(*minutes), strconv.Itoa(*tasks)).CombinedOutput()
			text := strings.TrimSpace(string(out))
			last := summaryLine(text)
			memFail := memOnly(last)
			if !memFail && (err != nil || !strings.HasSuffix(last, ", pass")) {
				if len(text) > 20000 {
					text = text[len(text)-20000:]
				}
				return "", false, fmt.Errorf("%s stress run %d: %v\n%s", b, i, err, text)
			}
			line := fmt.Sprintf("- %s run %d: %s, %s", b, i, last, time.Since(t0).Round(time.Second))
			fmt.Println(line)
			return line, memFail, nil
		})
		if err != nil {
			return "", fmt.Errorf("%s: %v", b, err)
		}
		return strings.Join(lines, "\n"), nil
	}
	// decision 0638: the same program built with --check=all, so a use
	// after free or an overrun faults and a block still live at exit
	// fails the run (the leak report is the last line, after the
	// program's own; the resident set is not sampled under the check,
	// which is why the plain runs stay)
	checkNote := ""
	steps := strings.Split(*backends, ",")
	if *checkMin > 0 {
		steps = append(steps, "check")
	}
	lines, err := runSteps(steps, *parallel, func(st string) (string, error) {
		if st != "check" {
			return runOne(st)
		}
		line, note, err := stressCheck(dir, tmp, *checkMin, *tasks)
		checkNote = note
		return line, err
	})
	if err != nil {
		return err
	}
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kind: stress\nmodule: %s\nresult: pass\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, commit, src, time.Now().UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven stress -minutes %d -tasks %d -backends %s -check-minutes %d -host %q %s\n", *minutes, *tasks, *backends, *checkMin, host, module)
	fmt.Fprintf(&b, "measure: %d tasks for %d min on each of %s, 0 mismatches, 0 faults, memory under 10%% growth from minutes 15 to 17 to the last 3 on 2 of 3 runs a backend (decision 0717)%s, %s\n\n", *tasks, *minutes, *backends, checkNote, time.Since(start).Round(time.Second))
	fmt.Fprintf(&b, "Stress evidence (decision 0616): the program %s, every result checked against Go's answer, the resident set sampled every 10 s.\n\n", dir)
	b.WriteString(strings.Join(lines, "\n") + "\n")
	return os.WriteFile(filepath.Join("std", "evidence", module, "stress.md"), []byte(b.String()), 0o644)
}

// twoOfThree: decision 0717's memory bar. run does run i (1 to 3) and
// returns its evidence line and whether it failed on memory alone (every
// result right); an error (a mismatch, a fault, a crash) fails at once.
// It stops after 2 runs pass the memory bar, or fails after 2 do not;
// every run's line is kept, failures included.
func twoOfThree(run func(i int) (string, bool, error)) ([]string, error) {
	var lines []string
	pass, fail := 0, 0
	for i := 1; i <= 3; i++ {
		line, memFail, err := run(i)
		if err != nil {
			return lines, err
		}
		lines = append(lines, line)
		if memFail {
			fail++
		} else {
			pass++
		}
		if pass == 2 {
			return lines, nil
		}
		if fail == 2 {
			break
		}
	}
	return lines, fmt.Errorf("memory grew on %d of %d runs (decision 0717 needs 2 of 3 to pass):\n%s", fail, len(lines), strings.Join(lines, "\n"))
}

// stressCheck runs the stress program built with --check=all (decision
// 0638) on arm64 for minutes: a use after free or an overrun faults and
// a block still live at exit fails the run. The leak report is the last
// line, after the program's own; the resident set is not sampled under
// the check, so the plain runs stay beside it. It returns the evidence
// line and the measure's note.
func stressCheck(dir, tmp string, minutes, tasks int) (string, string, error) {
	bin := filepath.Join(tmp, "s-check")
	nb := nativeBackend(runtime.GOARCH)
	if out, err := exec.Command(filepath.Join("bin", "xo"), "build", "--check=all", "--backend="+nb, "-o", bin, dir).CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("xo build --check=all --backend=%s %s: %v\n%s", nb, dir, err, out)
	}
	t0 := time.Now()
	cmd := exec.Command(bin, strconv.Itoa(minutes), strconv.Itoa(tasks))
	cmd.Env = append(os.Environ(), stressCheckEnv)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	var passLine, leakLine string
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "xo leakcheck: ") {
			leakLine = l
		} else if strings.HasSuffix(l, ", pass") {
			passLine = l
		}
	}
	if err != nil || passLine == "" || !strings.HasPrefix(leakLine, "xo leakcheck: 0 live ") {
		if len(text) > 20000 {
			text = text[len(text)-20000:]
		}
		return "", "", fmt.Errorf("%s --check=all stress: %v\n%s", nb, err, text)
	}
	line := fmt.Sprintf("- %s --check=all: %s; %s, %s", nb, passLine, leakLine, time.Since(t0).Round(time.Second))
	fmt.Println(line)
	return line, fmt.Sprintf(", %d min on %s under --check=all with 0 faults and 0 blocks live at exit", minutes, nb), nil
}

// stressCheckEnv marks the --check=all run for the stress program, whose
// rss_kib then samples nothing (lead, 2026-11-17, #152): the resident set
// under --check=all is no measure of a leak (std/udp's check run grew 25%
// with 0 mismatches and failed before its leak report). The leak report
// judges memory there instead: "xo leakcheck: 0 live" at exit, required
// below.
const stressCheckEnv = "XO_STRESS_CHECK=1"

// stressCheckOnly adds the --check=all run to a stress.md written before
// the stress kind had it (decision 0616's 2026-11-14 amendment): the
// file must pass at the module's current source hash; the run's command,
// commit and date go in a check: line, its note in measure, and its line
// below the others.
func stressCheckOnly(module, dir string, minutes, tasks int, host string) error {
	path := filepath.Join("std", "evidence", module, "stress.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(data)
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	if !strings.Contains(text, "\nresult: pass\n") || !strings.Contains(text, "\nsource: "+src+"\n") {
		return fmt.Errorf("%s: not a pass at the current source %s; run the whole stress kind", path, src)
	}
	if strings.Contains(text, "--check=all") {
		return fmt.Errorf("%s already has the --check=all run", path)
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "stdproven-stress-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	line, note, err := stressCheck(dir, tmp, minutes, tasks)
	if err != nil {
		return err
	}
	var out []string
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if strings.HasPrefix(l, "measure: ") {
			if i := strings.LastIndex(l, ", "); i > 0 {
				l = l[:i] + note + l[i:]
			}
		}
		out = append(out, l)
		if strings.HasPrefix(l, "command: ") {
			out = append(out, fmt.Sprintf("check: go run ./scripts/stdproven stress -check-only -check-minutes %d -tasks %d -host %q %s (commit %s, %s)", minutes, tasks, host, module, commit, time.Now().UTC().Format("2006-01-02")))
		}
	}
	out = append(out, line)
	return os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644)
}

// perfOwners names the module of a bench/perf directory whose name starts
// with no module's: the primitives of std/crypto (its AEADs, SHA-3, BLAKE2b,
// ML-KEM and Ed25519) are benched by algorithm, and the prelude's Str,
// Bytes, BytesBuilder, List and Map methods and number text by what they do
// (task #87, 2026-11-16).
var perfOwners = map[string]string{"aead": "crypto", "blake2b": "crypto", "aesgcm": "crypto", "aesmodes": "crypto", "chacha": "crypto", "sha3": "crypto", "mlkem": "crypto", "ed25519": "crypto",
	"bytebuf": "prelude", "bytespredicate": "prelude", "bytessearch": "prelude", "bytessplit": "prelude", "bytestransform": "prelude", "mapiter": "prelude",
	"repeat": "prelude", "sort": "prelude", "strconv": "prelude", "strsplit": "prelude", "trimchars": "prelude", "unicode": "prelude"}

// perfOwner is the std module a bench/perf directory measures: the one
// perfOwners names, else the longest std module name (std/NAME.xo) its
// name starts with, so bench/perf/htmlparse is htmlparse's and
// bench/perf/jsontoken json's.
func perfOwner(dir string) string {
	if m, ok := perfOwners[dir]; ok {
		return m
	}
	best := ""
	mods, _ := filepath.Glob(filepath.Join("std", "*.xo"))
	for _, m := range mods {
		n := strings.TrimSuffix(filepath.Base(m), ".xo")
		if strings.HasSuffix(n, "_test") {
			continue
		}
		if strings.HasPrefix(dir, n) && len(n) > len(best) {
			best = n
		}
	}
	return best
}

// perf times the module's Go twins: every bench/perf/MODULE* directory with
// a twin.txt (scripts/twincheck checks their outputs agree, then -time
// runs each case's Xo and Go programs -rounds times, alternating, at full
// size) and writes std/evidence/MODULE/perf.md, a table of the medians
// with Xo over Go, the performance axis's evidence (decision 0461:
// Proven at or under 1.05 of Go in every row). Rows under 5 ms on both
// sides are listed but marked: startup decides them.
func perf(args []string) error {
	fs := flag.NewFlagSet("perf", flag.ExitOnError)
	rounds := fs.Int("rounds", 9, "timed runs of each program")
	hostLabel := fs.String("host", "", "the host's name in the evidence")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	module := fs.Arg(0)
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	if dirty, _ := gitOut("status", "--porcelain", "--", "std", ":(exclude)std/evidence", "compiler", "bench/perf"); dirty != "" {
		return fmt.Errorf("std, compiler or bench/perf has uncommitted changes: evidence names a commit:\n%s", dirty)
	}
	cands, _ := filepath.Glob(filepath.Join("bench", "perf", module+"*"))
	for d, m := range perfOwners {
		if m == module {
			cands = append(cands, filepath.Join("bench", "perf", d))
		}
	}
	sort.Strings(cands)
	var dirs []string
	for _, d := range cands {
		if perfOwner(filepath.Base(d)) != module {
			continue // another module's bench: bench/perf/htmlparse is not html's
		}
		if _, err := os.Stat(filepath.Join(d, "twin.txt")); err == nil {
			dirs = append(dirs, d)
		}
	}
	if len(dirs) == 0 {
		return fmt.Errorf("no bench/perf/%s* directory has a twin.txt", module)
	}
	start := time.Now()
	cmd := exec.Command("go", append([]string{"run", "./scripts/twincheck", "-time", strconv.Itoa(*rounds)}, dirs...)...)
	cmd.Env = append(os.Environ(), "XO_CLI="+filepath.Join("bin", "xo"))
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		if len(text) > 20000 {
			text = text[len(text)-20000:]
		}
		return fmt.Errorf("twincheck: %v\n%s", err, text)
	}
	type row struct {
		name    string
		xo, gom float64
		ratio   float64
	}
	var rows []row
	for _, l := range strings.Split(text, "\n") {
		f := strings.Fields(l)
		if len(f) != 8 || f[0] != "time" || f[2] != "xo" || f[4] != "go" || f[6] != "ratio" {
			continue
		}
		x, e1 := strconv.ParseFloat(f[3], 64)
		g, e2 := strconv.ParseFloat(f[5], 64)
		r, e3 := strconv.ParseFloat(f[7], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		rows = append(rows, row{f[1], x, g, r})
	}
	if len(rows) == 0 {
		return fmt.Errorf("twincheck timed no case\n%s", text)
	}
	worst := 0.0
	var b strings.Builder
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	var tb strings.Builder
	tb.WriteString("| workload | Go ms | Xo ms | Xo / Go |\n|---|---|---|---|\n")
	for _, r := range rows {
		mark := ""
		if r.xo < 5 && r.gom < 5 {
			mark = " (under 5 ms: startup)"
		} else if r.ratio > worst {
			worst = r.ratio
		}
		fmt.Fprintf(&tb, "| %s%s | %.1f | %.1f | %.3f |\n", r.name, mark, r.gom, r.xo, r.ratio)
	}
	result := "pass"
	if worst > 1.05 {
		result = "fail"
	}
	fmt.Fprintf(&b, "kind: perf\nmodule: %s\nresult: %s\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, result, commit, src, time.Now().UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven perf -rounds %d -host %q %s\n", *rounds, host, module)
	fmt.Fprintf(&b, "measure: %d cases of %d twin directories, worst Xo / Go %.3f over the cases above 5 ms (Proven at or under 1.05), %s\n\n", len(rows), len(dirs), worst, time.Since(start).Round(time.Second))
	fmt.Fprintf(&b, "Performance against Go (decisions 0461, 0616): scripts/twincheck -time %d over %s; medians of wall time, the two programs alternating.\n\n", *rounds, strings.Join(dirs, ", "))
	b.WriteString(tb.String())
	outp := filepath.Join("std", "evidence", module, "perf.md")
	if err := os.MkdirAll(filepath.Dir(outp), 0o755); err != nil {
		return err
	}
	fmt.Print(tb.String())
	return os.WriteFile(outp, []byte(b.String()), 0o644)
}

// serverPerf is the perf evidence of a module whose twins are servers
// (decision 0616, "Servers"): bench/perf/MODULE/proven.sh runs them
// against Go and prints "row WORKLOAD BACKEND METRIC GO XO" lines. Each
// metric becomes a cost ratio, Xo's over Go's (Go's over Xo's for rps,
// where more is better); a workload's figure on a backend is the worst
// of its judged ratios (rps, cpu_us, wall_s; p99_us is shown only), and
// the module passes when every figure is at or under 1.05.
func serverPerf(args []string) error {
	fs := flag.NewFlagSet("serverperf", flag.ExitOnError)
	reps := fs.Int("reps", 5, "runs of each load benchmark (proven.sh's REPS)")
	hostLabel := fs.String("host", "", "the host's name in the evidence")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	module := fs.Arg(0)
	src, _, err := sourceHash(module)
	if err != nil {
		return err
	}
	commit, err := gitOut("rev-parse", "--short=9", "HEAD")
	if err != nil {
		return err
	}
	if dirty, _ := gitOut("status", "--porcelain", "--", "std", ":(exclude)std/evidence", "compiler", "bench/perf"); dirty != "" {
		return fmt.Errorf("std, compiler or bench/perf has uncommitted changes: evidence names a commit:\n%s", dirty)
	}
	script := filepath.Join("bench", "perf", module, "proven.sh")
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("no %s: a server module's perf evidence needs one", script)
	}
	xo, err := filepath.Abs(filepath.Join("bin", "xo"))
	if err != nil {
		return err
	}
	start := time.Now()
	cmd := exec.Command("sh", script)
	cmd.Env = append(os.Environ(), "XO="+xo, "REPS="+strconv.Itoa(*reps))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%s: %v\n%s", script, err, out)
	}
	type key struct{ work, be string }
	type cell struct {
		goV, xoV, ratio float64
		judged          bool
	}
	cells := map[key]map[string]cell{}
	var order []key
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) != 6 || f[0] != "row" {
			continue
		}
		g, e1 := strconv.ParseFloat(f[4], 64)
		x, e2 := strconv.ParseFloat(f[5], 64)
		if e1 != nil || e2 != nil || g <= 0 || x <= 0 {
			return fmt.Errorf("%s: bad row %q", script, l)
		}
		c := cell{goV: g, xoV: x, ratio: x / g, judged: true}
		switch f[3] {
		case "rps":
			c.ratio = g / x
		case "cpu_us":
		case "wall_s":
			if g < 0.05 || x < 0.05 {
				return fmt.Errorf("%s: %q: a wall under 50 ms is too short to judge (raise the workload's size)", script, l)
			}
		case "p99_us":
			c.judged = false
		default:
			return fmt.Errorf("%s: unknown metric in %q", script, l)
		}
		k := key{f[1], f[2]}
		if cells[k] == nil {
			cells[k] = map[string]cell{}
			order = append(order, k)
		}
		cells[k][f[3]] = c
	}
	if len(order) == 0 {
		return fmt.Errorf("%s printed no rows", script)
	}
	host := *hostLabel
	if host == "" {
		host = runtime.GOOS + "/" + runtime.GOARCH
	}
	metrics := []string{"rps", "cpu_us", "wall_s", "p99_us"}
	var tb strings.Builder
	tb.WriteString("| workload | backend | metric | Go | Xo | cost Xo / Go |\n|---|---|---|---|---|---|\n")
	worst := 0.0
	var figs []string
	for _, k := range order {
		fig := 0.0
		for _, m := range metrics {
			c, ok := cells[k][m]
			if !ok {
				continue
			}
			note := ""
			if !c.judged {
				note = " (shown)"
			} else if c.ratio > fig {
				fig = c.ratio
			}
			fmt.Fprintf(&tb, "| %s | %s | %s%s | %g | %g | %.3f |\n", k.work, k.be, m, note, c.goV, c.xoV, c.ratio)
		}
		figs = append(figs, fmt.Sprintf("%s %s %.3f", k.work, k.be, fig))
		if fig > worst {
			worst = fig
		}
	}
	result := "pass"
	if worst > 1.05 {
		result = "fail"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "kind: perf\nmodule: %s\nresult: %s\ncommit: %s\nsource: %s\ndate: %s\nhost: %s\n", module, result, commit, src, time.Now().UTC().Format("2006-01-02"), host)
	fmt.Fprintf(&b, "command: go run ./scripts/stdproven serverperf -reps %d -host %q %s\n", *reps, host, module)
	fmt.Fprintf(&b, "measure: %d workload and backend figures, worst Xo / Go cost %.3f (the worse of throughput and CPU or wall per figure; Proven at or under 1.05), %s\n\n", len(order), worst, time.Since(start).Round(time.Second))
	fmt.Fprintf(&b, "Server performance against Go (decision 0616, Servers): %s against Go twins on the native backends. A figure is the worst of its judged cost ratios (Go's rps over Xo's; Xo's CPU per request or wall over Go's); p99 is shown, not judged.\n\n", script)
	b.WriteString("Figures: " + strings.Join(figs, ", ") + "\n\n")
	b.WriteString(tb.String())
	outp := filepath.Join("std", "evidence", module, "perf.md")
	if err := os.MkdirAll(filepath.Dir(outp), 0o755); err != nil {
		return err
	}
	fmt.Print(tb.String())
	return os.WriteFile(outp, []byte(b.String()), 0o644)
}

// summaryLine is a stress program's summary: its last line that is not
// the "error: ..." line main prints for the Err it returns (crc32's
// stress program, 2026-11-15: "FAIL (memory grew)" then "error: memory
// grew 12.7%", so a race run read the error line and failed on memory
// growth that decision 0616 section 2a leaves to the task churn).
// cleanCount: a stress summary's zero wrong-result count. Decision 0616
// names it "0 mismatches"; std/dns's stress program says "0 wrong
// answers" and std/smtp's "0 wrong results" (2026-11-17: dns's race
// evidence failed on memory alone, the exemption below not seeing a
// clean count). Other programs say "0 wrong", "0 identities broken",
// "0 differences", "0 promises broken", "none lost" or "every ... "
// (grpc and jsonrpc's "every answer right", sse's "every event once and
// in order"), printed only when every result was right (2026-11-17:
// jsonrpc's race evidence failed on memory alone, 82% growth under
// --race with every answer right).
var cleanCount = regexp.MustCompile(` (0 (mismatches|wrong|wrong [a-z]+|identities broken|differences|promises broken)|none lost|every [a-z ]+)[,)]`)

// raceMemOnly: a stress summary under --race that failed on memory alone
// (every result right), which the task churn judges instead (decision
// 0616 section 2a).
func raceMemOnly(last string) bool {
	return memOnly(last)
}

// memOnly: a stress summary that failed on memory alone, every result
// right (decision 0717 lets 1 of 3 such runs pass the kind).
func memOnly(last string) bool {
	return strings.HasSuffix(last, ", FAIL (memory grew)") && cleanCount.MatchString(last)
}

func summaryLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if !strings.HasPrefix(lines[i], "error: ") {
			return lines[i]
		}
	}
	return lines[len(lines)-1]
}

// nativeBackend is xo's native backend for a Go GOARCH: amd64 on linux/amd64
// (decision 0708: Proven evidence may come from that host), arm64 otherwise.
func nativeBackend(goarch string) string {
	if goarch == "amd64" {
		return "amd64"
	}
	return "arm64"
}

// defaultBackends: the backends a fuzz, timing or stress run takes by
// default, the host's native backend and llvm (decision 0708).
func defaultBackends(goarch string) string {
	return nativeBackend(goarch) + ",llvm"
}
