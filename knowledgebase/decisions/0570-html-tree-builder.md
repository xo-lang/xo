# 0570: std/htmlparse: the HTML5 tree builder (port of golang.org/x/net/html's Parse)

Status: Accepted (owner, 2026-11-08, option 1 of the lead's question; Xo Lang 3; needed by the Polar Bear Blog port's html2text)

## Context

Decision 0548 ported the tokenizer of golang.org/x/net/html and said the sanitizer and the text converter need tokens,
not a DOM. That held for bluemonday (libs/sanitize matches Go byte for byte on 4578 inputs, 2026-11-08) and not for
jaytaylor.com/html2text, which calls `html.Parse` and walks the DOM it returns: the tree builder inserts the implied
html, head, body and tbody elements, closes `<p>` and `<li>` implicitly, reparents misnested formatting elements (the
adoption agency algorithm) and foster-parents table content. html2text's output depends on that tree. Go programs use
`html.Parse` widely (link extraction, scraping, test assertions), and Xo has no tree builder.

## Decision

1. New module `std/htmlparse`: a port of x/net/html's parser (parse.go insertion modes, the adoption agency algorithm,
   foster parenting, foreign content for svg and math, quirks mode from doctypes, fragment parsing with a context
   element), its node type (`Node` with type, data, namespace, attributes and tree links), `render` and `escape`,
   and the atom table it keys on, with attribution and the BSD-3 notice. The tokenizer stays std/htmltok.
2. API: `parse(text) -> Node`, `parse_fragment(text, context: Node?) -> List[Node]`, Node walking by index (an arena
   like libs/markdown, no shared pointers), `render(node) -> Str`. Go's names where Xo's idiom allows.
3. One Xo source for every backend, pure.
4. Proof (decision 0276): Go's parse_test.go cases (the html5lib tree construction tests it runs, `testdata/webkit`) ported
   as a corpus, a differential against the Go package over generated and mutated documents (internal/stdtest,
   scripts/stdcorpus: the rendered tree and the tree dump for each input), a fuzz test for no panic, and a timing row
   against x/net/html's Parse on a 12 KB page (bench/perf/htmlparse), as for htmltok.
5. Consumer: libs/html2text in the polarbearblog repo, proven against jaytaylor.com/html2text on the blog's markdown
   corpus.

## Consequences

- Unblocks html2text and any `html.Parse` based Go port; std grows by about 3,000 lines and an atom table.
- The x/net license notice goes in the module header and std/changes/htmlparse.md.

## Alternatives

- A small tree builder inside libs/html2text for the HTML blackfriday emits: about 300 lines and quick, but it differs from
  Go on the raw HTML a post may contain (implied tags, misnesting), so the blog would not match the Go app there.
- Rewriting html2text on tokens (no tree): the same divergence, and it drops html2text's structure rules.
