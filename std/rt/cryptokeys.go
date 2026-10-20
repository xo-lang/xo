package rt

import "encoding/pem"

// std/crypto's public key signatures, keys, and PEM (decision 0285): Go's
// packages, as std/native/crypto.xo ports them. Outside bytes become Str
// through textOf (decision 0259).

func (e Crypto_KeyErr) M_display(*Task) string { return e.F_msg }

func Crypto_pem_decode(tk *Task, text string) List[Crypto_PemBlock] {
	var out []Crypto_PemBlock
	rest := []byte(text)
	for {
		b, r := pem.Decode(rest)
		if b == nil {
			break
		}
		var hs Map[string, string]
		for k, v := range b.Headers {
			Map_put(tk, &hs, textOf(k), textOf(v))
		}
		out = append(out, Crypto_PemBlock{F_kind: textOf(b.Type), F_headers: hs, F_bytes: Bytes(b.Bytes)})
		rest = r
	}
	return ListFromSlice(out)
}

func Crypto_pem_encode(_ *Task, b Crypto_PemBlock) Result[string, Crypto_KeyErr] {
	h := map[string]string{}
	for k, v := range Map_All(b.F_headers) {
		h[k] = v
	}
	e := pem.EncodeToMemory(&pem.Block{Type: b.F_kind, Headers: h, Bytes: []byte(b.F_bytes)})
	if e == nil {
		return Err[string](Crypto_KeyErr{F_msg: "pem: cannot encode a header key that contains a colon"})
	}
	return Ok[string, Crypto_KeyErr](string(e))
}
