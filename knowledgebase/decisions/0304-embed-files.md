# 0304: Embedding files in the program at build time

Status: Accepted (owner approved embedding files, Go's embed for Xo,
2026-10-21; design by Xo Publish, 2026-10-21).

## Context

A server ships its HTML, templates, SQL migrations, and default config
with its binary; a tool ships its help pages. Go has `//go:embed` on a
`string`, `[]byte`, or `embed.FS` variable. Xo has no way to put a
file's contents into a program except pasting it into a literal (or a
build step that generates a .xo file), and `xo build --stamp` (decision
0165) only replaces a short string. Xo builds static binaries on five
backends, so whatever embeds must work the same on each.

## Decision

1. **Syntax: a built-in `embed(path)` as a constant's initializer.**

       const INDEX: Str = embed("static/index.html")
       const LOGO: Bytes = embed("static/logo.png")
       const MIGRATIONS: embed.Dir = embed("migrations/*.sql")
       const SITE: embed.Dir = embed("static")

   `embed` takes one string literal and is allowed only as the whole
   initializer of a top-level `const` whose type is written: `Str`,
   `Bytes`, or `embed.Dir` (from `std/embed`). Anywhere else is an error.
   A constant is the right place: the value is known at build time, it is
   immutable, and constants already lower to data on every backend.

2. **Paths.** Slash-separated, relative to the directory of the source
   file that declares the constant, and inside its module: no `..`
   segment, no absolute path, no path through a symbolic link, and no
   file of a nested module (a directory with its own xo.mod) or of
   `vendor/`. A path may be a glob (`*`, `?`, `[a-z]`, as Fs.glob, plus
   `**` for any number of directories). For `Str` and `Bytes` the path
   must name exactly one regular file. For `embed.Dir` it may name files
   and directories; a directory brings every file under it, except names
   starting with `.` or `_` (as Go's embed), unless the pattern starts
   with `all:` (`embed("all:static")`, as Go).

3. **Values.**
   - `Str`: the file's bytes, which must be valid UTF-8.
   - `Bytes`: the file's bytes.
   - `embed.Dir`: an immutable tree of the matched files, keyed by their
     path relative to the declaring file's directory. Its methods are
     pure (no effects: the data is in the binary):

         fn (d: embed.Dir) read(path: Str) -> Bytes?
         fn (d: embed.Dir) read_text(path: Str) -> Str?     // None also when not UTF-8
         fn (d: embed.Dir) paths() -> List[Str]             // sorted
         fn (d: embed.Dir) contains(path: Str) -> Bool
         fn (d: embed.Dir) sub(dir: Str) -> embed.Dir       // the tree under dir, rekeyed
         fn (d: embed.Dir) len() -> Int

     and `fn (d: embed.Dir) fs() -> Fs`, a read-only `Fs` over the tree
     (reads, stat, read_dir, walk, glob; writes are `FsErr.Denied`), for
     code written against `Fs` (a static file handler, a template loader).

4. **How it is built.** The checker resolves the pattern when it checks
   the constant (it has the module's Fs), and replaces the call with a
   literal of the constant's type: a string literal, a bytes literal, or
   `embed.Dir{files: {path: bytes, ...}}`. From there the constant is an
   ordinary one, so every backend compiles it as it compiles any constant
   of that type, and nothing in the code generators changes. Editors and
   `xo check` report a bad pattern at once.

5. **Cache keys.** The bytes of every embedded file are inputs of the
   module that embeds them: the module check cache, the front end cache
   (the constant's builder body), the backend cache, and the test-run
   cache key the file's content hash, so editing an embedded file
   rebuilds what uses it and nothing else. `xo build --watch` and
   `xo test --watch` watch embedded files as they watch sources.
   `xo mod tidy`, the module zip and the content hash already include
   every regular file of a module, so a module's embedded files travel
   with it.

6. **Errors** (XO1506 to XO1509, claimed with code-claim.sh):
   - a pattern that matches no file, or more than one for `Str`/`Bytes`;
   - a path outside the module (`..`, absolute, a symlink, a nested
     module or vendor);
   - a `Str` constant whose file is not valid UTF-8 (the note suggests
     `Bytes`);
   - `embed` anywhere but the whole initializer of a typed top-level
     constant.

7. **Limits.** One embedded file or tree may be up to 64 MiB, and a
   program's embedded total up to 256 MiB, so a mistaken `embed("**")`
   over a data directory fails clearly instead of making a giant binary;
   an error names the size.

## Consequences

- A web app ships its assets in one static binary on every backend, and
  `embed.Dir.fs()` feeds code written for `Fs`.
- The data is in the binary's read-only data like any constant;
  the Go backend emits it as a Go string or []byte literal.
- `xo vet` can later note an embedded file that no code reads.
- Spec: core 3.1 (constants) gains `embed`; std.md gains std/embed;
  core 10.6 the new codes; toolchain docs the cache and watch notes.

## Alternatives

- A directive comment (`// xo:embed path` above a `var`), as Go: a
  comment that changes meaning is hard to see and to type-check, and Xo
  has no package-level `var`.
- A build flag (`--embed NAME=path`), like `--stamp`: the program's
  meaning would depend on the command line; editors could not see it.
- A build-time code generator writing a .xo file of literals: works
  today, but leaves generated files to keep in sync and review.
- Reading files at run time from a path next to the binary: not a static
  binary any more.

## Plan

1. Claim the error codes (code-claim.sh XO15); std/embed with Dir and its
   methods (pure Xo, every backend) and the Fs adapter.
2. Checker: `embed` built-in, pattern resolution, rewriting to literals,
   the errors; the module check cache keys the embedded bytes.
3. Front end, backend, and test-run cache keys; watch.
4. Goldens on arm64, LLVM, Go, amd64 (Docker), wasm: Str, Bytes, a Dir
   with a glob and a nested directory, `all:`, `fs()` read through an
   `Fs`; errorcheck for each error; a cache test that an edit of an
   embedded file rebuilds.
5. Spec, std.md, toolchain docs, changelog.

## Notes

- 2026-10-21: `Dir.fs()` landed on top of decision 0307 (a std struct converted to `Fs` on the native backends); `open` on it is `Denied` for now (no stream over embedded bytes), so code that streams a file reads it whole instead.
