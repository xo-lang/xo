# A canceled resumer left parked and fired continuations unfreed, and a shared word was set with a plain store

Status: Observed (2026-10-31), fixed.

Symptom (main check 09:40): TestGoldenTasks http_pipe/http_reverse_proxy/
http_duplex/http_h2c: TSan race, a write in hserve.srv_new vs a read in
poll.poll_round (and srv_new vs srv_new); devgoldens and arm64 goldens
"1 allocations still live at exit" in nine http programs.

Cause: (1) poll.set_cont_fire stored one shared word with a plain write from
every srv_new, read by the poller. (2) When the scope canceled the resumer
task, continuation records still parked or fired (hserve PC records and
their resume closures) were not freed.

Fix: first-wins CAS and acquire load; drain() in resume_next's canceled exit.

Rule: a change to the server's connection lifecycle (hserve.xo, hconn.xo,
poll.xo pd_*, http.xo serve_conn) runs the http goldens that serve with
--race (TestGoldenTasks http_*), the leak-check devgoldens
(TestDevGoldensArm64 http_client_stream) and gen arena_escape in its gate;
my gate ran -short, the fixed point and the 40k proof, none of which runs
a server under TSan or counts live allocations.
