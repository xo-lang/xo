# 0531: std/oauth2 and std/oidc, an OAuth 2.0 client and OpenID Connect verification

Status: Proposed (Xo Azure 1, 2026-11-06; the owner accepts the API)

## Context

The owner approved (2026-11-06, relayed by the Lead) an OAuth 2.0 client and OpenID Connect support in std. The
references are golang.org/x/oauth2 (with its `clientcredentials` and device flow) for the client and
github.com/coreos/go-oidc/v3 for OIDC. Today every Xo program that needs a token writes its own form POST and JSON
decode: the Azure SDK's azidentity (client secret, certificate assertion, device code, refresh) and the AWS SDK's SSO
OIDC client both do. std/jwt already verifies compact tokens (RS*, PS*, ES*, EdDSA, HS*), checks registered claims
and parses an RFC 7517 key set (`jwt.jwks`), so OIDC needs discovery, a key cache and the ID token rules on top of it,
not new cryptography.

## Decision (proposed)

1. **std/oauth2**, with x/oauth2 as its reference:
   - `Config{client_id, client_secret, endpoint: Endpoint{auth_url, token_url, device_auth_url, auth_style}, redirect_url,
     scopes}` and `Token{access_token, token_type, refresh_token, expiry, extra: Map[Str, json.Value]}` with `valid(now)`
     (Go's 10 second expiry delta) and `type()` (Go's `Bearer` normalization).
   - Authorization code with PKCE (RFC 7636): `generate_verifier(r)`, `s256_challenge(v)`, `c.auth_code_url(state,
     opts)` (`access_type`, `prompt`, extra params; S256 challenge), `c.exchange(code, verifier: ...)`.
   - Client credentials (RFC 6749 4.4) as `ClientCredentialsConfig{..., endpoint_params}` and its token source.
   - Device authorization (RFC 8628): `c.device_auth()` returns `DeviceAuthResponse{device_code, user_code,
     verification_uri, verification_uri_complete, expires_in, interval}`; `c.device_access_token(resp)` polls with
     `authorization_pending`, `slow_down` (interval plus 5 seconds) and `expired_token`, waiting on the program's
     Clock so tests use a fake clock.
   - Refresh (RFC 6749 6) and a `TokenSource` interface: `c.token_source(t)` refreshes when invalid, keeps the old
     refresh token when the response has none (as Go), and `reuse_token_source(t, src)` caches under a Mutex.
   - Auth style as Go's `AuthStyleAutoDetect` (try header Basic auth, then params, remember which worked),
     `AuthStyleInHeader` and `AuthStyleInParams`; client id and secret are form-escaped inside Basic auth (RFC 6749
     2.3.1), as Go does.
   - Errors: `RetrieveError{status, body, error_code, error_description, error_uri}` for a 4xx/5xx or an `error`
     field; responses decode as JSON or as `application/x-www-form-urlencoded` (GitHub's), as Go; `expires_in` decoded as Go decodes it.
   - std/http's `Client` has no transport hook (immutable `with_*` builders), so instead of Go's `oauth2.Transport`
     there is a wrapper: `t.apply(req) -> Request` sets `Authorization: <type> <token>`, and `src.client(c) ->
     AuthClient` wraps `Client.send` with a token from the source on every request. std/http does not change. (Not
     server-side middleware, which is a different std/http concept.)
2. **std/oidc**, with go-oidc as its reference:
   - `discover(client, issuer)` fetches `issuer + "/.well-known/openid-configuration"` and requires its `issuer` to
     equal the one given (go-oidc's check; `insecure_issuer_url_context` as an explicit option for Azure's
     multi-tenant `{tenantid}` issuer).
   - `RemoteKeySet`: fetches `jwks_uri`, caches by `kid`, refetches on an unknown `kid` (one fetch shared by
     concurrent callers, as go-oidc), on top of `jwt.jwks`.
   - `Verifier{client_id, supported_algs (default RS256), skip_client_id_check, skip_expiry_check, skip_issuer_check,
     now}` and `v.verify(raw)` returning `IdToken{issuer, audience, subject, expiry, issued_at, nonce,
     access_token_hash, claims(T)}`; `id_token.verify_access_token(at)` checks `at_hash`.
   - The ID-token rules (issuer equality, `aud` and `azp`, nonce, `at_hash`) live in oidc, not jwt. The verifier
     calls `jwt.verify[T]` with a key callback backed by the `RemoteKeySet` (the refetch on an unknown `kid` happens
     inside the callback, under a Mutex), then applies those rules. `at_hash` takes its hash from the token's
     `jwt.Alg` (left half of SHA-256 for RS256, ES256 and PS256), not a second algorithm table.
   - `provider.user_info(token_source)` for the UserInfo endpoint, JSON or a signed JWT response.
3. **Wire identity**: each request's method, path, query, headers and form body are compared byte for byte with x/oauth2
   and go-oidc against a local fake identity provider (std/http test server), and each verification result with
   go-oidc's on the same tokens and keys. The oracle is internal/stdtest/oauthref (its own Go module, like cborref);
   no real identity provider is contacted.
4. **Maturity** (decision 0461): lands at least Solid on all three axes: the Go differential above plus x/oauth2's and
   go-oidc's own tests ported where they are table-driven; the token response, discovery document, key set and JWT
   header parsers fuzzed; and a timing row against Go's packages
   for the CPU-bound parts only (token response decode, PKCE verifier and challenge, key set parse, ID token verify);
   the flows themselves are network-bound and get no timing row.

## Consequences

- Two new std modules, a `std/changes/oauth2.md` and `std/changes/oidc.md`, and rows in
  knowledgebase/infrastructure/std-coverage.md.
- azidentity in the Azure SDK and the AWS SDK's SSO OIDC client can move onto std/oauth2 once a release has it
  (each through its own repo's ADR, since they change what those packages depend on).
- std/oidc imports std/oauth2 (Token, Config) and std/jwt; a missing algorithm or header field there is a std/jwt change, not an oidc workaround.

## Alternatives

- One module (`std/oauth2` with an `oidc` part): rejected, OIDC pulls in std/jwt and key caching that a plain client
  credentials caller does not need.
- Port x/oauth2's provider endpoint table (`google.Endpoint`, `github.Endpoint`, ...): left out; those URLs change and
  belong to the caller. A program writes the two URLs or uses `oidc.discover`.
- Token caching on disk: out of scope; a `TokenSource` wrapper can do it.

## Open questions for the owner

- Whether the device flow's polling belongs in std (as Go) or stays a helper the caller loops over.
- Whether `insecure_issuer_url_context` (go-oidc's name) keeps that name or becomes an `issuer_template` option.
- The authorized client: the `AuthClient` wrapper as proposed (no std/http change), or a request hook in std/http's
  `Client` that oauth2 plugs into. And whether `AuthClient` retries once after a refresh on a 401 (Go's Transport does
  not).
