# A Go test lost a child's last output: cmd.Wait closed the StdoutPipe

Status: Observed (2026-11-10, tier 76, 2026-11-20; task #576)

Symptom: cmd/xo TestDebugDAPAttach failed now and then with
"dap_test.go: reading a header: read |0: file already closed" right
after the stackTrace response, with every earlier response read.

Cause: the test took `cmd.StdoutPipe()`, started the adapter, and called
`cmd.Wait()` in a goroutine from the start (to notice an early exit).
os/exec says Wait closes the pipe once the command exits, and that
calling it before all reads have completed is incorrect. After
`disconnect` the adapter writes its response and exits: when Wait won
the race, the read end was closed with the response still unread.

Fix: the test makes its own pipe (`os.Pipe`, `cmd.Stdout = pw`, the
write end closed after Start), whose read end Wait does not close
(cmd/xo dap_test.go dapStdout).

Avoid: a test that waits for a child in a goroutine while it still reads
the child's output uses its own os.Pipe, not StdoutPipe. The same shape
exists where a test reads a server's last message after asking it to
exit.
