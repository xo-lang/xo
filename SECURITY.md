# Security

Report a vulnerability in the Xo compiler, toolchain, or standard library
(std/http, std/tls, std/crypto and the rest) privately, through GitHub's
private vulnerability reporting: the **Security** tab of xo-lang/xo, then
**Report a vulnerability**. Please do not open a public issue.

Include what is affected, how to reproduce it, and its impact as you
understand it. You will get an answer in the report's thread.

Response times: an acknowledgement within 3 days, and a fix or a plan
within 30 days, sooner for a critical issue.

Supported versions: security fixes go to the current release only,
shipped as a point release (for example 0.52.1 for 0.52.0); older
releases get no backports. Each fix is a `Security` line in the
CHANGELOG and, once the database is live, an entry in vuln.xolang.org,
which `xo vuln` checks a program against.
