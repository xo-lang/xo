# 0408: html/template with contextual autoescaping and trusted content

Status: Accepted (2026-10-29; owner approved the json.Value variant for
trusted content on 2026-10-29 through the lead; milestone 0.35 S6;
xo-lang-6). Supersedes the stub's rt.process_token plan (never built).

## Context

std/template (decision 0299) is Go's text/template over a json.Value: the
actions, pipelines, built in functions and errors, with no custom
functions. A web application needs Go's html/template: the same language
with contextual autoescaping (a value is escaped for where it lands:
HTML text, an attribute, a URL, JavaScript, CSS), trusted content a
program vouches for (template.HTML and its siblings), and custom
functions (FuncMap). Go's data is `any` and its trusted types are Go
types a type switch sees; Xo's data is a json.Value, so a trusted value
needs a form the executor can tell from decoded input, which an attacker
controls.

## Decision

1. **Html mode of std/template.** `template.parse_html(name, src, left,
   right, funcs)` returns a Template whose set renders with Go's
   html/template escaping: the context machine (escape.go, context.go,
   transition.go), the escapers (html.go, attr.go, url.go, js.go, css.go)
   and their errors (ErrAmbigContext and the others, with Go's texts)
   ported to std/native/template.xo (as a section of it: a std module's
   native implementation is one file, and a `native/X` helper module is
   an rt module whose ABI takes no struct or Option across it); the Go
   backend runs html/template itself. The walk returns rewritten trees
   (Go edits nodes in place at commit), one per (template, context)
   pair under Go's derived names, and runs once at `parse_html` (and
   `add`); its errors are kept and reported by `render`, as Go reports
   them on Execute. A template parsed with `parse` escapes nothing, as
   before. `add` keeps the set's mode.
2. **std/html** (spec 39): Go's html package, `escape_string` and
   `unescape_string`, with Go's entity table; the html mode's attribute
   escaper uses it.
3. **Functions.** `type TemplateFn = fn(List[json.Value]) -> json.Value
   ! Str`; `parse` and `parse_html` take `funcs: Map[Str, TemplateFn]`
   (Go's FuncMap as a parse argument, since a Template is a value). A
   name the parser does not know stays Go's `function "x" not defined`.
   Arguments arrive as json.Values (the executor's values converted, a
   missing or nil value as Null), the result is loaded as data is, and an
   Err is Go's `error calling NAME: text`. Pure: a function with
   effects cannot be passed.
4. **Trusted content is a json.Value variant.** `json.Value.Trusted(kind:
   Str, s: Str)`, kind one of html, css, js, jsstr, url, attr, srcset
   (Go's HTML, CSS, JS, JSStr, URL, HTMLAttr, Srcset); made only by
   `template.html(s)`, `css`, `js`, `jsstr`, `url`, `attr` and `srcset`.
   `parse_value` and `decode` never produce it, so decoded input cannot
   forge it: a decoded `{"$html": "<script>"}` is an object and renders
   escaped. The html mode passes a Trusted value through unescaped in
   its own context and escapes it as text in any other (Go's rule); the
   text mode and every other reader see the string: `encode` writes it as
   a plain JSON string (a page object that is logged or stored must not
   fail late because one field is trusted), `get` and the template's
   `of_json` treat it as `Str`.
5. **Breaking change.** An exhaustive `match` on json.Value in user code
   gains a variant. Migration: add `Trusted(_, s) => ...` treating s as
   the string, or a wildcard arm. The CHANGELOG and the 0.35 release
   notes list it as breaking; `xo fix` adds the arm where the
   match is non-exhaustive (XO0301's machine fix adds the missing arms
   as holes; the program fills `Trusted(..) => ???` in), so the
   migration is `xo fix` and one edit per hole.

Threat model: the attacker controls template data (decoded JSON, query
strings, database rows) and never the template source or the program.
The only way to make a Trusted value is a call in the program, which is
the same guarantee Go's html/template gives through its types. Nothing
is random, so renders stay deterministic and std/template stays pure.

## Consequences

- json.Value has eight variants; the native encoder, the Go backend's
  jsonvalue.go and template.go, and std/native/template.xo's loader
  carry the arm; the repo's other matches (sql, pg, the cli servers)
  are updated in the same change.
- The html mode is a port of 3900 lines of Go (1850 of Xo) with Go's
  TestEscape table (107 of its 111 cases, over its data as JSON, generated
  by a Go program), TestErrors, TestEscapeSet and the escapers' own tests
  as its corpus; bench/perf/htmltemplate and its timing row against
  html/template beside it.
- Programs that matched json.Value exhaustively need the arm (above).

## Alternatives

- A per-process token (`$xo-trusted:<hex>:html` objects, rt.process_token
  from getentropy): no new variant, but a new intrinsic on three
  backends, a key that any code reading the object could copy, and the
  purity argument hanging on "compared, never rendered". Rejected by the
  owner for the variant.
- A fixed marker object (`{"$html": s}`): forgeable from decoded input.
  Rejected (the lead, 2026-10-29).
- Trusted types outside json.Value (a `Content` enum wrapping the data):
  the data's whole tree would change type, and `render(data: json.Value)`
  with it. Rejected.
- Erroring in `encode` on a Trusted value: fails the common case (log the
  page's data) for no safety gain, since the string is what it was
  anyway. Rejected.
