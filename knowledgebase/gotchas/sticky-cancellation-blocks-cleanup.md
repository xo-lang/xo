# Sticky cancellation blocks cleanup in defer

Status: Observed (2026-10-07, paper test spec 0.2, task 04). Resolved by decision 0022.

Spec 0.2 makes cancellation sticky: every cancellation point reports it. A
`defer` that removes a temp file calls a fallible blocking `Fs` method, which
then returns `Canceled`, so cleanup never happens after a timeout or shutdown.

Fix (decision 0022): `defer` blocks run shielded from
cancellation with a deadline. Trio and Kotlin (`NonCancellable`) solve the
same problem this way.
