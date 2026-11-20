# A std/rt file that calls into http_net.go breaks every Go build without std/http

Status: Observed (2026-10-29, Xo Lang 2's 48da2bb2b, found by the lead's 2-hourly -short run)

## Symptom

Every Go-backend build of a program that does not use std/http fails:
`rt/http_client_stream.go:70:30: c.do undefined (type Http_Client has
no field or method do)`. TestDevGoldensGo, TestReplayGoCallback,
TestHoles, TestDeferFaultProperty, TestGoldenRelease and others fail in
`go test -short`, while every http golden passes on the Go backend.

## Cause

std/rt/http_net.go is `//go:build !xo_nohttp`: a program that does not
load std/http is built with the tag xo_nohttp so it does not link
net/http, and std/rt/http_nonet.go stubs the functions other files call.
A new untagged file (http_client_stream.go) called a method that only
http_net.go defines. The http goldens load std/http, so they never build
with the tag.

## Fix

Keep every net/http and Http_ use in a `//go:build !xo_nohttp` file
(since task #472, 2026-11-20: http*.go, std_gen_http.go and the `*_http.go`
halves of testing, replay, attenuate, arena, headers, stubs, fxcodec) and
give each name the untagged files use a stand in in a `xo_nohttp` file
(testing_nohttp.go's empty fakeReq and memRouter, replay_nohttp.go's
rpServer and fireRequest). An untagged struct field of an http type goes
through an alias that each side defines. TestStdRTNoHTTPDeps fails when
the xo_nohttp build imports any net/http package. The same holds for
std/go_optional.txt's modules (std/crypto, std/tls, std/x509 since
2026-11-20): a hand written file using a module's types carries
`!xo_no_M` for the module that it reaches with the fewest users (the sql*.go
files carry `!xo_no_crypto`), and scripts/stdgen gives each generated
module the tags of every gated module it reaches (stdgen.Gates and
needs). TestStdRTBuildTags builds each tag with the tags of the modules
that reach it, the sets a real program gets; alone, xo_no_tls would
leave out types std/http names. Gate a std/rt change
with `go build -tags xo_nohttp ./std/rt` and
`go test -short ./internal/devgoldens ./cmd/xo -run 'Go|Replay|Holes'`.
