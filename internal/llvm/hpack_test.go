package llvm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// hpackTablesPath is the generated table module of the native runtime's
// HPACK, written in Xo (std/native/llrt/hpack.xo, decision 0193 step 6).
var hpackTablesPath = filepath.Join("..", "..", "std", "native", "llrt", "hpack_tables.xo")

// hpackHuffCode and hpackHuffLen are RFC 7541 Appendix B: the Huffman
// code of each byte and its length in bits (EOS, symbol 256, is
// 0x3fffffff in 30 bits).
var hpackHuffCode = [256]uint32{
	0x1ff8, 0x7fffd8, 0xfffffe2, 0xfffffe3, 0xfffffe4, 0xfffffe5, 0xfffffe6, 0xfffffe7,
	0xfffffe8, 0xffffea, 0x3ffffffc, 0xfffffe9, 0xfffffea, 0x3ffffffd, 0xfffffeb, 0xfffffec,
	0xfffffed, 0xfffffee, 0xfffffef, 0xffffff0, 0xffffff1, 0xffffff2, 0x3ffffffe, 0xffffff3,
	0xffffff4, 0xffffff5, 0xffffff6, 0xffffff7, 0xffffff8, 0xffffff9, 0xffffffa, 0xffffffb,
	0x14, 0x3f8, 0x3f9, 0xffa, 0x1ff9, 0x15, 0xf8, 0x7fa,
	0x3fa, 0x3fb, 0xf9, 0x7fb, 0xfa, 0x16, 0x17, 0x18,
	0x0, 0x1, 0x2, 0x19, 0x1a, 0x1b, 0x1c, 0x1d,
	0x1e, 0x1f, 0x5c, 0xfb, 0x7ffc, 0x20, 0xffb, 0x3fc,
	0x1ffa, 0x21, 0x5d, 0x5e, 0x5f, 0x60, 0x61, 0x62,
	0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6a,
	0x6b, 0x6c, 0x6d, 0x6e, 0x6f, 0x70, 0x71, 0x72,
	0xfc, 0x73, 0xfd, 0x1ffb, 0x7fff0, 0x1ffc, 0x3ffc, 0x22,
	0x7ffd, 0x3, 0x23, 0x4, 0x24, 0x5, 0x25, 0x26,
	0x27, 0x6, 0x74, 0x75, 0x28, 0x29, 0x2a, 0x7,
	0x2b, 0x76, 0x2c, 0x8, 0x9, 0x2d, 0x77, 0x78,
	0x79, 0x7a, 0x7b, 0x7ffe, 0x7fc, 0x3ffd, 0x1ffd, 0xffffffc,
	0xfffe6, 0x3fffd2, 0xfffe7, 0xfffe8, 0x3fffd3, 0x3fffd4, 0x3fffd5, 0x7fffd9,
	0x3fffd6, 0x7fffda, 0x7fffdb, 0x7fffdc, 0x7fffdd, 0x7fffde, 0xffffeb, 0x7fffdf,
	0xffffec, 0xffffed, 0x3fffd7, 0x7fffe0, 0xffffee, 0x7fffe1, 0x7fffe2, 0x7fffe3,
	0x7fffe4, 0x1fffdc, 0x3fffd8, 0x7fffe5, 0x3fffd9, 0x7fffe6, 0x7fffe7, 0xffffef,
	0x3fffda, 0x1fffdd, 0xfffe9, 0x3fffdb, 0x3fffdc, 0x7fffe8, 0x7fffe9, 0x1fffde,
	0x7fffea, 0x3fffdd, 0x3fffde, 0xfffff0, 0x1fffdf, 0x3fffdf, 0x7fffeb, 0x7fffec,
	0x1fffe0, 0x1fffe1, 0x3fffe0, 0x1fffe2, 0x7fffed, 0x3fffe1, 0x7fffee, 0x7fffef,
	0xfffea, 0x3fffe2, 0x3fffe3, 0x3fffe4, 0x7ffff0, 0x3fffe5, 0x3fffe6, 0x7ffff1,
	0x3ffffe0, 0x3ffffe1, 0xfffeb, 0x7fff1, 0x3fffe7, 0x7ffff2, 0x3fffe8, 0x1ffffec,
	0x3ffffe2, 0x3ffffe3, 0x3ffffe4, 0x7ffffde, 0x7ffffdf, 0x3ffffe5, 0xfffff1, 0x1ffffed,
	0x7fff2, 0x1fffe3, 0x3ffffe6, 0x7ffffe0, 0x7ffffe1, 0x3ffffe7, 0x7ffffe2, 0xfffff2,
	0x1fffe4, 0x1fffe5, 0x3ffffe8, 0x3ffffe9, 0xffffffd, 0x7ffffe3, 0x7ffffe4, 0x7ffffe5,
	0xfffec, 0xfffff3, 0xfffed, 0x1fffe6, 0x3fffe9, 0x1fffe7, 0x1fffe8, 0x7ffff3,
	0x3fffea, 0x3fffeb, 0x1ffffee, 0x1ffffef, 0xfffff4, 0xfffff5, 0x3ffffea, 0x7ffff4,
	0x3ffffeb, 0x7ffffe6, 0x3ffffec, 0x3ffffed, 0x7ffffe7, 0x7ffffe8, 0x7ffffe9, 0x7ffffea,
	0x7ffffeb, 0xffffffe, 0x7ffffec, 0x7ffffed, 0x7ffffee, 0x7ffffef, 0x7fffff0, 0x3ffffee,
}

