# json.encode output written into HTML can close a script element

Status: Observed (2026-11-15, xo-4c in std/ui, fixed (private commit); audit of
std/, examples/, bench/ and the aws workstream repo found no other site)

## Symptom

A page that writes `json.encode_str(state)` into `<script>` (or into an
`on*` attribute) breaks when a string in the state holds `</script>`:
the browser ends the element there and parses the rest as markup, so
`"</script><b>"` in user data becomes an injection. std/ui's first page
state did this until (private commit).

## Cause

std/json's `encode` and `encode_str` do not escape HTML (spec/std.md 5,
by design: the output is JSON, byte for byte what a JSON consumer
expects). Go's `json.Marshal` does escape `<`, `>` and `&` as `<`,
`>` and `&`, so code ported from Go loses that protection
silently. Both escape U+2028 and U+2029 always.

## Fix

Pass JSON that goes into HTML through `json.html_escape` (Go's
HTMLEscape), write it with `json.json_writer()` (escape_html defaults to
true), or render it with std/template in HTML mode, whose JS context
escapes values as Go's jsValEscaper does. A test should feed
`"</script><b>"` and check the output holds `</script>` and not
the raw text (std/native/template_test.xo has one for the script element,
an object, an `on*` attribute and text).

## Rule

Never interpolate `json.encode` or `encode_str` output into HTML
directly. A JSON HTTP response is fine as `application/json`.
