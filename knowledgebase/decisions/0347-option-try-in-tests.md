# 0347: `opt?` on an Option fails the test in a test block

Status: Accepted (owner, 2026-10-22).

## Context

A test body has type `() ! Error` (core 9.2), so `r?` on a Result fails
the test with the error. `opt?` on an Option there is `XO0204` (`?` on an
optional value in a function that returns a Result, core 2.5), and the
fix it offers, `.or_err(???)`, asks the writer for an error value that
only restates what is missing:

```
test "finds the user" {
  let u = store.find(7).or_err("no user 7")?
  expect(u.name) == "Ada"
}
```

In production code the rule stays right: a function that returns a
Result should say what its error is. In a test the error has one reader,
the person reading the failure, and the expression that came back empty
says it already. Tests written by agents and people alike reach for
`opt?` first, hit `XO0204`, and add an `.or_err` whose text repeats the
expression, or replace the line with a `match` whose None arm is
`expect(false)`.

## Decision

1. **Scope.** Inside a test block, `x?` on an `Option[T]` yields the
   value or fails the test. It applies only where the enclosing function
   is the test body itself, at any depth of blocks, `if`, `match`, and
   loops in it. A `?` inside a fn literal written in a test keeps that
   function's own rules, as everywhere else: its type decides (a fn
   literal whose result is open becomes Option returning, one passed
   where `() -> T` is expected is `XO0205`). Property tests (`test ...
   for x: T`) are test bodies and take the same rule. Spec core 9.2 says
   so.
2. **Helpers stay rejected.** A function in a `_test.xo` file is not a
   test block, so `?` on an Option there keeps its codes: `XO0204` in a
   function that returns a Result, `XO0205` in one that cannot fail.
   Both get a note in a `_test.xo` file: "allowed directly in test
   blocks; here use `.or_err(...)?`". The codes' meanings do not change.
3. **Property tests.** A missing value is a failed case like any other:
   it is shrunk, and the report names the shrunk input as for any
   failure.
4. **Message.** The failure uses the existing output, `--- FAIL: name
   (file:line, seed N)`, and its detail line is the existing error line
   with the text `missing value: EXPR`, EXPR the source text of the
   operand of `?`:
   ```
   --- FAIL: finds the user (main.xo:12:1, seed 1)
       error: missing value: store.find(7)
   ```
   Tools that parse test output need no new pattern: it is the line a
   Result's `?` writes, with a fixed prefix in the message.
5. **Mixed.** `x?` on an Option and `y?` on a Result in one test both
   fail the test. The Option's failure is a `Str` error, which converts
   to the body's `Error` as any `Display` error does (core 2.6); the
   checker unifies nothing between the two.

Implementation: the checker marks such a `?` (by its span) with EXPR's
text; the SSA builder (native and LLVM backends) and the Go backend
lower it as `x.or_err("missing value: EXPR")?`, so the failure takes the
Result path, error frames included. Production code compiles exactly as
before: the mark exists only inside test bodies, which only `xo test`
compiles.

## Consequences

- `opt?` reads the same in a test as in an Option returning function;
  the test still names the line and the expression that was empty.
- A test that wants a better message keeps `.or_err("...")?` or
  `expect(opt.is_some())`.
- The rule depends on where the `?` is, which a reader sees from the
  enclosing `test` block; moving the line into a helper brings `XO0204`
  back, and the new note says why.
- Website and tour text change with the next release.

## Alternatives

- **Allow `opt?` in any function of a `_test.xo` file.** Rejected: a
  helper's result type would then mean something different depending on
  the file it is in, and a helper moved to a production file would
  change meaning.
- **A different message (`expected a value`, the value's type).**
  Rejected: the expression's text is what the reader needs, and keeping
  the `error:` line keeps tools working.
- **Panic instead of failing through the Result path.** Rejected: a
  fault skips the error frame and reports as a crash, not a failed
  test, and property test shrinking treats it differently.