var hpackHuffLen = [256]uint8{
	13, 23, 28, 28, 28, 28, 28, 28, 28, 24, 30, 28, 28, 30, 28, 28,
	28, 28, 28, 28, 28, 28, 30, 28, 28, 28, 28, 28, 28, 28, 28, 28,
	6, 10, 10, 12, 13, 6, 8, 11, 10, 10, 8, 11, 8, 6, 6, 6,
	5, 5, 5, 6, 6, 6, 6, 6, 6, 6, 7, 8, 15, 6, 12, 10,
	13, 6, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 8, 7, 8, 13, 19, 13, 14, 6,
	15, 5, 6, 5, 6, 5, 6, 6, 6, 5, 7, 7, 6, 6, 6, 5,
	6, 7, 6, 5, 5, 6, 7, 7, 7, 7, 7, 15, 11, 14, 13, 28,
	20, 22, 20, 20, 22, 22, 22, 23, 22, 23, 23, 23, 23, 23, 24, 23,
	24, 24, 22, 23, 24, 23, 23, 23, 23, 21, 22, 23, 22, 23, 23, 24,
	22, 21, 20, 22, 22, 23, 23, 21, 23, 22, 22, 24, 21, 22, 23, 23,
	21, 21, 22, 21, 23, 22, 23, 23, 20, 22, 22, 22, 23, 22, 22, 23,
	26, 26, 20, 19, 22, 23, 22, 25, 26, 26, 26, 27, 27, 26, 24, 25,
	19, 21, 26, 27, 27, 26, 27, 24, 21, 21, 26, 26, 28, 27, 27, 27,
	20, 24, 20, 21, 22, 21, 21, 23, 22, 22, 25, 25, 24, 24, 26, 23,
	26, 27, 26, 26, 27, 27, 27, 27, 27, 28, 27, 27, 27, 27, 27, 26,
}

