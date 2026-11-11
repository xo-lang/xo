# std/htmlparse changes

## 2026-11-11: refactor: std/htmlparse is one file (decisions 0588, 0597)

- Kind: refactor
- Commit: this push
- Found by: decision 0597 (std/native/htmlparse.xo imports nothing native)
- Go reference: none (no behavior change)
- Test: TestStdCorpora htmlparse on go, arm64, llvm; xo test --std-module=htmlparse (14 passed) on arm64 and llvm
- Shipped in: not yet released

std/native/htmlparse.xo moved into std/htmlparse.xo, its tests into
std/htmlparse_test.xo. The Go backend compiles it from that source; the
Go twin of its error displays (std/rt/htmlparse.go) is gone.

## 2026-11-08: perf: a node's namespace is tested empty by its length

- Kind: perf
- Commit: this push
- Found by: profile of `parse` on host 1 (XO_PROF=cpu, 5 kHz): obj.str_eq 4.6% of samples across 11 callers, 3 of 11 ms from `run`'s `namespace != ""`
- Go reference: golang.org/x/net v0.62.0 html (parse.go)
- Test: internal/stdtest/testdata/corpus/htmlparse unchanged on arm64, LLVM and the Go backend
- Timing: host 1 (linux/arm64) A/B against the parent, bench/perf/htmlparse `parse`: 0.989 (0.981 to 0.994, n=6; 247.2 to 244.4 ms)
- Shipped in: not yet released

The 17 `namespace == ""` and `!= ""` tests (every element visit in `run`, the scope walk and the adoption agency) compare lengths
instead of calling the string compare. Measured and not done: integer atoms for tags would save at most the remaining str_eq (about 3.5%, spread over
11 callers, none above 1.3%); a per-node flag needs a field on the public `Node`.

## 2026-11-08: perf: Parse follows the tokenizer's speedup

- Kind: perf
- Commit: this push
- Found by: the lead's queue after the port (tokenizer 55% of Parse at 1.65x x/net)
- Go reference: golang.org/x/net v0.62.0 html (parse.go)
- Test: internal/stdtest/testdata/corpus/htmlparse unchanged on arm64, LLVM and the Go backend
- Timing: host 1 (linux/arm64) A/B against the push's parent, 7 rounds, bench/perf/htmlparse `parse` workload: 0.749 of the parent (0.743 to 0.752, n=6; 315.5 to 236.2 ms; x/net 209 ms in the port's run, about 1.1x Go on this host); the changes are in std/htmltok (see its log)
- Shipped in: not yet released

No change to the parser itself; the tokenizer it drives is faster (std/changes/htmltok.md).

## 2026-11-08: port: std/htmlparse, golang.org/x/net/html's tree builder and Render

- Kind: port
- Commit: this push
- Found by: the Polar Bear Blog port (its html2text port walks the tree html.Parse builds), decision 0570
- Go reference: golang.org/x/net v0.62.0 html (parse.go, node.go, const.go, foreign.go, doctype.go, render.go, escape.go)
- Test: internal/stdtest/testdata/corpus/htmlparse (1,789 cases of x/net's html5lib and Go tree-construction files, curated documents and 1,200 seeded random documents, tree dumps against Go with and without scripting and in fragment contexts, and Go's Render of every parsed document) on arm64, LLVM and the Go backend; `xo test --std-module=native/htmlparse` (Go's issue 16318, render, escape, the 512 element limit, a render and re-parse property, a seeded no-fault run)
- Timing: host 1 (linux/arm64), best of 7, Xo against x/net v0.62.0 on a 12 KB page of posts (bench/perf/htmlparse, twincheck's programs cross built): Parse 1,000 times 344 ms against 209 ms (1.65x Go), Render 1,000 times 53 ms against 46 ms (1.15x). Profile of Parse (XO_PROF=cpu, 5 kHz, host 1) is flat: the tokenizer (std/htmltok next and token, itself 1.4x Go) is 55% of the samples, the tree builder's largest items are tag name comparisons as text where Go compares atoms (str_eq 6%), append_child 6% and run 6%; no one or two fast paths explain the gap, so none was ported. First port: Render 1.9x until the output went into one StrBuilder instead of a list of pieces; the scope functions take their tag names as strings, not lists, so no call allocates. Parallel lists for the node fields in place of an array of structs measured no faster and were dropped
- Shipped in: not yet released

`htmlparse.parse(text)`, `parse_with`, `parse_fragment`, `parse_fragment_with`, `Tree.render`, `render_node`, `dump`, `escape` (spec/std.md 75). One source (std/native/htmlparse.xo) for the native backends and, through std/go_compiled.txt, the Go backend; std/rt/htmlparse.go has the two error display methods. The tree is an arena (`Tree.nodes`, a node is its index, -1 is nil). Go's panics (a stack of open elements deeper than 512) are a `ParseErr` with Go's text.
