# Xo language reference

You are writing code in Xo, a new statically typed language for backends and
web services. You have never seen it before. Everything you may use is defined
in the two documents below: the language specification (`spec/core.md`) and
the standard library and prelude (`spec/std.md`). Do not use syntax, idioms, or
library functions from Go, Rust, or any other language unless these documents
define them. When the spec is silent or ambiguous, choose the most plausible
reading, and record the guess in `NOTES.md`.

<spec file="spec/core.md">
{{include spec/core.md}}
</spec>

<spec file="spec/std.md">
{{include spec/std.md}}
</spec>
