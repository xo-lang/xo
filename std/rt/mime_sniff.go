package rt

import (
	"bytes"
	"encoding/binary"
)

// Content sniffing for std/mime's detect_content_type (decision 0282):
// net/http's sniff.go of go1.26, copied (BSD, THIRD_PARTY_NOTICES) so
// that std/rt does not import net/http (programs built with xo_nohttp).

const mimeSniffLen = 512

// mimeDetect implements the algorithm described
// at https://mimesniff.spec.whatwg.org/ to determine the
// Content-Type of the given data. It considers at most the
// first 512 bytes of data. mimeDetect always returns
// a valid MIME type: if it cannot determine a more specific one, it
// returns "application/octet-stream".
func mimeDetect(data []byte) string {
	if len(data) > mimeSniffLen {
		data = data[:mimeSniffLen]
	}

	// Index of the first non-whitespace byte in data.
	firstNonWS := 0
	for ; firstNonWS < len(data) && mimeIsWS(data[firstNonWS]); firstNonWS++ {
	}

	for _, sig := range mimeSniffSigs {
		if ct := sig.match(data, firstNonWS); ct != "" {
			return ct
		}
	}

	return "application/octet-stream" // fallback
}

// isWS reports whether the provided byte is a whitespace byte (0xWS)
// as defined in https://mimesniff.spec.whatwg.org/#terminology.
func mimeIsWS(b byte) bool {
	switch b {
	case '\t', '\n', '\x0c', '\r', ' ':
		return true
	}
	return false
}

// isTT reports whether the provided byte is a tag-terminating byte (0xTT)
// as defined in https://mimesniff.spec.whatwg.org/#terminology.
func mimeIsTT(b byte) bool {
	switch b {
	case ' ', '>':
		return true
	}
	return false
}

type mimeSniffSig interface {
	// match returns the MIME type of the data, or "" if unknown.
	match(data []byte, firstNonWS int) string
}

// Data matching the table in section 6.
var mimeSniffSigs = []mimeSniffSig{
	mimeHTMLSig("<!DOCTYPE HTML"),
	mimeHTMLSig("<HTML"),
	mimeHTMLSig("<HEAD"),
	mimeHTMLSig("<SCRIPT"),
	mimeHTMLSig("<IFRAME"),
	mimeHTMLSig("<H1"),
	mimeHTMLSig("<DIV"),
	mimeHTMLSig("<FONT"),
	mimeHTMLSig("<TABLE"),
	mimeHTMLSig("<A"),
	mimeHTMLSig("<STYLE"),
	mimeHTMLSig("<TITLE"),
	mimeHTMLSig("<B"),
	mimeHTMLSig("<BODY"),
	mimeHTMLSig("<BR"),
	mimeHTMLSig("<P"),
	mimeHTMLSig("<!--"),
	&mimeMaskedSig{
		mask:   []byte("\xFF\xFF\xFF\xFF\xFF"),
		pat:    []byte("<?xml"),
		skipWS: true,
		ct:     "text/xml; charset=utf-8"},
	&mimeExactSig{[]byte("%PDF-"), "application/pdf"},
	&mimeExactSig{[]byte("%!PS-Adobe-"), "application/postscript"},

	// UTF BOMs.
	&mimeMaskedSig{
		mask: []byte("\xFF\xFF\x00\x00"),
		pat:  []byte("\xFE\xFF\x00\x00"),
		ct:   "text/plain; charset=utf-16be",
	},
	&mimeMaskedSig{
		mask: []byte("\xFF\xFF\x00\x00"),
		pat:  []byte("\xFF\xFE\x00\x00"),
		ct:   "text/plain; charset=utf-16le",
	},
	&mimeMaskedSig{
		mask: []byte("\xFF\xFF\xFF\x00"),
		pat:  []byte("\xEF\xBB\xBF\x00"),
		ct:   "text/plain; charset=utf-8",
	},

	// Image types
	// For posterity, we originally returned "image/vnd.microsoft.icon" from
	// https://tools.ietf.org/html/draft-ietf-websec-mime-sniff-03#section-7
	// https://codereview.appspot.com/4746042
	// but that has since been replaced with "image/x-icon" in Section 6.2
	// of https://mimesniff.spec.whatwg.org/#matching-an-image-type-pattern
	&mimeExactSig{[]byte("\x00\x00\x01\x00"), "image/x-icon"},
	&mimeExactSig{[]byte("\x00\x00\x02\x00"), "image/x-icon"},
	&mimeExactSig{[]byte("BM"), "image/bmp"},
	&mimeExactSig{[]byte("GIF87a"), "image/gif"},
	&mimeExactSig{[]byte("GIF89a"), "image/gif"},
	&mimeMaskedSig{
		mask: []byte("\xFF\xFF\xFF\xFF\x00\x00\x00\x00\xFF\xFF\xFF\xFF\xFF\xFF"),
		pat:  []byte("RIFF\x00\x00\x00\x00WEBPVP"),
		ct:   "image/webp",
	},
	&mimeExactSig{[]byte("\x89PNG\x0D\x0A\x1A\x0A"), "image/png"},
	&mimeExactSig{[]byte("\xFF\xD8\xFF"), "image/jpeg"},

	// Audio and Video types
	// Enforce the pattern match ordering as prescribed in
	// https://mimesniff.spec.whatwg.org/#matching-an-audio-or-video-type-pattern
	&mimeMaskedSig{
		mask: []byte("\xFF\xFF\xFF\xFF\x00\x00\x00\x00\xFF\xFF\xFF\xFF"),
		pat:  []byte("FORM\x00\x00\x00\x00AIFF"),
		ct:   "audio/aiff",
	},
	&mimeMaskedSig{
		mask: []byte("\xFF\xFF\xFF"),
		pat:  []byte("ID3"),
		ct:   "audio/mpeg",
	},
	&mimeMaskedSig{
		mask: []byte("\xFF\xFF\xFF\xFF\xFF"),
		pat:  []byte("OggS\x00"),
		ct:   "application/ogg",
	},
	&mimeMaskedSig{
		mask: []byte("\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xFF"),
		pat:  []byte("MThd\x00\x00\x00\x06"),
		ct:   "audio/midi",
	},
	&mimeMaskedSig{
		mask: []byte("\xFF\xFF\xFF\xFF\x00\x00\x00\x00\xFF\xFF\xFF\xFF"),
		pat:  []byte("RIFF\x00\x00\x00\x00AVI "),
		ct:   "video/avi",
	},
	&mimeMaskedSig{
		mask: []byte("\xFF\xFF\xFF\xFF\x00\x00\x00\x00\xFF\xFF\xFF\xFF"),
		pat:  []byte("RIFF\x00\x00\x00\x00WAVE"),
		ct:   "audio/wave",
	},
	// 6.2.0.2. video/mimeMP4
	mimeMP4Sig{},
	// 6.2.0.3. video/webm
	&mimeExactSig{[]byte("\x1A\x45\xDF\xA3"), "video/webm"},

	// Font types
	&mimeMaskedSig{
		// 34 NULL bytes followed by the string "LP"
		pat: []byte("\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00LP"),
		// 34 NULL bytes followed by \xF\xF
		mask: []byte("\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xFF\xFF"),
		ct:   "application/vnd.ms-fontobject",
	},
	&mimeExactSig{[]byte("\x00\x01\x00\x00"), "font/ttf"},
	&mimeExactSig{[]byte("OTTO"), "font/otf"},
	&mimeExactSig{[]byte("ttcf"), "font/collection"},
	&mimeExactSig{[]byte("wOFF"), "font/woff"},
	&mimeExactSig{[]byte("wOF2"), "font/woff2"},

	// Archive types
	&mimeExactSig{[]byte("\x1F\x8B\x08"), "application/x-gzip"},
	&mimeExactSig{[]byte("PK\x03\x04"), "application/zip"},
	// RAR's signatures are incorrectly defined by the MIME spec as per
	//    https://github.com/whatwg/mimesniff/issues/63
	// However, RAR Labs correctly defines it at:
	//    https://www.rarlab.com/technote.htm#rarsign
	// so we use the definition from RAR Labs.
	// TODO: do whatever the spec ends up doing.
	&mimeExactSig{[]byte("Rar!\x1A\x07\x00"), "application/x-rar-compressed"},     // RAR v1.5-v4.0
	&mimeExactSig{[]byte("Rar!\x1A\x07\x01\x00"), "application/x-rar-compressed"}, // RAR v5+

	&mimeExactSig{[]byte("\x00\x61\x73\x6D"), "application/wasm"},

	mimeTextSig{}, // should be last
}

