# An rt-hosted std module cannot call a go_compiled one

Status: Observed (2026-11-11)

## Symptom

After listing a merged module in std/rt_hosted.txt and running
`go run ./scripts/stdgen`, `go vet` of std/rt fails with
`undefined: Protobuf_read_varint` (or `MISSING_PKG_x_std_protowireF_ws`
for a merged module that is not rt-hosted).

## Cause

scripts/stdgen generates an rt-hosted module's bodies into package rt
(decision 0597 part 3). A go_compiled module (std/go_compiled.txt, such as
std/protobuf) and a plain merged module are compiled with each program as
their own Go code, so package rt has none of their functions to call.

## Fix

Make a module rt-hosted only when everything it calls is rt-hosted or
hand written in std/rt; otherwise merge it plainly (not rt-hosted). Use
rt_hosted.txt for modules std/rt's own Go calls (mime, multipart, url).
When an unmerged declaration file names a module's types (protojson's
declarations name protodesc's), merge them together instead
(decision 0588 progress).
