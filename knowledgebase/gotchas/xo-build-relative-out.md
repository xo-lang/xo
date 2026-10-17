# `xo build -o <relative path>` wrote the binary into the build cache

Status: Observed (2026-10-08, building `compiler/lexer` for timing). Fixed
the same day in `cmd/xo/build.go`.

Symptom: `xo build -o .tools/xolex compiler/lexer` printed
`darwin/arm64: .tools/xolex` and exited 0, but no file appeared there. It
was under `~/Library/Caches/xo/build/<hash>/.tools/xolex`.

Cause: the Go compiler (and clang and cc for the native backends) run
with the build directory as their working directory, and the `-o` path
was passed through unchanged, so a relative path resolved against the
build directory.

Fix: `xo build` makes `-o` absolute against the caller's working
directory before building. Code that calls `build.Build` directly with
`Options.Out` must pass an absolute path.
