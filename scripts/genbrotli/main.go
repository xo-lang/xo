// Command genbrotli writes the tables of std/brotli into std/brotli.xo
// (between its BEGIN GENERATED and END GENERATED lines) from
// github.com/andybalholm/brotli's Go sources (decision 0666): RFC 7932's
// static dictionary, the transforms, the context lookup, the command
// lookup and the bit reversal table, copied value for value so std/brotli
// decodes and encodes as andybalholm's package does.
//
// Usage: go run ./scripts/genbrotli BROTLIDIR FILE
// (BROTLIDIR: the github.com/andybalholm/brotli@VERSION directory in the
// module cache: go mod download -json github.com/andybalholm/brotli@v1.2.2).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func fail(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "genbrotli: "+f+"\n", a...)
	os.Exit(1)
}

func bytesLit(b []byte) string {
	var s strings.Builder
	s.WriteString(`b"`)
	for _, c := range b {
		fmt.Fprintf(&s, `\x%02x`, c)
	}
	s.WriteString(`"`)
	return s.String()
}

var intRe = regexp.MustCompile(`-?(?:0x[0-9a-fA-F]+|\d+)`)

// table returns the integers of `var NAME = ...{ ... }` in src, in order,
// with comments removed; struct elements are flattened field by field.
// transformTypes are transform.go's type constants (RFC 7932 Appendix B).
var transformTypes = map[string]int{
	"transformIdentity": 0, "transformOmitLast1": 1, "transformOmitLast2": 2, "transformOmitLast3": 3,
	"transformOmitLast4": 4, "transformOmitLast5": 5, "transformOmitLast6": 6, "transformOmitLast7": 7,
	"transformOmitLast8": 8, "transformOmitLast9": 9, "transformUppercaseFirst": 10, "transformUppercaseAll": 11,
	"transformOmitFirst1": 12, "transformOmitFirst2": 13, "transformOmitFirst3": 14, "transformOmitFirst4": 15,
	"transformOmitFirst5": 16, "transformOmitFirst6": 17, "transformOmitFirst7": 18, "transformOmitFirst8": 19,
	"transformOmitFirst9": 20, "transformShiftFirst": 21, "transformShiftAll": 22,
}

func table(src, name string) []int64 {
	i := strings.Index(src, "var "+name+" ")
	if i < 0 {
		fail("no var %s", name)
	}
	j := strings.Index(src[i:], "{")
	if j < 0 {
		fail("no { after var %s", name)
	}
	start := i + j
	depth := 0
	end := -1
	for k := start; k < len(src); k++ {
		switch src[k] {
		case '{':
			depth++
		case '}':
			depth--
		}
		if depth == 0 {
			end = k
			break
		}
	}
	if end < 0 {
		fail("unclosed var %s", name)
	}
	body := src[start+1 : end]
	body = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(body, "")
	body = regexp.MustCompile(`//[^\n]*`).ReplaceAllString(body, "")
	body = regexp.MustCompile(`cmdLutElement`).ReplaceAllString(body, "")
	// kTransformsData names its transform types (transformIdentity, ...)
	body = regexp.MustCompile(`\btransform[A-Z]\w*`).ReplaceAllStringFunc(body, func(id string) string {
		v, ok := transformTypes[id]
		if !ok {
			fail("%s: unknown name %s", name, id)
		}
		return strconv.Itoa(v)
	})
	// array types inside the literal ([32]byte{...}) carry lengths, not values
	body = regexp.MustCompile(`\[[^\]]*\]\w+`).ReplaceAllString(body, "")
	var out []int64
	for _, m := range intRe.FindAllString(body, -1) {
		v, err := strconv.ParseInt(m, 0, 64)
		if err != nil {
			// a uint64 table (kZeroRepsBits) above 1<<63 keeps its bits
			u, uerr := strconv.ParseUint(m, 0, 64)
			if uerr != nil {
				fail("%s: %v", name, err)
			}
			v = int64(u)
		}
		out = append(out, v)
	}
	return out
}

