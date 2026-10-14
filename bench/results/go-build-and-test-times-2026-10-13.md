# Go's own build and test times on the EC2 machine Xo uses (2026-10-13)

For comparison with Xo's self build and test tiers. Fresh on demand
r8g.8xlarge (32 vCPUs, Graviton4, linux/arm64, 247 GB), AL2023, Go
1.26.1 from go.dev as the bootstrap (GOROOT_BOOTSTRAP=/usr/local/go),
the source copied to /tmp/gosrc. Timings from `/usr/bin/time -v`. Logs:
the lead's EC2 queue (go-makebash-arm64, go-tests-arm64,
go-fulltest-arm64), not committed.

## Building the toolchain (make.bash: toolchain1, 2, 3, then std and cmd)

| run | wall | CPU busy | user CPU | peak RSS |
|---|---|---|---|---|
| cold (empty GOCACHE) | 46.40 s | 631% | 262.32 s | 0.90 GB |
| warm (same GOCACHE) | 44.36 s | 637% | 251.41 s | 0.83 GB |
| GOMAXPROCS=1, cold | 4:45.93 | 100% | 266.83 s | 0.87 GB |
| cold, second instance | 48.27 s | 614% | | |

## Tests (after a cold make.bash)

| run | wall | CPU busy | peak RSS | result |
|---|---|---|---|---|
| `go test -short std cmd` | 1:04.33 | 2512% | 2.96 GB | 364 ok; plugin build failed |
| `go tool dist test -k` (all.bash's test phase) | 1:52.54 | 2030% | 3.21 GB | 446 ok; plugin build failed |

`plugin` failed to build on this image in both runs (environment, not
Go). Without `-k`, dist test stops at the first failure: a first run of
run.bash ended after 43 s for that reason and is not counted.

## Against Xo on the same machine type

- Xo's self build: bootstrap stages s1 437 s, s2 419 s, s3 419 s
  (decision 0215 Verification, Linux arm64), against make.bash's 46 s
  for three toolchain stages plus std and cmd.
- Xo's full selfhost tier: 58 minutes cold on r8g.8xlarge (CLAUDE.md),
  against 1:53 for Go's full dist test.
- Go keeps about 6 cores busy building and 20 to 25 testing; Xo's self
  build is one whole-program compile (one core most of the time). Go on
  one core takes 4:46, closer to one Xo stage's 7 minutes than its
  parallel 46 s.
