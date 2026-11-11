# 0548: std/htmltok: an HTML5 tokenizer (port of golang.org/x/net/html's Tokenizer)

Status: Accepted (owner, 2026-11-11; implemented on main). Was: Proposed (Xo Lang 3, 2026-11-07; needed by the Polar Bear Blog port, whose bluemonday and html2text ports sit on it)

## Context

std/html has escaping only (decision 0408) and std/xml has a lenient XML decoder, which is not the HTML5 tokenization
state machine (raw text elements, `<!--` quirks, attribute and entity rules, `<script>` and `<textarea>` handling). The blog
port needs the HTML sanitizer (bluemonday) and the HTML to text converter (html2text); both consume x/net/html's Tokenizer
and its `Token` values. Std follows Go's standard library and x/ packages (decision 0276).

## Decision

1. New module `std/htmltok`: a port of the Tokenizer of golang.org/x/net/html (token.go, with attribution): `tokenizer(text)`,
   `next()` returning a Token (Error, Text, StartTag, EndTag, SelfClosingTag, Comment, Doctype) with `data`, `attrs`,
   entity-unescaped text and attribute values (through std/html's table), `raw` text, `raw_tag` for script, style,
   textarea, title, iframe, noembed, noframes, noscript, plaintext, xmp, and the `max_buf` bound; Go's error texts.
2. Only the tokenizer, not the tree builder (parse.go): the sanitizer and the text converter need tokens, not a DOM.
3. One Xo source for every backend, pure; the input stays borrowed (a view), like std/xml's Reader.
4. Proof: Go's token_test.go cases ported, a corpus differential against the Go package over generated and mutated
   documents (internal/stdtest, scripts/stdcorpus), a fuzz test for no panic and token reconstruction, and a timing row
   against Go's tokenizer.

## Consequences

- Unblocks std-level ports of bluemonday's UGC policy and html2text (libs/ of the polarbearblog repo).
- The x/net license notice goes in the module header and std/changes/htmltok.md.

## Alternatives

- Reuse std/xml's non-strict decoder: rejected, its states differ from HTML5 (raw text, comment and doctype edge cases).
- Port the whole x/net/html parser: rejected for now, about four times the size and no consumer.