// floatTable reads var name's float32 literals, each widened to the
// float64 Go computes with (fastLog2 returns float64(kLog2Table[v])).
func floatTable(src, name string) []string {
	i := strings.Index(src, "var "+name+" ")
	if i < 0 {
		fail("no var %s", name)
	}
	a := i + strings.Index(src[i:], "{")
	b := a + strings.Index(src[a:], "}")
	var out []string
	for _, m := range regexp.MustCompile(`[0-9]+\.[0-9]+`).FindAllString(src[a:b], -1) {
		f, err := strconv.ParseFloat(m, 32)
		if err != nil {
			fail("%s: %v", name, err)
		}
		t := strconv.FormatFloat(float64(float32(f)), 'g', -1, 64)
		if !strings.ContainsAny(t, ".e") {
			t += ".0"
		}
		out = append(out, t)
	}
	return out
}

func asBytes(name string, v []int64) []byte {
	b := make([]byte, len(v))
	for i, x := range v {
		if x < 0 || x > 255 {
			fail("%s[%d] = %d is not a byte", name, i, x)
		}
		b[i] = byte(x)
	}
	return b
}

// le16 writes each value as two little endian bytes.
func le16(name string, v []int64) []byte {
	b := make([]byte, 0, 2*len(v))
	for i, x := range v {
		if x < 0 || x > 65535 {
			fail("%s[%d] = %d is not a uint16", name, i, x)
		}
		b = append(b, byte(x), byte(x>>8))
	}
	return b
}

func intList(v []int64) string {
	s := make([]string, len(v))
	for i, x := range v {
		s[i] = strconv.FormatInt(x, 10)
	}
	return "[" + strings.Join(s, ", ") + "]"
}

