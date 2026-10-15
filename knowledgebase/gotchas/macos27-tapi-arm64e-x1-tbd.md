# TestSysCTSan's link fails on the macOS 27 SDK: "unknown architecture arm64e.x1-macos"

Status: Observed (2026-10-15, xo-fa, on clean main e304c853 and on
branches alike: not a code change).

## Symptom

`go test -run TestSysCTSan ./internal/llvm/` fails at the link:

    clang: ld: multiple errors: tapi error: malformed file
    /Library/Developer/CommandLineTools/SDKs/MacOSX27.0.sdk/usr/lib/libSystem.B.tbd:4:20: error: unknown architecture
                       arm64e.x1-macos, arm64e.x1-maccatalyst ]
     in '.../MacOSX.sdk/usr/lib/libpthread.tbd'; ... libm.tbd; ... libSystem.tbd;
     ... CoreFoundation.tbd; ... Security.tbd

## Cause

The linker's text based stub reader (tapi) is older than the SDK: the
macOS 27 SDK's .tbd files list an `arm64e.x1` slice it does not know.
Seen with the Command Line Tools' ld-27037.1 (built 2026-08-25) and
/Library/Developer/CommandLineTools/SDKs/MacOSX27.0.sdk.

## Affects

TestSysCTSan (internal/llvm: a program's IR from xotool.LLVMIR linked
with the runtime under -fsanitize=thread). The other ThreadSanitizer
tests that go through the same tsanBuild (TestRuntime*, TestGoldenTasks,
TestDeadlock) linked fine on the same day, so the trigger is narrower
than "every sanitizer link" (possibly the program's frameworks or its
IR's target): check any new failure with this message against it before
suspecting a change.

## Fix

Update the Command Line Tools (Software Update, or `xcode-select
--install` after removing the old ones) so ld and tapi know the SDK's
architectures. Until then, read a TestSysCTSan failure with this message
as the environment's, not the branch's.
