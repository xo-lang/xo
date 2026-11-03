# A multi-line string spliced into another lost its line break

Status: Observed (2026-11-03, compiler/wasm rt9.xo's per-kind fragments)

Symptom: a wasm build failed with `wat: call of unknown function
$sk_unwaitlocal.get`: two lines of runtime WAT ran together.

Cause: a `"""` literal drops the newline right after its opening quotes
and its common indentation (spec/core.md 1.6), so a fragment written as

    let f = """
          local.get $c
          ..."""

starts with `local.get`, not a newline. Spliced at the end of a line of
another `"""` string (`call $sk_unwait${f}`) it joined that line.

Fix: splice a fragment on a line of its own (`call $sk_unwait`, then
`${f}` alone on the next line), so the template's own newlines separate
it whatever the fragment starts or ends with.

How to avoid: never splice a `"""` fragment mid-line in generated text;
and build one program with the change before a gate (a wasip2 golden
with `xo build` fails at once), since a type check cannot see it.
