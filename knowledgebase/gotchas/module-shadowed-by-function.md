# A module import named like a function of the importing module

Status: Observed (2026-10-13, decision 0201).

**Symptom.** Adding `use native/llrt/scan` to std/native/llrt/jsontext.xo
made its existing calls `scan(b)` fail with `XO0407 scan is a module and
cannot be called`.

**Cause.** The import's name and jsontext's own function `scan` share
the module scope; the import wins at the call site.

**Fix.** Import under another name: `use native/llrt/scan as scans`.

**Same trap with a local name (Observed 2026-10-14, zero-C small files
batch).** Rewriting `rt.str_slice(raw, 0, hash)` to raw.xo's
`raw.str_slice(raw, 0, hash)` in std/native/llrt/http1.xo, whose
function has a parameter `raw: Str`, gave `XO0601 Str has no field or
method str_slice`: the parameter shadows the module. Fix as above:
`use native/llrt/raw as rawb`. A mechanical `rt.` to `<module>.`
rewrite should grep each file for a local of the module's name first.

