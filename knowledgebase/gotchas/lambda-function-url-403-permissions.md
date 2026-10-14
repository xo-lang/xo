# The GitHub webhook to the autostart function URL got 403

Status: Observed (2026-10-14), fixed.

Symptom: every `workflow_job` delivery to xo-ci-autostart's function URL
(auth NONE) failed with `Invalid HTTP Response: 403` (repository
Settings, Webhooks, or `gh api repos/josephspurrier/xo/hooks/697891712/deliveries`).
CI still started machines, through the one minute sweep, so the
failure only showed as starts about a minute slower than designed; the
"75 s from stopped to running" of the first test was the sweep.

Cause: since AWS's 2025 change a public function URL needs two
resource policy statements: `lambda:InvokeFunctionUrl` (which
aws_lambda_function_url's `FunctionURLAllowPublicAccess` provides) and
`lambda:InvokeFunction` for the public principal. Terraform adds only the
first. The AWS provider 5.x cannot limit the second to URL calls.

Fix: AWS provider 6.x (infra/ci-runners, `~> 6.0`) and an
aws_lambda_permission with `invoked_via_function_url = true`, so the
function still cannot be invoked directly from outside the account. The
handler keeps rejecting requests without GitHub's HMAC signature (401).
After any change to the URL or its permissions, check one delivery
returns 200 (redeliver: `gh api -X POST .../deliveries/ID/attempts`).
