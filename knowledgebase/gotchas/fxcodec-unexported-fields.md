# The effect codec drops unexported fields

Status: Anticipated (2026-10-09)

Symptom (expected): a recorded result replays as a zero value: a
`go.Error` from the trace would display as an empty string and
`go_type()` as `<nil>`.

Cause: `fxEncode` (std/rt/fxcodec.go) walks exported struct fields only;
`rt.Go_Error` keeps its text and Go error in unexported fields, so it
would encode as `{}`.

Fix: the codec has a case for `Go_Error` (`{"msg": ..., "type": ...}`),
and a decoded one carries the recorded Go type (`Go_Error.typ`). Any new
runtime type that crosses a recorded boundary with unexported state needs
the same: a case in `fxEnc` and `fxDec` and a round trip test.
