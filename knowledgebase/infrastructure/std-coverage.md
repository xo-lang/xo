# Std coverage against Go's standard library

Decision 0276: Go's standard library is std's reference scope. This
table maps each Go package (go1.26, `go list std` without internal and
vendor packages) to the Xo module that covers it, with a status:

- **done**: the module covers what a backend uses of the package, on
  the native backends and the Go backend.
- **partial**: some of it; the row names what is missing.
- **planned**: nothing yet, wanted (ranked by the survey below).
- **not planned**: one line says why.

"Prelude" means the builtin types and capabilities every module sees
(spec/std.md sections 1 to 3). Spec sections are spec/std.md's.

Last verified: 2026-10-29 (main c21a17a84, and main edb47f207 for the
html, html/template, crypto/aes, crypto/hkdf, x/crypto rows and survey
gaps 4, 5, 15 and 16; spec/std.md, `xo doc` of the
prelude types, `xo symbols --module=std/... --pub` of every module, the
partial and planned rows rechecked; done rows from 2026-10-20 stand).

## Modules and change logs

Each module's change log is `std/changes/MODULE.md` (decision 0276 item
6); a module without a file has no entries yet.

| Module | Spec | Change log |
|---|---|---|
| prelude (Str, Bytes, List, Map, Fs, Net, Clock, Rand, Env, Stdio, Proc) | 1 to 3 | `std/changes/prelude.md` |
| std/http | 4 | `std/changes/http.md` |
| std/json | 5 | `std/changes/json.md` |
| std/log | 6 | `std/changes/log.md` |
| std/task | 7 | `std/changes/task.md` |
| std/testing | 8 | `std/changes/testing.md` |
| std/path | 9 | `std/changes/path.md` |
| std/sql | 10 | `std/changes/sql.md` |
| std/sqlite | 65 | `std/changes/sqlite.md` |
| std/crypto | 11 | `std/changes/crypto.md` |
| std/go | 12 | `std/changes/go.md` |
| std/time | 13 | `std/changes/time.md` |
| std/cli | 14 | `std/changes/cli.md` |
| std/term | 15 | `std/changes/term.md` |
| std/csv | 16 | `std/changes/csv.md` |
| std/regexp | 17 | `std/changes/regexp.md` |
| std/archive | 18 | `std/changes/archive.md` |
| std/toml | 19 | `std/changes/toml.md` |
| std/url | 20 | `std/changes/url.md` |
| std/base64, std/base32 | 21 | `std/changes/base64.md`, `std/changes/base32.md` |
| std/binary | 22 | `std/changes/binary.md` |
| std/tls | 27 | `std/changes/tls.md` |
| std/mime | 23 | `std/changes/mime.md` |
| std/uuid | 24 | `std/changes/uuid.md` |
| std/mail | 25 | `std/changes/mail.md` |
| std/multipart | 26 | `std/changes/multipart.md` |
| std/smtp | 29 | `std/changes/smtp.md` |
| std/netip | 28 | `std/changes/netip.md` |
| std/template | 30 | `std/changes/template.md` |
| std/embed | 31 | `std/changes/embed.md` |
| std/compress | 32 | `std/changes/compress.md` |
| std/big | 33 | `std/changes/big.md` |
| std/xml | 34 | `std/changes/xml.md`, `std/changes/xmlcodec.md` |
| std/yaml | 63 | `std/changes/yaml.md` |
| std/websocket | 35 | `std/changes/websocket.md` |
| std/debug | 36 | `std/changes/debug.md` |
| std/metrics | 37 | `std/changes/metrics.md` |
| std/trace | 38 | `std/changes/trace.md` |
| std/html | 39 | `std/changes/html.md` |
| std/tabwriter | 40 | `std/changes/tabwriter.md` |
| std/x509 | 41 | `std/changes/x509.md` |
| std/rate | 42 | `std/changes/rate.md` |
| std/backoff | 43 | `std/changes/backoff.md` |

## Go packages

### Text, strings, and encodings

