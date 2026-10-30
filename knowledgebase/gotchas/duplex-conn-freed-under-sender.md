# A full duplex client exchange freed its connection under the sender

Status: Observed (2026-10-30, Xo lang 5)

## Symptom

ThreadSanitizer, tier 302 on the linux/arm64 offload host:
`data race in x.hclient.end_keep`, the previous read in `x.hclient.send`,
both threads from `pool.run_item` (TestGoldenTasks/http_reverse_proxy).
The Mac's TSan runs passed 3 of 3; the race needs the sender to be
inside a write when the response ends, which the host's timing hit.

## Cause

`Client.open` with a streamed body (decision 0433) sends the body from a
task while the caller reads the response. At the response's end or
failure the reader called `hclient.end_keep(r, false, ...)`, which runs
`close_at`: `tlsio.free` and zeroing the connection record that the
sender's `hclient.send` and `tlsio.write` were still using. A use after
free, not a modeling gap; the sender's group was only closed afterwards.

## Fix

`hclient.end_shared(r, s, why)` for a duplex exchange: `poll.pd_close`
on the connection's descriptor (it shuts the fd and wakes a sender
waiting to write, without freeing the record) and the failure recorded;
the TLS state and record are freed by `cresp_free` at r's last
reference, which the sender's closure holds until it ends. http.xo
`end_conn` picks it when the stream has a sender (`dsrv`).

Rule: a record a spawned task holds is freed by the last reference, not
by whichever side finishes first; ending early only closes the
descriptor.
