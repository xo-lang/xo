# Go's JSON-RPC servers treat `"id": null` as a request, and keep raw ids as written

Status: Observed (2026-10-12, decision 0178 stage 5: the Xo `xo mcp` and
`xo lsp` must answer byte for byte as internal/mcp and internal/lsp).

## Symptom

Porting the servers, three Go behaviors looked like bugs to "fix" but are
what clients see, so the Xo servers copy them:

- internal/mcp decodes `id` into a `json.RawMessage`: `"id": null`
  becomes the four bytes `null`, which is not empty, so the message is a
  request and gets `{"jsonrpc":"2.0","id":null,"result":{}}`.
  internal/lsp decodes it into a `*json.RawMessage`, where null is a nil
  pointer: there it is a notification and gets no answer.
- A raw id is echoed as written, only compacted: `21.50` stays `21.50`,
  `{"a":[1, 2]}` becomes `{"a":[1,2]}`.
- Struct fields match keys without case (`{"Method":"ping"}` works), a
  decoded `map[string]any` does not (`{"Kind":"x"}` is not the `kind`
  argument), and a later duplicate key wins in both.

## Fix

selfhost/cli/servers json.xo keeps every member's source text (`raws`)
for RawMessage, `get` matches without case for struct fields, `mget`
exactly for maps; cmd/xo servers_xo_test.go's transcripts cover each case.
