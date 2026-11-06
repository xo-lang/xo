# `==` on a value holding an http.Response was XO1201, and only `xo test` said so

Status: Observed (2026-11-06, azure field report, xo abd04eb1f)

## Symptom

A test file of a library compared a `Result[_, azcore.CallErr]` with `==`
(the error's `ResponseError` holds an `http.Response`). `xo test` reported
XO1201 "`==` on values of type `Result[...]`, which hold functions or
runtime handles", plus eight XO1201 "an interface value's type number
given after skipped code" at files nobody touched. `xo check` in the
package directory reported 0 errors.

## Cause

Three separate gaps:

- The checker types `http.Response` from std's declaration (status,
  headers, body: comparable, core 2.9). Its native implementation
  (std/native/http.xo) adds hidden fields, one an `rt.Any?` holding a
  streamed body's function, so the SSA builder's `comparable` said no.
  `http.Request` had a special case, but only for `==` on a Request
  itself, not on a value holding one.
- `type_id` (compiler/ssa iface.xo) reported every interface type number
  given after any skipped code as XO1201: a rule from comparing with the
  Go builder (decision 0150, stage 2), deleted in decision 0215, that
  turned one unsupported construct into an error at every later
  interface value.
- `xo check`'s support check (decision 0523) lowered every function of
  the root modules, but not their test blocks.

## Fix

`eq_fields` names the std fields of the native `http.Request` and
`http.Response`; `comparable` looks only at them, and a value holding
either compares through `eq_coll`, which compares only them (golden
`response_eq`). The late type number XO1201 is gone. The check's support
check declares a function per test block of the root modules
(`declare_test_roots`; cmd/xo `TestCheckSupportTests`).

A std type whose native implementation adds fields must be listed in
`eq_fields` if the hidden fields are not comparable, or every `==` on a
value holding it fails natively while the checker accepts it.
