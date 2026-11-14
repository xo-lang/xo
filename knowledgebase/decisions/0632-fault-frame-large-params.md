# 0632: A fault frame formats a large parameter's Debug text when it is captured, instead of keeping a copy

Status: Proposed (Xo Lang 1, 2026-11-14). It changes the cost model that spec section 11.1 describes, not the
text a trace prints. The lead may take it to the owner.

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
- **Runtime, on the error path only**: one 53 KB allocation and copy per frame per propagated error, to show at
  most 64 bytes of it. A request that fails through three `?` levels copies its request three times.
  The success path is unaffected.

## Decision (proposed)

A parameter whose value is 256 bytes or more (decision 0151's by-reference threshold) gets its 64-byte `Debug`
prefix formatted when the frame is captured, as `Buf` and `Atomic` already are. The frame keeps that text, not
a copy of the value. Smaller parameters keep the copy and the lazy format.

- The printed trace is the same text. Values are immutable or copied on write, so the text at capture is the
  text at print. The 64-byte truncation already bounds the formatting work: the writer stops after 64 bytes.
- The cost moves from a 53 KB copy to a bounded Debug format of the first fields. The 64-byte cap applies only
  if formatting stops early. A Debug implementation that builds its whole string first costs its full size, which is
  the case to check before this lands.
- `--error-args=off` is unchanged.

## Consequences

- The spec 11.1 sentence "A frame keeps a copy of each parameter value" gains the size rule.
- The error path of SDK calls with large requests stops allocating per frame. The frameargs closure of such a
  function captures a Str instead of the struct, which shrinks code further (no memcpy call).
- Debug's early stop at 64 bytes must hold for derived Debug of large structs (verify: the writer's limit).

## Alternatives

- **Keep copies (status quo with farcopy4)**: the code-size problem is solved, but the runtime copy stays.
- **Reference instead of copy**: a by-reference parameter lives in the caller's frame memory, which ends
  before the trace prints. It would need boxing at the call, a cost on the success path. Rejected.
- **Omit large parameters** (print `(...)` for them): loses information the trace shows today. Rejected
  unless the eager format proves costly.
- **Pass large request structs by reference in the SDK generator** (an API change of the generated SDKs): the
  value semantics already pass them by reference (0151). Only the frame copy remains, so this does not help.
