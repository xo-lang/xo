# gofmt splits a statement after its //line directive, shifting positions

Status: Observed (2026-11-07, Xo Lang 2)

## Symptom
On the Go backend a fault raised inside std's `cli.parse` reported the
wrong line: `at main.xo:11` for a call on line 6 (the line of the last
statement after it), and a line past the end of the file when the call
was a match scrutinee. Native and LLVM reported line 6. Faults in other
calls (`cli.help`, `Parsed.str`) were right, so the goldens never showed
it.

## Cause
compiler/gen writes one `//line main.xo:L:C` directive per statement and
the statement on one Go line. When the call's result holds lists, the
generator wraps it in a closure that marks them (`own_result`:
`func(v rt.Result[T, E]) rt.Result[T, E] { if v.Ok { rt.MarkP(&v.V) };
return v }(call)`). gofmt, run on the generated files, spreads that
closure over five lines. A `//line` directive sets the position of the
next line only, and Go counts every line after it as the next source
line, so the call, now on the sixth Go line of the statement, read as
L + 5.

## Fix
compiler/xo/gobackend.xo runs `gen.pin_line_directives` on each file
after gofmt: every further line of a statement gets a copy of the
statement's directive (lines inside a raw string or a block comment are
left alone). Goldens fault_line_after_call and
fault_line_match_scrutinee check the line on every backend, Go parity
included. Any new wrapper the generator writes on one line is covered;
a directive is only correct for one Go line unless repeated.
