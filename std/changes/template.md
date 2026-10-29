# std/template changes

## 2026-10-29: feature: the html mode (contextual autoescaping, decision 0408)

- Kind: feature
- Commit: this commit
- Found by: milestone 0.35 S6
- Go reference: html/template (go1.26.1: context.go, transition.go, escape.go, html.go, attr.go, url.go, js.go, css.go, content.go, error.go)
- Test: std/native/template_test.xo (Go's TestEscape over its data as JSON, TestErrors, TestEscapeSet, trusted content, and Go's escaper tests)

`parse_html`: Go's html/template as a mode of the set; the Trusted
values of `template.html` and its siblings pass through in their context.
The escaping is done once at parse_html (and add) for the root template
and kept with the parsed set; render of another name escapes from it.
The four TestEscape cases left out, none a bug (the data is a json.Value):
- `typedNilValue` (`{{.Z}}` over a nil `*int` prints `&lt;nil&gt;`) and
  `jsNilValueTyped` (`typeof{{.Z}}` gives ` null `): JSON has no typed nil,
  a `null` is Go's untyped nil, whose cases (`nilValue`, `jsNilValue`) pass.
- `badMarshaler` (a `json.Marshaler` whose MarshalJSON returns bad JSON
  renders ` /* json: error calling MarshalJSON ... */null `) and
  `jsMarshaler` (a custom MarshalJSON): a json.Value carries no
  marshaler; a program converts with `json.to_value` first, so the text a
  template marshals is always valid JSON.
Timing against Go's html/template: see bench/perf/template.

## 2026-10-29: feature: custom functions (FuncMap) and the trusted content constructors

- Kind: feature
- Commit: this commit
- Found by: milestone 0.35 S6 (decision 0408)
- Go reference: text/template Funcs, html/template HTML, CSS, JS, JSStr, URL, HTMLAttr, Srcset
- Test: std/native/template_test.xo ("custom functions", "trusted content prints its string")

`parse(..., funcs: Map[Str, TemplateFn])`: a `TemplateFn` takes its
arguments as json.Values and returns one, or an Err text (`error calling
NAME: text`); `template.html(s)` and the six siblings make
`json.Value.Trusted` values for the html mode to come.

## 2026-10-28: perf: the lexer finds `{{` and `*/` with Bytes.find

- Kind: perf
- Commit: this commit
- Found by: xo vet --cost XO0913 (cmd/xo TestVetCostStd): index_of compared a slice of the input at every offset
- Go reference: text/template/parse lex.go (strings.Index(l.input[l.pos:], l.leftDelim))
- Test: internal/stdtest/testdata/corpus/template (go, arm64, LLVM)

index_of is `b[from..].find(p)`, Go's strings.Index, where it made and
compared a view at each offset. Quiet --ab, 15 rounds, the parent's std as
A (instructions, /usr/bin/time -l): bench/perf/template parse 0.956,
render 1.000; quiet --ab, 15 rounds: parse wall 0.948 (0.943 to 0.956).

## 2026-10-28: perf: an integer is written into the output in place

- Kind: perf
- Commit: this commit
- Found by: bench/perf/template render's heap profile (num.append_int 37.5% of its allocations: an IntV's "${n}" text made by fmt_v, then appended to the output and dropped)
- Go reference: text/template exec.go printValue (fmt.Fprint into the writer, no intermediate string for an int)
- Test: internal/stdtest/testdata/corpus/template (go, arm64, LLVM); twincheck's checksums agree on none, parse, render

An action whose value is an integer appends its digits to the output
(`"${s.out}${n}"`, which llrt_concat_int writes in place when the output
is unique), where fmt_v first made the number's text as a Str of its own.
Other values print as before. Output is unchanged. Quiet --ab, 15
rounds, the parent's std as A: render wall 0.934 (0.931 to 0.943); Xo / Go about 1.08 (estimate: 1.156 x 0.934), from 1.156, instructions 0.974.

## 2026-10-27: perf: a visit sets its node context from the variant it already matched

