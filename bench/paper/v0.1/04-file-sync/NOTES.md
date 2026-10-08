# Paper test 04: file sync, notes

Files: `main.xo` (CLI, arg parsing, capability setup, host rules), `sync.xo`
(walk, pure plan, apply, temp file copy), `sync_test.xo` (the four required tests).

## 1. Spec gaps

| Spec section | Gap | What I guessed |
|---|---|---|
| 6.3, 14 (`std/fs`) | The entire `Fs` method surface is undefined. Only `sub` and `read_only` appear. | `read_dir(path) -> List[DirEntry] ! Error` (entry has `name`, `is_dir`, `size`, `modified`), `exists -> Bool ! Error`, `read -> Bytes ! Error`, `write(path, Bytes)`, `rename`, `remove`, `remove_dir`, `create_dir_all`. All as methods on the capability, by analogy with `os.net.listen` and `os.fs.sub`. |
| 6.3 | Path syntax inside a capability: separator, how to name the root. | `/` on every OS (like Go `io/fs`), `"."` is the root. |
| 6.3 | Does `read_only()` return `Fs` or a distinct type? Does `sub` on a missing directory fail? | Same type `Fs` (writes fail at runtime), so `if dry_run { ...read_only() } else { ... }` type checks. Assumed `sub` is lazy so dry run on a missing dst works; `walk` treats a missing root as empty. |
| 6.1 / 6.2 | Is the effect label `fs.read` a separate namespace from a parameter or module named `fs`? | Assumed yes, but I named parameters `src`, `dst`, never `fs`, to avoid the question. |
| 6.2 | Pure `pub` functions: "pub functions must declare uses explicitly", but what does a pure one write? `uses` with an empty list is not in the grammar. | Sidestepped: this is a single CLI module, so nothing is `pub`. |
| 6.3 | Args, printing, exit. `Env`, `Stdio`, `Proc` have no listed methods. | `os.env.args() -> List[Str]` including program name at index 0; `out.println` / `eprintln` infallible returning `()`; `os.proc.exit(code)` has a bottom type so it can end a `match` arm whose other arm yields `Options`. |
| 6.4, 9.2 | How a `test` block gets the test `Os`. `TestDecl` has no parameters. | A predeclared `os` binding inside test blocks. |
| 9.2 | Can `?` be used in a test block, and what happens? | Allowed; an error fails the test. |
| 9.2, 14 | `expect` is used unqualified in examples but lives in `std/test`, and `test` is a keyword, so `use std/test` would bind a keyword. | Did not import anything; assumed `expect` is in scope in test blocks. |
| 2.5, 3.4 | No wildcard pattern `_` is defined for `match`, only for `let _ =`. | `ok(_)` and `Skip(_)` as wildcards. Needed because `ok(u)` with unused `u` is error XO0110. |
| 3.4 | Variant patterns `NotFound(id)`: positional or by field name? | Used binding names equal to field names (`Copy(file)`, `Delete(rel)`) so either reading works. |
| 2.6 | How a `! E` function returns an error early. | `return err(UsageErr.Unknown(flag: a))`. |
| 1.6, 2.1 | Empty Set literal. | `#{}` (empty Map `{}` appears in 5.4, empty Set never does). |
| 2.1 | Collection methods: only `push`, `reverse`, `indexed`, `get`, `put` appear. | Also used `len()`, indexing `xs[i]`, `Set.contains`, `Str.starts_with`, `Str.to_lower`. |
| 2.1 | Is `Time` `Ord`? | Yes, `f.modified > d.modified`. |
| 3.4 | `defer` captures: when are captured bindings read? | Avoided the question: the defer always removes the temp path and ignores the error, so no mutable flag is captured. |
| 2.6 | `let _ =` "is flagged by `xo vet` unless commented". What counts as the comment? | A trailing `//` comment on the same line. |
| 2.9 | `derive Display` on an enum: output format. | Whatever it is, used only in an error message. |
| 12.2 | Rename and case folding semantics per OS (macOS also normalizes Unicode). | Simple `to_lower` on Darwin and Windows. |

## 2. Habits

- Started to write `else if` on a new line after `}` (a habit from some brace
  styles). Under the Go newline rule (1.3) that terminates the `if`. Kept
  `} else if` on one line.
