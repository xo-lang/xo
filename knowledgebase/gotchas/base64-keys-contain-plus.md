# Checksum log keys: base64 holds '+', the field separator

Status: Observed (2026-10-10, decision 0124)

Symptom: `sumdb.ParseSigner` and `ParseVerifier` rejected about half of
freshly generated keys; the test server then had a nil signer and
panicked on the first request.

Cause: keys are `<name>+<key id>+<base64 key>` (Go's note key shape), and
standard base64 uses '+' as a digit. `strings.Split(key, "+")` cut the
key material into extra fields whenever it contained a '+'.

Fix: split with a limit (`strings.SplitN(s, "+", 3)` for verifier keys,
5 for signer keys): the name and the hex key id never contain '+', so
everything after the last separator they need is the key. Any new
`+`-separated format with a base64 tail needs the same.