- Kind: perf
- Commit: this commit
- Found by: bench/perf/template render at about 1.26x Go after the map order change; its CPU profile (at_node, at_arg and at_pipe about 20%: node_pos and node_tag, or arg_pos and arg_tag, each a match over the variant at every visit); a stub that set no context measured 0.886 of the instructions
- Go reference: text/template exec.go state.at (a pointer stored per visit)
- Test: internal/stdtest/testdata/corpus/template (go, arm64, LLVM; its error cases name the node at the failure); internal/llvm and internal/arm64 template_basic; twincheck's checksums agree on none, parse, render

walk sets the context (kind, variant index and position) in each arm of
its match on the node, eval_command in the arms of its match on the
first argument (fields, chains, variables) and walk_range from the range
node it unpacks, through Ex.at, where at_node and at_arg matched the
node again for its position and again for its variant. The context an
error names is unchanged. Quiet --ab, 15 rounds, the parent's std as A:
render wall 0.920 (0.915 to 0.926), instructions 0.893; Xo / Go about
1.16 (estimate: 1.595 x 0.789 x 0.920), from 1.26.

## 2026-10-27: perf: a map's fields are sorted only where their order shows

- Kind: perf
- Commit: this commit
- Found by: bench/perf/template render at 1.60x Go (re-measured on main d088ecee3); its CPU profile (of_json 18% cumulative, list_sort_by of every object's fields 11%, at every render)
- Go reference: text/template exec.go walkRange (a map ranged in sorted key order) and fmt's sorted map printing; a field read is a map lookup (go1.26.1)
- Test: internal/stdtest/testdata/corpus/template (go, arm64, LLVM); internal/llvm and internal/arm64 template_basic; twincheck's checksums agree on none, parse, render

render converted each object of its data to a map sorted by key, every
render. A field read, index and len do not depend on the order (a read
takes a key's first field either way, as before), so the fields now
stay in the data's order and map_order sorts them where the order shows:
a range over a map and a map printed with %v. A repeated key keeps the
old form there, every entry of it taking its first field's value. Output
is unchanged. Quiet --ab, 15 rounds, the parent's std as A: render wall
0.789 (0.785 to 0.795), instructions 0.817; Xo / Go about 1.26
(estimate: 1.595 x 0.789), from 1.595.

## 2026-10-27: perf: render converts its data once and ranges with no list of pairs

- Kind: perf
- Commit: this commit
- Found by: bench/perf/template render; its CPU profile (of_json 22%: a key list sorted and searched for every object of the data, every render; the variable list copied at every range iteration and if or with)
- Go reference: text/template exec.go walkRange, oneIteration, state.pop (go1.26.1)
- Test: internal/stdtest/testdata/corpus/template (go, arm64, LLVM); internal/llvm and internal/arm64 template_basic; twincheck bench/perf/template

An object of the data is sorted by key once (not at all when its keys
are in order; a repeated key keeps the old form, every entry of it
taking its first field's value), where every key was searched for among
the fields; the variables a range iteration, if or with declared are
dropped in place instead of copying the list, and only when there are
some; range calls one iteration per element instead of building the
list of (index, element) pairs first. Output is unchanged. Quiet --ab,
15 rounds, one pair per job: render wall 0.859 (0.851 to 0.865), instructions
0.831, for the data converted once and the variables dropped in place
(the parent's std as A); 0.994 (0.989 to 0.999), instructions 0.994, for
range with no list of pairs (against the first).

## 2026-10-27: perf: a node visit stores no value

- Kind: perf
- Commit: this commit
- Found by: bench/perf/template, the std Go twin (render 2.92x Go; dup and drop of nodes and contexts about a quarter of it)
- Go reference: text/template exec.go state.at (go1.26.1)
- Test: internal/stdtest template corpus (Go's error texts with their `at <...>` node); internal/arm64 template_basic; bench/perf/template twincheck

state.at kept the node in the executor (a Ctx made per visit, the node
retained, the last one dropped); it now records the node's kind, variant
and position, and an error finds that node in the tree to print it
(kind, variant and position name one node: a parenthesized pipeline and
its first argument share a position, not a variant). Quiet --ab, 15
rounds: render wall 0.639 against the parent (a stub that kept no context
at all: 0.771), 1.86x Go.

## 2026-10-23: behavior: TemplateErr prints its message (decision 0358)

- Kind: behavior
- Commit: this commit
- Found by: the Intel field report on 0.26.0 (error texts nested Debug output)
- Go reference: net/http, net/smtp and text/template error texts
- Test: golden error_text_http (arm64, LLVM, Go parity)

`Display` of `TemplateErr` printed `TemplateErr(msg: "...")` on the Go
backend; it prints Go's text (`template: t:1: function "nope" not defined`)
everywhere. `Debug` and value comparison are unchanged.

## 2026-10-21: perf: a Template keeps its parsed set

- Kind: perf
- Commit: 69271e2a
- Found by: XO_PROF=cpu (a render parsed the set again: 52% of what was left after the error text fix)
- Go reference: text/template (go1.26.1: Parse once, Execute many)
- Test: internal/stdtest/testdata/corpus/template (go, arm64, LLVM); golden template_basic

`Template` gains an opaque field `parsed` (an opaque `Parsed` struct; the
trees stay internal to std/native/template.xo): `parse` and `add` fill it,
`render` and `render_named` read it instead of parsing the sources again, as
Go's Execute does. 100,000 renders of a template with a range, an if, and
four fields, user CPU: 1.38 s to 0.47 s with the error text change (Go's
text/template 0.22 s). The Go backend is unchanged (text/template itself).

## 2026-10-21: perf: error context written only for errors

- Kind: perf
- Commit: cdfa66e6
- Found by: XO_PROF=cpu of 200,000 renders (the text of every visited node, pipeline, and command was built for the error message `executing "t" at <...>`, 40% of a render)
- Go reference: text/template exec.go state.at and errorf (go1.26.1)
- Test: internal/stdtest/testdata/corpus/template (go, arm64, LLVM)

The executor keeps the node it is at and writes its text only when it builds
an error, as Go's state.at keeps the node. 200,000 renders of a template
with a range, an if, and four fields: native CPU 2,640 ms to 1,660 ms. A
render still parses the set each time (a Template holds sources, not
trees), which is now half of it; Go parses once.

## 2026-10-21: fix: printf, the escapers, eq, and slice as Go's text/template

- Kind: fix
- Commit: dfaf6b05
- Found by: differential test against Go (scripts/stdcorpus template: 3,000 generated templates)
- Go reference: text/template exec.go, funcs.go (evalArgs, printableValue, eq, canCompare); fmt print.go and format.go (badVerb, printValue, fmtFloat); strconv ftoa.go fmtX (go1.26.1)
- Test: internal/stdtest/testdata/corpus/template

The native port differed from Go in eight ways:

- `%c` and `%q` of an integer that is not a code point faulted; they are U+FFFD now.
- `%U` of a negative printed it signed.
- A bad verb printed its value without the verb's width and precision (`%!f(string=   he)`).
- A nil inside a list or map printed as `%!d(<nil>)` instead of `<nil>`.
- Strings and bools were not padded with `0`.
- `%x` and `%X` of a float were bad verbs instead of hexadecimal floats.
- `html`, `js`, and `urlquery` printed a nil argument as `<nil>`, not `<no value>`.
- `eq` of a list and a map gave the single-type error.

`slice` of a string cut inside a rune now keeps the rest, with U+FFFD for
the cut bytes, where it gave "". On the Go backend the output is valid
text (decision 0259).

## 2026-10-21: port: std/template, Go's text/template

- Kind: port
- Commit: 2f308b5f
- Found by: survey (mail and config templates; decision 0299)
- Go reference: text/template parse/lex.go, parse/parse.go, parse/node.go, exec.go, funcs.go; fmt's print.go and format.go for printf (go1.26.1)
- Test: std/native/template_test.xo (Go's output and errors on 205 templates); internal/llvm/testdata/golden/template_basic (every backend)

New module over `json.Value` data: every action, pipeline, and built in
function of text/template with Go's output and error texts.
