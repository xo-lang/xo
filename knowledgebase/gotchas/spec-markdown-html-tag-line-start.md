# Spec lines starting with an HTML tag name render as raw HTML

Status: Observed (2026-10-10)

Symptom: building xolang.dev, Hugo warned "Raw HTML omitted" for
`spec/core.md`; the rendered spec dropped the text of
"(`xo bind --c <header>` does it explicitly)" and showed `std/sys/` for
`std/sys/<os>` in the diagnostic table. GitHub's renderer does the same.

Cause: a code span wrapped so that the next line starts with `<header>`
(an HTML block tag name) starts a raw HTML block, ending the paragraph and
the code span. `<os>` outside backticks is read as an inline tag.

Fix: keep a code span holding `<...>` on one line (wrap before the
backtick), and put every `<placeholder>` inside backticks. Fixed in
commit 34f119c. To check, build the site (`hugo` in the site repo) and look
for the "Raw HTML omitted" warning.
