# Diagnostic messages that embed a directory change with the checkout location

Status: Observed (2026-10-11).

Symptom: `internal/format` TestCorpus failed on
`internal/check/testdata/golden/effect_ceiling_vendor`: "diagnostics
changed by formatting", the XO1306 message naming
`/Users/.../effect_ceiling_vendor/xo.mod` before and
`/var/folders/.../TestCorpus.../effect_ceiling_vendor/xo.mod` after.

Cause: XO1306 printed the requiring module's xo.mod through `relDir`,
which keeps a path absolute when it is outside the working directory.
TestCorpus checks the corpus from `../..` and from a temp copy, so both
paths were absolute and differed.

Fix: a message names a module by its path, not its directory (XO1306 now
says "not required by the xo.mod of example.com/shop"; same in
selfhost/check/deps.xo). The file a diagnostic is about belongs in its
span, which tests normalize; never put a directory in the message text.
