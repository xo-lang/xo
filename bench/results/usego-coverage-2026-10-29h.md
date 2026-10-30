# `use go` coverage: Go channels as go.Recv[T], 2026-10-29

The survey set and method of usego-coverage-2026-10-29b.md (47 Go std packages and 40 third-party
modules, one project, `go run ./cmd/xo-gobind --dir PROJ --json PKG...`, Go 1.26.1, darwin/arm64).
Before: several results as an Xo tuple (binding format 24, usego-coverage-2026-10-29g.md). After:
a receive-only Go channel, or one with both directions, given to Xo is `go.Recv[T]` (decision
0421, binding format 25).

| set | functions + methods before | after | fields before | fields after |
|---|---|---|---|---|
| std | 1902/2102 (90.5%) | 1903/2102 (90.5%) | 446/461 | 446/461 |
| third party | 12452/12650 (98.4%) | 12463/12650 (98.5%) | 2088/2139 | 2090/2139 |

14 skip entries were resolved: samber/lo's `async` to `async6` and `slice_to_channel`, go-redis's
`PubSub.channel`, `channel_size` and `channel_with_subscriptions`, fsnotify's `Watcher.events` and
`Watcher.errors` getters, and net/http's `CloseNotifier.close_notify`. Two were added, by design:
fsnotify's `Watcher.set_events` and `set_errors` (a `go.Recv` cannot go back to Go as a channel;
before, the whole field was one skip). Still skipped: package time's channels (`time.after`,
`time.tick`, `Timer.C`: Xo's clock), send-only channels and channel parameters.
