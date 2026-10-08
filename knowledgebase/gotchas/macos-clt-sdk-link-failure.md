# clang cannot link with the default macOS SDK

Status: Observed (2026-10-08, first `xo build --backend=llvm`)

Symptom: `clang -O2 prog.ll xo_rt.c -o prog` compiled but failed to link:

```
/Library/Developer/CommandLineTools/SDKs/MacOSX27.0.sdk/usr/lib/libSystem.B.tbd:4:20: error: unknown architecture
                   arm64e.x1-macos, arm64e.x1-maccatalyst ]
ld: tapi error: malformed file
```

Cause: `/usr/bin/clang` is Apple clang 21 from Xcode 26.5, but
`xcrun --show-sdk-path` (and so clang's default sysroot) is the Command
Line Tools SDK `MacOSX27.0.sdk`, whose `.tbd` stubs name an architecture
that Xcode 26.5's linker does not know. A newer CLT was installed next to
an older Xcode.

Fix: `internal/build/llvm.go` tries sysroots in order: the one that
linked last time (remembered in `<user cache>/xo/llvm-sysroot`), clang's
default, `xcrun --show-sdk-path`, the Xcode SDK
(`/Applications/Xcode.app/.../MacOSX.sdk`), then each
`/Library/Developer/CommandLineTools/SDKs/MacOSX*.sdk`, newest first. Only
a failure that says "linker command failed" moves on to the next SDK, and
an error from a non SDK cause is reported in preference to the SDK
mismatch. `XO_SYSROOT=<sdk>` forces one. By hand:
`clang -isysroot /Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk ...`.
