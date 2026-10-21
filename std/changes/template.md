# std/template changes

## 2026-10-21: port: std/template, Go's text/template

- Kind: port
- Commit: 2f308b5f
- Found by: survey (mail and config templates; decision 0299)
- Go reference: text/template parse/lex.go, parse/parse.go, parse/node.go, exec.go, funcs.go; fmt's print.go and format.go for printf (go1.26.1)
- Test: std/native/template_test.xo (Go's output and errors on 205 templates); internal/llvm/testdata/golden/template_basic (every backend)

New module over `json.Value` data: every action, pipeline, and built in
function of text/template with Go's output and error texts.