type mimeExactSig struct {
	sig []byte
	ct  string
}

func (e *mimeExactSig) match(data []byte, firstNonWS int) string {
	if bytes.HasPrefix(data, e.sig) {
		return e.ct
	}
	return ""
}

type mimeMaskedSig struct {
	mask, pat []byte
	skipWS    bool
	ct        string
}

func (m *mimeMaskedSig) match(data []byte, firstNonWS int) string {
	// pattern matching algorithm section 6
	// https://mimesniff.spec.whatwg.org/#pattern-matching-algorithm

	if m.skipWS {
		data = data[firstNonWS:]
	}
	if len(m.pat) != len(m.mask) {
		return ""
	}
	if len(data) < len(m.pat) {
		return ""
	}
	for i, pb := range m.pat {
		maskedData := data[i] & m.mask[i]
		if maskedData != pb {
			return ""
		}
	}
	return m.ct
}

type mimeHTMLSig []byte

func (h mimeHTMLSig) match(data []byte, firstNonWS int) string {
	data = data[firstNonWS:]
	if len(data) < len(h)+1 {
		return ""
	}
	for i, b := range h {
		db := data[i]
		if 'A' <= b && b <= 'Z' {
			db &= 0xDF
		}
		if b != db {
			return ""
		}
	}
	// Next byte must be a tag-terminating byte(0xTT).
	if !mimeIsTT(data[len(h)]) {
		return ""
	}
	return "text/html; charset=utf-8"
}

var mimeMP4Ftype = []byte("ftyp")
var mimeMP4 = []byte("mimeMP4")

type mimeMP4Sig struct{}

func (mimeMP4Sig) match(data []byte, firstNonWS int) string {
	// https://mimesniff.spec.whatwg.org/#signature-for-mimeMP4
	// c.f. section 6.2.1
	if len(data) < 12 {
		return ""
	}
	boxSize := int(binary.BigEndian.Uint32(data[:4]))
	if len(data) < boxSize || boxSize%4 != 0 {
		return ""
	}
	if !bytes.Equal(data[4:8], mimeMP4Ftype) {
		return ""
	}
	for st := 8; st < boxSize; st += 4 {
		if st == 12 {
			// Ignores the four bytes that correspond to the version number of the "major brand".
			continue
		}
		if bytes.Equal(data[st:st+3], mimeMP4) {
			return "video/mimeMP4"
		}
	}
	return ""
}

type mimeTextSig struct{}

func (mimeTextSig) match(data []byte, firstNonWS int) string {
	// c.f. section 5, step 4.
	for _, b := range data[firstNonWS:] {
		switch {
		case b <= 0x08,
			b == 0x0B,
			0x0E <= b && b <= 0x1A,
			0x1C <= b && b <= 0x1F:
			return ""
		}
	}
	return "text/plain; charset=utf-8"
}
