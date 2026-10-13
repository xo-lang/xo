# `when` cannot name a field or a variable

Status: Observed (2026-10-13, decision 0193 step 5)

Symptom: a struct field `when: List[Int]` and a local `var when:
List[Int]` in std/native/llrt/tz.xo (Go's zoneTrans.when) gave a cascade
at the first use: `XO0502: cannot assign to this expression`, `XO0406:
operator <= is not defined for List[Int] and Int`, `XO0301: match on
Bool is not exhaustive`, and `XO0160: expected -> after the when branch
condition`, none naming the keyword.

Cause: `when` is a keyword (match guards and `when` branches), so
`when[j] <= now` parses as the start of a `when` expression.

Fix: rename (`times`). Like requires-is-a-keyword.md, the parser could
say "`when` is a keyword" when an identifier is expected; that is the
next step if it bites again.
