Bug classes (one variant per class):

| Id | Class |
|---|---|
| X1 | ignored error |
| X2 | nil dereference or nil map write (in Xo: the nearest analog, for example a fault on a missing key or an unwrap of an empty optional) |
| X3 | missing case in a switch or match over a closed set |
| X4 | data race on shared state (in Xo: also atomicity, for example check then act across two lock sections) |
| X5 | goroutine, task, or resource leak |
| X6 | lost update (a copy is modified and never written back) |
| N1 | off by one |
| N2 | wrong HTTP status or error kind |
| N3 | wrong time, backoff, or limit arithmetic |
| N4 | inverted or wrong condition |
