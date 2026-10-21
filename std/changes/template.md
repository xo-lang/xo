# std/template changes

## 2026-10-21: perf: a Template keeps its parsed set

- Kind: perf
- Commit: (filled in after the push)
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
- Commit: (filled in after the push)
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
