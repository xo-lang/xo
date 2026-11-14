# 0632: A fault frame formats a large parameter's Debug text when it is captured, instead of keeping a copy

Status: Accepted (owner, 2026-11-14, on the lead's recommendation): the 64-byte Debug prefix is formatted at
capture for parameters of 256 bytes or more; the text a trace prints is unchanged. It changes the cost model
that spec section 11.1 describes, which the change updates.

## Context

Spec 11.1: each `?` that propagates an error appends a frame. "A frame keeps a copy of each parameter
value and produces its `Debug` text only when the trace is printed ... formatting no more of a value
than the 64 bytes it shows." The native backends build the copy as a closure (`xo.frameargs(...)`, compiler/ssa
show.xo frame_args_func) whose environment holds every parameter.

The generated GCP SDKs pass request structs by value. aiplatform/apiv1's GoogleCloudAiplatformV1AugmentPromptRequest
is about 6,700 words (53,880 bytes for the frame's environment). Every `?` in a service method builds that
environment.

- **Code size** (measured 2026-11-14, aiplatform/apiv1 test build, arm64, caches off). Before 2026-11-14 the copy
  was 6,700 word loads and stores inline per `?`, each with far-page address adds. ProjectsLocationsService.augment_prompt
  was 3.97 MB of text, and 27% of the build's function text was those adds. The farcopy and farcopy4 codegen
  fixes (far page kept in x28, the page base made once per run, large captures copied with memcpy) cut the
  build's instructions 47%, 23.06 M to 12.28 M. What remains per `?` is one memcpy call.
- **Runtime, on the error path only** (verified: compiler/ssa/call.xo's `?` lowering calls push_frame only in the
  failure branch, before the error return, so a success path builds no frame and copies nothing): one 53 KB allocation and copy per frame per propagated error, to show at
  most 64 bytes of it. A request that fails through three `?` levels copies its request three times.
  The success path is unaffected.

## Decision

A parameter whose value is 256 bytes or more (decision 0151's by-reference threshold) gets its 64-byte `Debug`
prefix formatted when the frame is captured, as `Buf` and `Atomic` already are. The frame keeps that text, not
a copy of the value. Smaller parameters keep the copy and the lazy format.

- The printed trace is the same text. Values are immutable or copied on write, so the text at capture is the
  text at print. The 64-byte truncation already bounds the formatting work: the writer stops after 64 bytes.
- The cost moves from a 53 KB copy to a bounded Debug format of the first fields. The 64-byte cap applies only
  if formatting stops early. A Debug implementation that builds its whole string first costs its full size, which is
  the case to check before this lands.
- `--error-args=off` is unchanged.

## Results (2026-11-14)

- Debug did not stop at 64 bytes: the generated Debug functions build the whole text and the frame
  cut it after (frame_args_body). So the change adds xo.debugcap[T] (Gen.DebugCap, show.xo
  debug_body with cap): a struct adds each field only while the text is 64 bytes or less, and a
  field (or an Option or enum payload, or a newtype's value) of 256 bytes or more goes through its
  own capped function. Small fields format in full, as before. push_frame takes a parameter of 32
  words or more (cost.xo type_words) through it and keeps the text (frameargs captures `text`).
- Same text: the trace of golden error_frame_large (a large struct, one nested in a small one, an
  Option of one, a newtype of one, a struct of many short Options, a small struct beside them) is
  byte-identical from main's xo and the change's, arm64 and LLVM, as are five single-frame variants.
- `--ab` (host 1, bench/perf/errframes, 3731d64fb against the change, 7 rounds, ~/xo-lead/notes/lang1/
  ffp-ab-big.txt and ffp-ab-small.txt): big (20,000 errors through three `?` frames whose parameter
  is a 40 KB struct) wall 0.334 (59.0 to 19.7 ms; 0.329 to 0.339, better, 0 of 6 rounds loaded);
  small (a 4-word parameter) within noise (0.975).

## Consequences

- The spec 11.1 sentence "A frame keeps a copy of each parameter value" gains the size rule.
- The error path of SDK calls with large requests stops allocating per frame. The frameargs closure of such a
  function captures a Str instead of the struct, which shrinks code further (no memcpy call).
- Debug's early stop at 64 bytes must hold for derived Debug of large structs (verify: the writer's limit).

## Alternatives

Each option's effect on the trace text and on code size (aiplatform test build, after farcopy4):

- **(a) Capture large parameters by reference instead of copying.** Trace text: the same while the memory lives.
  But the memory does not live: a by-reference parameter is in a caller's frame (0151). The trace is printed
  after every function it passes through has returned (main's report, a handler's log), so the frame's
  closure runs after that memory is gone. Making it valid needs a heap box at the call, a cost on the
  success path. Code size: smallest (one pointer). Rejected: unsound without the box, and the box costs every call.
- **(b) Capture a size-capped summary above a threshold.** This is the proposed decision, with the summary being the
  64-byte Debug prefix the trace would print anyway (type name and first fields), made at capture. The trace text
  is unchanged. A shorter fixed summary (type name only) would change the text and lose the field values. Code
  size: the closure captures a Str. The per-`?` memcpy call (8 instructions) becomes a call to the formatter, about
  the same. Runtime on errors: a bounded format instead of a 53 KB allocation and copy.
- **(c) Build the closure only on the error path.** Already so (Context). Nothing to gain.
- **Keep copies (status quo with farcopy4).** Trace text unchanged. Code size is already solved by the memcpy. The
  cost left is 53 KB per frame on errors only.
- **Omit large parameters** (print `(...)` for them): the trace text loses what it shows today. Rejected
  unless the eager format proves costly.
- **Pass request structs by reference in the SDK generator**: the backend already passes them by reference
  (0151), and only the frame's copy remains. No gain.

Since the copy is error-path only, this is not a success-path speed issue. It matters for services whose errors
are frequent (retries, validation failures), each paying a 53 KB copy per frame.
