# Notes: 04-file-sync in Xo 0.4

## Layout

- `xo.mod`: module `filesync`, edition 2026.
- `main.xo`: argument parsing, platform rules, `copy_file`, `sync`, `main`.
- `main_test.xo`: 12 tests (the 4 required ones plus newer source, nested
  dirs, no delete without the flag, failed temp write, case insensitive plus
  `.DS_Store`, argument parsing, and two `testing.run_main` exit status tests).

## Design summary

- Authority: `src` is `os.fs.sub(src).read_only()`, `dst` is `os.fs.sub(dst)`,
  and on `--dry-run` `dst` is also `read_only()`, so a dry run cannot write
  even if the code had a bug.
- Copy: `create_temp` in the target's directory, `write`, `set_modified` (to
  the source mtime, so the next run skips it), `rename`. A block scoped
  `defer` with a `committed` flag removes the temp file on any failure, using
  `.ignore_err("reason")`.
- Platform: `host_platform()` matches on `target.os` and returns a `Platform`
  value (case sensitivity plus ignored names). `sync` takes the `Platform` as
  a parameter so tests can exercise the macOS rules on any host.
- Delete: walk dst in reverse (children before parents). A directory is
  removed only if no kept entry lies under it; this works the same in dry run
  mode without touching the filesystem.
- Summary printed from `main`; `main` returns `Err(CliErr.Failed(...))` when
  `failed > 0`, which exits 1 (std 3.7). Usage errors exit 2 via `proc.exit`.

## Spec gaps and guesses

1. **Walking the root of a sub capability.** std 3.1 says `"."` means the root
   of a sub capability and `walk` excludes `dir` itself with paths relative to
   the capability root. I assumed `src.walk(".")` returns `a.txt`, `dir`,
   `dir/b.txt` (no `./`). The spec implies it but no example shows it.
2. **Missing dst.** `walk` fails with `NotFound` on a missing dir, so I guard
   with `dst.exists(".")?`. Whether `exists(".")` on a lazy, not yet created
   sub root returns `false` (rather than `true` because the root is "always
   there") is not stated.
3. **`create_dir_all(".")`** for files at the dst root: assumed to succeed
   (and create the root if missing). Not stated.
4. **Case insensitivity source of truth.** The task says "case insensitive on
   Windows and macOS". std has both `target.os` and `Fs.case_sensitive()`. I
   followed the task literally with `target.os`. `Fs.case_sensitive()` may be
   the more correct choice (an APFS volume can be case sensitive); the spec
   gives no guidance on which to prefer.
5. **`fail_next` glob and sub capability paths.** I assumed `fail_next(Write,
   "dst/.filesync-*", ...)` matches the temp file written through
   `os.fs.sub("dst")`, since std 8.3 says paths are relative to the test's
   `os.fs` root. Also assumed `create_temp` returns a name starting with the
   prefix; std says it returns "the new file's path" but not its shape.
6. **Does `create_temp` count as a `Write` op for `fail_next`?** `FsOp` has no
   `CreateTemp`. If `create_temp` matches `Write`, then my "failed temp write"
   test fails at `create_temp`, which still leaves no temp file, so the test
   passes either way, but it may not test what its name says.
7. **Relative paths given to `run_main` and `os.fs.sub("src")` in a test.**
   Core 6.3 says relative paths resolve against the working directory; for
   `MemFs` I assumed the working directory is the MemFs root.
8. **Passing a `var` parameter at the call site.** Core 3.2 mentions `var`
   parameters (in out) but never shows the call syntax (is it `f(x)` or
   `f(var x)`?). I avoided the question by keeping all counter updates inside
   `sync` instead of splitting out helpers that mutate the `Summary`.
9. **Return type of an `if` without `else` whose body ends in a `T?`.** In the
   `defer` I wrote `let _ = dst.remove(tmp).ignore_err(...)` (the form shown in
   core 2.6) rather than a bare expression, because 3.5 says `if` without
   `else` has type `()` but does not say whether the body must also be `()`.
10. **`Never` arm joining with `()` arms** (`Err(e) => { ...; os.proc.exit(2) }`
    alongside `Ok(o) => o`; `continue` in a match arm). Core 2.1 says `Never`
    converts to every type, and 2.10 says conversions are not applied when
    joining branches. I read `Never` as exempt; the checker agreed.
11. **Exit code for usage errors.** Spec says prefer `Err` from main (exit 1).
    I used `proc.exit(2)` for usage, which is conventional for CLIs but means
    defer blocks do not run (none are pending at that point).

## Suspected checker bugs

None observed. The checker reported 0 errors on the first run. I cannot tell
from a clean result whether it actually checked things like the `Never` arm
join, `target.os` exhaustiveness, guards in `match`, or `fail_next(Rename, ...)`
with a bare variant; if a deliberately broken variant of any of these also
passes, that would be a checker gap worth a follow up experiment (for example,
remove the `Wasi` arm from `host_platform` and confirm XO0301 fires).

## Go or Rust habits caught while writing

- Go: wanted `filepath.Dir`. There is no path helper in std, so `parent_dir`
  splits on `/` and rejoins with list slicing.
- Go: wanted `defer os.Remove(tmp)` at function scope. Xo `defer` is block
  scoped and there is no "error happened" signal, so the idiom is a `var
  committed` flag set as the last statement.
- Go: reached for `_ = os.Remove(tmp)`. Xo requires `.ignore_err("reason")`.
- Rust: almost wrote `if let Some(d) = existing && d.is_dir`. Xo match guards
  (`Some(d) if d.is_dir =>`) cover it.
- Rust: almost wrote `&mut summary` helper parameters. Value semantics plus
  unclear `var` argument call syntax made inlining simpler.
- Go: almost used `strings.EqualFold` style comparison per pair; instead
  normalized keys once (`to_lower`) into a map.

## Friction

1. **No path utilities.** No `path.join`, `path.dir`, `path.base`. Every file
   tool needs them:
   ```
   fn parent_dir(path: Str) -> Str {
     let parts = path.split("/")
     if parts.len() <= 1 { "." } else { parts[..parts.len() - 1].join("/") }
   }
   ```
   Also no `Str.rfind`/`last_index`, which would make this one line.
2. **Temp file cleanup needs a manual flag.** There is no `errdefer` or
   "defer on failure". The pattern is easy to get wrong (forget to set the flag
   or set it before the rename):
   ```
   var committed = false
   defer {
     if !committed {
       let _ = dst.remove(tmp).ignore_err("best effort cleanup")
     }
   }
   dst.write(tmp, data)?
   dst.set_modified(tmp, info.modified)?
   dst.rename(tmp, to)?
   committed = true
   ```
3. **Platform behaviour vs. testability.** `match target.os` is resolved at
   compile time, so a test cannot exercise the macOS rules on Linux. I had to
   introduce a `Platform` value and thread it through `sync`, which is
   reasonable design but is extra plumbing the language pushes onto every
   platform sensitive function.
4. **Effects are not per capability.** `uses fs` on `sync` cannot express "reads
   only from src, writes only to dst"; that guarantee comes entirely from the
   attenuated capability values (`sub(...).read_only()`), not the signature.
5. **Mutating counters from helpers.** With value semantics and no documented
   call syntax for `var` arguments, splitting `sync` into `sync_dirs`,
   `sync_files`, `delete_extras` would mean returning partial summaries and
   adding them up. I kept one long function instead.
6. **Unclear `MemFs` details** (glob semantics in `fail_next`, whether
   `create_temp` is a `Write` op, temp name shape) make the failure tests a
   guess until the runtime exists.