// hpackStatic is RFC 7541 Appendix A: the static table (index 1 first).
var hpackStatic = [61][2]string{
	{":authority", ""},
	{":method", "GET"},
	{":method", "POST"},
	{":path", "/"},
	{":path", "/index.html"},
	{":scheme", "http"},
	{":scheme", "https"},
	{":status", "200"},
	{":status", "204"},
	{":status", "206"},
	{":status", "304"},
	{":status", "400"},
	{":status", "404"},
	{":status", "500"},
	{"accept-charset", ""},
	{"accept-encoding", "gzip, deflate"},
	{"accept-language", ""},
	{"accept-ranges", ""},
	{"accept", ""},
	{"access-control-allow-origin", ""},
	{"age", ""},
	{"allow", ""},
	{"authorization", ""},
	{"cache-control", ""},
	{"content-disposition", ""},
	{"content-encoding", ""},
	{"content-language", ""},
	{"content-length", ""},
	{"content-location", ""},
	{"content-range", ""},
	{"content-type", ""},
	{"cookie", ""},
	{"date", ""},
	{"etag", ""},
	{"expect", ""},
	{"expires", ""},
	{"from", ""},
	{"host", ""},
	{"if-match", ""},
	{"if-modified-since", ""},
	{"if-none-match", ""},
	{"if-range", ""},
	{"if-unmodified-since", ""},
	{"last-modified", ""},
	{"link", ""},
	{"location", ""},
	{"max-forwards", ""},
	{"proxy-authenticate", ""},
	{"proxy-authorization", ""},
	{"range", ""},
	{"referer", ""},
	{"refresh", ""},
	{"retry-after", ""},
	{"server", ""},
	{"set-cookie", ""},
	{"strict-transport-security", ""},
	{"transfer-encoding", ""},
	{"user-agent", ""},
	{"vary", ""},
	{"via", ""},
	{"www-authenticate", ""},
}

// hpackDecodeFSM is the Huffman decoder of hpack.xo as a state machine
// over nibbles: the states are the 256 inner nodes of the code tree (0 is
// the root). Entry 4*(16*s+x) is the step from state s on the nibble x
// (high bit first): the next state, the symbol completed on the way (at
// most one: every code is at least 5 bits), and flags: 1 a symbol was
// completed, 2 the bits reached EOS (an error, as hpack.huffmanDecode),
// 4 the next state may end the string (its path from the root is at most
// 7 one bits: the padding RFC 7541 5.2 allows).
func hpackDecodeFSM() []byte {
	// the tree: a child is an inner node (1 and up) or -(sym+1)
	type node struct{ child [2]int }
	nodes := []node{{}}
	add := func(code uint32, n int, sym int) {
		cur := 0
		for b := n - 1; b >= 0; b-- {
			bit := int(code>>b) & 1
			if b == 0 {
				nodes[cur].child[bit] = -(sym + 1)
				return
			}
			if nodes[cur].child[bit] == 0 {
				nodes = append(nodes, node{})
				nodes[cur].child[bit] = len(nodes) - 1
			}
			cur = nodes[cur].child[bit]
		}
	}
	for s := range 256 {
		add(hpackHuffCode[s], int(hpackHuffLen[s]), s)
	}
	add(0x3fffffff, 30, 256)
	if len(nodes) != 256 {
		panic(fmt.Sprintf("hpack tree has %d inner nodes", len(nodes)))
	}
	// the states that may end a string: the all ones path, depth <= 7
	accept := make([]bool, 256)
	cur := 0
	for d := 0; d <= 7; d++ {
		accept[cur] = true
		cur = nodes[cur].child[1]
	}
	out := make([]byte, 0, 256*16*4)
	for s := range 256 {
		for x := range 16 {
			cur, sym, flags := s, 0, 0
			for b := 3; b >= 0; b-- {
				nx := nodes[cur].child[(x>>b)&1]
				if nx < 0 {
					if -nx-1 == 256 {
						flags |= 2
						cur = 0
						break
					}
					sym = -nx - 1
					flags |= 1
					cur = 0
				} else {
					cur = nx
				}
			}
			if flags&2 == 0 && accept[cur] {
				flags |= 4
			}
			out = append(out, byte(cur), byte(sym), byte(flags), 0)
		}
	}
	// padding, so the last step reads as a word (rt.bytes_word)
	return append(out, 0, 0, 0, 0, 0, 0, 0, 0)
}

