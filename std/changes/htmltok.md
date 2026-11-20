# std/htmltok changes

## 2026-11-19: fix: &nLt; and &nGt; are their two code points

- Kind: fix
- Commit: this push
- Found by: stdproven gotests (decision 0720, task #403): x/net v0.62.0's unescapeTests "nLt then text" and five more
- Go reference: golang.org/x/net v0.62.0 html/entity.go (entity2 holds nLt; and nGt;, which Go's html and std/htmlent leave out)
- Test: std/htmltok_test.xo "nLt; and nGt; are x/net's two code point references (escape_test.go)" (failed before, passes after); std/evidence/htmltok/gotests (248 cases, 0 mismatches)
- Timing: none (two name checks after the entity tables miss)


## 2026-11-16: feature: Text, TagName, TagAttr and the String methods; fix: token takes a tag's data

- Kind: feature, fix
- Commit: this push
- Found by: task #89 (features Proven): x/net/html pinned in stdaudit (outside.txt), its Tokenizer.Text, TagName, TagAttr, Token.String, TokenType.String and UnescapeString were missing
- Go reference: golang.org/x/net v0.62.0 html/token.go and escape.go
- Test: testdata/corpus/htmltok's API table (596 streams through Text, TagName, TagAttr and Token.String against x/net, go, arm64, LLVM); std/htmltok_test.xo "token takes the token's data" (it failed before: a second `token` gave the tag again)
- Timing: none (`next` and `token` keep their paths)

`text()`, `tag_name()` and `tag_attr()` are Go's accessors, each taking
what it returns; `Token.display()` and `TokenType.display()` are Go's
String; `escape_string` and `unescape_string` are EscapeString and
UnescapeString. `token()` now takes a tag's name and attributes as Go's
Token does, so a second call gives an empty token.

## 2026-11-16: perf: character references resolved with fewer compares and no allocation for ASCII

- Kind: perf
- Commit: this push
- Found by: task #87 (std/evidence/htmlparse/perf.md, ent 1.136 of Go on host 1), XO_PROF=cpu on bench/perf/htmlparse ent: htmlent.search 33%, the numeric reference's builder and UTF-8 check
- Go reference: golang.org/x/net/html token.go and escape.go (unescapeEntity; Go's entity map is a hash)
- Test: std/htmlent_test.xo (ENTITY_AT against ENTITY, every name found, names outside the table), std/htmltok_test.xo (numeric references below 128 and around it); bench/perf/htmlparse ent on the Mac: instructions 7.50G to 6.24G (0.832), parse and tok unchanged
- Shipped in: not yet released

A character reference is resolved faster: a numeric reference below 128 is a one byte view of an ASCII constant (no builder, copy or UTF-8 check), and a named reference searches only the entity names that share its first letter (std/htmlent's ENTITY_AT, about 6 compares where the whole table took 11). Output unchanged.

## 2026-11-10: internal: merged into std/htmltok.xo

- Kind: internal
- Commit: this push
- Found by: decision 0588 (one file per std module), after std/htmlent merged
- Go reference: golang.org/x/net/html token.go (v0.62.0)
- Test: `xo test --std-module=htmltok` and `htmlparse` (native, LLVM); the htmltok and htmlparse corpora on go, arm64, llvm; bench/perf/htmltok next and token checksums equal on arm64, llvm, go
- Timing: host 1 A/B against main, bench/perf/htmltok token: 0.987 (0.98 to 1.006, n=7), within noise
- Shipped in: not yet released

std/native/htmltok.xo's bodies and std/native/htmltok_test.xo's tests move into std/htmltok.xo and std/htmltok_test.xo; the generated Go (std/rt/std_gen_htmltok.go) is replaced by the merged module's own package. Output unchanged.

## 2026-11-08: perf: the tokenizer reads in place and skips copies it does not need

- Kind: perf
- Commit: this push
- Found by: a host 1 profile of tokenizing alone (tokenizer_rb 16%, entity_of 16%, enum equality calls 8%), the queue after decision 0570
- Go reference: golang.org/x/net v0.62.0 html (token.go readByte, Token)
- Test: internal/stdtest/testdata/corpus/htmltok and htmlparse unchanged on arm64, LLVM and the Go backend; `xo test --std-module=native/htmltok`
- Timing: host 1 (linux/arm64) A/B against the push's parent, 7 rounds, bench/perf/htmlparse `tok` workload (tokenize only, a Token per token): 0.633 of the parent (0.622 to 0.664, n=6; 188.2 to 119.1 ms). By step: names with no upper-case byte or NUL skip the lower_bytes and replace_nul copies 0.962; `entity_of` answers the common references first 0.879; `tokenizer_token` reads the token type with one match 0.969; `read_tag` and the attribute value reader peek the next byte in place 0.903; `next` reads `<` and the byte after it in place 0.842. A run copy in `unescape` measured 1.008 (worse) and was dropped. The `entity_of` step costs 1.7% on a page of only uncommon references (`ent` workload, 1.017, 1.012 to 1.03, n=7).
- Shipped in: not yet released

Output unchanged (the corpora are byte for byte as before). The in-place reads apply when the token has no size limit (the default); with `set_max_buf` the byte-at-a-time path is as before.

## 2026-11-07: port: std/htmltok, golang.org/x/net/html's Tokenizer

- Kind: port
- Commit: this push
- Found by: the Polar Bear Blog port (its bluemonday and html2text ports consume this tokenizer), decision 0548
- Go reference: golang.org/x/net v0.62.0 html (token.go, escape.go unescape)
- Test: internal/stdtest/testdata/corpus/htmltok (about 2,000 inputs in the plain, AllowCDATA and fragment modes and with buffer limits, token streams and raw texts against Go) on arm64, LLVM and the Go backend; `xo test --std-module=native/htmltok`
- Timing: host 1 best of 7, Xo against x/net v0.62.0 Tokenizer on a 12 KB page tokenized 2,000 times (bench/perf/htmltok): next 174 ms against 96 ms (1.8x Go), next plus token 287 ms against 207 ms (1.4x); the first port ran 328 ms and 450 ms (3.3x and 2.1x) until the scans of text, tag names, attribute keys and values and white space ran over a local index when there is no buffer limit (profile: tokenizer_rb 54% of the samples)
- Shipped in: not yet released

`htmltok.tokenizer(text)`, `next`, `token`, `raw`, `err` (spec/std.md 71). One source (std/native/htmltok.xo) for the native backends and, through std/go_compiled.txt, the Go backend. std/native/html.xo exports its entity lookup (`entity_of`, `entity2_of`, `REPLACEMENT`) for the attribute and text unescape, which differs from html.UnescapeString (x/net's attribute rule and numeric reference limits).
