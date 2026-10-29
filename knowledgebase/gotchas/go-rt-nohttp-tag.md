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

Keep every net/http use in http_net.go and give each function other
files call a stub in http_nonet.go (here `openNet`). Gate a std/rt change
with `go build -tags xo_nohttp ./std/rt` and
`go test -short ./internal/devgoldens ./cmd/xo -run 'Go|Replay|Holes'`.
