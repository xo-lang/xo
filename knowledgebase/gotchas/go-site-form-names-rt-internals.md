# A `use go` call site form names something only package rt can name

Observed 2026-10-30 (decision 0441, binding version 33; fixed in 34).

## Symptom

A `use go` program fails to build on the Go backend with
`undefined: rt.xoNew_Go_io_Reader` (an implements constructor such as
`io.reader(read: ...)`, `http.handler`), `undefined: rt.T` or `rt.K` (a
method of a generic Go type), or earlier the same day
`undefined: rt.goCall_Go_time_sleep` (a value mode adapter). The adapter
itself is fine; the pasted call site form is not.

## Cause

The site form (internal/gobind gen.go siteForm) is the adapter's body
with every name of package rt qualified `rt.`, pasted into the program's
package (compiler/gen gosite.xo). Package rt's unexported names
(`xoNew_X`, `goCall_X`) and a generic receiver's type parameters are
visible inside rt's adapter and nowhere else, so the paste does not
compile. TestSiteForms on net/url and crypto/sha256 saw none of them.

## Fix

siteForm flattens `goCall_X` (its statements into the literal), and
writes a `//xo:site-error` record instead of a site form when the
adapter names another unexported rt identifier or is a generic type's
method: the Go backend then calls the adapter as before 0441 (that call
records the adapter's frame as the caller, 0441's only loss, for those
calls). Binding version 34 rebinds the version 33 records. When adding a
new kind of rt helper to the adapters, check `go test ./cmd/xo -run
'^TestUseGo'` on the Go backend, which builds programs through the site
forms, not only TestSiteForms.