// hpackTables is the text of std/native/llrt/hpack_tables.xo.
func hpackTables() []byte {
	var b bytes.Buffer
	b.WriteString("// Code generated by internal/llvm TestHPACKTables -golden-update from\n")
	b.WriteString("// RFC 7541 Appendices A and B. DO NOT EDIT.\n")
	b.WriteString("//\n")
	b.WriteString("// The tables of std/native/llrt/hpack.xo (decision 0193 step 6).\n\n")
	var code []byte
	for _, c := range hpackHuffCode {
		code = binary.LittleEndian.AppendUint32(code, c)
	}
	b.WriteString("/// The Huffman code of each byte (RFC 7541 Appendix B), 4 bytes each,\n")
	b.WriteString("/// little endian.\n")
	fmt.Fprintf(&b, "pub const HUFF_CODE: Bytes = %s\n\n", xoBytesLit(code))
	b.WriteString("/// The length in bits of the Huffman code of each byte.\n")
	fmt.Fprintf(&b, "pub const HUFF_LEN: Bytes = %s\n\n", xoBytesLit(hpackHuffLen[:]))
	b.WriteString("/// The Huffman decoder as a state machine over nibbles, 4 bytes a step:\n")
	b.WriteString("/// at 4 * (16 * state + nibble) the next state, the symbol completed,\n")
	b.WriteString("/// and flags (1 a symbol, 2 EOS: invalid, 4 the next state may end the\n")
	b.WriteString("/// string); 8 bytes of padding follow, so each step is one word.\n")
	fmt.Fprintf(&b, "pub const HUFF_DEC: Bytes = %s\n\n", xoBytesLit(hpackDecodeFSM()))
	var text, at []byte
	at = append(at, 0, 0, 0, 0)
	for _, e := range hpackStatic {
		at = binary.LittleEndian.AppendUint16(at, uint16(len(text)))
		at = append(at, byte(len(e[0])), byte(len(e[1])))
		text = append(text, e[0]...)
		text = append(text, e[1]...)
	}
	at = append(at, 0, 0, 0, 0) // padding: entry 61 reads as a word
	b.WriteString("/// The names and values of the static table (RFC 7541 Appendix A), one\n")
	b.WriteString("/// after the other.\n")
	fmt.Fprintf(&b, "pub const STATIC_TEXT: Bytes = %s\n\n", xoBytesLit(text))
	b.WriteString("/// Entry k (1 to 61) of the static table at 4 * k: the offset of its\n")
	b.WriteString("/// name in STATIC_TEXT (2 bytes, little endian), the length of the name,\n")
	b.WriteString("/// the length of the value (the value follows the name); 4 bytes\n")
	b.WriteString("/// of padding follow.\n")
	fmt.Fprintf(&b, "pub const STATIC_AT: Bytes = %s\n\n", xoBytesLit(at))
	// the entries by the length of their names (the encoder's lookup)
	var order []byte
	bylen := make([]byte, 33)
	for l := 0; l < 32; l++ {
		bylen[l] = byte(len(order))
		for k, e := range hpackStatic {
			if len(e[0]) == l {
				order = append(order, byte(k+1))
			}
		}
	}
	bylen[32] = byte(len(order))
	b.WriteString("/// The static table's entries ordered by the length of their names,\n")
	b.WriteString("/// then by index.\n")
	fmt.Fprintf(&b, "pub const STATIC_ORDER: Bytes = %s\n\n", xoBytesLit(order))
	b.WriteString("/// Where the names of each length (0 to 31) start in STATIC_ORDER;\n")
	b.WriteString("/// entry 32 is the end.\n")
	fmt.Fprintf(&b, "pub const STATIC_BYLEN: Bytes = %s\n", xoBytesLit(bylen))
	return b.Bytes()
}

// TestHPACKTables checks that std/native/llrt/hpack_tables.xo is current
// (-golden-update rewrites it).
func TestHPACKTables(t *testing.T) {
	want := hpackTables()
	if *updateGolden {
		if err := os.WriteFile(hpackTablesPath, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(hpackTablesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is stale: run go test ./internal/llvm -run TestHPACKTables -golden-update", hpackTablesPath)
	}
}
