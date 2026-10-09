package diag

import (
	"strings"
)

// Explanation is the long form text of a diagnostic code for `xo explain`
// (core 10.1).
type Explanation struct {
	Title   string // short title
	Detail  string // what the rule is, why Xo has it, the spec section
	Example string // a small Xo program fragment that triggers the code
	Fix     string // the fragment fixed, or for reports, how to act on it
}

// Explanations maps every code to its long form explanation. Kept in sync
// with the constants in codes.go by TestExplanationsComplete.
var Explanations = map[Code]Explanation{
	// ---------------------------------------------------------------
	// 01xx: lexical structure and naming

	NamingConvention: {
		Title: "name breaks the enforced naming rules",
		Detail: "Xo enforces naming in the compiler instead of leaving it to linters, so every program reads the same way and a name's kind is visible at a glance. " +
			"Types, enum variants, interfaces, and type parameters are UpperCamel; functions, methods, variables, fields, parameters, modules, and effect variables are snake_case; constants are UPPER_SNAKE. " +
			"Exported names are marked with `pub`, never by capitalization. The diagnostic carries a fix that renames the declaration. See core 1.5.",
		Example: `fn DoIt(userId: Int) -> Int {
  userId
}`,
		Fix: `fn do_it(user_id: Int) -> Int {
  user_id
}`,
	},
	UnusedBinding: {
		Title: "unused local binding or import",
		Detail: "A local `let` or `var` binding, a local constant, a pattern binding, or an import that is never read is an error, because dead names usually mean a forgotten step. " +
			"Function and closure parameters, names starting with `_`, and `_` itself are not covered. A `var` that is only assigned and never read also counts as unused. " +
			"`xo fix` removes unused bindings. See core 3.1.",
		Example: `use std/json

fn f() -> Int {
  let x = 1
  2
}`,
		Fix: `fn f() -> Int {
  let x = 1
  x + 1
}`,
	},
	InvalidChar: {
		Title: "character that cannot start any token",
		Detail: "The lexer found a character that begins no Xo token, such as `@` or a stray Unicode symbol outside a string or comment. " +
			"Xo source is UTF-8 and identifiers are `[A-Za-z_][A-Za-z0-9_]*`. Remove the character or move it into a string literal. See core 1.1 and 1.5.",
		Example: `fn f(x: Int) -> Int {
  x @
}`,
		Fix: `fn f(x: Int) -> Int {
  x
}`,
	},
	UnterminatedString: {
		Title: "string, raw string, bytes, or triple quoted string not closed",
		Detail: "A string literal `\"...\"`, raw string `r\"...\"` or `r#\"...\"#`, bytes literal `b\"...\"`, or triple quoted string `\"\"\"...\"\"\"` reached the end of the line (or file, for triple quoted strings) without its closing delimiter. " +
			"Plain strings cannot span lines; use a triple quoted string for multi line text. A raw string opened with `r#\"` must close with `\"#`. See core 1.6.",
		Example: `fn f() -> Str {
  let s = "abc
  s
}`,
		Fix: `fn f() -> Str {
  let s = "abc"
  s
}`,
	},
	InvalidRune: {
		Title: "rune literal that is empty, unterminated, or holds more than one character",
		Detail: "A rune literal is exactly one character (or one escape) between single quotes: `'a'`, `'\\n'`, `'\\u{1F600}'`. " +
			"`''`, `'ab'`, and an unclosed `'a` are rejected. Text of several characters is a `Str` and uses double quotes. See core 1.6.",
		Example: `fn f() -> Str {
  let s = 'ab'
  "${s}"
}`,
		Fix: `fn f() -> Str {
  let s = "ab"
  s
}`,
	},
	InvalidEscape: {
		Title: "unknown or malformed escape sequence",
		Detail: "Escapes in `Str` and `Bytes` are `\\n \\t \\r \\0 \\\\ \\\" \\' \\$ \\xHH \\u{...}`. Anything else after a backslash is an error, as are `\\x` without two hex digits, `\\u` without braces, an invalid code point, and in `Str` a `\\xHH` above `\\x7f` (use `\\u{...}` for non ASCII text). " +
			"To write a backslash, double it, or use a raw string `r\"...\"`, which has no escapes. See core 1.6.",
		Example: `fn f() -> Str {
  "\q"
}`,
		Fix: `fn f() -> Str {
  r"\q"
}`,
	},
	MalformedNumber: {
		Title: "bad digit, misplaced `_`, missing digits, or out of range integer",
		Detail: "Integer literals are decimal, `0x` hex, `0b` binary, or `0o` octal, with `_` allowed only between digits (`1_000_000`). " +
			"The lexer rejects a digit that does not belong to the base (`0b102`), a prefix with no digits (`0x`), a doubled, leading, or trailing `_`, an integer that does not fit in 64 bits, and a duration literal whose nanosecond count overflows. See core 1.6.",
		Example: `const LIMIT = 1__000`,
		Fix:     `const LIMIT = 1_000`,
	},
	InvalidDuration: {
		Title: "unknown duration unit, fractional or non decimal duration",
		Detail: "A duration literal is a decimal integer followed by one of `ns`, `us`, `ms`, `s`, `m`, or `h`: `5s`, `250ms`. " +
			"Any other suffix on a number (`5sec`, or `0xg`, where `g` is read as a suffix) is reported here, and so are fractional or non decimal durations such as `1.5s` or `0x10s`. " +
			"Write a fractional duration in a smaller unit: `1500ms`. See core 1.6.",
		Example: `const TIMEOUT = 1.5s`,
		Fix:     `const TIMEOUT = 1500ms`,
	},
	UnterminatedInterp: {
		Title: "`${` without a matching `}` in the same string",
		Detail: "Inside a string, `${` starts an interpolated expression that must be closed by `}` before the string ends. " +
			"If a literal `${` is meant, escape the dollar sign: `\\${`. A `$` not followed by `{` is always literal. See core 1.6.",
		Example: `fn f(b: Int) -> Str {
  "a ${b"
}`,
		Fix: `fn f(b: Int) -> Str {
  "a ${b}"
}`,
	},
	EmptyInterp: {
		Title: "`${}` with no expression",
		Detail: "An interpolation must contain an expression whose type implements `Display`. `${}` (or `${ }`) has nothing to interpolate. " +
			"Put an expression inside, or escape the dollar sign (`\\${}`) to write the characters literally. See core 1.6.",
		Example: `fn f() -> Str {
  "a ${}"
}`,
		Fix: `fn f() -> Str {
  "a \${}"
}`,
	},
	MalformedImportPath: {
		Title: "`use` not followed by a valid import path",
		Detail: "`use` takes a slash separated module path (`use std/http`, `use github.com/acme/billing/invoice`) or, for Go packages, `use go \"path\"` with a non empty, relative, plain string. " +
			"Empty segments (`std//http`), a missing path, an empty or absolute Go path, and interpolation in a Go path are rejected. See core 4.1 and 4.4.",
		Example: `use std//http

fn ok() -> http.Response { http.text(200, "ok") }`,
		Fix: `use std/http

fn ok() -> http.Response { http.text(200, "ok") }`,
	},
	UnexpectedToken: {
		Title: "a token that does not fit the grammar here",
		Detail: "The general syntax error: the parser expected one thing (an expression, `,`, `)`, a type, `in` in a `for`, ...) and found another. " +
			"The message names what was expected and what was found. It is also used for a keyword written where a name is needed, bitwise compound assignments (only `+= -= *= /= %=` exist), an interface method with a body, and a statement such as `return` used as a value. " +
			"The parser recovers at the next statement, so one mistake gives one diagnostic. See core 15 for the grammar.",
		Example: `fn f() -> Int {
  let x = (1 2)
  x
}`,
		Fix: `fn f() -> Int {
  let x = (1 + 2)
  x
}`,
	},
	MissingComma: {
		Title: "list element followed by a newline instead of a comma",
		Detail: "Newlines end statements (core 1.3), so a comma separated list (arguments, list, map, or struct literal elements, parameters) that spans lines needs a comma after every element, including the last one. " +
			"Trailing commas are allowed in every list. The diagnostic carries a fix that inserts the comma. See core 1.3.",
		Example: `fn f() -> List[Int] {
  let xs = [
    1,
    2
  ]
  xs
}`,
		Fix: `fn f() -> List[Int] {
  let xs = [
    1,
    2,
  ]
  xs
}`,
	},
	ElseOnNewLine: {
		Title: "`else` must be on the line of the closing brace",
		Detail: "Because a newline after `}` ends the `if` statement (core 1.3), an `else` on the next line would start a new statement. " +
			"Write `} else {` and `} else if` on the same line as the closing brace. See core 1.3.",
		Example: `fn f(a: Bool) -> Int {
  if a {
    1
  }
  else {
    2
  }
}`,
		Fix: `fn f(a: Bool) -> Int {
  if a {
    1
  } else {
    2
  }
}`,
	},
	StructLitInCondition: {
		Title: "unparenthesized struct literal in a condition",
		Detail: "In the condition of `if`, `while`, `for`, `match`, `requires`, or `ensures`, a `{` opens the body, so a struct literal or struct pattern (`p is Point{x: 0, ..}`) there is ambiguous. " +
			"Parenthesize the condition, as in Go. The diagnostic carries that fix. See core 1.4.",
		Example: `struct Point { x: Int }

fn f(p: Point) -> Bool {
  if p == Point{x: 0} {
    true
  } else {
    false
  }
}`,
		Fix: `struct Point { x: Int }

fn f(p: Point) -> Bool {
  if (p == Point{x: 0}) {
    true
  } else {
    false
  }
}`,
	},
	UnclosedDelimiter: {
		Title: "`(`, `[`, or `{` not closed before end of file",
		Detail: "The file ended while a delimiter was still open. The diagnostic points at the opening delimiter and notes where the file ended. " +
			"The prototype reports this for an unclosed `{`; an unclosed `(` or `[` currently surfaces as XO0160 at the token that cannot continue the list. See core 15.",
		Example: `fn f() -> Int {
  1
`,
		Fix: `fn f() -> Int {
  1
}`,
	},
	InvalidPattern: {
		Title: "syntax that is not a pattern",
		Detail: "Where a pattern is expected (a `match` arm, `let`, `if let`, `for`, or after `is`), the parser found something that is not one: an operator, a missing pattern after `let`, or a non literal in a range pattern. " +
			"A pattern after `is` tests without binding, so it may not bind names: write `_`, or bind with `if let` or `match`. See core 3.5 for the pattern table.",
		Example: `fn f(r: Result[Int, Str]) -> Bool {
  r is Ok(v)
}`,
		Fix: `fn f(r: Result[Int, Str]) -> Bool {
  r is Ok(_)
}`,
	},
	WhileCondAsExpr: {
		Title: "`while cond` used as an expression; only `while { }` is one",
		Detail: "A `while` loop with a condition may end without producing a value, so it is a statement. Only the infinite loop `while { ... }` is an expression, whose value comes from `break`. " +
			"Move the loop into its own statement and compute the value separately. See core 3.5.",
		Example: `fn f(a: Bool) {
  let x = while a { }
}`,
		Fix: `fn f(a: Bool) {
  while a { }
}`,
	},
	DeclInBlock: {
		Title: "top level declaration inside a block",
		Detail: "`fn`, `struct`, `enum`, `interface`, `type`, `use`, `test`, and anything marked `pub` may appear only at the top level of a file. " +
			"Inside a function, use a closure (`let name = fn(...) { ... }`) instead of a nested function. `const` is the exception: local constants are allowed in blocks. See core 3.1, 3.3, and 15.",
		Example: `fn f() -> Int {
  fn g() -> Int { 1 }
  g()
}`,
		Fix: `fn f() -> Int {
  let g = fn() -> Int { 1 }
  g()
}`,
	},
	StmtAtTopLevel: {
		Title: "statement (let, expression, ...) at top level",
		Detail: "A file holds only `use` declarations and declarations (`fn`, `struct`, `enum`, `interface`, `type`, `const`, `test`). " +
			"Statements such as `let`, assignments, and expressions are allowed only inside a function or test body. A module level value is a `const`. See core 15 (File).",
		Example: `let max_users = 10

fn f() -> Int { max_users }`,
		Fix: `const MAX_USERS = 10

fn f() -> Int { MAX_USERS }`,
	},
	MatchArmSeparator: {
		Title:  "match or select arms separated by `,` instead of newlines",
		Detail: "`match` and `select` arms are separated by newlines, not commas, so arms read like statements. Remove the comma. See core 3.5.",
		Example: `fn f(b: Bool) -> Int {
  match b {
    true => 1,
    false => 2
  }
}`,
		Fix: `fn f(b: Bool) -> Int {
  match b {
    true => 1
    false => 2
  }
}`,
	},
	FnTypeEffectsParens: {
		Title: "several effects in a function type must be parenthesized",
		Detail: "In a function type, `uses a, b` would be ambiguous with the comma that separates parameters or fields, so several effects are written `uses (a, b)`. " +
			"In a declaration clause (`fn f() uses net, clock`) the parentheses are optional. The diagnostic carries a fix that adds them. See core 6.2 and 15.",
		Example: `fn f(g: fn() uses net, clock, n: Int) uses (net, clock) {
  g()
}`,
		Fix: `fn f(g: fn() uses (net, clock), n: Int) uses (net, clock) {
  g()
}`,
	},
	MisplacedRest: {
		Title: "`..` used more than once or not last in pattern fields",
		Detail: "In variant and struct patterns, `..` ignores the remaining fields and must come last: `Db(msg: m, ..)`. A list pattern may contain `..` once, anywhere: `[first, .., last]`. " +
			"`..` is not allowed outside list, variant, and struct patterns. See core 3.5.",
		Example: `enum E { Db(msg: Str, code: Int) }

fn f(e: E) -> Int {
  match e {
    Db(.., code: c) => c
  }
}`,
		Fix: `enum E { Db(msg: Str, code: Int) }

fn f(e: E) -> Int {
  match e {
    Db(code: c, ..) => c
  }
}`,
	},
	MissingBody: {
		Title: "function declaration without a body",
		Detail: "Every `fn` declaration outside an interface needs a `{ ... }` body. Only interface method signatures have no body. " +
			"If the body is not written yet, use a typed hole `???` (core 3.7) so the rest of the program still checks. See core 3.2 and 15.",
		Example: `fn f() -> Int
fn g() {}`,
		Fix: `fn f() -> Int { ??? }
fn g() {}`,
	},
	UseAfterDecl: {
		Title:  "`use` after the first declaration",
		Detail: "All `use` declarations of a file come first, before any `fn`, `struct`, or other declaration, so a file's imports are visible at the top. Move the `use` up. See core 4.1 and 15 (File).",
		Example: `fn f() -> Int { 1 }
use std/http

fn ok() -> http.Response { http.text(200, "ok") }`,
		Fix: `use std/http

fn f() -> Int { 1 }

fn ok() -> http.Response { http.text(200, "ok") }`,
	},
	MisplacedSelf: {
		Title: "bare `self` parameter outside an interface method signature",
		Detail: "A bare `self` parameter is only allowed first in an interface method signature (`fn get(self, id: Int) -> T?`). " +
			"A method on a concrete type names its receiver before the method name: `fn (p: Point) norm() -> Int`. See core 2.4 and 3.4.",
		Example: `struct P { x: Int }

fn norm(self, scale: Int) -> Int { scale }`,
		Fix: `struct P { x: Int }

fn (p: P) norm(scale: Int) -> Int { p.x * scale }`,
	},
	ExpectedNewline: {
		Title: "two statements or arms on one line",
		Detail: "Newlines terminate statements and separate declarations and `match` arms, and Xo has no semicolons in source (core 1.3). " +
			"Two statements, declarations, or arms on one line are rejected. Put each on its own line. " +
			"Several `match` or `select` arms on one line (`match r { Ok(v) => v Err(_) => 0 }`) get a machine fix that puts each arm on its own line (`xo fix`), and the parser keeps every arm, so no false missing arm error (XO0301) follows.",
		Example: `fn a() {}
fn b() {}

fn f() {
  a() b()
}`,
		Fix: `fn a() {}
fn b() {}

fn f() {
  a()
  b()
}`,
	},
	ForeignSyntax: {
		Title: "syntax from another language (Go, Rust, TypeScript, Python, Swift, Kotlin); names the habit and the Xo form (decision 0087)",
		Detail: "The code uses a form that is correct in Go, Rust, TypeScript, Python, Swift, or Kotlin but not in Xo. The message names the habit and the Xo form, and when the rewrite is unambiguous a machine fix applies it (`xo fix`). " +
			"The parser then continues as if the Xo form had been written, so one habit gives one diagnostic. Common ones: " +
			"`func`, `fun`, `def`, `function` are `fn`; `x := v` and Kotlin `val` are `let` (or `var`); Rust `let mut` is `var`; " +
			"`T: Hash + Eq` is one constraint, `T: Hash` (`Eq` and `Debug` are automatic); `Vec<T>` and `[]T` are `List[T]`, `map[K]V` is `Map[K, V]`; " +
			"`var x` or `&x` at a call site is just `x` (`var` belongs on the parameter); `*p`, `&T`, and `*T` have no Xo form (no pointers); " +
			"`c ? a : b` and Python's `a if c else b` are `if c { a } else { b }`; Kotlin `?:` is `??`; `x!!` and `x!` are `x.or_fault(\"why\")`; " +
			"`switch`/`case` and Kotlin `when` are `match` with `pattern => body` arms; `elif` is `else if`; `and`, `or`, `not` are `&&`, `||`, `!`; `x in xs` is `xs.contains(x)`; " +
			"`for i := 0; i < n; i++` is `for i in 0..n`; Go `for k, v := range m` is `for (k, v) in m` (the checker names the form for the type); `for cond` is `while cond`; `loop` and `for {}` are `while {}`; " +
			"`0..<n` is `0..n`, Swift `0...n` is `0..=n`, Kotlin `until` is `..`; `x++` is `x += 1`; `===` is `==`; `**` is `.pow(n)`; `xs[a:b]` is `xs[a..b]`; " +
			"`throw e` and `raise e` are `return Err(e)`; Swift `try f()` is `f()?` and `throws` is `! Error`; `try`/`catch` has no Xo form (handle the Result); " +
			"`|x| e`, `(x) => e`, and `lambda x: e` are `fn(x) { e }`; `x as T` is `T(x)`; `A::B` is `A.B`; `impl T { fn m(&self) }` is `fn (self: T) m()` at top level; `trait` is `interface`; " +
			"`): T` and Go's bare result type are `-> T`; Go `(T, error)` results are `-> T ! Error`, and `return v, nil` is `return v`; `x, err := f()` with `if err != nil { return err }` is `let x = f()?`; " +
			"backtick templates and Python f-strings are `\"...${x}...\"`; `/* */` is `//`. See decision 0087 for the policy.",
		Example: `func add(a: Int, b: Int) -> Int {
  x := a + b
  x > 0 ? x : -x
}`,
		Fix: `fn add(a: Int, b: Int) -> Int {
  let x = a + b
  if x > 0 { x } else { -x }
}`,
	},
	MisplacedPub: {
		Title:  "`pub` before something that cannot be exported",
		Detail: "`pub` exports a declaration from its module (core 1.5). Imports (`use`) and `test` blocks cannot be exported, so `pub use` and `pub test` are rejected. Remove the `pub`.",
		Example: `pub test "adds" {
  expect(1 + 1) == 2
}`,
		Fix: `test "adds" {
  expect(1 + 1) == 2
}`,
	},

	// ---------------------------------------------------------------
	// 02xx: errors and results

	UnhandledResult: {
		Title: "Result value is never used",
		Detail: "Handling errors is mandatory. A `Result` value that is not propagated with `?`, matched, passed, returned, or stored is an error, and so is a `Result` bound to a name that is never used, whatever the name (including `_x`). " +
			"This covers expression statements, `defer`, loop bodies, `if` without `else`, and the tail of a unit function or closure. The diagnostic offers `?` as a fix. " +
			"To discard on purpose, write `.ignore_err(\"reason\")`. See core 2.6.",
		Example: `enum E { A }

fn g() -> Int ! E { 1 }

fn f() {
  g()
}`,
		Fix: `enum E { A }

fn g() -> Int ! E { 1 }

fn f() ! E {
  g()?
}`,
	},
	ErrorNoConvert: {
		Title: "error type does not convert to the enclosing error type",
		Detail: "`expr?` on a `Result[T, E2]` returns the error from the enclosing function, so `E2` must convert to its error type `E`: `E2` is `E`, or `E` is `Error` and `E2` implements `Display`, or `E` has a `from E2` variant. " +
			"Add a `from` variant, derive `Display` for an `Error` target, or convert inline with `map_err`. The same rule applies inside a closure whose error type is fixed. See core 2.6.",
		Example: `enum LoadErr { Timeout }
enum ApiErr { Bad }

fn g() -> Int ! LoadErr { 1 }

fn f() -> Int ! ApiErr { g()? }`,
		Fix: `enum LoadErr { Timeout }
enum ApiErr { Bad, Load(from LoadErr) }

fn g() -> Int ! LoadErr { 1 }

fn f() -> Int ! ApiErr { g()? }`,
	},
	ResultNeedsOk: {
		Title: "success of a Result typed T must be written Ok(x)",
		Detail: "In a function declared `-> T ! E`, a returned `T` means success and a returned `Result[T, E]` is passed through as is. " +
			"When `T` is itself a `Result`, a returned `Result` would be ambiguous, so success must be written `Ok(x)` explicitly. " +
			"The same applies to closures whose results are collected, as in `xs.map(fn(x) { Ok(g()) })`. See core 2.6.",
		Example: `enum E { A }

fn f(r: Result[Int, Str]) -> Result[Int, Str] ! E { r }`,
		Fix: `enum E { A }

fn f(r: Result[Int, Str]) -> Result[Int, Str] ! E { Ok(r) }`,
	},
	OptionInResultFn: {
		Title: "`?` on an Option in a function returning a Result",
		Detail: "`opt?` returns `None` from a function that returns an optional. In a function that returns a `Result` there is no error value to return for `None`, so say which error a missing value is: `opt.or_err(E.Missing)?`. " +
			"The diagnostic offers that fix. See core 2.5.",
		Example: `enum E { Missing }

fn f(s: Str?) -> Str ! E { s? }`,
		Fix: `enum E { Missing }

fn f(s: Str?) -> Str ! E { s.or_err(E.Missing)? }`,
	},
	TryNeedsFailing: {
		Title: "`?` where the enclosing function cannot return the failure",
		Detail: "`?` on a `Result` needs a function or closure that can fail, and `?` on an optional needs one that returns an optional. " +
			"A function without `!` cannot propagate an error, and a `requires ... else` clause in such a function has nowhere to return its error either. " +
			"Declare the error type (the diagnostic offers `! E` as a fix), or handle the failure with `match`, `??`, or `ignore_err`. See core 2.5 and 2.6.",
		Example: `enum E { A }

fn g() -> Int ! E { 1 }

fn f() -> Int { g()? }`,
		Fix: `enum E { A }

fn g() -> Int ! E { 1 }

fn f() -> Int ! E { g()? }`,
	},
	DiscardNeedsReason: {
		Title: "`let _ =` discards a Result; use `.ignore_err(\"reason\")`",
		Detail: "`let _ = expr` on a `Result` silently drops its error. Xo requires a reason instead: `expr.ignore_err(\"why this failure does not matter\")` yields `T?` and records the reason, so every discarded error is searchable and reviewable (`xo vet` lists them, XO0208). " +
			"The diagnostic carries a fix that inserts `.ignore_err(\"TODO: reason\")`. Use `r.ok()` when failure simply means no value. See core 2.6.",
		Example: `fn f(fs: Fs) uses fs.write {
  let _ = fs.remove("tmp")
}`,
		Fix: `fn f(fs: Fs) uses fs.write {
  let _ = fs.remove("tmp").ignore_err("best effort temp file cleanup")
}`,
	},
	CoalesceChainTrap: {
		Title: "method chain on the right of `??` starts at a literal",
		Detail: "Reported by `xo vet`, not `xo check`. A method call binds tighter than `??` (core 15 precedence), so in `x ?? [].f()` the call `.f()` applies only to the default `[]`, not to the unwrapped `x`. " +
			"That is rarely what was meant when the chain starts at a literal. If the chain should apply to both, parenthesize the coalesce: `(x ?? []).f()`. If it really is only for the default, bind the default to a name to make that clear.",
		Example: `fn names(xs: List[Str]?) -> List[Str] {
  xs ?? [].reverse()
}`,
		Fix: `fn names(xs: List[Str]?) -> List[Str] {
  (xs ?? []).reverse()
}`,
	},
	IgnoredErrorReview: {
		Title: "`.ignore_err(\"reason\")` discards an error; listed for review",
		Detail: "Reported by `xo vet` as a note, never an error. Every `.ignore_err(\"reason\")` throws away an error, and its reason is the only record of why that is safe (core 2.6). " +
			"`xo vet` lists each one with its reason so a reviewer can confirm the reason still holds, catch placeholder reasons such as the `TODO: reason` inserted by the XO0206 fix, and decide whether the error should be handled after all.",
		Example: `fn cleanup(fs: Fs) uses fs.write {
  fs.remove("tmp").ignore_err("TODO: reason")
}`,
		Fix: `// Either give the real reason:
fn cleanup(fs: Fs) uses fs.write {
  fs.remove("tmp").ignore_err("best effort temp file cleanup")
}

// or handle the error:
fn cleanup_checked(fs: Fs) -> () ! FsErr uses fs.write {
  fs.remove("tmp")?
}`,
	},

	// ---------------------------------------------------------------
	// 03xx: match and patterns

	NonExhaustiveMatch: {
		Title: "match does not cover every case",
		Detail: "A `match` must cover every value of its subject. The message lists example patterns that are missing (such as `C`, `Some(Err(_))`, `[_]`, or `_` for integers and strings). " +
			"Guarded arms (`if ...`) do not count toward coverage. A `match` on `target.os` must name every OS, so a missing platform fails on every host. " +
			"Add the missing arms. For an enum declared in the same module, list the variants rather than adding `_` (XO0305). See core 2.3 and 3.5.",
		Example: `enum E { A, B, C }

fn f(e: E) -> Int {
  match e {
    A => 1
    B => 2
  }
}`,
		Fix: `enum E { A, B, C }

fn f(e: E) -> Int {
  match e {
    A => 1
    B | C => 2
  }
}`,
	},
	PatternTypeMismatch: {
		Title: "pattern does not fit the type of the matched value",
		Detail: "The pattern can never match because it describes a different type: a variant of another enum, `Ok` on an optional, a literal of the wrong type, a tuple of the wrong size, a list pattern on a non list. " +
			"It also covers malformed variant and struct patterns: parentheses on a variant without fields (or none on one with fields), too many or repeated fields, and fields left unmentioned without `..`. See core 3.5.",
		Example: `fn f(o: Int?) -> Bool {
  o is Ok(_)
}`,
		Fix: `fn f(o: Int?) -> Bool {
  o is Some(_)
}`,
	},
	RefutablePattern: {
		Title: "refutable pattern where an irrefutable one is required",
		Detail: "`let` and `for` bind every value they get, so their patterns must match every value of the type, such as `(a, b)` for a tuple or `P{x, y}` for a struct. " +
			"A pattern that can fail, such as `Some(v)`, needs a construct that says what happens otherwise: `if let`, `match`, `??`, or `?`. The message names a value that is not covered. See core 3.5.",
		Example: `fn f(o: Int?) -> Int {
  let Some(v) = o
  v
}`,
		Fix: `fn f(o: Int?) -> Int {
  if let Some(v) = o { v } else { 0 }
}`,
	},
	OrPatternBindings: {
		Title: "alternatives of an or pattern bind different names or types",
		Detail: "The arm body of `p1 | p2` runs for either alternative, so every alternative must bind the same names with the same types; otherwise a name would be unbound in one case. " +
			"Bind the name in every alternative, or bind it in none and use separate arms. See core 3.5.",
		Example: `fn f(r: Result[Int, Int]) -> Int {
  match r {
    Ok(v) | Err(_) => v
  }
}`,
		Fix: `fn f(r: Result[Int, Int]) -> Int {
  match r {
    Ok(v) | Err(v) => v
  }
}`,
	},
	WildcardOwnEnum: {
		Title: "`_` arm hides variants of an enum declared in this module",
		Detail: "Warning. An unguarded `_` or catch all binding arm on an enum declared in the same module hides the variants it covers, so adding a variant later compiles silently instead of failing at every `match`. " +
			"List the variants (an or pattern such as `B | C` keeps it short). Enums from other modules, the prelude (`Option`, `Result`), and non enum values may use `_`, since their owners can add variants. " +
			"A `_` arm that hides at least 8 variants and more than half of the enum is a deliberate default and is not reported (decision 0080). See core 3.5.",
		Example: `enum E { A, B, C }

fn f(e: E) -> Int {
  match e {
    A => 1
    _ => 2
  }
}`,
		Fix: `enum E { A, B, C }

fn f(e: E) -> Int {
  match e {
    A => 1
    B | C => 2
  }
}`,
	},

	// ---------------------------------------------------------------
	// 04xx: types and generics

	TypeMismatch: {
		Title: "value of the wrong type; only three implicit conversions exist",
		Detail: "The value's type is not the expected one. Xo converts implicitly in exactly three cases, each only where the target type is known: `T` to `T?`, a value to an interface it implements, and an error at `?` to the enclosing error type. " +
			"Everything else is explicit, including all numeric conversions (`Float(i)`) and newtypes (`Email(s)`). Branches of `if` and `match` must already agree; annotate the binding to give them an expected type. See core 2.10.",
		Example: `fn f(i: Int) -> Float { i }`,
		Fix:     `fn f(i: Int) -> Float { Float(i) }`,
	},
	AmbiguousTypeArgs: {
		Title: "type arguments cannot be inferred",
		Detail: "A generic function's type parameter was determined by nothing: not the arguments, the expected type, or later use. " +
			"Write the type argument explicitly (`json.decode[Int](b)`), or give the result an expected type. See core 2.7.",
		Example: `use std/json

fn f(b: Bytes) -> Int {
  let _ = json.decode(b).ok()
  1
}`,
		Fix: `use std/json

fn f(b: Bytes) -> Int {
  let _ = json.decode[Int](b).ok()
  1
}`,
	},
	BadArguments: {
		Title: "wrong arguments or fields: count, names, missing, or duplicates",
		Detail: "A call, struct literal, variant construction, or type application does not match its declaration: too many or too few arguments, an unknown or repeated parameter name, a positional argument after a named one, a missing struct field without a default, or the wrong number of type arguments. " +
			"Variants with several fields are constructed with names (`A(x: 1, y: 2)`), and `ignore_err` needs its reason. See core 2.2, 2.3, and 3.2.",
		Example: `struct P { x: Int, y: Int }

fn f() -> P { P{x: 1} }`,
		Fix: `struct P { x: Int, y: Int }

fn f() -> P { P{x: 1, y: 0} }`,
	},
	NotImplemented: {
		Title: "type does not implement a required interface or constraint",
		Detail: "The value is used in a way that needs an interface its type does not implement: interpolation needs `Display`, `==` needs `Eq`, `<` needs `Ord`, `json.encode` needs `Json`, a generic call needs its constraint (`T: Ord`), and `derive X` needs every field to implement `X`. " +
			"Add `derive` to the type, implement the interface's methods, or convert the value first. See core 2.4 and 2.9.",
		Example: `struct P { x: Int }

fn f(p: P) -> Str { "${p}" }`,
		Fix: `struct P { x: Int } derive Display

fn f(p: P) -> Str { "${p}" }`,
	},
	CannotInfer: {
		Title: "type cannot be inferred here; annotate it",
		Detail: "The checker needs a type at this point and nothing determines it yet: a closure parameter with no expected function type, a value whose method or field is used before its type is known, a `match` subject, or an iterated value. " +
			"Add a type annotation to the parameter or binding. See core 3.3.",
		Example: `fn f() -> Int {
  let g = fn(p) { p }
  g(1)
}`,
		Fix: `fn f() -> Int {
  let g = fn(p: Int) -> Int { p }
  g(1)
}`,
	},
	BadOperand: {
		Title: "operator, index, slice, or iteration not defined for the type",
		Detail: "The operation does not exist for this type: arithmetic between `Duration` and a plain integer literal, `Time + Time`, indexing a `Str` by position, slicing something other than `List`, `Bytes`, or `Str`, `for` over a value that is not a List, Set, Map, Chan, or range, `??` on a non optional, or `expect(x)` without a comparison. " +
			"Durations need units (`5s`), and a `Str` is read with `runes()` or by slicing. See core 2.1, 3.5, and 3.6.",
		Example: `fn f(d: Duration) -> Duration { d + 5 }`,
		Fix:     `fn f(d: Duration) -> Duration { d + 5s }`,
	},
	WrongKind: {
		Title: "name used as the wrong kind of thing",
		Detail: "A name resolved, but to the wrong kind of entity: a type used as a value, a module or constant called like a function, a value given type arguments, a non interface used as a constraint, a method declared on an interface, or a type alias or constant that refers to itself. " +
			"A common case is `t.await()`: `await` is written without parentheses. See core 2.4, 2.7, and 7.1.",
		Example: `fn f() -> Int {
  scope s {
    let t = s.spawn(fn() { 1 })
    t.await()
  }
}`,
		Fix: `fn f() -> Int {
  scope s {
    let t = s.spawn(fn() { 1 })
    t.await
  }
}`,
	},
	ConstNotConstant: {
		Title: "const initializer is not a compile time value",
		Detail: "A `const` initializer may use only literals, other constants, struct and enum construction, and collection literals of those. Calls and runtime values such as parameters are not compile time values. " +
			"Use a `let` binding, or compute the value with a function. See core 3.1.",
		Example: `fn g() -> Int { 1 }

const X = g()`,
		Fix: `const X = 1`,
	},
	BadConversion: {
		Title:   "invalid explicit conversion",
		Detail:  "Numeric conversions such as `Int(x)` and `Float(x)` take a number. Text is not a number: parse it with `Int.parse(s)`, which returns `Int?`, and decide what a bad input means. See core 2.1.",
		Example: `fn f(s: Str) -> Int { Int(s) }`,
		Fix:     `fn f(s: Str) -> Int { Int.parse(s) ?? 0 }`,
	},
	MapIndexRead: {
		Title: "reading a Map by index; use `get` or `get_or_fault`",
		Detail: "A missing key is the Map form of a nil dereference, so `m[k]` in an expression, or an in place change through it (`m[k].f = v`, `m[k] += 1`), is an error. " +
			"Use `m.get(k)` (an optional), `m.get_or_fault(k, \"why the key must exist\")`, or `m.update(k, init, fn(var v) { ... })`. On a `var` map the whole value assignment `m[k] = v` is allowed and means `m.put(k, v)`. See core 3.6.",
		Example: `fn f(m: Map[Str, Int]) -> Int {
  m["a"]
}`,
		Fix: `fn f(m: Map[Str, Int]) -> Int {
  m.get("a") ?? 0
}`,
	},

	// ---------------------------------------------------------------
	// 05xx: mutation and moves

	VarReceiverNeedVar: {
		Title: "var method called on a non var binding",
		Detail: "A method with a `var` receiver (including mutating collection methods such as `push` and `put`) changes the caller's binding in place. It can be called only on a `var` binding, a `var` parameter, or a field of one. " +
			"A `Mutex.with` closure parameter declared without `var` is a read only view. Declare the binding with `var` (the diagnostic offers that fix). See core 3.4 and 5.2.",
		Example: `fn f() -> List[Int] {
  let xs = [1]
  xs.push(2)
  xs
}`,
		Fix: `fn f() -> List[Int] {
  var xs = [1]
  xs.push(2)
  xs
}`,
	},
	AssignImmutable: {
		Title: "assignment to an immutable binding or to a captured variable",
		Detail: "Only `var` bindings, `var` parameters, `var` receivers, and their fields and elements can be assigned or passed to a `var` parameter. `let`, constants, pattern bindings, and `for` bindings are immutable. " +
			"Closures capture by value, so mutating a captured `var` inside a closure is also an error; share state through a `Mutex` or `Atomic` instead. See core 3.3 and 5.2.",
		Example: `fn f() -> Int {
  let x = 1
  x = 2
  x
}`,
		Fix: `fn f() -> Int {
  var x = 1
  x = 2
  x
}`,
	},
	UseOfMovedValue: {
		Title: "use of a moved Buf, StrBuilder, or BytesBuilder",
		Detail: "`Buf[T]`, `StrBuilder`, and `BytesBuilder` are mutable in place and move only. `freeze()`, assigning, passing to a function, sending on a channel, or capturing in a spawned task moves the buffer, and the old name may not be used again (until it is reassigned). " +
			"Finish all writes before moving, or start a new buffer. See core 5.3.",
		Example: `fn f() -> List[Int] {
  var b = Buf[Int].with_capacity(8)
  b.push(1)
  let xs = b.freeze()
  b.push(2)
  xs
}`,
		Fix: `fn f() -> List[Int] {
  var b = Buf[Int].with_capacity(8)
  b.push(1)
  b.push(2)
  b.freeze()
}`,
	},
	LostUpdate: {
		Title: "mutation of a copied or stored collection element is never written back",
		Detail: "Xo collections hold values, not references. A local `var` initialized from a collection element (`m.get(k)`, `m[k]`, `xs[i]`, `first`, `find`, optionally with `??`, `?`, or `.or_err(...)?`) and then mutated changes only the copy; the collection keeps the old value. " +
			"The same holds for a `var` mutated after being pushed or put into a collection. Store it again (`m.put(k, b)`), or use `m.update(k, init, fn(var b) { ... })`. Reading or returning the copy does not count as storing. See core 5.2.",
		Example: `struct Bucket { tokens: Int }

fn take(m: Map[Str, Bucket], k: Str) {
  var b = m.get(k) ?? Bucket{tokens: 3}
  b.tokens -= 1
}`,
		Fix: `struct Bucket { tokens: Int }

fn take(var m: Map[Str, Bucket], k: Str) {
  var b = m.get(k) ?? Bucket{tokens: 3}
  b.tokens -= 1
  m.put(k, b)
}`,
	},
	SplitCriticalSection: {
		Title: "value read in one `Mutex.with` is written back in a later one",
		Detail: "Warning. A value read (or derived) inside one `m.with(...)` is used inside a later `m.with(...)` on the same mutex, or a branch on it decides whether the later one runs. " +
			"Another task may change the state between the two sections (check then act), so the write can overwrite a newer value. Do the read and the write in one `with`. See core 5.4.",
		Example: `struct B { n: Int }

fn bump(m: Mutex[Map[Str, B]]) {
  let b = m.with(fn(s) { s.get("k") ?? B{n: 0} })
  m.with(fn(var s) { s.put("k", b with {n: b.n + 1}) })
}`,
		Fix: `struct B { n: Int }

fn bump(m: Mutex[Map[Str, B]]) {
  m.with(fn(var s) {
    let b = s.get("k") ?? B{n: 0}
    s.put("k", b with {n: b.n + 1})
  })
}`,
	},

	// ---------------------------------------------------------------
	// 06xx: resolution

	UnresolvedName: {
		Title: "name does not resolve",
		Detail: "A variable, function, type, field, method, module, or effect name does not exist where it is used. The diagnostic suggests the nearest real names, so an API cannot be hallucinated silently. " +
			"It also covers `os`, `expect`, and `testing` outside tests, a tuple field out of range, and `result` outside an `ensures` clause. " +
			"`Float.from(n)` and other unknown associated functions of a numeric type called with one numeric argument get the fix `Float(n)`: numeric conversions are calls on the type name (std.md 2.1). See core 4.3.",
		Example: `fn f() -> Int {
  let count = 1
  coutn
}`,
		Fix: `fn f() -> Int {
  let count = 1
  count
}`,
	},
	ImportNeedsAlias: {
		Title: "import path does not yield a valid local name",
		Detail: "An import binds its last path segment as the local name, with `-` replaced by `_`. For a Go package the local name is the Go package name. " +
			"If the result is still not a valid identifier, or is a keyword, give the import a name with `as`. See core 4.1.",
		Example: `// main/main.xo, with a module directory 03-lib
use example.com/app/03-lib

fn f() -> Int { 1 }`,
		Fix: `use example.com/app/03-lib as lib

fn f() -> Int { lib.one() }`,
	},
	Duplicate: {
		Title: "name declared twice in one scope, or a predeclared name redefined",
		Detail: "A name may be declared once per scope: top level declarations, fields, variants, methods, parameters, imports, and bindings in one block. Shadowing is allowed only in a nested block. " +
			"Predeclared names (`Option`, `Result`, `true`, `dbg`, builtin types, ...) cannot be redefined at top level, and a type cannot have a field and a method with the same name. See core 1.4 and 3.1.",
		Example: `fn f() -> Int {
  let x = 1
  let x = 2
  x
}`,
		Fix: `fn f() -> Int {
  let x = 1
  let y = 2
  x + y
}`,
	},
	ImportCycle: {
		Title: "cyclic module imports",
		Detail: "Modules may not import each other in a cycle, so every module can be checked and built after its dependencies. " +
			"Move the shared declarations into a third module that both import, or merge the two modules. See core 4.1.",
		Example: `// a/a.xo
use example.com/app/b

pub fn fa() -> Int { b.fb() }

// b/b.xo
use example.com/app/a

pub fn fb() -> Int { 1 }
pub fn g() -> Int { a.fa() }`,
		Fix: `// a/a.xo
use example.com/app/b

pub fn fa() -> Int { b.fb() }

// b/b.xo (no longer imports a)
pub fn fb() -> Int { 1 }`,
	},
	NotVisible: {
		Title: "name is not `pub` in its module, or the std type is an opaque handle",
		Detail: "Only `pub` declarations, fields, and methods are visible outside their module. Mark the declaration `pub` if it is part of the module's API, or use an exported function instead. " +
			"Some std types are opaque handles with no visible fields; they cannot be built with a struct literal and are created by their constructor function. See core 1.5.",
		Example: `// lib/a.xo
fn hidden() -> Int { 1 }

// main/main.xo
use example.com/app/lib

fn f() -> Int { lib.hidden() }`,
		Fix: `// lib/a.xo
pub fn hidden() -> Int { 1 }

// main/main.xo
use example.com/app/lib

fn f() -> Int { lib.hidden() }`,
	},
	ForeignMethod: {
		Title: "method declared outside the module of its type",
		Detail: "Methods may be declared only in the module that declares the type, so a type's method set is fixed by its owner and two modules cannot add conflicting methods. " +
			"For a builtin or imported type, write a plain function instead. See core 3.4.",
		Example: `fn (s: Str) shout() -> Str { s.to_upper() }`,
		Fix:     `fn shout(s: Str) -> Str { s.to_upper() }`,
	},
	BadJump: {
		Title: "`break` or `continue` outside a loop, or an unknown label",
		Detail: "`break` and `continue` must be inside a loop of the same function or closure, and a label must name an enclosing labeled loop (`outer: for ... { break outer }`). " +
			"Inside a closure, `return` leaves the closure; a loop outside it cannot be broken from inside. See core 3.5.",
		Example: `fn f(xs: List[Int]) {
  for _x in xs {
    break outr
  }
}`,
		Fix: `fn f(xs: List[Int]) {
  outer: for _x in xs {
    break outer
  }
}`,
	},

	// ---------------------------------------------------------------
	// 07xx: effects

	UndeclaredEffect: {
		Title: "effect performed outside the declared set",
		Detail: "`pub` functions, interface methods, function types, and any function with a `uses` clause are checked against their declared effects; no clause means pure. " +
			"Performing an effect outside that set, directly or through a call chain, a closure, an interface implementation, or a Go call (effect `go`), is an error, and the diagnostic shows the chain that performs it. " +
			"Contract clauses (`requires`, `ensures`, `invariant`) must be pure. Declare the effect, or pass the work in from a caller that has it. See core 6.2.",
		Example: `pub fn now(c: Clock) -> Time { c.now() }`,
		Fix:     `pub fn now(c: Clock) -> Time uses clock { c.now() }`,
	},
	UnknownEffect: {
		Title: "unknown effect label",
		Detail: "Effect labels are `net` (`net.dial`, `net.listen`), `fs` (`fs.read`, `fs.write`), `clock`, `rand`, `env`, `proc`, `stdio`, `ffi`, and `go` for calls into Go packages, plus effect variables declared as `fx: Effects`. " +
			"Any other name in a `uses` clause or effect argument is an error, as is an effect argument that is not a label, variable, or parenthesized list of them. See core 6.1.",
		Example: `pub fn g() uses network {}`,
		Fix:     `pub fn g() uses net {}`,
	},
	UnusedEffect: {
		Title: "effect declared in `uses` is never performed",
		Detail: "Reported by `xo vet` as a warning, never by `xo check`. Declaring more effects than the body performs is allowed, for example to keep an interface or a `pub` API stable while its implementation changes (core 6.2). " +
			"Unless that is the intent, the extra effect overstates what the function can do and forces callers to declare it too. Remove it, or keep it deliberately.",
		Example: `pub fn now(c: Clock) -> Time uses clock, net {
  c.now()
}`,
		Fix: `pub fn now(c: Clock) -> Time uses clock {
  c.now()
}`,
	},

	// ---------------------------------------------------------------
	// 08xx: concurrency

	ScopeErrorConvert: {
		Title: "child or group task error does not convert to the enclosing error type",
		Detail: "If a task fails with an error that no `await` consumes, the scope cancels everything else and the error propagates out of the scope as if by `?`, so it must convert to the enclosing function's error type. " +
			"The same holds for tasks on a task group. When the group or scope handle came from outside the function, a task started on it must not fail unless its handle is bound and awaited. " +
			"Bind the handle and handle `t.await`, handle the error inside the task, or declare a compatible error type. See core 7.1.",
		Example: `enum E { Boom }

fn work() -> Int ! E { 1 }

fn f() {
  scope s {
    s.spawn(fn() { work() })
  }
}`,
		Fix: `enum E { Boom }

fn work() -> Int ! E { 1 }

fn f() -> Int {
  scope s {
    let t = s.spawn(fn() { work() })
    t.await.ok() ?? 0
  }
}`,
	},
	ScopeEscape: {
		Title: "scope handle, or a value containing a task group, escapes its scope",
		Detail: "No task outlives its scope. A scope handle, a closure that captures one, or a value whose type contains a `task.Group` from `s.group()` cannot be the scope's value, be returned, or be stored outside the scope (in an outer `var`, a collection, or a channel), because it could start tasks after the scope ended. " +
			"`os.tasks`, the program lifetime group from `main`, may be stored anywhere. See core 7.1.",
		Example: `fn f() -> Int {
  let h = scope s { s }
  1
}`,
		Fix: `fn f() -> Int {
  scope s {
    let t = s.spawn(fn() { 1 })
    t.await
  }
}`,
	},
	SpawnWithoutScope: {
		Title: "task started without a scope handle",
		Detail: "Xo has structured concurrency only: tasks are started with `s.spawn(f)` or `s.spawn_background(f)` on a scope handle inside `scope s { ... }`, or on a task group, so every task belongs to a scope that waits for it. " +
			"There is no free `spawn` or `go` statement, and `spawn` on a value that is not a scope handle or group is an error. See core 7.1.",
		Example: `fn work() {}

fn f() {
  spawn(fn() { work() })
}`,
		Fix: `fn work() {}

fn f() {
  scope s {
    s.spawn(fn() { work() })
  }
}`,
	},

	// ---------------------------------------------------------------
	// 09xx: holes and contracts

	Hole: {
		Title: "typed hole report",
		Detail: "Warning. `???` is a typed hole, valid anywhere an expression is expected. `xo check` succeeds and reports, for each hole, the expected type, the effects in scope, and in scope values or functions that fit. " +
			"`xo build` rejects programs with holes; `xo run --holes` and `xo test --holes` allow them and fault if a hole is evaluated. Holes let an agent write a type correct skeleton first and fill it in steps. See core 3.7.",
		Example: `fn retry_after(base: Duration, attempt: Int) -> Duration {
  ???
}`,
		Fix: `fn retry_after(base: Duration, attempt: Int) -> Duration {
  base * attempt
}`,
	},
	DebugLeft: {
		Title: "`dbg(...)` left in code",
		Detail: "Reported by `xo vet` as a warning. `dbg(expr)` prints the expression source and its `Debug` value and returns the value, which is useful while debugging but noise in committed code; `xo build --ci` rejects it. " +
			"Remove the call and keep the expression, or log through `std/log` if the output is meant to stay. See core 11.4.",
		Example: `fn area(w: Int, h: Int) -> Int {
  dbg(w * h)
}`,
		Fix: `fn area(w: Int, h: Int) -> Int {
  w * h
}`,
	},

	// ---------------------------------------------------------------
	// 10xx: platform

	PlatformAPIOutside: {
		Title: "std/sys/<os> used outside the matching target arm",
		Detail: "Platform specific APIs live in `std/sys/<os>` and may be referenced only inside the arm of a `match target.os` for that OS, so code for one platform cannot leak into another's build while every arm is still type checked on every host. " +
			"The prototype does not emit this code yet: no `std/sys/<os>` modules exist, and the example below names a hypothetical `std/sys/linux` function. See core 12.2.",
		Example: `use std/sys/linux

pub fn page_size() -> Int {
  linux.page_size()
}`,
		Fix: `use std/sys/linux

pub fn page_size() -> Int {
  match target.os {
    Linux => linux.page_size()
    Darwin => 16384
    Windows => 4096
    Wasi => 65536
  }
}`,
	},

	// ---------------------------------------------------------------
	// 11xx: Go interop

	GoBindFailed: {
		Title: "Go package cannot be bound",
		Detail: "`use go \"path\"` generates a binding for the Go package (into `.xo/gobind/` or `$XO_GOBIND_DIR`). This failed: the package was not found by Go's module resolution, does not build, is a `main` package, or the binding generator reported an error (the message carries Go's reason). " +
			"Check the import path and that the module is available to the Go toolchain; `xo bind <path>` runs the binding step alone. See core 4.4 and decision 0041.",
		Example: `use go "example.invalid/nope"

fn f() {}`,
		Fix: `use go "strings"

fn f(s: Str) -> Str { strings.to_upper(s) }`,
	},
	GoNameSkipped: {
		Title: "Go name exists but its signature does not map to Xo",
		Detail: "The Go package exports this function, method, or type, but its signature uses something the binding cannot express in Xo (for example complex numbers, channels, or unsafe pointers), so the binding skipped it. " +
			"Skipped names and their reasons are listed at the top of the generated binding. Use another function, or write a small Go wrapper with a mappable signature. A name that does not exist at all is XO0601 instead. See core 4.4.",
		Example: `use go "strconv"

fn f() {
  strconv.parse_complex("1", 128)
}`,
		Fix: `use go "strconv"

fn f(s: Str) -> Float ! Error {
  strconv.parse_float(s, 64)?
}`,
	},
	GoUsageReport: {
		Title: "per module report of `use go` packages and the functions that perform `go`",
		Detail: "Reported by `xo vet` as a note, once per module. Go code can reach files and the network without a capability, so every call into Go performs the effect `go`, and agents may reach for Go idioms through the boundary out of habit (decision 0041). " +
			"The note lists the Go packages the module imports and the functions that perform `go`, so the authority escape can be reviewed. See core 4.4.",
		Example: `use go "strings"

pub fn shout(s: Str) -> Str uses go {
  strings.to_upper(s)
}`,
		Fix: `// Review each listed use. Prefer Xo std where it covers the need:
pub fn shout(s: Str) -> Str {
  s.to_upper()
}`,
	},

	// ---------------------------------------------------------------
	// 12xx: native builds without a garbage collector

	NoGCUnavailable: {
		Title: "not available without a garbage collector",
		Detail: "`xo build --no-gc` (also `xo run`, `xo test`, and `xo check`) selects a native backend (arm64 on arm64 hosts, LLVM otherwise), whose memory is reference counted instead of collected. " +
			"Before any code is generated, a check reports every construct, std API, or effect the native backends do not support, at the call site, naming the feature. " +
			"The message says whether support is planned (the program will build once the native backends grow it; until then build without `--no-gc`) or impossible: code from a `use go` package needs Go's runtime and garbage collector, so it can never run in a native build. " +
			"The check is derived from what the native code generator supports, so a program it accepts builds. See core 10.1 and decision 0083.",
		Example: `use go "strings"

fn main(os: Os) uses go {
  os.stdio.println(strings.to_upper("hi"))
}`,
		Fix: `fn main(os: Os) {
  os.stdio.println("hi".to_upper())
}`,
	},

	// ---------------------------------------------------------------
	// 13xx: modules and dependencies (decision 0086)

	ModFileInvalid: {
		Title: "malformed xo.mod require, xo.lock, or xo.work line",
		Detail: "A `require` line of xo.mod is `require <module path> <version>`: the module path is its git location (its first element is a host name) and the version a tag `vX.Y.Z`. A module is required once. " +
			"xo.mod has no `replace`, `exclude`, or `retract`; local code is reached through an xo.work workspace. " +
			"xo.lock lines are `<module path> <version>[/xo.mod] h1:<hash>` and are written by `xo get` and `xo mod tidy`. xo.work lines are `xo <edition>` and `use <dir>`, each dir holding an xo.mod. See core 4.2, 4.3, and decision 0086.",
		Example: `// xo.mod
module example.com/api
require github.com/acme/billing 1.4`,
		Fix: `// xo.mod
module example.com/api
require github.com/acme/billing v1.4.0`,
	},
	ModuleFetch: {
		Title: "module version cannot be fetched",
		Detail: "A module path is its location: `github.com/acme/billing` is fetched with git from https://github.com/acme/billing.git (XO_GIT_BASE=<dir or URL> or XO_GIT_REWRITE=<prefix>=<url>,... point it elsewhere, for mirrors and tests), and version v1.4.0 is the tree of the tag `v1.4.0`, extracted with `git archive` (no code runs). " +
			"This failed: the repository cannot be reached or cloned, it has no such tag, the tag has no xo.mod at the repository root, or that xo.mod declares a different module path. The message carries git's reason. See core 4.2 and decision 0086.",
		Example: `// xo.mod
module example.com/api
require github.com/acme/billing v1.9.9   // no such tag`,
		Fix: `// pick an existing tag (xo get github.com/acme/billing takes the latest)
module example.com/api
require github.com/acme/billing v1.4.0`,
	},
	ModuleOffline: {
		Title: "module version not in the cache while offline",
		Detail: "With XO_OFFLINE=1 or `--offline`, the toolchain never touches the network: every required module version must already be in the module cache ($XO_MODCACHE, default <user cache dir>/xo/mod) or in the main module's vendor/. " +
			"This version is in neither. Fetch it once while online (`xo mod tidy`, `xo get <path>@<version>`, or any build), or commit a vendor/ directory with `xo vendor`. See core 4.2 and decision 0086.",
		Example: `XO_OFFLINE=1 xo build .   # billing v1.4.0 was never fetched`,
		Fix: `xo mod tidy               # online, once
XO_OFFLINE=1 xo build .   # from the cache from now on`,
	},
	ChecksumMismatch: {
		Title: "module content does not match xo.lock",
		Detail: "xo.lock records the content hash of every module version the build uses (and of the xo.mod of every version minimal version selection read); every build recomputes them from the cache or vendor/ and compares. " +
			"A mismatch means the code changed under the same version: a tag was moved or rewritten upstream, or a cached or vendored copy was edited. Nothing is built. " +
			"If the new content is expected (you control the dependency and retagged it), delete its lines from xo.lock and run `xo mod tidy`; for an edited vendor/, run `xo vendor`. See core 4.3.",
		Example: `# xo.lock
github.com/acme/billing v1.4.0 h1:AAAA...   # the tag now has other content`,
		Fix: `# delete the stale lines, then
xo mod tidy`,
	},
	LockMissing: {
		Title: "xo.lock has no hash for a module version",
		Detail: "Every module version the build selects must be recorded in xo.lock, so the content of a build is pinned and checked. A require line was added or changed by hand, or a dependency upgrade raised the version of a module further down the graph. " +
			"`xo mod tidy` (or `xo get`) records the hashes; commit xo.lock with xo.mod. See core 4.3 and decision 0086.",
		Example: `// xo.mod edited by hand, xo.lock not updated
require github.com/acme/billing v1.5.0`,
		Fix: `xo mod tidy   # writes the v1.5.0 hashes to xo.lock`,
	},
	ModuleNotRequired: {
		Title: "import from a module xo.mod does not require",
		Detail: "An import path whose first element is a host name (`github.com/acme/billing/invoice`) names a package of another module, and that module must be required in xo.mod so its version is selected and locked. " +
			"`xo get <module path>` adds the latest version; `xo mod tidy` adds a requirement for every such import. See core 4.1, 4.2, and decision 0086.",
		Example: `use github.com/acme/billing/invoice   // xo.mod has no require for it

fn total() -> Int { invoice.total() }`,
		Fix: `// xo get github.com/acme/billing adds to xo.mod:
require github.com/acme/billing v1.4.0`,
	},
	VendorInconsistent: {
		Title: "vendor/ does not match xo.mod",
		Detail: "When the main module has vendor/modules.txt, builds use vendor/ only: no module cache, no network. Every requirement of xo.mod must then be vendored at its version (or above, as minimal version selection chose), and vendor/ must be readable. " +
			"xo.mod changed since the last `xo vendor`; run it again. A vendored tree that was edited is XO1304 instead. See core 4.2 and decision 0086.",
		Example: `// xo.mod now requires v1.5.0, vendor/modules.txt still lists
# github.com/acme/billing v1.4.0`,
		Fix: `xo mod tidy
xo vendor`,
	},
	WorkspaceRelease: {
		Title: "release build while a workspace overrides modules",
		Detail: "An xo.work workspace builds the modules it uses from their local directories, overriding the versions xo.mod requires. That is for development only: a release build (`xo build --ci`) must build exactly what xo.mod and xo.lock pin, so it fails while a workspace overrides a required module, naming those modules. " +
			"Build with `--workspace=off` (or XO_WORK=off) to ignore xo.work, or release the overridden modules, require their new versions, and run `xo mod tidy`. See decision 0086.",
		Example: `# xo.work: use ./api, use ./billing; api requires billing
xo build --ci ./api`,
		Fix: `xo build --ci --workspace=off ./api`,
	},
	EffectCeiling: {
		Title: "call into a dependency above its effect ceiling",
		Detail: "A `uses` clause after a require's version in xo.mod (`require github.com/acme/billing v1.4.0 uses none`) is the effect ceiling of that dependency: every call from this module into it, and every use of one of its functions as a value, may perform only the effects listed (`uses none`: none; `fs` admits `fs.read`). " +
			"The effects counted are those the dependency's function declares itself, not the effects of callbacks you pass it. A `use <dir> uses ...` line of xo.work sets a ceiling for calls from the other workspace modules. " +
			"The error sits on the call, so an upgrade whose new version adds an effect fails where it matters. Review why the dependency now needs the effect; then raise the ceiling, or stay on a version within it. See core 4.2 and decision 0086.",
		Example: `// xo.mod: require github.com/acme/billing v1.5.0 uses none
fn total(items: List[Item]) -> Int { invoice.total(items) }   // v1.5.0 declares uses net`,
		Fix: `// after review, in xo.mod:
require github.com/acme/billing v1.5.0 uses net`,
	},
	BreakingRelease: {
		Title: "breaking change in a minor or patch release",
		Detail: "`xo publish` compares the exported API and effects of the module (the declarations `xo query api` lists, in every package of the module) with the previous vX.Y.Z tag. A minor or patch release may add exports but must not remove one, change its signature, or add an effect to a function (callers' effect ceilings and `uses` clauses would break). " +
			"A breaking change needs a new major version, whose module path ends in /vN from v2 on; before v1.0.0 (v0.x.y), a minor bump may break and a patch may not. Each breaking change is listed with the old and new signature. See decision 0086.",
		Example: `# v1.4.0: pub fn total(items: List[Item]) -> Int
# now:    pub fn total(items: List[Item]) -> Int uses net
xo publish --check --version v1.5.0`,
		Fix: `# keep the old function pure and add a new one, or release a major version:
xo publish --check --version v2.0.0   # module path .../v2`,
	},
	InvalidRelease: {
		Title:   "the release cannot be made",
		Detail:  "Publishing a module version is creating its git tag; `xo publish` validates first and prints the tag command, it never pushes. It needs the module at the root of a git repository, no uncommitted changes (without --check), a version tag that does not exist yet and is above the last release, a major version that matches the module path (/vN from v2 on), and an xo.mod and xo.lock that load. See decision 0086.",
		Example: `xo publish --version v1.4.0   # v1.4.0 is already a tag`,
		Fix:     `xo publish --version v1.4.1`,
	},

	// ---------------------------------------------------------------
	// std/time layouts (decision 0094)

	TimeLayout: {
		Title: "constant std/time layout is invalid",
		Detail: "A layout passed to `time.format`, `time.parse`, or `time.check_layout` is written with named, lowercase fields: `{year}`, `{month}` (also `{month:1}`, `{month:short}`, `{month:name}`), `{day}` (`{day:1}`), `{weekday:short}`, `{weekday:name}`, `{hour}`, `{hour12}` with `{ampm}`, `{minute}`, `{second}`, `{frac}` (`{frac:1}` to `{frac:9}`), `{offset}` (`{offset:compact}`), `{zone}`, and `{lbrace}`/`{rbrace}` for literal braces (std.md 13.3). " +
			"When the layout is a constant (a string literal, a `const`, or a `+` of them) the checker parses it: an unknown field, a field form that does not exist, a stray brace, or `{hour12}` without `{ampm}` is this error, with a did you mean fix for a misspelled field. " +
			"Date patterns of other languages (`{yyyy}`, `yyyy-MM-dd`, `%Y-%m-%d`, Go's `2006-01-02`) are the habit XO0177 instead, with a fix to the named fields. A layout built at run time is not checked here: `format` and `parse` fault on a bad one, and `time.check_layout` returns a `Result` for layouts that come from input. See decision 0094.",
		Example: `use std/time

fn stamp(t: Time) -> Str {
  time.format(t, time.utc(), "{year}-{month}-{dya}")
}`,
		Fix: `use std/time

fn stamp(t: Time) -> Str {
  time.format(t, time.utc(), "{year}-{month}-{day}")
}`,
	},
	ImportDenied: {
		Title: "import forbidden by a deny rule",
		Detail: "A `deny <package> -> <package>, ...` line of xo.mod is an import direction rule: a package matching the left side may not `use` a package matching any pattern on the right. A pattern is a package path, or a path ending in `/...` that matches it and every package under it. " +
			"Each direct `use` is checked, whether or not it is used, so a chain of imports is caught at its first forbidden step. The rules of the main module (and of each workspace module) apply to its own packages. " +
			"There is no machine fix: removing an import is a design change. Move the code the package needs to a package it may import (often an interface or a plain data type in the lower layer), or change the rule if the architecture changed. `xo query imports <pkg>` lists the imports of a package with the `use` line of each. See core 4.2 and decision 0095.",
		Example: `// xo.mod: deny example.com/notes/domain -> example.com/notes/store
// domain/note.xo:
use example.com/notes/store`,
		Fix: `// domain declares what it needs; store implements it
// domain/note.xo:
pub interface NoteStore {
  fn get(self, id: Int) -> Note ! NotFound
}`,
	},
}