- Wanted `os.Args[1:]` / `args[1..]` slicing. No slice syntax in the spec, so
  iterated with `for (i, a) in args.indexed()` and `continue` on `i == 0`.
- Wanted a stack with `pop()` for the directory walk (Go `stack = stack[:len-1]`,
  Rust `Vec::pop`). Only `push` is specified, so I wrote a breadth first queue
  that only appends and advances an index. Bonus: BFS order reversed gives
  children before parents for directory removal, so no sort was needed.
- Wanted `filepath.Dir(rel)` / `Path::parent()`. No path API, so `FileInfo`
  stores `dir` and `name` at walk time.
- Wanted Go's `defer func() { if !ok { os.Remove(tmp) } }()` with a captured
  flag. Replaced with an unconditional best effort remove in a block `defer`.
- Wanted to name a parameter `fs` (Go habit, `fs fs.FS`). Renamed to
  `src`/`dst` to avoid colliding with the effect label and module name.
- Wanted `if x == nil` / `is_none()`. Wrote `src.files.get(key) == none`.
- Wanted Rust `match a { "--x" => ..., _ => ... }` on strings. Used an
  `if` / `else if` chain since string patterns and `_` are not specified.
- Almost made helpers `pub` out of Go habit of exporting. Kept everything
  private since it is one module.

## 3. Friction

- **Unused binding rule vs. match patterns.** A variant with a field I do not
  need forces a wildcard that the spec never defines:
  ```
  Skip(_) => { // binding `rel` would be unused (XO0110)
    stats.skipped += 1
  }
  ```
- **Errors cannot be ignored, even in cleanup.** Best effort temp cleanup
  needs `let _ =` plus a comment to keep `xo vet` quiet:
  ```
  defer {
    let _ = dst.remove(tmp) // ignored on purpose: best effort cleanup
  }
  ```
- **Capability attenuation and types.** Making dry run safe by construction
  (read only dst) only works if `read_only()` returns the same type as the
  full capability. If it returns a distinct `ReadFs`, `sync` would need two
  code paths or a generic over an interface:
  ```
  let dst = if opts.dry_run { os.fs.sub(opts.dst)?.read_only() } else { os.fs.sub(opts.dst)? }
  ```
- **Capability sandbox vs. creating the root.** `sub(dst)` can only be
  taken after `dst` exists, so `main` must use the root `os.fs` to
  `create_dir_all(opts.dst)` first, which is outside the attenuated sandbox.
- **Per file failures vs. `?`.** The task wants counting, not propagating.
  Every effectful action becomes a `match` on the `Result` with two block
  arms, which makes `apply` deeply nested (match inside if inside match arm).
- **Value semantics for counters.** `var stats = Stats{}` and
  `stats.copied += 1` rebinds a copy each time. Fine here, but it means
  `apply` cannot be split into per action helpers without either `var`
  parameters or returning a new `Stats` from each one.
- **Testing failure paths with fakes.** No fault injection API is specified
  for the fake fs, so the failure test relies on renaming onto a non empty
  directory, which depends on the fake reproducing POSIX semantics.
- **Platform rules vs. tests.** `match target.os` is resolved at compile
  time, so tests could not cover macOS rules on Linux. I passed a `Rules`
  value into `sync` so tests use explicit rules and only `host_rules()`
  matches on `target.os`.
- **Whole file reads.** No streaming file API, so `copy_file` reads the
  whole file into `Bytes`. Large files would be a memory problem.

## 4. Confidence

| File | Estimated remaining spec violations |
|---|---|
| `main.xo` | 1 to 3. Most likely: `os.proc.exit` not being usable as a value of type `Options` in the `err` arm; `read_only()` type mismatch in the `if` expression; positional `paths[0]` indexing. |
| `sync.xo` | 2 to 4. Most likely: `_` patterns; mixing `return Tree{...}` struct literal handling; `Skip(_)`-style patterns; `uses` on non pub functions with an effect label (`fs.read`) whose namespace relation to modules is unclear; `== none` requiring `Eq` on `FileInfo`. Many guessed `Fs` methods are API guesses, not syntax violations. |
| `sync_test.xo` | 2 to 4. Most likely: the implicit `os` binding in tests, `?` inside test blocks, `expect` without an import, and `let (src, dst) = ...` tuple destructuring (allowed by `Pattern` in the grammar but never shown). |
