# Changing a std error text: the error_text_* goldens print it too

Status: Observed (2026-11-13)

## Symptom

TestGolden/error_text_http failed on main after ea70c82ee (GO-2026-5039:
SmtpErr.display quotes its message, as Go 1.26.2 and later). Its gate ran
the http_* goldens, the module corpora and the Go-oracle diffs, all green;
want.txt line 14 still read `smtp 550 5.1.1 no such user`.

## Cause

The internal/llvm goldens named error_text_* print the text of one error per
std module (http, smtp, json, ...), so a module's error format shows up in a
golden whose name is not the module's. A gate that picks goldens by the
module's name misses them.

## Fix

When a change alters what an error's text or display prints, grep every
golden for the old form before gating, and run what it finds:

    grep -rln 'OLD TEXT' --include=want.txt internal test

and add `error_text_*` to the gate's golden pattern (TestGolden,
TestGoldenGoParity, arm64 TestGoldenRun). Fixed on main by 1986db811.
