# 0603: Agent protocols: std/jsonrpc, std/sse, std/mcp, derive JsonSchema, and the oauth2, a2a and OpenAPI modules

Status: Proposed (Xo Azure 1, 2026-11-11, at the lead's request, for the owner). The owner's direction so far
(2026-11-11, relayed by the lead): MCP goes in std, because it is core to the language and std ships with fast
releases. A2A, OAuth 2 and an OpenAPI SDK generator are modules. This record proposes the pieces, their order
and how each is checked.

## Context

What exists (origin/main, 2026-11-11):

- **JSON-RPC 2.0 has no library.** Each server in compiler/cli/servers inlines its own over the shared JSON tree
  `J` (json.xo, 739 lines, Go-byte-exact):
  - mcp.xo (724 lines): newline framing on stdio, raw ids, notifications without a reply, errors -32700, -32600,
    -32601 and -32602. A batch array is rejected.
  - lsp.xo (1,397 lines plus completion and features): Content-Length framing, LSP 3.17.
- **`xo mcp`** (mcp.xo):
  - Speaks MCP `2025-06-18` only, with no version negotiation.
  - Supports initialize, ping, tools/list and tools/call. Its capabilities are tools only.
  - stdio is its only transport.
  - Its seven tools (check, query, edit, explain, fmt, test, build) run the xo CLI with `--json`.
  - Their input schemas are built by hand from `prop(...)` helpers. Tests: cmd/xo/agent_test.go
    (TestMCPSession, TestMCPTestTool).
- **Server-sent events: none in xo.** std only flushes `http.stream` bodies, and the reverse proxy flushes
  `text/event-stream` per piece. The SDK repos each carry a reader:
  - azure/streaming/sse.xo (423 lines, azcore's) has frames, a 1 MiB line limit, `last_event_id` and reconnect.
  - openai/stream.xo (378 lines, openai-go's ssestream) reads chat-completion chunks.
- **JSON Schema: none.** No generator or validator exists. `derive OpenApi` is reserved in
  compiler/check/derivables.xo but generates nothing ("after 1.0", core.md).
- **Derives are compiler built-ins** (core.md 0: no macros). A new one is an entry in derivables.xo, a marker
  interface in std/prelude.xo and its expansion in compiler/check/derive.xo. Precedents: Json (0074, 0277, 0309,
  0478, 0543, 0550, 0553, 0555), Xml 0315, Cbor 0527 and 0559, Proto 0530. 0549 (Proposed) lets derives expand
  inside std modules.
- **HTTP**: streaming responses (0035, `http.stream`, `BodyWriter.write`, `flush`, trailers), streaming request
  bodies (0310, `Router.handle_stream`), client streaming (0404), full duplex (0433), and HTTP/2 and h2c (0121,
  0434). std/websocket is 0313.
- **Protobuf and gRPC**: std/protobuf (0530 `derive Proto`), std/protoparse (`xo proto gen FILE.proto` writes
  derive Proto types), std/protodesc (descriptor values, byte-exact with protobuf-go), and std/grpc (0535 and
  0546, Accepted, being built). Nothing generates Xo types from a descriptor set yet.
- **OAuth**: decision 0531 (std/oauth2 and std/oidc, Accepted 2026-11-06, not built) designs a client with PKCE,
  the device flow and client credentials. azure/azidentity has auth code with PKCE and a loopback listener,
  plus the device code flow. gcp/auth has the JWT bearer and refresh grants.
- **OpenAPI**: azure/codegen (Xo, about 6,400 lines) reads OpenAPI 2.0 (Swagger) JSON with autorest directives
  and TypeSpec names. It is Azure-shaped throughout (x-ms-* extensions, azcore, ARM); spec/spec.xo's $ref
  resolution (133 lines) is the generic part.
- **Layout**: std is one file per module (0588), and a std module may import native code (0597). Go's std is
  the reference scope, plus what backends need (0276). No ADR sets a std-versus-module policy or a release
  cadence (maturity-gaps.md lists cadence as open).

## Decision (proposed)

Seven pieces in five stages. Each piece has a Go reference program as its oracle. The oracle runs against
local fakes or in-process peers only, never a real service, the same method as the SDK ports.

### 1. std/jsonrpc (stage 1)

JSON-RPC 2.0 messages and a connection, transport-neutral:

- `Request{id: Id?, method, params: json.Value?}`, `Response{id, result | error}`, and `Id` = `Int | Str`.
  Batches are parsed and answered as the spec says (an empty batch is -32600; a notifications-only batch gets
  no reply). Error codes -32700 to -32603, plus the server range.
- Framings: newline-delimited (MCP stdio) and Content-Length (LSP), over any `Reader` and `Writer`.
- `Conn`: one reader task, requests in flight by id, cancellation, a handler registry, and notifications both
  ways. Effects are only what the transport uses.
- The dogfood step: `xo mcp` and `xo lsp` move onto it, and compiler/cli/servers' copies go.
- Oracle: golang.org/x/tools/internal/jsonrpc2 is internal, so the oracle is the Go MCP SDK's jsonrpc package
  (modelcontextprotocol/go-sdk, which uses it) driven over pipes. It prints each wire line for a fixed case list
  (batches, notifications, errors, unknown method, parse error, id types). The LSP framing replays
  `xo lsp`'s own TestLSP* transcripts.

### 2. std/sse (stage 1)

The WHATWG `text/event-stream` format, both directions:

- A reader: fields `event`, `data` (joined with `\n`), `id` and `retry`; comments; BOM; CRLF, LF and CR line
  ends; a line limit; `last_event_id`. It is an iterator of `Event` over a `BodyReader`.
- A writer over `http.stream`'s `BodyWriter` that writes and flushes each event, with keep-alive comments.
- A client helper that reconnects with `Last-Event-ID` and the server's `retry`.
- The azure and openai readers move onto it in a later SDK release; the SDKs keep their typed wrappers.
- Oracle: a Go program with the event-stream cases of azcore's streaming/sse tests and openai-go's ssestream
  tests, plus the WHATWG parsing examples, printed as event lists. Go's net/http serves the writer's output to
  the reader for round trips.

### 3. `derive JsonSchema` and validation (stage 2)

- `derive JsonSchema` gives a type `json_schema() -> json.Value`, a JSON Schema 2020-12 document that follows
  the type's `derive Json` form, so the schema describes what `json.marshal` writes and `json.unmarshal`
  accepts:
  - wire names (0277), omitempty (0309), `via:` (0478) and tagged enums (0478);
  - `Int` as `integer`, `T?` as not required (or `null` in the union where Json writes null);
  - `List` as `array`, `Map[Str, T]` as `additionalProperties`;
  - doc comments as `description`.
- Schema-only attributes, such as `minimum` or `pattern`, are field annotations next to the existing Json ones.
  Which annotations go in is a stage 2 detail; the derive works without them.
- Validation: `jsonschema.validate(schema, value) -> () ! List[SchemaErr]` for the keywords MCP and A2A
  servers meet (type, properties, required, additionalProperties, items, enum, const, minimum, maximum, minLength,
  maxLength, pattern, anyOf, oneOf, allOf, $ref within the document, $defs). It lives in std as a small module
  (std/jsonschema) because std/mcp needs it.
- `derive OpenApi` stays reserved. When it comes, it is JsonSchema plus OpenAPI's dialect.
- Oracle: google/jsonschema-go. Its `For[T]` builds a schema from a Go type, and its validator checks values.
  For each Xo test type, a Go twin with the same JSON tags prints the schema and the validation errors for a
  fixed value list. The Xo output is compared after key sorting. A table records where the two type systems
  differ (a sum type with no Go twin).

### 4. std/mcp (stage 3)

The Model Context Protocol, server and client, over std/jsonrpc:

- **Versions**: every dated protocol version the Go SDK supports at the time of the port, negotiated in
  `initialize` as the spec says. The server answers the client's version when it supports it, else its latest.
  Each version's differences are a table in the module, not branches spread through it.
- **Server**:
  - tools, where a typed handler `fn(In) -> Out ! ToolErr` gets its input and output schemas from
    `derive JsonSchema` and validates the input before calling;
  - resources and resource templates, subscriptions and prompts;
  - logging, completion, progress, cancellation and pagination, plus listChanged notifications;
  - `structuredContent` and output schemas where the version has them.
- **Client**: sampling, elicitation and roots handlers; calling tools; reading resources; prompts.
- **Transports**: stdio (newline JSON-RPC on `os.stdio`), and Streamable HTTP, both server (an
  `http.Router` handler: POST for messages, a response as JSON or as an SSE stream, GET for the server's stream,
  `Mcp-Session-Id`, resumption by event id, DELETE to end a session) and client. Plus the deprecated HTTP+SSE
  transport, client side only, for old servers.
- **Auth hook**: the Streamable HTTP server takes an `Authorizer` interface. It verifies a bearer token, names
  the protected resource metadata URL for `WWW-Authenticate`, and gives the handler the principal. The client
  takes a `TokenSource`-like interface. std/mcp has no OAuth code of its own; the oauth2 module implements both
  interfaces (piece 5).
- **Effects**: a server's handlers declare their own effects through the generic `fx`, as std/http's do; stdio
  servers use `stdio`, HTTP ones `net.listen`.
- **Dogfood**: `xo mcp` moves onto std/mcp. It gains version negotiation, typed tool inputs (its seven tools'
  schemas from derive JsonSchema instead of `prop(...)`), and progress for test and build. Its tests stay the
  regression check.
- **Oracle**: modelcontextprotocol/go-sdk. A Go server and a Go client each talk to an Xo client and server
  over stdio pipes and over Streamable HTTP on loopback. Both sides print every wire message (ids and session
  ids fixed by a seeded source). The case list covers each method of each supported version, negotiation down,
  cancellation mid-call, progress, a validation error, session resumption and a dropped stream. The official
  MCP conformance suite, if one exists at the time, runs too.

### 5. github.com/xo-lang/oauth2 (stage 4, module)

- OAuth 2.1 for MCP and A2A:
  - authorization server metadata (RFC 8414) and protected resource metadata (RFC 9728) discovery;
  - dynamic client registration (RFC 7591);
  - auth code with PKCE (S256 only) and a loopback redirect;
  - client credentials, refresh, and the resource indicator (RFC 8707);
  - on the resource side, bearer validation through introspection (RFC 7662) or JWT access tokens (RFC 9068,
    with std/jwt).
- It implements std/mcp's `Authorizer` and token interfaces. It is a module, so it versions apart from std.
- **Conflict for the owner**: decision 0531 (Accepted 2026-11-06, not built) places an OAuth 2.0 client and
  OIDC in std. Options:
  - (a) 0531 moves to this module whole (one OAuth code base, and std stays protocol-neutral);
  - (b) 0531's client stays in std, and the module adds only the MCP/A2A authorization-server side;
  - (c) both in std.

  Recommendation: (a), since the owner's direction for this record puts oauth2 in a module and nothing in std
  uses 0531 yet.
- Oracle: golang.org/x/oauth2 for the client grants (as 0531 planned). The go-sdk's auth package covers the
  MCP flows: discovery, registration and the 401 to token path. Fakes only.

### 6. github.com/xo-lang/a2a (stage 5, module)

- Agent2Agent:
  - the agent card and its well-known URL;
  - tasks, messages, parts and artifacts;
  - the JSON-RPC, HTTP+JSON and gRPC bindings;
  - streaming over SSE, plus push notifications.
- Types are generated from a2a.proto through std/protodesc. This needs a new
  `xo proto gen --descriptor-set` path, since generation today works from .proto text through std/protoparse.
  ProtoJSON comes from 0546.
- Bindings sit on std/jsonrpc, std/sse and std/grpc (0535). Auth uses the oauth2 module.
- Oracle: a2a-go (a2aproject). A Go agent and a Go client against Xo ones on loopback in each binding, printing
  each wire message.

### 7. `xo gen openapi` (stage 5, module first)

- A generator from OpenAPI 3.0 and 3.1 (and 2.0) to an Xo client module: models with `derive Json` (and
  JsonSchema), operations over std/http, pagination and SSE streaming where the spec marks it.
- The survey shows azure/codegen is Azure-shaped. The proposal is to extract its spec layer ($ref resolution,
  the schema model) into github.com/xo-lang/openapi and write a new emitter there. azure/codegen then uses the
  shared layer for parsing and keeps its Azure emitter.
- It is a module at first (`go run`-like: `xo run github.com/xo-lang/openapi/cmd/gen`). It becomes an
  `xo gen openapi` subcommand once its output is stable, since a subcommand ties it to xo's release.
- Proof: regenerate the openai module from OpenAI's published OpenAPI spec and diff it with the hand-written
  github.com/xo-lang/openai. Each difference is either fixed in the generator or recorded as a deliberate
  hand-written choice. The vectors the openai module already has (from openai-go against a fake) must pass on
  the generated client.

### Stages

1. std/jsonrpc and std/sse, with `xo mcp` and `xo lsp` on jsonrpc.
2. derive JsonSchema and std/jsonschema.
3. std/mcp: server stdio, then Streamable HTTP, then the client side; then `xo mcp` on it.
4. The oauth2 module, after the owner rules on 0531.
5. The a2a module and the OpenAPI generator. They do not depend on each other.

Each stage is a release note and a std/changes entry. Stages 1 to 3 are std and ship with xo releases, which is
the reason the owner gave for MCP in std.

## Consequences

- Xo programs can be MCP servers with typed tools in a few lines, and `xo mcp` becomes the first user.
- One JSON-RPC and one SSE implementation replace three inlined copies.
- `derive JsonSchema` is a general feature, not an MCP one: API docs, config validation, and OpenAPI later.
- std grows by four modules (jsonrpc, sse, jsonschema, mcp). Each follows 0588's one-file layout and 0461's
  maturity levels, starting experimental.
- MCP dated versions keep coming. Supporting several in one module means a version table that grows with each
  spec release; old versions leave on a schedule the owner sets.

## Alternatives

- **MCP as a module too**: it versions faster than std, but the owner chose std. `xo mcp` itself needs it, and a
  module that the toolchain needs would be a bootstrap problem.
- **No derive, schemas by hand**: this is what `xo mcp` does now. It is error prone and duplicates the Json form.
- **Wrap the Go SDK through std/go bindings**: Go-backend only, so native and LLVM builds lose it.
- **Port azure/codegen whole as the OpenAPI generator**: it is Azure-specific throughout, so it is not a lift.

## Open questions for the owner

1. 0531's placement: (a), (b) or (c) above.
2. How long old MCP protocol versions stay supported (proposal: each version for 12 months after the next).
3. Schema annotations for `derive JsonSchema`: the minimal set (description from doc comments only) first, or
   min/max/pattern from stage 2.
4. `xo gen openapi` as a subcommand from the start, or a module until it is stable (proposal: module first).
