# A module import named like a function of the importing module

Status: Observed (2026-10-13, decision 0201).

**Symptom.** Adding `use native/llrt/scan` to std/native/llrt/jsontext.xo
made its existing calls `scan(b)` fail with `XO0407 scan is a module and
cannot be called`.

**Cause.** The import's name and jsontext's own function `scan` share
the module scope; the import wins at the call site.

**Fix.** Import under another name: `use native/llrt/scan as scans`.
