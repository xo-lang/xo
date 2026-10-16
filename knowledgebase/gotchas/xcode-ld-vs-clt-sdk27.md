# clang cannot link on the Mac: Xcode's ld against the CLT's macOS 27 SDK

Status: Observed (2026-10-16), fixed on the development Mac the same day: the owner ran `sudo xcode-select -s /Library/Developer/CommandLineTools`, and clang links by default again (no DEVELOPER_DIR needed)

## Symptom

Every link through clang fails, even a C hello (`clang t.c -o t`), and
with it internal/llvm's TestRuntime* tests, TestCryptoDiff, and any
`--ld=cc` build:

```
.../SDKs/MacOSX27.0.sdk/usr/lib/libSystem.B.tbd:4:20: error: unknown architecture
                   arm64e.x1-macos, arm64e.x1-maccatalyst ]
 in '/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk/usr/lib/libpthread.tbd'
clang: error: linker command failed with exit code 1
```

(or `ld: multiple errors: tapi error: malformed file`). cc links worked
earlier the same day.

## Cause

The active developer directory (`xcode-select -p`) was
/Applications/Xcode.app (Xcode 26.6), so clang ran Xcode's ld
(ld-1267), while the SDK it resolved was the Command Line Tools'
MacOSX.sdk, a link to MacOSX27.0.sdk. That SDK's .tbd stubs list
`arm64e.x1` targets, which the older ld cannot parse. The Command Line
Tools' own ld (ld-27037.1) reads them.

## Fix

The owner (needs sudo): `sudo xcode-select -s
/Library/Developer/CommandLineTools`, or an Xcode whose ld knows the
macOS 27 SDK. Until then, per command: `DEVELOPER_DIR=/Library/Developer/CommandLineTools`
(internal/llvm -short passes with it), or link against the older SDK
with `SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk`.
To check: `xcode-select -p`, `clang -### t.c` (the ld path), and
`ld -v` of that path.