func main() {
	if len(os.Args) != 3 {
		fail("usage: go run ./scripts/genbrotli BROTLIDIR FILE")
	}
	dir := os.Args[1]
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			fail("%v", err)
		}
		return string(b)
	}
	dict := read("dictionary.go")
	transform := read("transform.go")
	context := read("context.go")
	prefix := read("prefix_dec.go")
	huffman := read("huffman.go")

	data := asBytes("kBrotliDictionaryData", table(dict, "kBrotliDictionaryData"))
	meta := table(dict, "kBrotliDictionary")
	// size_bits_by_length (32), offsets_by_length (32), data_size
	if len(meta) < 65 || meta[64] != int64(len(data)) {
		fail("kBrotliDictionary: %d values, data_size %v, data %d", len(meta), meta[64:], len(data))
	}
	m := regexp.MustCompile(`(?s)const kPrefixSuffix string = (".*?")\n`).FindStringSubmatch(transform)
	if m == nil {
		fail("no kPrefixSuffix")
	}
	ps, err := strconv.Unquote(strings.ReplaceAll(strings.ReplaceAll(m[1], "\" +\n\t\"", ""), "\" + \"", ""))
	if err != nil {
		fail("kPrefixSuffix: %v", err)
	}
	psMap := table(transform, "kPrefixSuffixMap")
	tdata := asBytes("kTransformsData", table(transform, "kTransformsData"))
	tr := table(transform, "kBrotliTransforms")
	// 217, 121, then the 10 cutoffs (the rest are references)
	if len(tr) < 12 || tr[0] != 217 || tr[1] != 121 {
		fail("kBrotliTransforms: %v", tr)
	}
	cutoffs := tr[len(tr)-10:]
	if len(ps) != 217 || len(psMap) != 50 || len(tdata) != 3*121 {
		fail("transforms: prefix_suffix %d bytes (want 217), map %d (want 50), data %d (want 363)", len(ps), len(psMap), len(tdata))
	}
	ctx := asBytes("kContextLookup", table(context, "kContextLookup"))
	lut := table(prefix, "kCmdLut")
	if len(lut) != 704*6 {
		fail("kCmdLut: %d values, want %d", len(lut), 704*6)
	}
	// 8 bytes an entry: insert extra bits, copy extra bits, distance code
	// + 1 (it is -1 to 3), context, insert offset and copy offset (LE16)
	var cmd []byte
	for i := 0; i < 704; i++ {
		e := lut[i*6 : i*6+6]
		cmd = append(cmd, byte(e[0]), byte(e[1]), byte(e[2]+1), byte(e[3]), byte(e[4]), byte(e[4]>>8), byte(e[5]), byte(e[5]>>8))
	}
	rev := asBytes("kReverseBits", table(huffman, "kReverseBits"))
	maxHuff := table(huffman, "kMaxHuffmanTableSize")

	// the encoder's tables (quality 0 and 1 first, decision 0666)
	fastLog := read("fast_log.go")
	frag := read("compress_fragment.go")
	static := read("entropy_encode_static.go")
	dhash := le16("kStaticDictionaryHash", table(read("dictionary_hash.go"), "kStaticDictionaryHash"))
	dlut := read("static_dict_lut.go")
	dbuckets := le16("kStaticDictionaryBuckets", table(dlut, "kStaticDictionaryBuckets"))
	wv := table(dlut, "kStaticDictionaryWords")
	if len(dbuckets) != 65536 || len(wv) != 3*31705 {
		fail("static dictionary lut: %d buckets bytes, %d word numbers", len(dbuckets), len(wv))
	}
	dwords := make([]byte, 0, 4*31705)
	for i := 0; i < len(wv); i += 3 {
		if wv[i] < 0 || wv[i] > 255 || wv[i+1] < 0 || wv[i+1] > 255 {
			fail("kStaticDictionaryWords[%d] = %v", i/3, wv[i:i+3])
		}
		dwords = append(dwords, byte(wv[i]), byte(wv[i+1]))
		dwords = append(dwords, le16("kStaticDictionaryWords.idx", wv[i+2:i+3])...)
	}
	scd := table(static, "kStaticCommandCodeDepth")
	scb := table(static, "kStaticCommandCodeBits")
	sdd := table(static, "kStaticDistanceCodeDepth")
	sdb := table(static, "kStaticDistanceCodeBits")
	if len(dhash) != 65536 || len(scd) != 704 || len(scb) != 704 || len(sdd) != 64 || len(sdb) != 64 {
		fail("static tables: %d %d %d %d %d", len(dhash), len(scd), len(scb), len(sdd), len(sdb))
	}
	log2 := floatTable(fastLog, "kLog2Table")
	if len(log2) != 256 {
		fail("kLog2Table: %d values, want 256", len(log2))
	}
	seed := table(frag, "kCmdHistoSeed")
	zrb := table(static, "kZeroRepsBits")
	zrd := table(static, "kZeroRepsDepth")
	nzb := table(static, "kNonZeroRepsBits")
	nzd := table(static, "kNonZeroRepsDepth")
	cld := table(static, "kCodeLengthDepth")
	clb := table(static, "kCodeLengthBits")
	if len(seed) != 128 || len(zrb) != 704 || len(zrd) != 704 || len(nzb) != 704 || len(nzd) != 704 || len(cld) != 18 || len(clb) != 18 {
		fail("encoder tables: %d %d %d %d %d %d %d", len(seed), len(zrb), len(zrd), len(nzb), len(nzd), len(cld), len(clb))
	}

	var o strings.Builder
	fmt.Fprintf(&o, "// BEGIN GENERATED\n// From github.com/andybalholm/brotli (scripts/genbrotli): a %d byte dictionary, %d transforms.\n", len(data), tr[1])
	fmt.Fprintf(&o, "const DICT: Bytes = %s\n", bytesLit(data))
	fmt.Fprintf(&o, "const DICT_SIZE_BITS: List[Int] = %s\n", intList(meta[0:32]))
	fmt.Fprintf(&o, "const DICT_OFFSETS: List[Int] = %s\n", intList(meta[32:64]))
	fmt.Fprintf(&o, "const PREFIX_SUFFIX: Bytes = %s\n", bytesLit([]byte(ps)))
	fmt.Fprintf(&o, "const PREFIX_SUFFIX_MAP: List[Int] = %s\n", intList(psMap))
	fmt.Fprintf(&o, "const NUM_TRANSFORMS: Int = %d\n", tr[1])
	fmt.Fprintf(&o, "const TRANSFORMS: Bytes = %s\n", bytesLit(tdata))
	fmt.Fprintf(&o, "const TRANSFORM_CUTOFFS: List[Int] = %s\n", intList(cutoffs))
	fmt.Fprintf(&o, "const CONTEXT_LOOKUP: Bytes = %s\n", bytesLit(ctx))
	fmt.Fprintf(&o, "const CMD_LUT: Bytes = %s\n", bytesLit(cmd))
	fmt.Fprintf(&o, "const REVERSE_BITS: Bytes = %s\n", bytesLit(rev))
	fmt.Fprintf(&o, "const MAX_HUFFMAN_TABLE_SIZE: List[Int] = %s\n", intList(maxHuff))
	fmt.Fprintf(&o, "const ENC_LOG2: List[Float] = [%s]\n", strings.Join(log2, ", "))
	fmt.Fprintf(&o, "const ENC_CMD_HISTO_SEED: List[Int] = %s\n", intList(seed))
	fmt.Fprintf(&o, "const ENC_ZERO_REPS_BITS: List[Int] = %s\n", intList(zrb))
	fmt.Fprintf(&o, "const ENC_ZERO_REPS_DEPTH: List[Int] = %s\n", intList(zrd))
	fmt.Fprintf(&o, "const ENC_NONZERO_REPS_BITS: List[Int] = %s\n", intList(nzb))
	fmt.Fprintf(&o, "const ENC_NONZERO_REPS_DEPTH: List[Int] = %s\n", intList(nzd))
	fmt.Fprintf(&o, "const ENC_CODE_LENGTH_DEPTH: List[Int] = %s\n", intList(cld))
	fmt.Fprintf(&o, "const ENC_CODE_LENGTH_BITS: List[Int] = %s\n", intList(clb))
	fmt.Fprintf(&o, "const ENC_STATIC_CMD_DEPTH: List[Int] = %s\n", intList(scd))
	fmt.Fprintf(&o, "const ENC_STATIC_CMD_BITS: List[Int] = %s\n", intList(scb))
	fmt.Fprintf(&o, "const ENC_STATIC_DIST_DEPTH: List[Int] = %s\n", intList(sdd))
	fmt.Fprintf(&o, "const ENC_STATIC_DIST_BITS: List[Int] = %s\n", intList(sdb))
	fmt.Fprintf(&o, "// kStaticDictionaryHash, 32768 little endian uint16 entries\n")
	fmt.Fprintf(&o, "const ENC_DICT_HASH: Bytes = %s\n", bytesLit(dhash))
	fmt.Fprintf(&o, "// kStaticDictionaryBuckets, 32768 little endian uint16 entries\n")
	fmt.Fprintf(&o, "const ENC_DICT_BUCKETS: Bytes = %s\n", bytesLit(dbuckets))
	fmt.Fprintf(&o, "// kStaticDictionaryWords, 31705 entries of len, transform, idx (little endian uint16)\n")
	fmt.Fprintf(&o, "const ENC_DICT_WORDS: Bytes = %s\n", bytesLit(dwords))
	o.WriteString("// END GENERATED\n")

	file := os.Args[2]
	old, err := os.ReadFile(file)
	if err != nil {
		fail("%v", err)
	}
	s := string(old)
	a := strings.Index(s, "// BEGIN GENERATED\n")
	b := strings.Index(s, "// END GENERATED\n")
	if a < 0 || b < a {
		fail("%s has no BEGIN GENERATED ... END GENERATED block", file)
	}
	s = s[:a] + o.String() + s[b+len("// END GENERATED\n"):]
	if err := os.WriteFile(file, []byte(s), 0o644); err != nil {
		fail("%v", err)
	}
}