| Go package | Xo | Status | Notes |
|---|---|---|---|
| strings | prelude Str | done | contains, find, rfind, count, fields, split, split_n, split_after(_n), cut, cut_prefix, cut_suffix, equal_fold, find_any, rfind_any, contains_any, contains_rune, the `_where` forms (IndexFunc, FieldsFunc, TrimFunc...), trim_chars (Trim, TrimLeft, TrimRight), replace_n, map_runes (Map), repeat, lines, to_lower/upper, join, StrBuilder (2026-10-29). Not offered: Replacer (one pass over several replacements), Title (deprecated in Go). |
| bytes | prelude Bytes, BytesBuilder | done | Str's methods on Bytes: find, rfind, count, contains (any, rune, where), split (n, after), cut, fields, trim (prefix, suffix, where), replace(_n), to_lower/upper, equal_fold, starts_with, ends_with; `==` is Equal; hex, base64, decode_rune, zeroed; BytesBuilder with put_le and put_be (decision 0399) (2026-10-29). Missing: a Reader (streams are per handle). |
| strconv | prelude Int.parse, parse_base, to_str_base, Float.parse, Bool.parse, quote, unquote | done | ParseInt, FormatInt, ParseBool, Unquote added (decision 0287). Not planned: the Append functions (StrBuilder covers them). |
| unicode | prelude Rune, std/uniclass (std/regexp's \p classes, internal) | done | is_letter, is_digit, is_number, is_space, is_upper, is_lower, is_title, is_punct, is_control, is_graphic, is_mark, is_symbol, is_print, to_lower, to_upper, to_title (Go's tables, decision 0079) (2026-10-29). Not offered: range tables by name (unicode.In, unicode.Han). |
| unicode/utf8 | prelude Str, Bytes | done | rune_count, decode_rune, UTF-8 validity rules (decision 0259). |
| unicode/utf16 | std/charset | done | `utf16_units` and `from_utf16_units` (spec/std.md 54, decision 0470); the one-rune steps `utf16_append_rune`, `utf16_encode_rune`, `utf16_decode_rune`, `utf16_is_surrogate`, `utf16_rune_len` (2026-11-15). |
| x/net idna | std/idna | done | to_ascii and to_unicode on the Lookup, Registration, Display and Punycode profiles, UTS 46, punycode, joiner, hyphen and Bidi rules, Unicode 15.0.0 tables generated from x/net (spec/std.md 61, decision 0476). |
| x/text unicode/norm | std/norm | done | NFC, NFD, NFKC, NFKD and is_normalized, Unicode 15.0.0 tables generated by scripts/gennorm (decision 0476); no streaming Reader or Writer, no IDNA. |
| x/text encoding/unicode, charmap | std/charset | done | UTF-16 (LE, BE, BOM), Latin-1 and Windows-1252 decode and encode, WHATWG labels (decision 0470). |
| encoding/json | std/json | done | `derive Json`, Patch, typed decode with paths, wire key names (`kind as "type"` and `derive Json(case: ...)`, decision 0277), json.Value for JSON of any shape (2026-10-21) and as a field or argument (Go's `any` and RawMessage, decision 0408 S3), `as "key,omitempty"`, the push-style json.Decoder of value streams (decision 0309), `encode_indent` (MarshalIndent), `indent`, `compact`, `html_escape` (2026-11-12), numbers as Int or Num (decision 0299). The stdaudit of 2026-11-12 lists no gap. |
| encoding/base64 | std/base64, prelude Bytes | done | Std, Url, RawStd, RawUrl with Go's decoding rules (decision 0279); custom alphabets, padding and Strict (2026-11-15). |
| encoding/base32 | std/base32 | done | Std, Hex, RawStd, RawHex (decision 0279). |
| encoding/hex | prelude Bytes | done | to_hex, from_hex, the same as EncodeToString and DecodeString on 1,500 strings (2026-10-20). Dump is not offered. |
| encoding/binary | std/binary | done | Byte orders on Bytes and varints (decision 0279); no Read and Write of structs (no reflection). |
| encoding/csv | std/csv | done | Native port (decision 0164); `parse` in memory, and `csv.reader(file)` streams records from a File (2026-11-03). Records match Go's Reader (FieldsPerRecord -1) on 2,022 texts (2026-10-20); Go's options, error texts, and the Reader's `field_pos` and `input_offset` (FieldPos, InputOffset, 2026-11-15) match Go on the corpus's 2,812 texts. |
| encoding/pem | std/crypto | done | pem_decode, pem_encode (decision 0285). |
| encoding/asn1 | std/asn1 (std/native/llrt/asn1.xo for X.509) | done | `parse`, `parse_prefix` and `Value.der` over a Value tree with Go's DER rules (decision 0601 stage 1). Missing: struct marshaling (`derive Asn1`, stage 2). Last verified: 2026-11-15. |
| encoding/xml | std/xml, std/xmlcodec (derive Xml) | done | Go's Decoder tokens (strict, HTML-lenient, raw), an element tree, and the Encoder, on every backend (decision 0314, 2026-10-21); Marshal and Unmarshal of structs (derive Xml, `encode_value` and `decode`); streaming over a source (`stream`, NewDecoder over an io.Reader, 2026-11-16). |
| encoding/gob | none | not planned | Go specific wire format; Xo programs exchange JSON. |
| encoding/ascii85 | none | not planned | No backend use (PostScript and PDF). |
| encoding | none | not planned | Interfaces only; Xo uses derive (Json, SqlRow). |
| mime | std/mime | done | ParseMediaType, FormatMediaType, TypeByExtension and ExtensionsByType (Go's builtin table only, no system files), and net/http's DetectContentType (decision 0282); RFC 2047 encoded words (`encode_word` for WordEncoder.Encode with `WordEncoding.Q` and `B`, `decode_word` and `decode_header` for WordDecoder, `decode_word_with` and `decode_header_with` for its CharsetReader; corpus internal/stdtest/testdata/corpus/mime). Not planned: AddExtensionType (no process-wide mutable state). |
| mime/multipart | std/multipart | done | Reader (NextPart, ReadForm), Writer, Request.multipart_form, in memory (decision 0282); a streamed body's parts as they arrive (`reader`, decision 0597; `http.BodyReader.multipart`); the streaming `Writer` (NewWriter, SetBoundary, CreatePart, CreateFormField, CreateFormFile, WriteField, Close, FileContentDisposition) for any subtype, multipart/mixed responses included (#109, Go's writer_test.go). Not planned: Form.RemoveAll (no temporary files). |
| mime/quotedprintable | std/mime | done | `qp_decode` (Reader) and `qp_encode` (Writer, its Binary mode as `binary: true`), Go's reader and writer tests and corpus internal/stdtest/testdata/corpus/qp; std/multipart and std/mail keep their own internal codecs. |
| html | std/html, std/htmlent (the entity tables, internal) | done | escape_string and unescape_string, Go's html on its escape_test.go cases, the entity table generated from Go's entity.go (decision 0408, 2026-10-29). |
| protobuf descriptors | std/protodesc | done | descriptor.proto's file, message, field, enum and service descriptors with a hand written codec, byte exact on protobuf-go's and grpc-go's descriptors (spec/std.md 72, decision 0546). Built from std/protoparse as protoc does (`protodesc.build`: scoping, map entries, synthetic oneofs, json_name, groups, extend, ranges, descriptor.proto's options), byte for byte against protoc 36.2 on 47 files (scripts/protocdesc, #109, 2026-11-16). Not planned: generated code (decision 0549). |
| protobuf well known types | std/protowkt | done | Timestamp, Duration, Empty, FieldMask, Any, the wrappers, Struct, Value, ListValue and codecs, byte exact against protobuf-go (spec/std.md 73, decision 0546); the types/known helpers (New, AsTime, AsDuration, CheckValid, Any packing and MessageIs, field mask Union, Intersect, Normalize, structpb to and from json.Value; task #102); field mask New, IsValid and Append against a std/protodesc message (`new_mask`, `is_valid`, `append`, #109). Not planned: generated code (decision 0549). |
| x/net html (Tokenizer) | std/htmltok | done | Go's Tokenizer on a text in memory: tokens, raw text elements, comments, doctypes, CDATA on request, attributes, entities, buffer limit, differential against x/net v0.62.0 (spec/std.md 71, decision 0548, 2026-11-07). Timing 2026-11-07: 1.8x Go's Tokenizer (next), 1.4x (next plus token), bench/perf/htmltok. Text, TagName, TagAttr and the String methods (2026-11-16; corpus API table, 596 streams). The parser and tree are std/htmlparse. Not offered: an io.Reader source (the tokenizer reads a text in memory), the atom package (names are Str). |
| x/net html (Parse, Render) | std/htmlparse | done | Go's tree builder on a text in memory: Parse, ParseFragment with a context element, scripting option, Render, the html5lib tree-construction files and Go's own, differential against x/net v0.62.0 (spec/std.md 75, decision 0570, 2026-11-08). Timing 2026-11-08 (host 1): Parse 1.65x x/net, Render 1.15x, bench/perf/htmlparse; flat profile, tokenizer 55%. Tree editing (InsertBefore, AppendChild, RemoveChild) and walking (Ancestors, ChildNodes, Descendants) on node indexes (2026-11-16, Go's iter_test.go ported). Not offered: an io.Reader source (a text in memory). |
| protobuf text format | std/prototext | done | the writer (Marshal, Format and their options) and the reader (Unmarshal and its options, 2026-11-17) against protobuf-go prototext (spec/std.md 74.1, decision 0683 item 2); extension fields wait for std/protojson's (task #228). |
| protobuf JSON | std/protojson | done | descriptor driven marshal and unmarshal with the well known types, against protobuf-go protojson (spec/std.md 74, decision 0546). Fuzzed: TestProtojsonFuzz (fresh seeds of the corpus generator, two in five unmarshal inputs mutated; seeds 2 to 4 pass on arm64 and LLVM, 2026-11-16). Timing: the Go twin bench/perf/protojson (host 1 row with task #87). |
| html/template | std/template | done | `template.parse_html`: Go's contextual autoescaping, trusted content as `json.Value.Trusted` (`template.html`, `attr`; Go's template.HTML, HTMLAttr), template functions (decision 0408, 2026-10-29); the html/template symbols in scripts/stdaudit/map.txt map with no gap (#109, 2026-11-16). Not offered, as text/template's row: Clone and Lookup, ParseFS; not planned (map.txt): the escaper error codes, typed JS and CSS safe strings, parse tree access. |
| text/template | std/template | done | Go's text/template over json.Value data (decision 0299). Template functions (`funcs: Map[Str, TemplateFn]`, Go's FuncMap; decision 0408). Not offered: Clone and Lookup, ParseFS. |
| text/tabwriter | std/tabwriter (decision 0410) | done | Elastic tabstops with Go's flags and escapes; the native backends run an Xo port of Go's code and tests, the Go backend Go's own Writer; `flush` returns the text. |
| text/scanner | none | not planned | Go source tokenizer; Xo's own lexer is in selfhost. |
| regexp, regexp/syntax | std/regexp | done | Go's parser, compiler, and Pike VM ported, with every Unicode class of Go's regexp natively (2026-10-21); Go's results on re2-search.txt and on each class. |
| index/suffixarray | none | not planned | Niche; a module can provide it. |
| fmt | string interpolation, Display, Debug | partial | `${x}`, Display, Debug, Float shortest text. Missing: width, padding, precision, and base verbs (`%08.3f`, `%x`, `%-10s`). |

### Containers, sorting, iteration

| Go package | Xo | Status | Notes |
|---|---|---|---|
| sort, slices, maps, cmp | prelude List, Map, Set, Ord | done | sort, sort_by (stable), sort_in_place, group_by, filter, map, fold, find, index_of, keys, values; min, max, clamp on any Ord value. Missing: binary search, List min and max (slices.Min), dedup (slices.Compact), chunks. |
| container/heap | std/heap | done | Heap[T] by a comparison function with remove, fix and a moved callback (spec/std.md 56, decision 0472). |
| container/list, container/ring | none | not planned | List and Buf cover them. |
| iter | for loops over List, Map, Set, Range | not planned | No iterator protocol in the language (core). |
| unique, weak, structs | none | not planned | Runtime and layout concerns the language does not expose. |

### Numbers and math

| Go package | Xo | Status | Notes |
|---|---|---|---|
| math | prelude Float | partial | Go's portable algorithms bit for bit on every backend (decision 0287): pow, exp, exp2, expm1, log, log2, log10, log1p, trigonometric and hyperbolic functions, cbrt, hypot, fma, mod, is_inf, to_bits, from_bits. Missing: constants (Float.PI, Float.INF; a compiler change), erf, gamma, Nextafter. |
| math/bits | prelude Int word methods | done | rotate, swap_bytes, mul_hi_unsigned, unsigned compare (decision 0260); leading_zeros, trailing_zeros, ones_count (decision 0287). |
| math/big | std/big | done | big.BigInt (Go's Int, immutable; decision 0311, 2026-10-21) with Go's results on every backend; natively schoolbook (no Karatsuba or recursive division yet). BigRat (decision 0608) and BigFloat (decision 0602) with Go's results; BigRat and BigFloat in JSON as Go's MarshalText strings (2026-11-16). BigInt in JSON as Go's bare number through json.Number (decision 0673, f7edd9e9f, 2026-11-16). Last verified: 2026-11-17. |
| math/rand, math/rand/v2 | prelude Rand capability, std/random (NormFloat64, ExpFloat64, Perm, Shuffle; decision 0606; Zipf 2026-11-15) | done | Secure in programs, seeded in tests. |
| math/cmplx | none | not planned | No backend use. |
| hash | std/crypto Hasher | done | A streaming hasher for the cryptographic hashes (decision 0279) with Reset, BlockSize, Size and Go's marshaled state (MarshalBinary, UnmarshalBinary). No interface user types implement (Xo calls functions directly). Last verified: 2026-11-15. |
| image/jpeg | std/jpeg | done | Decode and DecodeConfig as `jpeg.decode`, `jpeg.decode_config` over one `Image` with Go's four decoded types, Go's integer IDCT and error texts (spec/std.md 49, decision 0467, 2026-11-03); Encode as `jpeg.encode` (decision 0510). |
| image/png | std/png | done | Decode, DecodeConfig and Encode as `png.decode`, `png.decode_config`, `png.encode` over one `Image` with Go's seven pix layouts (spec/std.md 48, decision 0466, 2026-11-03); byte exact encode, Go's error texts. |
| image | std/image | done | Go's Image as one type over the nine pixel models (Gray, Gray16, Paletted, RGBA, NRGBA, RGBA64, NRGBA64, CMYK, YCbCr), Rect, At, Set, SubImage, to and from std/png and std/jpeg images (spec/std.md 75, decision 0578); Uniform as `Image.fill`. Not offered: the Image interface (decision 0578: one Image struct with a Model, so no user defined image types). |
| image/color | std/image | done | Color (Go's RGBA() channels) and the models' conversions (Gray, Gray16, NRGBA, NRGBA64, Alpha, CMYK, YCbCr, NYCbCrA) as methods and `from_*` functions, Palette.Index and Convert as `palette_index` and `palette_convert` (spec/std.md 75, 0578 step 1). Not offered: the Color and Model interfaces (decision 0578). |
| image/color/palette | std/image | done | Plan9 and WebSafe as `image.plan9()` and `image.web_safe()`, 0xRRGGBBAA entries (spec/std.md 75). |
| image/draw | std/image | done | Draw and DrawMask as `Image.draw` and `Image.draw_mask` with Op Over and Src, Go's clipping (0578); `Image.floyd_steinberg` (FloydSteinberg's Draw). Not offered: the Drawer and Quantizer interfaces (decision 0578). |
| image/gif | std/gif | done | Decode, DecodeAll, DecodeConfig, Encode and EncodeAll over whole buffers, Go's error texts (spec/std.md 76, 0578 step 5). |
| hash/crc32 | std/crc32 | done | IEEE and Castagnoli, `checksum` and `update` (spec/std.md 44); Castagnoli in software (Go uses CRC32C instructions). `table(poly)` for Koopman or any polynomial with `checksum_table` and `update_table` (MakeTable, Checksum, Update; 2026-11-15). |
| hash/fnv | std/fnv | done | 32, 64 and 128 bit FNV-1 and FNV-1a, `sum*` and `update*` (spec/std.md 45). |
| hash/adler32 | std/adler32 | done | `checksum` and `update` (spec/std.md 52); Go's values on its golden strings and buffers across the block size. |
| hash/crc64 | std/crc64 | done | ISO and ECMA, `checksum` and `update`, slicing-by-8 (spec/std.md 51). |
| hash/maphash | none | planned | maphash is internal to Map. |

### Cryptography

| Go package | Xo | Status | Notes |
|---|---|---|---|
| crypto/sha256 | std/crypto | done | sha256 and Hash.Sha224, Sha256, with the arm64 SHA2 instructions natively (decision 0231). |
| crypto/hmac | std/crypto hmac, hmac_hasher | done | Over every Hash (decision 0279). |
| crypto/sha1 | std/crypto Hash.Sha1 | done | Decision 0279. |
| crypto/sha512, crypto/sha3 | std/crypto Hash, Shake | done | SHA-384, SHA-512, SHA3-256, SHA3-512 (decision 0279); SHA3-224, SHA3-384, SHA-512/224, SHA-512/256, SHAKE128, SHAKE256 and cSHAKE (Shake, 51cfc6def); legacy Keccak-256 and Keccak-512 (the x/crypto row, 2026-11-15). |
| crypto/md5 | std/crypto Hash.Md5 | done | Ported from Go (decision 0279); for protocols that require it. |
| crypto/subtle | std/crypto equal, xor, ct_*, with_data_independent_timing | done | Constant time comparison, XORBytes, and ConstantTimeByteEq, Eq, Select, Copy and LessOrEq (`ct_byte_eq`, `ct_eq`, `ct_select`, `ct_copy`, `ct_less_or_eq`, 2026-11-15), and WithDataIndependentTiming (`with_data_independent_timing`, decision 0656; the checker refuses a section that can park, XO0704). Last verified: 2026-11-15. |
| crypto/rand | prelude Rand, crypto.random_bytes, token, random_text; std/big | done | Secure random bytes and Ints; rand.Int and rand.Prime over the Rand (`big.random_below`, `big.random_prime`) and rand.Text (`crypto.random_text`), 2026-11-15. Last verified: 2026-11-15. |
| crypto/pbkdf2 | std/crypto pbkdf2, hash_password | done | `pbkdf2(h, password, salt, iterations, key_len)` over every Hash (Go's pbkdf2.Key), and PBKDF2-HMAC-SHA256 in a PHC like string. Last verified: 2026-11-15. |
| crypto/ed25519 | std/crypto | done | Decision 0189: Ed25519 sign, verify and key generation; Ed25519ph and Ed25519ctx (`ed25519_sign_with`, `ed25519_verify_with`, Go's Options and VerifyWithOptions). Last verified: 2026-11-15. |
| crypto/hkdf | std/crypto | done | `hkdf_extract`, `hkdf_expand`, `hkdf` over every Hash (Go's Extract, Expand, Key), on TLS's llrt code (2026-10-29, `std/changes/crypto.md`). |
| crypto/aes, crypto/cipher | std/crypto | done | AEAD: `aead_key(Aead.AesGcm, ChaCha20Poly1305, XChaCha20Poly1305 or Aes128CbcHmacSha256, key)`, `seal`/`open` with a nonce and additional data, `seal_random`/`open_random` (Go's NewGCMWithRandomNonce) (2026-10-29), `aes_gcm_key_with_nonce_size` and `aes_gcm_key_with_tag_size`; the raw block cipher (`aes_block`), CTR and CBC (decision 0605), and the `Block` and `KeyStream` interfaces with CBC and CTR over any Block. Not planned: CFB and OFB (deprecated in Go; stdaudit 2026-11-12), StreamReader and StreamWriter (Xo has no io.Reader or io.Writer). Last verified: 2026-11-15. |
| crypto/ecdh | std/crypto | done | X25519, P-256, P-384 and P-521: `ecdh_generate_key`, `ecdh_private_key`, `ecdh_public_key`, `EcdhPrivateKey.ecdh` (decision 0599 stage 1; stdaudit 2026-11-12). An ECDSA key as an ECDH key is stage 2 (the crypto/ecdsa row). |
| crypto/ecdsa, crypto/rsa, crypto/elliptic | std/crypto | done | Sign and verify (PKCS #1 v1.5, PSS with its salt length options, ECDSA DER and raw, and each over a caller's digest), ECDSA, Ed25519 and RSA key generation (decision 0285); RSA encryption, OAEP with a separate MGF1 hash and legacy PKCS #1 v1.5 (decision 0618); an RSA key's numbers and keys from them with Validate and Precompute (`rsa_parts`, `rsa_public_key`, `rsa_private_key`); ECDSA raw points and scalars (`ecdsa_public_key`, `ecdsa_private_key`, `bytes()`) and an ECDSA key as an ECDH key (`ecdh()`). Not planned: multi-prime keys (deprecated in Go), DecryptPKCS1v15SessionKey (TLS 1.2's RSA key exchange, which std/tls does not offer), crypto/elliptic (deprecated by Go). Last verified: 2026-11-15. |
| crypto/mlkem | std/crypto | done | ML-KEM-768 and ML-KEM-1024 key generation, encapsulation and decapsulation (decision 0604; stdaudit 2026-11-12), the same code as the TLS key exchange (std/native/llrt/mlkem.xo). |
| crypto/hpke | std/hpke | done | Go's crypto/hpke: RFC 9180 base mode, DHKEM P-256, P-384, P-521 and X25519, ML-KEM-768 and ML-KEM-1024 (decision 0646, b4ede954a). |
| crypto/x509, crypto/x509/pkix | std/x509, std/crypto (keys) | done | Certificates: `x509.parse`, `parse_pem`, `create_certificate`, requests (`create_request`, `parse_request`), revocation lists (`parse_revocation_list`, `create_revocation_list`), PKCS #12, chain verification; key parsing and marshaling in std/crypto (decision 0285). The audit's ten missing pieces (aws-43 against Go 1.26) are done: creating a revocation list, the parsed Certificate fields, their Template fields, the OID type and typed errors (2026-11-14), VerifyOptions.certificate_policies, parse_certificates, the CertPool methods, the PKCS #1, SEC 1 and PKCS #8 typed parsers and marshalers, and CertificateRequest attributes (2026-11-15); Go's tests in the corpora x509, x509create, x509keys, x509oid, x509policy and x509fuzz. Row done 2026-11-16 (#109). Not planned: the deprecated encrypted PEM functions and ParseCRL. |
| crypto/tls | std/tls (client and server), std/http (with_tls, with_roots, with_cert, tls_cert) | done | TLS 1.2 and 1.3 in Xo (decision 0202); `tls.client` on a dialed Conn (decision 0281: Postgres sslmode, SMTP STARTTLS, Redis); client certificates (`Client.with_cert`, the server's ClientAuth). Server side, Listen, dial, ConnectionState, close_write and addresses (decision 0644 B1, 2026-11-15); Min/MaxVersion, CipherSuites, CurvePreferences, ExportKeyingMaterial and TLSUnique, `ClientConfig`, VerifyPeerCertificate, VerifyConnection, GetClientCertificate, GetCertificate, GetConfigForClient (B2). KeyLogWriter, session tickets and keys, client resumption and ClientSessionCache, typed errors (RecordHeaderError, CertificateVerificationError, tls.Client and Handshake), OCSP stapling and SCTs (B3). Encrypted Client Hello, client and server, ECHRejectionError, AlertError (B4). QUICClient, QUICServer, QUICConn and 0-RTT tickets (T14; no EnableSessionEvents). std/http's typed TLS error (`ClientErr.Tls`, owner's choice, 0644 T12). `tls.Dialer` (config and timeout, `dial`; Go's Dialer.Dial and DialContext) 2026-11-16 (golden tls_dialer). |
| crypto | none | not planned | The Go registry of hash functions; Xo calls functions directly. |
| crypto/des, crypto/dsa, crypto/rc4 | none | not planned | Broken or deprecated algorithms. |
| crypto/fips140 | none | not planned | No FIPS module validation. |

### Networking and HTTP

| Go package | Xo | Status | Notes |
|---|---|---|---|
| net | prelude Net (TCP, Unix sockets, lookup_host, dial_timeout, interfaces, Conn deadlines), std/udp, std/dns (MX, TXT, SRV, PTR, CNAME, NS), std/netip (join_host_port, split_host_port), DNS client in std/native/llrt/dns.xo (Windows resolver config too) | done | listen, dial, attenuation, UDP, record lookups, network interfaces, deadlines on a Conn. IP and CIDR types are std/netip. Last verified: 2026-11-17. |
| net/netip | std/netip | done | Addr, Prefix, ParseAddr, ParsePrefix, Contains, Masked, the Is predicates (decision 0287). Missing: AddrPort, Prefix.Overlaps, Next and Prev. |
| net/url | std/url, std/http parse_url and Url | done | Escaping (QueryEscape, PathEscape and their inverses) and Values (ParseQuery, Encode) (decision 0279); the URL type: Parse, ParseRequestURI, String, Redacted, ResolveReference, Query, RequestURI, Hostname, Port, JoinPath and Userinfo (decision 0642, 2026-11-14). Request.form and query_all are in std/http. |
| net/http (server) | std/http | partial | Router, middleware, mount, JSON, streaming responses, TLS, HTTP/2, gzip_responses (decision 0305), Request.form and query_all (decision 0279), repeated header fields and cookies (decision 0288), multipart forms (decision 0282), streamed request bodies (`handle_stream`, BodyReader, decision 0310), `serve_content` (ServeContent: Range, conditional requests), client_ip behind proxies (2026-10-29). Missing: FileServer over an Fs directory, MaxBytesReader per route, read and idle timeouts per server. |
| net/http (client) | std/http Client | partial | get, send, post_json, redirects (decision 0404), gzip, HTTP/2, TLS roots and client certificates, a timeout per client and per request (`Request.with_timeout`), streamed response bodies (`Client.open`); std/cookiejar's Jar (decision 0475) by hand. Missing (stdaudit 2026-11-12; decision 0617 B): proxies (HTTP_PROXY, CONNECT), the jar used by the Client, a public suffix list. |
| net/http/httptest | std/testing FakeNet, Router.call | done | In process servers and fakes (section 8). |
| net/http/httputil | std/http reverse_proxy | partial | ReverseProxy as `http.reverse_proxy` (a handle_stream route); DumpRequest and the rest missing (stdaudit 2026-11-10). |
| net/http/cookiejar | std/cookiejar | done | Jar with Go's matching rules, the clock a parameter (spec/std.md 59, decision 0475); `new_with(psl:)` consults std/publicsuffix (Options.PublicSuffixList, decision 0617 B), and Client.with_jar wires it. Last verified: 2026-11-15. |
| net/http/pprof | std/debug router (decision 0325) | done | CPU and heap profiles over HTTP under a prefix the program mounts, behind its auth function; XO_PROF for files (decision 0238). |
| net/http/httptrace | std/http Request.with_trace, TraceEvent | done | WithClientTrace as `Request.with_trace(on)` (decision 0677): GetConn, GotConn, DNS, Connect, TLS handshake, WroteHeaders, WroteRequest, GotFirstResponseByte, Got100Continue, PutIdleConn, each with its time, delivered when send returns, open returns the head, or an opened body ends; WroteHeaderField, Wait100Continue and Got1xxResponse not offered. |
| net/http/cgi, net/http/fcgi | none | not planned | Legacy deployment models. |
| net/smtp | std/smtp | done | SendMail as `smtp.send` (STARTTLS required by default, PLAIN, CRAM-MD5), decision 0290. Not planned: the step by step Client. |
| net/mail | std/mail | done | ParseAddress, ParseAddressList, Address.String (decision 0282); ReadMessage, Header.Get, ParseDate, and a message builder, compose (decision 0290). Not planned: AddressParser with a custom charset reader. |
| net/textproto | std/textproto | done | Header, Reader (lines, continued lines, headers, code lines, dot encoding) and canonical_key, on Go's tests; every symbol of net/textproto mapped in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| net/rpc, net/rpc/jsonrpc | none | not planned | Superseded by HTTP JSON APIs. |

### Files, processes, and the OS

| Go package | Xo | Status | Notes |
|---|---|---|---|
| os | prelude Fs, Env, Proc, Stdio | partial | Capability based (sections 3.1 to 3.7). Missing (survey 2026-10-29): Hostname, Getpid, process rusage, statfs (disk size and free space); File has read_at and write_at (decision 0479) and seek (decision 0600). |
| os/exec | prelude Proc, Cmd, Child | done | Section 3.8. |
| os/signal | core 7.5 shutdown, task.stop_requested, Proc.kill | partial | Two phase stop on SIGTERM and SIGINT. Missing: subscribing to other signals (SIGHUP for a reload, SIGUSR1), signal.Notify (2 of 5 services in the 2026-10-29 survey). |
| os/user | Env.home | partial | Home directory only. |
| io, io/fs, io/ioutil | File, Conn, Child read and write, Fs.walk | partial | Byte streams exist per handle. Missing: a common reader and writer interface. |
| bufio | std/bufio | done | Buffered readers over a Conn or Stream (spec/std.md 55, decision 0471); writes go to BytesBuilder or straight to File and Conn. Last verified: 2026-11-15. |
| path | std/path | done | Clean, Join, Dir, Base, Ext, Split and IsAbs, and Go's Match as `is_match` (ErrBadPattern as `PathErr`, 2026-10-23). |
| path/filepath | prelude Fs paths, std/path | done | Every Fs path uses `/` on every OS (section 3.1). `rel` and `is_local` (Rel, IsLocal, 2026-11-15); EvalSymlinks open. |
| embed | std/embed (with core 3.1's `embed`) | done | a constant of `Str`, `Bytes`, or `embed.Dir` set to `embed("path or glob")`, decision 0304: one file as text or bytes, or a tree; `Dir.fs()` is a read-only `Fs` over it (decision 0307), as embed.FS is an fs.FS. Not covered: `open` as a stream over an embedded file (read the whole file). |
| syscall | none | not planned | Capabilities replace raw system calls. |
| log | std/log | done | |
| log/slog | std/log fields, Format.Json, std/trace | done | Structured fields and JSON; request ids and a ring of spans in std/trace (decision 0325). Handlers: `Handler`, `configure_handler`, text and JSON handlers over a `Sink`, `multi` and `level_range` (decision 0617 C); a logger value carrying fields, `Logger.with` and `with_group` (decisions 0413, 0678); records exported over OTLP as the otelslog bridge does (`otlp.send_logs`). Attr, Value and Kind are not planned (scripts/stdaudit/map.txt). Last verified: 2026-11-17. |
| log/syslog | none | not planned | Services log to standard error; the platform collects it. |
| expvar | std/metrics | done | Counters, gauges, histograms with labels in a Registry, its Prometheus text and handler (decision 0325, section 37). expvar's JSON form is not offered. |
| flag | std/cli | done | Decision 0160. |
| time | prelude Time, Duration, std/time | done | Zones, civil time, named layouts (decision 0094). |
| time/tzdata | std/tzdata, embedded per use | done | |
| context | scopes, cancellation, task.with_timeout | done | Structured concurrency (core 7). |
| sync, sync/atomic | prelude Mutex, Atomic, Chan, std/task (once, semaphore, groups, concurrent maps) | done | |
| errors | `!` results, Error interface | done | Typed errors (core 2.6). |

### Data and compression

| Go package | Xo | Status | Notes |
|---|---|---|---|
| database/sql | std/sql | done | API, pool limits, transactions with isolation, read only and DEFERRABLE, savepoints (`Tx.begin`), a connection held across calls (`Db.conn`, Go's DB.Conn; decision 0648 item 14), prepared statements (`Stmt`, decision 0648 item 6), row streaming (`Rows`, decision 0479), multi results (`query_all`), column types, derive SqlRow, an in memory driver; Blob, JsonText, and Array values and SQLSTATE codes (decision 0278 stage A); the Postgres (decision 0278) and MySQL (decision 0648) drivers written in Xo, SQLite through std/sqlite; a public Driver interface and `open_driver`. pool statistics (`Db.stats`, Go's DB.Stats; decision 0648 item 16). Named arguments (`sql.named`, `named_args`; pgx's NamedArgs and StructArgs, decision 0682). Not planned: out arguments and the Null* Scan types (Row's typed Option getters, `str_opt` and the rest, instead; scripts/stdaudit/map.txt). Last verified: 2026-11-17. |
| database/sql/driver | std/sql Driver, DriverConn, DriverRows, open_driver | done | A driver is an Xo value passed to `sql.open_driver` (2026-10-29). |
| compress/gzip, compress/flate | std/compress (and std/archive gzip, gunzip) | done | Every level, Go's bytes and errors, max_size, streaming Compressor and Decompressor (decision 0305, 2026-10-21); preset dictionaries and gzip header fields (2026-11-15). http.gzip_responses next. |
| compress/zlib | std/compress zlib, unzlib | done | Go's writer and reader (decision 0305, 2026-10-21). |
| compress/bzip2, compress/lzw | none | not planned | Read only legacy formats. |
| archive/tar, archive/zip | std/archive | done | Go's results natively on Go's small test archives (zip64 and PAX sparse files included) and on written archives (2026-10-21); zip comments, Store and SetOffset with Go's bytes; tar_writer, tar_reader, raw zip entries and both writers' add_fs (Writer.AddFS) with Go's bytes (2026-11-15). |

### Testing

| Go package | Xo | Status | Notes |
|---|---|---|---|
| testing | test blocks, std/testing | done | Property tests, fakes, captured logs (core 9, section 8). |
| testing/quick | property tests | done | |
| testing/fstest | MemFs | done | |
| testing/synctest | FakeClock, deterministic scheduling under xo test | done | |
| testing/iotest, testing/slogtest, testing/cryptotest | none | not planned | Go interface conformance kits. |

### Not planned: toolchain and runtime

| Go package | Status | Why |
|---|---|---|
| go/* (ast, build, constant, doc, format, importer, parser, printer, scanner, token, types, version) | not planned | Go source tooling; Xo's compiler is in selfhost/. |
| debug/* (buildinfo, dwarf, elf, gosym, macho, pe, plan9obj) | not planned | Binary formats are the toolchain's (selfhost/macho and the linkers), not a backend need. |
| runtime, runtime/debug, runtime/metrics, runtime/pprof, runtime/trace, runtime/race, runtime/coverage, runtime/cgo | not planned | The runtime is Xo's own; profiling (0238), tracing (0240), race (0273) are toolchain features. |
| plugin | not planned | Static binaries only. |
| reflect, unsafe | not planned | No reflection; derive replaces it. |

## Function level gaps (2026-11-12, scripts/stdaudit)

`go run ./scripts/stdaudit` compares every exported symbol (functions, methods,
types, consts, vars) of the Go packages above with Xo's public surface (`xo
doc` of the builtin types, the type constants of std/prelude.xo, `xo symbols
--module --pub` of every std module). Its mapping file scripts/stdaudit/map.txt
records each symbol (or a package or type with `pkg.*`, `pkg.Type.*`) as an
Xo name, `not planned: why`, or `gap: what`. Counts (fourth pass,
bench/results/std-audit-2026-11-12.md, origin/main 2baf5826a): 192 named by an
Xo doc comment, 168 matched by snake_case name, 2008 mapped by hand, 8425
skipped (packages marked not planned above, or a not planned line), 295 gaps
(633 on 2026-11-11), 0 unreviewed, 0 stale. Rerun 2026-11-15 (go1.26.11):
206 doc-named, 218 matched, 2095 mapped, 8352 skipped, 219 gaps, 0
unreviewed; four gaps had landed (tls.Server, tls.Listen,
x509.CreateRevocationList, url.JoinPath) and image/color/palette's two
vars were unmapped. With the twelve outside modules of scripts/stdaudit/outside.txt
(decision 0655, 2026-11-15) and the five more after them (pgx, x/crypto, x/net
http2): 209 doc-named, 221 matched, 3263 mapped, 8965 skipped, 1401 gaps (595
of them pgx's, mostly pgtype's value types), 0 unreviewed, 0 stale. With a row
for every std module (2026-11-15, the 13 rows below the OTLP row and eight Go
package rows): 215 doc-named, 240 matched, 3423 mapped, 8986 skipped, 1433 gaps,
0 unreviewed, 0 stale. A module's open gaps keep its features mark below
Proven (scripts/stdcov, decision 0655). A module that no row names (std/ui) is
scored by its feature audit instead, `std/evidence/MODULE/features.md`: every
feature its ADRs promise with a test, Proven when none is open (decision 0691). Use is counted over GOROOT/src
(go1.26.1, outside cmd) and the pinned modules of scripts/stdaudit/corpus.txt
(chi, gin, cobra, pgx, aws-sdk-go-v2, client_golang, mux): files that use the
symbol; method counts are an upper bound (any `.Name` selector in a file that
imports the package). Top 30 gaps by twice the module files plus the stdlib
files, grouped by feature (`-rank REPORT -status gap DIRS...`; Last verified:
2026-11-12):

| # | Gap | Score | Files (modules / stdlib) | Symbols (most used first) |
|---|---|---|---|---|
| 1 | no MAC address type (the score is mostly `.String` in files that import net, an upper bound; the type alone: 1 module file) | 41 | 12 / 17 | `net.HardwareAddr.String`, `net.HardwareAddr` |
| 2 | no HPKE (std-coverage.md: planned after a public AEAD and ECDH) | 13 | 0 / 13 | `crypto/hpke.Recipient`, `crypto/hpke.Sender`, `crypto/hpke.PublicKey`, `crypto/hpke.KDF` and 35 more |
| 3 | no Request.without_header / Response.without_header | 13 | 5 / 3 | `net/http.Header.Del` |
| 4 | no relative path between two paths (std/path has join, clean, dir) | 7 | 0 / 7 | `path/filepath.Rel` |
| 5 | no Env.temp_dir (TMPDIR by hand) | 6 | 1 / 4 | `os.TempDir` |
| 6 | no Rune.REPLACEMENT constant ('\u{FFFD}' literal) | 6 | 0 / 6 | `unicode.ReplacementChar` |
| 7 | no Writer interface to fan out (io.Copy is a gap too) | 6 | 0 / 6 | `io.MultiWriter` |
| 8 | no public RFC 2047 encoded-word codec (std/mail's decode_word and encode_word are internal; closed 2026-11-16: std/mime encode_word, decode_word, decode_header) | 6 | 0 / 6 | `mime.WordEncoder.Encode`, `mime.WordDecoder`, `mime.WordDecoder.Decode`, `mime.QEncoding` and 3 more |
| 9 | the Client reads no HTTP_PROXY / HTTPS_PROXY / NO_PROXY | 6 | 2 / 2 | `net/http.ProxyFromEnvironment` |
| 10 | no ${VAR} expansion helper | 5 | 0 / 5 | `os.ExpandEnv`, `os.Expand` |
| 11 | no decode of the last rune (Str.runes().last() walks the text) | 5 | 0 / 5 | `unicode/utf8.DecodeLastRuneInString` |
| 12 | no test for a complete UTF-8 sequence at the end of a buffer (stream decoding) | 5 | 0 / 5 | `unicode/utf8.FullRune` |
| 13 | no List.binary_search_by | 4 | 0 / 4 | `slices.BinarySearchFunc` |
| 14 | no Request.with_basic_auth | 4 | 1 / 2 | `net/http.Request.SetBasicAuth` |
| 15 | no decode of the last rune (Bytes.decode_rune reads forward) | 4 | 0 / 4 | `unicode/utf8.DecodeLastRune` |
| 16 | no equality of two lists by a function (== compares elements by ==) | 4 | 0 / 4 | `slices.EqualFunc` |
| 17 | no first-nonzero helper (Option.or covers Option values only) | 4 | 1 / 2 | `cmp.Or` |
| 18 | no whole-path symlink resolution (Fs.read_link reads one link) | 4 | 0 / 4 | `path/filepath.EvalSymlinks` |
| 19 | Float.round only (check rounding mode) | 3 | 1 / 1 | `math.RoundToEven` |
| 20 | maphash is internal to Map (no public seeded hash) | 3 | 0 / 3 | `hash/maphash.Hash`, `hash/maphash.Hash.Sum64`, `hash/maphash.Hash.Reset`, `hash/maphash.Seed` and 14 more |
| 21 | no 128 by 64 bit division | 3 | 0 / 3 | `math/bits.Div` |
| 22 | no Bytes join (List.join takes Str only) | 3 | 1 / 1 | `bytes.Join` |
| 23 | no Conn.close_write (half close) | 3 | 0 / 3 | `net.TCPConn.CloseWrite` |
| 24 | no DATE_TIME layout constant (std/time.layout("2006-01-02 15:04:05")) | 3 | 0 / 3 | `time.DateTime` |
| 25 | no Float math constants (E, PI, LN2 and the rest are literals today) | 3 | 0 / 3 | `math.Log10E`, `math.Ln2`, `math.Ln10`, `math.Sqrt2` and 1 more |
| 26 | no Float.signbit | 3 | 0 / 3 | `math.Signbit` |
| 27 | no Request.basic_auth (decode the Authorization header by hand) | 3 | 1 / 1 | `net/http.Request.BasicAuth` |
| 28 | no capture positions | 3 | 1 / 1 | `regexp.Regexp.FindStringSubmatchIndex`, `regexp.Regexp.FindSubmatchIndex` |
| 29 | no content sniffing (Go's DetectContentType) | 3 | 0 / 3 | `net/http.DetectContentType` |
| 30 | see rsa.EncryptOAEP (closed 2026-11-12, decision 0618) | 3 | 0 / 3 | `crypto/rsa.PKCS1v15DecryptOptions`, `crypto/rsa.PrivateKey.Decrypt`, `crypto/rsa.EncryptPKCS1v15`, `crypto/rsa.ErrMessageTooLong` and 6 more |

Closed since the 2026-11-11 table (every row of it but 13,
HPKE): image/color models (0578), textproto, ECDH, File.seek (0600), asn1
(0601 stage 1), AddrPort, BigFloat (0602), ML-KEM (0604), CTR and CBC (0605),
file_server (0607), Hasher clone, IPv6 zones, HPKE's prerequisites,
cross_origin, BigRat (0608), Float.ldexp/frexp/copysign and the limit consts,
normal and exponential distributions, the streaming XML writer,
join_host_port, Bytes XOR, Fs.create_temp_dir, List.is_sorted, URL user and
password, column types, prefix_from, Bytes.trim_start_chars and
trim_end_chars, and PATH lookup. Open from the owner's medium list: RSA
encryption (OAEP and PKCS #1 v1.5, row 30's group) and HPKE (row 2).

## Outside Go's standard library

| Need | Go source | Xo | Status | Notes |
|---|---|---|---|---|
| Postgres driver | github.com/jackc/pgx, lib/pq | std/sql | done | Pure Xo wire protocol (v3, SCRAM-SHA-256, TLS) over Net (decision 0278); several hosts with target_session_attrs, COPY FROM, prepared statements, cursors, CancelRequest on cancellation; LISTEN/NOTIFY (`Conn.wait_notification`), notice handlers (`on_notice`), savepoints, DEFERRABLE transactions and a connection held outside a transaction (`Db.conn`, pgxpool's Acquire) since decision 0648 item 14; batches and pipelines (`Db.batch`, `Db.pipeline`, pgx's SendBatch and pgconn's Pipeline) since decision 0648 item 15; pool statistics (`Db.stats`, pgxpool's Stat) and tracing (`on_trace`, pgx's tracers) since decision 0648 item 16. Missing: COPY TO, named arguments, binary format results, typed values beyond int, float, bool, text, bytea, json, timestamps and their arrays (numeric, date, interval, uuid, ranges and others read as text), registered and composite types, large objects, GSSAPI. Symbol audit (decision 0655): pgx v5.12.0 with pgconn, pgtype and pgxpool, 1107 symbols in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| SQLite driver | mattn/go-sqlite3, modernc.org/sqlite | std/sqlite | done | The SQLite library through `use c`, a driver for `sql.open_driver` (decision 0534, spec/std.md 66, 2026-11-06), native backends; prebuilt objects per release target (scripts/sqlite-prebuilt.sh); a differential corpus against mattn/go-sqlite3 v1.14.53 (internal/stdtest/testdata/corpus/sqlite, 180 cases); timing std/evidence/sqlite/perf.md (worst 0.869 of mattn, linux/arm64 host 1). Audited against mattn v1.14.53 (decision 0655, #109): mattn's DSN keys, named extended result codes, its time text and Unix integer reading; not planned: Go callbacks (functions, collations, hooks), run-time extensions and the deprecated user authentication extension. No LISTEN/NOTIFY (Postgres only). Last verified: 2026-11-16. |
| MySQL driver | go-sql-driver/mysql | std/sql | done | Pure Xo wire protocol over Net, `sql.open(net, "mysql", dsn)` (decision 0648, spec/std.md 10.10): go-sql-driver's DSN, native, caching_sha2 and sha256 auth, TLS, prepared statements over the binary protocol, multi results, time and location, KILL QUERY on cancellation; text-protocol values converted as go-sql-driver's readRow (a value that does not convert is the query's error), checked by TestMysqlFuzz against go-sql-driver v1.10.1 over a fake server. Native backends; the Go backend reports an unknown driver. Timing against go-sql-driver (decision 0648 item 12): 1.06 of Go's wall reading 300,000 rows over TLS (bench/perf/sqlmysql, bench/results/std-sql-mysql-2026-11-17.md). Last verified: 2026-11-17. |
| Redis client | redis/go-redis | none | planned | RESP over Net. |
| Argon2, BLAKE2b | golang.org/x/crypto | std/crypto | done | Decision 0059: Argon2id through `hash_password_argon2id` and `verify_password`. BLAKE2b as `Hash.Blake2b_256`, `Blake2b_384`, `Blake2b_512`, keyed and of any size through `blake2b_hasher`, Go's MarshalBinary, and BLAKE2Xb (`blake2b_xof`), 2026-11-15 (std/native/llrt/blake2b.xo; x/crypto's code vendored in std/rt). Symbol audit (decision 0655): golang.org/x/crypto v0.58.0 argon2 and blake2b, 17 symbols in scripts/stdaudit/map.txt, no gap. RSA PSS and OAEP take BLAKE2b, as Go (2026-11-15). Timing row (bench/perf/blake2b, twincheck -time 5 on the Mac Studio, 2026-11-15): BLAKE2b-256 of 1 MiB 0.94x x/crypto, BLAKE2b-512 0.95x, keyed 32 byte hasher on short messages 1.09x, Sum256 of 200,000 short messages (0 to 149 bytes) 1.93x, then 1.07x with the one shot digest (same day; host 1 --ab small 0.534 of the old). Last verified: 2026-11-15. |
| ChaCha20-Poly1305, HKDF, SHA-3 | golang.org/x/crypto | std/crypto | done | ChaCha20-Poly1305 and XChaCha20-Poly1305 through `aead_key`, HKDF (`hkdf`), SHA3-224 to SHA3-512 and legacy Keccak-256 and Keccak-512 as `Hash`, SHAKE and cSHAKE (2026-10-29; XChaCha and Keccak 2026-11-15). Symbol audit (decision 0655): golang.org/x/crypto v0.58.0 chacha20poly1305, hkdf and sha3, 26 symbols in scripts/stdaudit/map.txt (sha3 shares crypto/sha3's keys), no gap. Last verified: 2026-11-15. |
| PKCS #12 (pfx) | golang.org/x/crypto/pkcs12 | std/x509 | done | `parse_pkcs12` (2026-11-05): 3DES and RC2 with SHA-1 as Go; chains read too. Missing: ToPEM. Symbol audit (decision 0655): golang.org/x/crypto v0.58.0 pkcs12, 6 symbols in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| bcrypt | golang.org/x/crypto/bcrypt | std/bcrypt | done | `hash`, `compare`, `cost` (spec/std.md 53); one source for every backend; Go's hashes and error texts on seeded salts and malformed hashes. Symbol audit (decision 0655): golang.org/x/crypto v0.58.0 bcrypt, 15 symbols in scripts/stdaudit/map.txt, no gap. Last verified: 2026-11-15. |
| WebSocket | golang.org/x/net/websocket, gorilla, coder/websocket | std/websocket | partial | gorilla's behavior, ws:// both sides and wss:// in dial (decision 0313). Has a writer (a message in pieces), join (JoinMessages), prepared messages, on_close and addresses. Missing: permessage-deflate, the Autobahn run (stage 3). Symbol audit (decision 0655): gorilla/websocket v1.5.3, 71 symbols in scripts/stdaudit/map.txt. Last verified: 2026-11-17. |
| HTTP/2 | golang.org/x/net/http2 | std/http | done | Decision 0121: server and client, h2c, H2Config (decision 0434), trailers; TestH2Diff checks frames against Go. The frame layer, HPACK and the write scheduler are internal (net/http exposes none), server push and stream priorities not planned. Missing: extended CONNECT (RFC 8441), an h2spec run. Symbol audit (decision 0655): golang.org/x/net v0.62.0 http2, h2c and hpack, 201 symbols in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| JWT, JOSE | golang-jwt, go-jose | std/jwt | done | sign, verify_with, verify (key by kid), peek_header, jwks; JWS only, no JWE (spec/std.md 62, decision 0486); all_audiences and require_nbf (2026-11-15). Missing: JWE. Symbol audit (decision 0655): golang-jwt v5.3.1, 133 symbols in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| AMQP 1.0 | Azure/go-amqp | std/amqp | done | Type system and encoding (2026-11-05), connections (amqp:// and amqps://, SASL PLAIN, ANONYMOUS, EXTERNAL and XOAUTH2), sessions, sender and receiver links with settle modes, credit and drain, dispositions, and CBS tokens (decision 0521, spec/std.md 64; std/changes/amqp.md). Symbol audit (decision 0655): go-amqp v1.7.0, 134 symbols in scripts/stdaudit/map.txt. Last verified: 2026-11-17. |
| CBOR | fxamacker/cbor | std/cbor | done | RFC 8949 `Value` tree, `decode`, `encode` (preferred and deterministic), diagnostic notation, JSON bridge (decision 0468, section 50, 2026-11-03); `derive Cbor` with marshal and unmarshal (decision 0527); a type's tag number, `derive Cbor(tag: N, tag_rule:)` (decision 0665, 2026-11-16). Missing: big integers (tags 2 and 3), indefinite length encoding, length first map order (CTAP2), a raw (undecoded) field, Time encoding options, decode limits and refusals (element counts, indefinite lengths, tags, NaN, chosen simple values), diagnostic options. Symbol audit (decision 0655): fxamacker/cbor v2.9.0, 246 symbols in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| Protocol Buffers | google.golang.org/protobuf | std/protobuf | done | Decision 0530 Accepted (owner, 2026-11-06). Wire layer and `derive Proto` (Int, Int32, UInt32, UInt64, Float, Float32, Bool, Str, Bytes with zigzag and fixed, Option, List, maps, nested, Int enums, oneofs, unknown fields kept) match protowire and dynamicpb on about 8,900 differential cases; timing row in bench/perf/protobuf against protoc-gen-go messages (unmarshal 0.46x Go, marshal 1.06x). Fuzzed: TestProtobufFuzz (fresh seeds against protowire and dynamicpb). `xo proto gen` writes the messages and enums of `.proto` files (decision 0530). Imports across the files `xo proto gen` is given (protoparse `gen_set`, decision 0683 item 4), the well known types (std/protowkt), the JSON mapping (std/protojson), the text format (std/prototext, writer and reader), descriptors (std/protodesc) and `merge_from` (proto.Merge). Service stubs are protoc-gen-go-grpc's, not protobuf-go's: they belong to std/grpc's row. Not planned (map.txt): extensions (proto2; with proto2 support, its own ADR) and MarshalOptions beyond the one deterministic mode. Symbol audit (decision 0655): protobuf-go v1.36.12 proto, protowire, protojson, prototext and the well known types in scripts/stdaudit/map.txt. Last verified: 2026-11-17. |
| Go time layouts | time.Format, time.Parse | std/timego | done | Decision 0537 Accepted (owner, 2026-11-06). Go's own layout scanner, formatter and parser over core time (every element of nextStdChunk, Go's constants, RFC 3339 fast path); Format and Parse of Go's output and of mutated texts match Go on 44 layouts (internal/stdtest/timeref, no allowance for core being stricter). Fuzzed: TestTimegoFuzz (fresh seeds, mutated parse inputs). Timing row (bench/perf/timego, host 2, n=5, d3fcfb4f9): format 1.15x Go (1.12-1.19), parse 1.94x (1.80-2.03); next perf item: layout_parse allocation. Differences: a Time spans 1677 to 2262 (a year outside it, or none, is a ParseErr), gap and overlap pick the earlier time, ParseErr wording is core's. |
| .proto parser | bufbuild/protocompile (descriptor shape only) | std/protoparse | done | Decision 0530. Hand written lexer and recursive parser for proto3 and proto2 (labels, required, groups; messages, nested messages, enums, oneofs, maps, services) with positioned errors, and a generator to Xo `derive Proto` structs; 9 tests. Options (file, message, field, enum, value, service, rpc; flat aggregates), reserved and extension ranges, `extend` blocks and public and weak imports are kept, and std/protodesc builds protoc's descriptors from the result, imports resolved through their descriptors (#109, 2026-11-16). `xo proto gen` writes a set of files that import one another: repeated numbers unpacked as protoc writes them, enums of other files, the well-known types as std/protowkt's (#109, 2026-11-16). Not yet: fuzzing (testing), a timing row (performance), services (std/grpc stubs). |
| UUID | google/uuid | std/uuid | done | v4 and v7 from Rand and Clock, parsing (decision 0282). Missing: versions 1, 3, 5 and 6 with the namespaces, the Max UUID, variant, a time based UUID's timestamp, the urn:uuid: form. Symbol audit (decision 0655): google/uuid v1.6.0, 80 symbols in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| OAuth2 client | golang.org/x/oauth2 | none | planned | Needs form encoding. |
| TOTP | pquerna/otp | std/otp | done | hotp, totp, verify, generate_secret, uri (spec/std.md 57, decision 0473). Missing: parsing an otpauth:// URI, the otpauth://hotp/ URI, a QR code image, Steam Guard codes. Symbol audit (decision 0655): pquerna/otp v1.5.0 with hotp and totp, 47 symbols in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| Migrations | golang-migrate, goose | std/migrate | done | goose's `.sql` format and up/status with a version table and an advisory lock (decision 0658 item 8, spec/std.md 85); forward only (no down). goose v3.27.3 symbol audit (scripts/stdaudit/map.txt `goose.`, decision 0655, task #190, 2026-11-17): 111 mapped, 46 not planned, no gaps. Not planned: down migrations (forward only, decision 0658 item 8), package level registries (up_code and up_list take migrations as values), goose's CLI dispatch and color, Store plug-ins, and dialects with no std/sql driver (decision 0693). Last verified: 2026-11-17. |
| Cron schedules | robfig/cron | std/cron | done | parse and next of robfig/cron v3.0.1 (spec/std.md 58, decision 0474); the scheduler, job wrappers and loggers are not planned (the caller runs jobs). `parser(options)` with seconds and optional fields (NewParser, 2026-11-15). Symbol audit (decision 0655): robfig/cron v3.0.1, 57 symbols in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| Prometheus exposition | prometheus/client_golang | std/metrics | done | Registry, counters, gauges, histograms, summaries, scrape time gauges and collectors, series removal, text exposition, handler and HTTP middleware (section 37). Missing: curried label values and partial match removal, label constraints, untyped series, collector histograms and summaries, created and sample timestamps, exemplars, native histograms, bucket helpers, runtime and process collectors, merged registries and prefix views, compressed exposition, request and response size middleware, client side instrumentation. Symbol audit (decision 0655): client_golang v1.23.2 prometheus and promhttp, 207 symbols in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| S3 and AWS SigV4 | aws-sdk-go-v2 | none | not planned | A third party module; std provides HMAC and SHA-256. |
| gRPC | google.golang.org/grpc | std/grpc | in progress | Decision 0535. Wire interop with grpc-go both ways (TestGrpcInterop: unary, status, unknown method, three streaming shapes), codecs checked against grpc-go (testdata/corpus/grpc), server deadlines cancel the handler, gzip compression (Client.with_gzip, both ways against grpc-go) and `-bin` metadata (bin_encode, bin_decode), both 2026-11-06. Response metadata on the client (ClientCall.header, trailer) and send and receive limits, both checked against grpc-go in TestGrpcInterop, 2026-11-17. Missing: the health service (grpc.health.v1, grpc-go's grpc/health), `unix:` targets (dial over a Unix socket), response headers from a handler, service stubs from `xo proto gen` (it generates messages and enums only), a timing row against grpc-go. Symbol audit (decision 0655): grpc-go v1.79.2 with codes, status, metadata, health, keepalive and credentials, 307 symbols in scripts/stdaudit/map.txt. Behavior checklist: health service missing; server reflection missing; keepalive missing; interceptors missing; `unix:` targets missing; mTLS possible through std/http but untested against grpc-go (TestGrpcInterop is cleartext); name resolution and balancing missing. Last verified: 2026-11-15. |
| OTLP export | go.opentelemetry.io/otel exporters (otlptrace, otlpmetric) | std/otlp | done | Decision 0539. Trace request bytes checked against protobuf-go (testdata/corpus/otlp). Metrics checked too; `Exporter` over OTLP/HTTP with retry (golden otlp_export). gRPC transport (`grpc_exporter`), partial success, Retry-After dates, gzip and JSON encodings and size limits since 2026-11-17; log export (Exporter.send_logs) too. Missing: a timing row. Symbol audit (decision 0655): otlptracehttp v1.47.0, 26 symbols in scripts/stdaudit/map.txt (otlpmetrichttp has the same options). Last verified: 2026-11-17. |
| YAML | gopkg.in/yaml.v3 | std/yaml | done | A port of yaml.v3 (decision 0512): decode, decode_all (strict), encode_value, encode_all, the Node tree with comments, styles and aliases. Symbol audit (decision 0655): yaml.v3 v3.0.1 in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| TOML | pelletier/go-toml/v2, BurntSushi/toml | std/toml | done | TOML 1.0 decode and encode through derive Json (decision 0164), strict decode (`decode(strict: true)`, go-toml's DisallowUnknownFields) and encode options (`encode_with`). Symbol audit (decision 0655): go-toml v2.4.3 in scripts/stdaudit/map.txt. Last verified: 2026-11-17. |
| Rate limiting | golang.org/x/time/rate | std/rate | done | Limiter with allow, reserve, wait and their n forms, and Sometimes (2026-11-15) (section 42). Symbol audit (decision 0655): x/time v0.16.0 in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| Backoff and retry | cenkalti/backoff | std/backoff | done | ExponentialBackOff, Retry, RetryNotify and Ticker (2026-11-15) (section 43). Symbol audit (decision 0655): backoff v4.3.0 in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| Terminal | golang.org/x/term | std/term | done | Colors that follow NO_COLOR and the terminal (section 15); is_tty, width, read_secret and raw_input on Stdio. Missing: a line editor (Terminal). Symbol audit (decision 0655): x/term v0.48.0 in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| HTTP proxy settings | golang.org/x/net/http/httpproxy | std/httpproxy | done | HTTP_PROXY, HTTPS_PROXY and NO_PROXY as x/net's httpproxy. Symbol audit (decision 0655): x/net v0.62.0 in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| Public suffix list | golang.org/x/net/publicsuffix | std/publicsuffix | done | public_suffix and effective_tld_plus_one over Mozilla's list. Symbol audit (decision 0655): x/net v0.62.0 in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| Brotli | andybalholm/brotli | std/brotli | done | Decision 0666 (Accepted). decode (RFC 7932) with andybalholm v1.2.2's errors at its points and encode at qualities 0 to 11, byte identical to its Writer (spec/std.md 87, 2026-11-15). Streaming compressor (`brotli.Compressor`, Write, Flush and Close byte identical, 2026-11-16). Streaming decompressor (`brotli.Decompressor`, decode's output and errors in any pieces, 2026-11-16). HTTPCompressor as `brotli.responses` over `http.encoded_responses` (decision 0674, 2026-11-16). Speed against Go: bench/results/brotli-*-vs-go-2026-11-15.md. |
| JSON-RPC 2.0 | modelcontextprotocol/go-sdk jsonrpc | std/jsonrpc | done | Messages, batches, line and header framing, a Conn with calls, notifications and cancellation (decision 0603). Symbol audit (decision 0655): go-sdk v1.8.0 jsonrpc in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| Protobuf wire format | google.golang.org/protobuf/encoding/protowire | std/protowire (internal) | done | The wire reader and writers std/protodesc, std/protowkt and std/protojson share; the field number range and Number.IsValid are std/protobuf's (2026-11-15). Symbol audit (decision 0655): protobuf-go v1.36.12 protowire in scripts/stdaudit/map.txt. Last verified: 2026-11-15. |
| Server-sent events | none (WHATWG HTML 9.2.6) | std/sse | done | Parser, Reader, Source with reconnection, and the writer side (decision 0603). No Go reference: spec/std.md 79 and a behavior checklist in scripts/stdaudit/map.txt (decision 0655). Missing: a run of the web-platform-tests eventsource cases. Last verified: 2026-11-15. |
| Go interop values | none (use go) | std/go | done | Error, Any, Out and Recv across the Go boundary (decision 0041). No Go reference: spec/std.md 12 and a behavior checklist (decision 0655). Last verified: 2026-11-15. |
| AMQP test broker | none (Azure Service Bus behavior) | std/amqptest | done | An in-process broker for std/amqp's tests (decision 0521). No Go reference: spec/std.md 64.6 and a behavior checklist (decision 0655). Missing: subscription rules (SQL and correlation filters). Last verified: 2026-11-15. |
| Double ended queue | container/list as a queue | std/deque | done | A ring buffer deque (section 47; container/list itself is not planned). No Go reference: spec/std.md 47 and a behavior checklist (decision 0655). Last verified: 2026-11-15. |

## Performance evidence (decision 0461)

Committed result files with a Go twin that set a module's performance
mark, newest first. A row gives the twin's program, the host, and the
measured range of Xo's wall over Go's (below 1 is faster).

| Module | Result file | Program, host | Xo/Go |
|---|---|---|---|
| prelude (equal_fold, to_lower), http (header names), html (escape) | bench/results/simd-survey-2026-11-06.md (step 1b) | bench/perf/simdscan fold, fold_32, lower; bench/perf/html escape; linux/arm64 host 1, Go 1.26.1 | equal_fold 0.165 (4 KiB) and 0.245 (32 B), to_lower 0.136, html escape within noise of Go (0.721) |
| prelude (Str, Bytes byte paths) | bench/results/simd-survey-2026-11-06.md | bench/perf/simdscan, linux/arm64 host 1, Go 1.26.1 | 0.26 (to_hex) to 1.91 (one byte find); equal 1.65, compare 1.34, ASCII validation 1.26 after step 1: within 2x (Solid) |

## Survey of backend gaps (2026-10-30)

Five more services, of kinds the two earlier surveys did not cover,
written from spec/core.md and spec/std.md alone (std's sources unread)
and type checked with `xo check` to 0 errors, each missing piece marked
`// GAP:` where the code needed it (bench/survey-2026-10-30): a
WebSocket chat and notification hub with presence (chathub, 600 lines),
a reliable queue worker speaking the Redis wire protocol (queueworker,
713), a gRPC service and client over HTTP/2 with hand written protobuf
(grpcsvc, 686), a double entry money ledger on Postgres (ledger, 607),
and a CSV, TSV and XLSX import and export batch job (dataport, 537).
Channels, `select`, `task.stopping`, `with_timeout`, std/metrics, rate,
backoff, uuid, HMAC, `json.Value`, websocket upgrade, BigInt, xml and
archive covered most of what they needed. Ranked blockers first, then by
how many services hit the gap:

| Rank | Gap | Services | Blocking | Go reference | Size |
|---|---|---|---|---|---|
| 1 | Closed 2026-10-30 (decision 0433): trailers both ways on HTTP/1.1 and HTTP/2 (`BodyWriter.set_trailer`, `with_trailer_names`, `Request.with_trailers`, `ClientStream.trailers`, `BodyReader.trailers`; API and Go backend ae79de06c, native e0444461a) and full duplex (`Response.with_full_duplex`, `Client.open` with an `http.pipe` body; 88126f462, native client 5b3fe8065); goldens http_trailers, http_duplex; grpc-go v1.67.1 interop 11 of 11 and 9 of 9 on the Go and arm64 backends (0433). Was: HTTP/2 trailers: a handler cannot send trailers after the body, the client cannot read them; and no full duplex stream (a `handle_stream` body cannot be read while `http.stream` writes) | grpcsvc (every gRPC server and client) | blocker | net/http ResponseWriter.Trailer, TrailerPrefix, Response.Trailer, ResponseController.EnableFullDuplex | M to L |
| 2 | Closed 2026-10-30 (decision 0433): `Server.with_client_auth(mode, roots)` in Go's five modes, `Client.with_cert`, `peer_certificates` and `verified_chains` (API and Go backend f770533b2, native over TLS 1.3 and 1.2 0aa379fa2); golden tls_mtls. Was: Mutual TLS: a server cannot request and verify client certificates, a client cannot present one | grpcsvc (service meshes, internal APIs) | blocker | crypto/tls Config ClientAuth, ClientCAs, Certificates | M |
| 3 | Closed 2026-11-03 (`csv.reader`). A streaming CSV reader over a File (csv.parse takes the whole text; read_line breaks quoted newlines) | dataport | blocker for large files | encoding/csv NewReader(io.Reader).Read | S to M |
| 4 | Closed 2026-11-03 (`std/charset`). Text encodings: UTF-16 (Excel's Unicode Text) and Windows-1252 decoding and encoding | dataport | blocker | x/text/encoding/unicode, charmap | S to M |
| 5 | Closed 2026-10-30 (decision 0430): `set_deadline`, `set_read_deadline`, `set_write_deadline` on Conn (29db8153f), TlsConn (35f235e69) and WsConn with `on_ping`, `on_pong` and `pong` (07400a89c), and `task.deadline(clock)` (721b4d194); goldens conn_deadline, conn_deadline_move, tls_deadline, websocket_deadline, websocket_tls_deadline, task_deadline. Still open: the setters are on the concrete types only, so code holding a `Stream` (queueworker's `if tls { tls_conn } else { conn }`) cannot set one; adding them to `Stream` is BREAKING and is 0430's option for the owner, not decided. Was: Deadlines on Conn, TlsConn and WsConn (read and write), a WebSocket pong handler; and reading the current task's deadline to pass on (grpc-timeout) | queueworker, chathub, grpcsvc | awkward (with_timeout cancels, and the connection is then lost) | net.Conn SetDeadline, gorilla SetWriteDeadline, SetPongHandler, context.Deadline | M |
| 6 | An exact decimal type (parse, format, rounding modes, allocation) | ledger, dataport | awkward (hand written on BigInt or cents) | shopspring/decimal, cockroachdb/apd (outside Go's std) | M |
| 7 | A type's own JSON form (Money as "12.34") and internally tagged enums (`{"type": "say", ...}`) | ledger, chathub | awkward | encoding/json Marshaler, Unmarshaler | M |
| 8 | Closed 2026-11-03 (`std/bufio`). A buffered reader over a Conn or Stream (read_line, read_exact, peek) | queueworker (every line protocol) | awkward | bufio.Reader | S |
| 9 | Large data paths: a row cursor for queries (results are read whole), COPY FROM, a zip reader and writer by entry, a streaming XML token reader | dataport | awkward | database/sql Rows, pgx CopyFrom, archive/zip, encoding/xml Decoder | M each |
| 10 | Closed 2026-10-30 (decision 0434): h2c with prior knowledge on server and client (`with_protocols`, `Protocol.UnencryptedHttp2`) and `H2Config` (max concurrent streams, frame and buffer sizes, header tables, `send_ping_timeout` and `ping_timeout` keepalive, `write_byte_timeout`) on `Server.with_h2_config` and `Client.with_h2_config` (native 4d83e777b, golden commit 5b220d641); goldens http_h2c, http_h2c_sniff, http_h2_config. Still open: GOAWAY by connection age (grpc-go's MaxConnectionAge), left out by 0434 because net/http has none; `Upgrade: h2c` is not supported, as in Go. Was: HTTP/2 without TLS (h2c, prior knowledge) and server HTTP/2 settings (max streams, keepalive ping, GOAWAY by age) | grpcsvc | awkward | x/net/http2 AllowHTTP, http.Protocols, http2.Server | M |
| 11 | Protobuf runtime and a gRPC library (`derive Proto` is reserved for after 1.0) | grpcsvc | awkward (hand written) | google.golang.org/protobuf, grpc (outside Go's std) | L |
| 12 | Unicode normalization (NFC) and IDNA. Closed 2026-11-03 (`std/norm`, `std/idna`, decision 0476) | dataport | awkward | x/text/unicode/norm, x/net/idna | M |
| 13 | Minor: done 2026-11-03: `Db.begin(isolation:, read_only:)`, `Duration.round` and `truncate`, `Bytes.to_str_lossy` and `to_valid_utf8`. Left (new design, for a later ADR): identifier quoting for SQL; two digit years; trace ids on outbound requests; Redis and JWT clients (outside Go's std too) | ledger, dataport, chathub, queueworker | minor | sql.TxOptions, Duration.Round, strings.ToValidUTF8, pgx.Identifier, otelhttp | S each |

Language obstacles the services met (for the language owners): enum
variants cannot carry explicit numbers (gRPC codes became Int
constants); no bit preserving Int to UInt64 conversion (`UInt64(x)`
faults on a negative int64 wire value); no cursor to consume a `Bytes`
from the front (each fill copies the unread rest); a `Str` cannot be
indexed by byte; a struct of handles is copied into every spawn
(XO0534, 44 and 58 heap fields) with no way to share it; no
`List.filter_map` and no deque; `backoff.retry` has no notify hook (Go's
RetryNotify) and its `retryable` must be pure; no 128 bit integer.
Status 2026-11-03: the std level ones are done (`List.filter_map`
5f339ffc8, std/deque e71fc47f2, `backoff.retry_notify` with effectful
`retryable` and `notify` e44acf4a8, `UInt64.from_bits` b77c120c5, all
2026-10-30) except an append and consume buffer for non Stream sources,
decision 0480 (Proposed; std/bufio covers a Stream, Bytes slices are
views). Open for the language owner: explicit enum variant numbers, a
`Str` byte index, a 128 bit integer, and sharing a struct of handles
across spawns (XO0534).

Found wrong or missing in the spec: std 2.4 shows no way to write an
empty `Set` (no `Set.new`, no literal; `[].to_set()` works); std 33
lists BigInt's methods without result types (`quo_rem`, `cmp`, `sign`);
core 2.10 says branches are not converted when joined, yet `if tls {
tls_conn } else { conn }` checks as a `Stream` (queueworker:244); core
7.3 does not give a select arm's bound type (`T?`, None when the channel
closed); std 4.4 does not say whether an upgraded WebSocket counts as an
in flight request at a stop; std 37's `reg.middleware(clock, label)` is
only in the prose; std 2.6 lists `Time.UNIX_EPOCH` as a value and
Appendix A as a function. Fixed 2026-10-30: six in the spec (e5fe92b08);
the WebSocket at a stop was a backend difference (the Go backend did not
wait), decided in 0429 (code after the seed).

Proposed (blockers that unblock a class of service first): 5 (deadlines,
three services), 1 and 2 (gRPC and mTLS, one design), 3 and 8 (streaming
CSV over a buffered reader), 4, 7, then 6 (decimal, likely outside std
as in Go).
Audit 2026-11-03 (Xo Lang 2, against code and goldens on main): ranks 1,
2, 3, 4, 5, 8, 10 and 12 are closed; 9 and 7 have Proposed ADRs (0479,
0478); rank 13's straight Go ports are done; 6 and 11 are open. What is
left of the closed ranks: deadline setters on `Stream` (0430's owner
option) and GOAWAY by connection age (outside net/http).

## Survey of backend gaps (2026-10-29)

Five services written from spec/std.md and spec/core.md alone, without
reading std's sources, and type checked with `xo check` (each 0 errors),
each missing piece marked where the code needed it: an API gateway with
a reverse proxy (468 lines), a server rendered web app with templates,
encrypted cookie sessions and CSRF (575), an ops CLI with tables and a
concurrent hashing walk (719), a job scheduler with priorities, cron and
retries on Postgres (726), and a node metrics exporter and sidecar with
statsd, DNS discovery and certificate expiry checks (583). Ranked by how
many services hit the gap, blockers (no workaround) first:

| Rank | Gap | Services | Blocking | Go reference | Size |
|---|---|---|---|---|---|
| 1 | HTTP client: streamed response and request bodies, a redirect policy, a timeout per request, whether a failed request reached the server; then a reverse proxy | gateway (and every proxy or large download) | blocker | net/http Response.Body, Request.Body, CheckRedirect, httputil.ReverseProxy | M |
| 2 | Signals other than SIGINT and SIGTERM (SIGHUP reload) | gateway, exporter | blocker | os/signal Notify | S |
| 3 | json.Value as a Json field or argument (RawMessage, `any`); indented output | scheduler, CLI | blocker | encoding/json RawMessage, MarshalIndent | S |
| 4 | Closed 2026-10-29 (std/crypto aead_key, hkdf). Public AEAD (AES-GCM, ChaCha20-Poly1305) and HKDF, for encrypted cookies and data at rest (both exist inside TLS) | web app | blocker | crypto/cipher, crypto/aes, crypto/hkdf, x/crypto/chacha20poly1305 | S |
| 5 | Closed 2026-10-29 (std/html, template.parse_html, decision 0408). HTML escaping and contextual auto escaping templates, a trusted HTML type, template functions | web app | blocker (XSS by default) | html, html/template, template.FuncMap | L |
| 6 | Closed 2026-10-29 (std/udp, Net.lookup_host, Net.dial_timeout, decision 0406; std/dns lookup_mx, lookup_txt, lookup_srv, lookup_addr, decision 0417). UDP sockets and a DNS lookup API (host, SRV), a dial timeout | exporter | blocker | net.ListenPacket, net.LookupSRV, net.DialTimeout | M |
| 7 | X.509 certificate parsing and a TLS connection's peer certificates | exporter | blocker | crypto/x509 ParseCertificate, tls.ConnectionState | M (the parser exists inside TLS) |
| 8 | Closed 2026-11-03 (`std/cron`). Cron schedules | scheduler | blocker | robfig/cron (outside Go's std) | M |
| 9 | Host facts: Hostname, Getpid, rusage, statfs, network interfaces | exporter, scheduler | blocker (statfs), awkward | os, syscall.Statfs, net.Interfaces | S each |
| 10 | std/metrics: gauges and collectors read at scrape time, deleting a series, Summary quantiles | exporter, scheduler | awkward | prometheus Collector, GaugeFunc, Summary | S to M |
| 11 | HTTP server: read and idle timeouts, header limits, a body limit per route, Request host, TLS state, raw query and escaped path, HTML error pages, Redirect | gateway, web app | awkward | net/http Server, MaxBytesReader, Request.Host, URL.RawQuery | S each |
| 12 | Text formatting: width, padding, zero padding, fixed precision floats; an aligned table writer | CLI | awkward | fmt verbs, strconv.FormatFloat, text/tabwriter | S |
| 13 | Closed 2026-11-03 (`std/heap`). A priority queue with Remove and Fix | scheduler | awkward | container/heap | S |
| 14 | A logger that carries fields (Logger.With) | scheduler | awkward | log/slog | S |
| 15 | Rate limiting and backoff closed 2026-10-29 (std/rate, std/backoff); migrations open. Rate limiting (token bucket), backoff, migrations | gateway, web app, scheduler | awkward | x/time/rate, cenkalti/backoff, golang-migrate (all outside Go's std) | S each |
| 16 | Closed 2026-10-29 (walk_dir, FileInfo.is_symlink, create_dir_all_mode, write_atomic, toml.encode, cli float_flag). Fs walk with SkipDir and early stop, the symlink bit in FileInfo, a mode on create_dir_all and atomic writes; a TOML encoder; a float flag | CLI | awkward | filepath.WalkDir, os.WriteFile, BurntSushi/toml | S each |
| 17 | Time.truncate, a Weekday number, a ticker | scheduler | minor | time | S |
| 18 | Design item (Xo Lang 3, 2026-11-03): a lazy `DirEntry` for `fs.walk_dir`. Go's WalkDir hands each entry with its name and type from the directory read and stats only on `Info()`; Xo's walk gives a full `FileInfo` per entry, so a walk that only wants names or the directory bit pays one stat each (bench/perf/walkdir walk_info 1.54 of Go's wall). A lazy entry (name, is_dir, is_symlink from the read; `info()` stats) would match Go and keep the clean path fast path | CLI, build tools | awkward | filepath.WalkDir, fs.DirEntry | M | **Implemented 2026-11-03** (decision 0483 Accepted by the owner): `DirEntry`, `read_dir_entries`, `walk_entries`, `walk_dir_entries`, `DirEntry.info`; host row in std-twin-walkdir-2026-11-03.md once measured.

Language obstacles the services met (not std, for the language owners):
a sort with a comparator (`sort_by` takes a key only, so a descending
`Str` sort cannot be written); `T??` does not parse (XO0160), so "leave
this optional field unchanged" is `Option[T?]`; no storable cancel
handle for one task (context.WithCancel); `after (expr) =>` in a
`select` parses as a call to `after`; no literal for json.Value objects.

Found wrong in the spec: spec/std.md section 27's TLS example returns
`Err(Error.msg("server refused TLS"))`, but Error has no `msg` (XO0601);
the services had to declare their own message error type, as Go's
`errors.New` would give them.

Proposed for 0.35 (the blockers that are small or that unblock a class
of service, in order): 1, 2, 3, 4, 7, 5, 6, then a small batch of 12, 13
and 9 (Hostname, Getpid). Cron (8) belongs in a module outside std, as
it is outside Go's.

## Survey of backend gaps (2026-10-20)

Five services written from spec/std.md alone (a CRUD API on Postgres, a
webhook receiver for GitHub, Stripe, and Twilio, a file upload service,
a job worker, and an auth service with JWT, TOTP, and OAuth2), type
checked with `xo check`, each missing piece marked where the code needed
it. Ranked by how many of the five hit the gap:

| Rank | Gap | Services | Go reference |
|---|---|---|---|
| 1 | Postgres driver (natively `sql.open` has no driver at all) | 5 of 5 | pgx, database/sql/driver |
| 2 | SqlValue types: bytes, JSON, arrays; driver error codes | 3 of 5 | database/sql, pgx |
| 3 | URL query and form encoding, escaping, repeated values | 3 of 5 | net/url |
| 4 | Base64 URL and raw encodings, base32, binary | 2 of 5 | encoding/base64, base32, binary |
| 5 | SHA-1, HMAC over other hashes, streaming hash | 2 of 5 | crypto/sha1, crypto/hmac, hash |
| 6 | Repeated header fields and cookies | 2 of 5 | net/http Header, Cookie |
| 7 | TLS on a Conn, SMTP, mail addresses | 2 of 5 | crypto/tls, net/smtp, net/mail (all done 2026-10-20) |
| 8 | JSON key names (keywords, camelCase) | 1 of 5 (most third party payloads) | encoding/json struct tags |
| 9 | Multipart, MIME types, content sniffing, ServeContent, streaming request bodies | 1 of 5 | mime, mime/multipart, net/http |
| 10 | Public RSA and ECDSA verify, big integers | 1 of 5 | crypto/rsa, crypto/ecdsa, math/big |
| 11 | UUID | 1 of 5 | google/uuid |
| 12 | Int to text in other bases, Float math, embed, XML, client IP parsing | 1 of 5 each | strconv, math, embed, encoding/xml, net/netip |
