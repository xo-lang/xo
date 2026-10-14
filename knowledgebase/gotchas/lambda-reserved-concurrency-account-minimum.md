# A Lambda cannot reserve concurrency on this account

Status: Observed (2026-10-14).

Symptom: `terraform apply` in infra/ci-runners failed with
`PutFunctionConcurrency ... InvalidParameterValueException: Specified
ReservedConcurrentExecutions for function decreases account's
UnreservedConcurrentExecution below its minimum value of [10]`, leaving
xo-ci-autostart created but tainted.

Cause: the account's Lambda concurrency limit is 10 (a new account's
default), and Lambda always keeps 10 unreserved, so no function can
reserve even 1.

Fix: no `reserved_concurrent_executions`. Two webhooks handled at once
can start one machine too many; it stops itself after the idle time. To
serialize the autostart later, raise the account limit (Service Quotas,
"Concurrent executions") first.
