# Go test sources the corpora read

Copies of Go 1.26.11's `src/net/url/url_test.go` and
`src/net/mail/message_test.go` (Copyright 2009 The Go Authors, BSD-style
license in LICENSE). The httpurl and mail generators take their input
strings from these copies instead of the local GOROOT, so
`stdcorpus -check` gives the same answer with any Go patch release
(go1.26.1's url_test.go lacks two mongodb cases that go1.26.11's has).
Refresh them with a Go toolchain bump: copy the two files from the new
GOROOT and regenerate httpurl and mail.

Last verified: 2026-11-15 (go1.26.11)
