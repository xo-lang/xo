# 0281: A TLS client on an existing connection (std/tls)

Status: Proposed (2026-10-20, Xo Lang 3; stub, being filled; spec
section for owner approval)

## Context

The native Postgres driver (decision 0278) upgrades its plain
connection after SSLRequest, as SMTP STARTTLS and Redis TLS do.
