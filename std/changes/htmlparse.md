# std/htmlparse changes

## 2026-11-08: port: std/htmlparse, golang.org/x/net/html's tree builder and Render

- Kind: port
- Commit: this push
- Found by: the Polar Bear Blog port (its html2text port walks the tree html.Parse builds), decision 0570
- Go reference: golang.org/x/net v0.62.0 html (parse.go, node.go, const.go, foreign.go, doctype.go, render.go, escape.go)
- Test: internal/stdtest/testdata/corpus/htmlparse (1,789 cases of x/net's html5lib and Go tree-construction files, curated documents and 1,200 seeded random documents, tree dumps against Go with and without scripting and in fragment contexts, and Go's Render of every parsed document) on arm64, LLVM and the Go backend; `xo test --std-module=native/htmlparse` (Go's issue 16318, render, escape, the 512 element limit, a render and re-parse property, a seeded no-fault run)
- Timing: Mac, best of 7, Xo against x/net v0.62.0 on a 12 KB page of posts (bench/perf/htmlparse, twincheck): Parse 1,000 times 356 ms against 219 ms (1.6x Go), Render 1,000 times 56.5 ms against 49.1 ms (1.15x); about half of Parse is the tokenizer (std/htmltok, 1.4x Go). First port: Render 1.9x until the output went into one StrBuilder instead of a list of pieces; the scope functions take their tag names as strings, not lists, so no call allocates. Parallel lists for the node fields in place of an array of structs measured no faster and were dropped
- Shipped in: not yet released

`htmlparse.parse(text)`, `parse_with`, `parse_fragment`, `parse_fragment_with`, `Tree.render`, `render_node`, `dump`, `escape` (spec/std.md 75). One source (std/native/htmlparse.xo) for the native backends and, through std/go_compiled.txt, the Go backend; std/rt/htmlparse.go has the two error display methods. The tree is an arena (`Tree.nodes`, a node is its index, -1 is nil). Go's panics (a stack of open elements deeper than 512) are a `ParseErr` with Go's text.
