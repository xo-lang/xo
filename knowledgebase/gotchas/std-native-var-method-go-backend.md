# A std method with a var receiver is not compiled from std/native on the Go backend

Status: Observed (2026-10-27)

## Symptom

A new std type declared in std/xmlcodec.xo (`struct Writer {}`, methods
`fn (var w: Writer) start(...)`) and written in std/native/xmlcodec.xo
built and ran on arm64 and LLVM, but `go run ./scripts/stdgen` wrote a
std/rt/std_sigs_xmlcodec.go that expects `(*Xmlcodec_Writer).M_start`
and the rest by hand in std/rt, and the Go backend's corpus failed to
build (`undefined: rt.Xml_escape_to` for the new free function of
std/xml, a module the Go backend does not compile from std/native).

## Cause

The Go backend compiles a std module from its std/native file only for
the members `impl_member` (compiler/gen call.xo) finds there: a free
function by name, a method as `type_method`, and never a static method,
a method with a `var` receiver, an interface method or a generic one
(decision 0312). Everything else is a hand written function in std/rt,
whose signature std_sigs checks. std/xml itself is hand written in
std/rt/xml.go (Go's encoding/xml).

## Fix

For a type the Go backend should compile from std/native, declare its
fields in the declaration file and give it free functions with a `var`
parameter (`fn write_start(var w: Writer, ...)`), as xmlcodec.Writer
does. A new function of a module the Go backend writes by hand (std/xml)
also gets its Go body in std/rt (`Xml_escape_to` in std/rt/xml.go);
run `go run ./scripts/stdgen` and build the corpus on `--backend=go`
before the gate.
