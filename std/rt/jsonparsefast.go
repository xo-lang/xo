package rt

import (
	"encoding/json"
	"unicode/utf8"
)

// jsonParseFast is jsonParse for the usual text: a hand written reader of
// the same values (*jsonObj, []any, json.Number, string, bool, nil) that
// gives up (ok false) at anything it does not read exactly as the
// token decoder does (an error, an escape it does not handle, a string that
// is not valid UTF-8, a deep nesting), so jsonParse then runs the decoder
// for its value or its error text. json.Decoder.Token costs several
// allocations a token, which made a stream of small values 3.5 times Go's
// instructions (bench/results/json-vs-go-2026-10-21.md).
func jsonParseFast(data []byte) (any, bool) {
	p := jsonFast{b: data}
	p.skip()
	v, ok := p.value(0)
	if !ok {
		return nil, false
	}
	p.skip()
	if p.i != len(p.b) {
		return nil, false
	}
	return v, true
}

const jsonFastDepth = 200

type jsonFast struct {
	b []byte
	i int
}

func (p *jsonFast) skip() {
	for p.i < len(p.b) {
		switch p.b[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *jsonFast) value(depth int) (any, bool) {
	if p.i >= len(p.b) || depth > jsonFastDepth {
		return nil, false
	}
	switch c := p.b[p.i]; {
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == '"':
		s, ok := p.str()
		if !ok {
			return nil, false
		}
		return s, true
	case c == 't':
		if p.lit("true") {
			return true, true
		}
	case c == 'f':
		if p.lit("false") {
			return false, true
		}
	case c == 'n':
		if p.lit("null") {
			return nil, true
		}
	case c == '-' || (c >= '0' && c <= '9'):
		return p.num()
	}
	return nil, false
}

func (p *jsonFast) lit(s string) bool {
	if p.i+len(s) > len(p.b) || string(p.b[p.i:p.i+len(s)]) != s {
		return false
	}
	p.i += len(s)
	// the literal must end at a delimiter
	return p.endsValue()
}

// endsValue: the next byte (if any) cannot continue a value.
func (p *jsonFast) endsValue() bool {
	if p.i >= len(p.b) {
		return true
	}
	switch p.b[p.i] {
	case ' ', '\t', '\n', '\r', ',', ']', '}', ':':
		return true
	}
	return false
}

func (p *jsonFast) num() (any, bool) {
	s := p.i
	i := s
	b := p.b
	if i < len(b) && b[i] == '-' {
		i++
	}
	if i >= len(b) {
		return nil, false
	}
	if b[i] == '0' {
		i++
	} else if b[i] >= '1' && b[i] <= '9' {
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	} else {
		return nil, false
	}
	if i < len(b) && b[i] == '.' {
		i++
		d := i
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
		if i == d {
			return nil, false
		}
	}
	if i < len(b) && (b[i] == 'e' || b[i] == 'E') {
		i++
		if i < len(b) && (b[i] == '+' || b[i] == '-') {
			i++
		}
		d := i
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
		if i == d {
			return nil, false
		}
	}
	p.i = i
	if !p.endsValue() {
		return nil, false
	}
	return json.Number(b[s:i]), true
}

func jsonHex4(b []byte) (rune, bool) {
	if len(b) < 4 {
		return 0, false
	}
	var r rune
	for _, c := range b[:4] {
		switch {
		case c >= '0' && c <= '9':
			r = r<<4 | rune(c-'0')
		case c >= 'a' && c <= 'f':
			r = r<<4 | rune(c-'a'+10)
		case c >= 'A' && c <= 'F':
			r = r<<4 | rune(c-'A'+10)
		default:
			return 0, false
		}
	}
	return r, true
}

// str reads a string at the opening quote.
func (p *jsonFast) str() (string, bool) {
	b := p.b
	i := p.i + 1
	start := i
	// no escape: the bytes between the quotes as they are
	for i < len(b) {
		c := b[i]
		if c == '"' {
			s := b[start:i]
			if !utf8.Valid(s) {
				return "", false
			}
			p.i = i + 1
			return string(s), true
		}
		if c < 0x20 {
			return "", false
		}
		if c == '\\' {
			break
		}
		i++
	}
	if i >= len(b) {
		return "", false
	}
	// with escapes
	out := append([]byte(nil), b[start:i]...)
	for i < len(b) {
		c := b[i]
		switch {
		case c == '"':
			if !utf8.Valid(out) {
				return "", false
			}
			p.i = i + 1
			return string(out), true
		case c < 0x20:
			return "", false
		case c != '\\':
			out = append(out, c)
			i++
		default:
			i++
			if i >= len(b) {
				return "", false
			}
			switch b[i] {
			case '"', '\\', '/':
				out = append(out, b[i])
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'u':
				r, ok := jsonHex4(b[i+1:])
				if !ok || (r >= 0xD800 && r <= 0xDFFF) {
					return "", false
				}
				out = utf8.AppendRune(out, r)
				i += 4
			default:
				return "", false
			}
			i++
		}
	}
	return "", false
}

func (p *jsonFast) object(depth int) (any, bool) {
	p.i++
	obj := &jsonObj{vals: map[string]any{}}
	p.skip()
	if p.i < len(p.b) && p.b[p.i] == '}' {
		p.i++
		return obj, true
	}
	for {
		p.skip()
		if p.i >= len(p.b) || p.b[p.i] != '"' {
			return nil, false
		}
		k, ok := p.str()
		if !ok {
			return nil, false
		}
		p.skip()
		if p.i >= len(p.b) || p.b[p.i] != ':' {
			return nil, false
		}
		p.i++
		p.skip()
		v, ok := p.value(depth + 1)
		if !ok {
			return nil, false
		}
		if _, dup := obj.vals[k]; !dup {
			obj.keys = append(obj.keys, k)
		}
		obj.vals[k] = v
		p.skip()
		if p.i >= len(p.b) {
			return nil, false
		}
		if p.b[p.i] == ',' {
			p.i++
			continue
		}
		if p.b[p.i] == '}' {
			p.i++
			return obj, true
		}
		return nil, false
	}
}

func (p *jsonFast) array(depth int) (any, bool) {
	p.i++
	arr := []any{}
	p.skip()
	if p.i < len(p.b) && p.b[p.i] == ']' {
		p.i++
		return arr, true
	}
	for {
		p.skip()
		v, ok := p.value(depth + 1)
		if !ok {
			return nil, false
		}
		arr = append(arr, v)
		p.skip()
		if p.i >= len(p.b) {
			return nil, false
		}
		if p.b[p.i] == ',' {
			p.i++
			continue
		}
		if p.b[p.i] == ']' {
			p.i++
			return arr, true
		}
		return nil, false
	}
}
