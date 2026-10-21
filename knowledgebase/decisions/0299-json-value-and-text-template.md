# 0299: json.Value, and std/template (Go's text/template) over it

Status: Accepted (owner, 2026-10-20)

## Context

The coverage table lists text/template as planned (mail bodies with
`mail.compose`, decision 0290; config files; code generation). Go's
templates walk any value through reflection; Xo has none, so a template
needs data of a shape it can walk. std/json had only typed `encode` and
`decode`, nothing like Go's decoding into `any`.

## Decision

1. std/json gains `Value`: Null, Bool, Int, Num, Str, List, and Object
   (fields in the order of the text), with `to_value[T: Json]`,
   `parse_value`, `encode`, and `get` (spec/std.md 5.1). It is useful on
   its own (JSON whose shape varies).
2. A new module std/template ports Go's text/template (lexer, parser,
   executor, its actions, pipelines, and built in functions) and renders
   over a `json.Value`: `.Name` is an object field, `index` an array or
   object element, `range` iterates arrays and objects (objects in key
   order), `len` counts strings, arrays, and objects. `printf` takes Go's
   verbs over JSON values. No custom function maps in the first step; no
   html/template contextual escaping (`html` is the explicit escaper).
   Tests: Go's exec_test.go and parse_test.go tables translated to JSON
   data, natively and on the Go backend (which wraps text/template over
   the value's `any` form).

## Consequences

- Any type that derives `Json` is template data through
  `json.to_value(x)`; field names are its JSON names (decision 0277).
- Methods cannot be called from a template, and numbers are JSON
  numbers.
- std/template lands after json.Value (step 1 first, its own spec
  section and golden json_value).

## Alternatives

- A new `derive Template` generating a walker per type: exact Xo names
  and types, but a compiler derive on every backend for one module.
- `Map[Str, Str]` data: no loops or nesting, too small for mail and
  config templates.
- Mustache: smaller, but not Go's (decision 0276).
