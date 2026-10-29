# A std module's native implementation is one file, and its `use std/NAME` only for types

Status: Observed (2026-10-29, std/html)

## Symptom

A new std module's native implementation split over two files
(`std/native/html.xo` and a generated `std/native/htmlentity.xo` with
its tables) fails to build a program that uses it: `XO0601: name
ENTITY not found` in html.xo, with XO1201 "this tuple pattern" and
"this `for` iteration ... support is planned" cascading from the
unresolved name. And the std store build (decision 0388) fails with
`std/native/html.xo:5:1: error XO0110: import std/html is never used`
when the native file names none of the module's types.

## Cause

The native implementation of `std/NAME` is the one file
`std/native/NAME.xo`; another file in std/native is its own module,
reached as `use native/OTHER` (std/native/json.xo uses the parser of
`native/rtjson` that way), not a second file of the same module. The
`use std/NAME` line at the top of a native file exists to name the
module's declared types (`url.EscapeErr`); a module whose
implementation needs none (std/native/binary.xo) has no such line, and
an unused one is the usual XO0110.

A `native/X` helper module is not a second file of the module either: it
loads as the intrinsics modules do (load_native_rt), so its functions are
runtime calls with the runtime ABI, and a call across the boundary with a
struct argument or an Option result fails at code generation (`arm64
backend: the runtime call rt.eq takes a htmlesc.Context`, `the runtime
call rt.index_tag_end returns a Int?`; html/template's escaper, 2026-10-29).
Its tests cannot run on their own either (`xo test --std-module=native/X`
finds no std module to load it with).

## Fix

Keep the tables and the helper code in the module's file (std/html's entity table is a
section at the end of std/native/html.xo; html/template's 1800 line
escaper is a section of std/native/template.xo). A `native/X` module
fits only plain functions over scalars, Str and Bytes. Write `use std/NAME` only when the file
names a type of the module. Run the module's tests with
`xo test --std-module=native/NAME` from the repository root with
XO_STD set (the module name is the test file's path under std without
`_test.xo`, as internal/stdtest names it; `--std-module=NAME` finds no
tests and says so without failing, and `xo test std/native --run=x` is
a directory error). A generated table of strings needs its control
characters escaped (`\n`, `\t`, `"`, `\`): Go's entity table has
`NewLine;` and `Tab;`, and a raw newline inside a literal is
XO0151/XO0161 at the next line.