// SeverityOf is the severity a code is reported with.
func SeverityOf(c Code) Severity {
	if c == Hole {
		return Warning
	}
	d := Descriptions[c]
	switch {
	case strings.HasPrefix(d, "warning:"):
		return Warning
	case strings.HasPrefix(d, "note:"):
		return Note
	}
	return Error
}

// NormalizeCode turns "XO0201", "xo0201", or "0201" into Code("XO0201").
func NormalizeCode(s string) Code {
	s = strings.ToUpper(strings.TrimSpace(s))
	if !strings.HasPrefix(s, "XO") {
		s = "XO" + s
	}
	return Code(s)
}

// Explain renders the explanation of code as plain text: the code, title,
// severity, detail, and indented Example and Fix blocks. ok is false when
// the code is unknown. code may be written "XO0201", "xo0201", or "0201".
func Explain(code string) (string, bool) {
	c := NormalizeCode(code)
	e, ok := Explanations[c]
	if !ok {
		return "", false
	}
	var b strings.Builder
	b.WriteString(string(c))
	b.WriteString(": ")
	b.WriteString(e.Title)
	b.WriteString("\nseverity: ")
	b.WriteString(string(SeverityOf(c)))
	b.WriteString("\n\n")
	b.WriteString(e.Detail)
	b.WriteString("\n\nExample:\n")
	indent(&b, e.Example)
	b.WriteString("\nFix:\n")
	indent(&b, e.Fix)
	return b.String(), true
}

// indent writes s with every non empty line indented by four spaces.
func indent(b *strings.Builder, s string) {
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if line != "" {
			b.WriteString("    ")
			b.WriteString(line)
		}
		b.WriteByte('\n')
	}
}
