// Fresh seed fuzz runs of std modules whose corpus generators draw seeded
// random cases from a Go reference (task #116, decision 0616's fuzz kind):
// the generator runs with XO_<MOD>FUZZ_SEED added to each of its seeds
// (0, the default there, is the committed corpus TestStdCorpora runs), and
// Xo must give the reference's answers on the new cases. Each Test<Mod>Fuzz
// runs XO_<MOD>FUZZ_SEED (default 2) on XO_<MOD>FUZZ_BACKENDS (default arm64
// and llvm); its case count is the generated table rows.
//
//   - brotli: scripts/stdcorpus/brotlioracle (github.com/andybalholm/brotli),
//     random inputs at every quality and window, and changed streams;
//   - charset: charsetoracle (golang.org/x/text/encoding), random text and
//     bytes through every encoding both ways;
//   - htmlparse: htmlparseoracle (golang.org/x/net/html Parse and
//     ParseFragment), random documents from markup pieces;
//   - htmltok: htmltokoracle (x/net/html's Tokenizer) in each mode;
//   - publicsuffix: psloracle (golang.org/x/net/publicsuffix), random
//     domains;
//   - protodesc: protodescoracle (google.golang.org/protobuf's descriptorpb),
//     random FileDescriptorProtos round tripped byte for byte, and changed
//     bytes decoded whenever protobuf-go accepts them with UTF-8 strings;
//   - protoparse: protoparseoracle (bufbuild/protocompile's parser and its
//     validation), random proto2 and proto3 files, half changed at the
//     token level: the same outline when protocompile accepts, an error
//     when it rejects;
//   - migrate: migrateoracle (pressly/goose v3.27.3's sqlparser, copied,
//     and NumericComponent), random migration files, most changed at the
//     line level: the same Up, Down and transaction mode, or an error,
//     and the same version for a file name;
//   - testing: fakenetoracle (net/http's ServeMux), random sets of FakeNet
//     patterns and requests: the pattern ServeMux picks and its wildcards;
//   - metrics: metricsoracle (prometheus/client_golang v1.23.2), random
//     registries and operations: the same text exposition;
//   - protowkt: protowktoracle (protobuf-go's well known types), random
//     Struct, Value, ListValue, Duration and Timestamp values;
//   - embed: embedoracle (Go's io/fs over std/evidence/embed's stress
//     tree), random paths, directories and glob patterns: Dir.read,
//     contains and sub, and fs().glob and read_dir, as fs.ReadFile, Sub,
//     Glob and ReadDir answer;
//   - term: termoracle (golang.org/x/term's Terminal), random sessions of
//     keys mixed with runes, bytes that are not UTF-8 and escape sequences
//     cut short, at random widths and read sizes, with SetSize, Write,
//     ReadPassword and bracketed paste: the same lines and bytes written;
//   - prototext: prototextoracle (protobuf-go's encoding/prototext), random
//     messages of every field kind written as text.
package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// corpusFuzz runs scripts/stdcorpus GEN with XO_ENV_SEED and runs the
// GEN_test.xo it writes.
func corpusFuzz(t *testing.T, gen, env string) {
	if testing.Short() {
		t.Skip("a corpus fuzz is a heavy test; its seed 0 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_"+env+"_SEED", 2)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("go", "run", "./scripts/stdcorpus", gen)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_"+env+"_SEED="+strconv.Itoa(seed), "XO_STDCORPUS_OUT="+out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scripts/stdcorpus %s: %v\n%s", gen, err, b)
	}
	name := gen + "_test.xo"
	text, err := os.ReadFile(filepath.Join(out, name))
	if err != nil {
		t.Fatal(err)
	}
	runFuzzDir(t, name, string(text), envBackends("XO_"+env+"_BACKENDS", []string{"arm64", "llvm"}), strings.Count(string(text), "\n  ("))
}

func TestBrotliFuzz(t *testing.T)       { corpusFuzz(t, "brotli", "BROTLIFUZZ") }
func TestCharsetFuzz(t *testing.T)      { corpusFuzz(t, "charset", "CHARSETFUZZ") }
func TestHtmlparseFuzz(t *testing.T)    { corpusFuzz(t, "htmlparse", "HTMLPARSEFUZZ") }
func TestHtmltokFuzz(t *testing.T)      { corpusFuzz(t, "htmltok", "HTMLTOKFUZZ") }
func TestPublicsuffixFuzz(t *testing.T) { corpusFuzz(t, "publicsuffix", "PUBLICSUFFIXFUZZ") }
func TestRandomFuzz(t *testing.T)       { corpusFuzz(t, "random", "RANDOMFUZZ") }
func TestProtodescFuzz(t *testing.T)    { corpusFuzz(t, "protodesc", "PROTODESCFUZZ") }
func TestProtoparseFuzz(t *testing.T)   { corpusFuzz(t, "protoparsefuzz", "PROTOPARSEFUZZ") }
func TestMigrateFuzz(t *testing.T)      { corpusFuzz(t, "migratefuzz", "MIGRATEFUZZ") }
func TestFakenetFuzz(t *testing.T)      { corpusFuzz(t, "fakenetfuzz", "FAKENETFUZZ") }
func TestProtowktFuzz(t *testing.T)     { corpusFuzz(t, "protowkt", "PROTOWKTFUZZ") }
func TestPrototextFuzz(t *testing.T)    { corpusFuzz(t, "prototext", "PROTOTEXTFUZZ") }
func TestMetricsFuzz(t *testing.T)      { corpusFuzz(t, "metricsfuzz", "METRICSFUZZ") }
func TestEmbedFuzz(t *testing.T)        { corpusFuzz(t, "embedfuzz", "EMBEDFUZZ") }
func TestTermFuzz(t *testing.T)         { corpusFuzz(t, "term", "TERMFUZZ") }

// Seeds that found a bug in a generator, kept as tests of their own (#116).

// TestBrotliFuzzSeed182: a quality 10 input whose stream from Go on arm64
// had a fused multiply-add choose other blocks (the oracle now runs with
// fusion off, as Go on amd64 and Xo run).
func TestBrotliFuzzSeed182(t *testing.T) {
	t.Setenv("XO_BROTLIFUZZ_SEED", "182")
	corpusFuzz(t, "brotli", "BROTLIFUZZ")
}

// TestRandomFuzzSeed4: a Zipf imax past Int's range, which the generator
// wrote as a literal that did not compile.
func TestRandomFuzzSeed4(t *testing.T) {
	t.Setenv("XO_RANDOMFUZZ_SEED", "4")
	corpusFuzz(t, "random", "RANDOMFUZZ")
}

// TestRandomFuzzSeed17: a Zipf draw where Go's arm64 math.Log fused a
// multiply-add (the generator's Zipf now uses strictExp and strictLog).
func TestRandomFuzzSeed17(t *testing.T) {
	t.Setenv("XO_RANDOMFUZZ_SEED", "17")
	corpusFuzz(t, "random", "RANDOMFUZZ")
}

// TestRandomFuzzSeed87: a NormFloat64 draw one ulp apart on arm64 Go
// (scripts/stdcorpus now runs with fusion off, noFMA).
func TestRandomFuzzSeed87(t *testing.T) {
	t.Setenv("XO_RANDOMFUZZ_SEED", "87")
	corpusFuzz(t, "random", "RANDOMFUZZ")
}
