# A Go oracle's own panic becomes a fuzz case's expected answer

Status: Observed (2026-11-16, #152).

Symptom: a corpus fuzz seed fails with a want of "ERR runtime error:
invalid memory address or nil pointer dereference" (TestHtmlparseFuzz seed
33: `</button></html></tr>text` parsed as a fragment in an svg context with
scripting on). The Xo side gives a tree; the reference crashed.

Cause: golang.org/x/net/html's parser recovers its own panics and returns
them as `fmt.Errorf("%s", panicErr)`, so the oracle saw an ordinary error
and wrote it as the answer. A recover in the oracle never fires, and
errors.As(runtime.Error) does not match because the type is gone.

Fix: the oracle (scripts/stdcorpus/htmlparseoracle) treats an error whose
text starts "runtime error: " as a panic and drops that random case
(keepRandom; test TestKeepRandomDropsGoPanics). Other oracles over a library
that recovers panics need the same check: a reference crash is no answer to
copy.
