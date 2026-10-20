# `when` cannot name a field or a variable

Status: Observed (2026-10-13, decision 0193 step 5); fixed 2026-10-20

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

Correction (2026-10-20): `when` is not a keyword of Xo (lexer KEYWORDS
has no `when`, and Xo has no `when` expressions). The cascade came from
the parser's habit diagnostic for Kotlin's `when (x) {` and Swift's
`switch x {` (compiler/parser habits_expr.xo foreign_primary): it fired
on any `when` or `switch` at the start of an expression with a `{` later
on the line, so `if when == 1 {` reported Kotlin syntax and then
"expected an expression, found `==`" (seen again in xo lsp, 2026-10-20).
The habit now fires only when `(`, `{`, or a name follows the word; a
variable named `when` or `switch` works (test/errorcheck/
habit_when_index_of).
