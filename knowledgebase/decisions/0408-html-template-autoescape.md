# 0408: html/template with contextual autoescaping, trusted content, and rt.process_token

Status: Proposed (stub, 2026-10-29; xo-lang-6; milestone 0.35 S6). Design:
~/xo-lead/notes/xo-lang-6/html-template-design.md (to be folded in).

## Context

std/template is Go's text/template over a json.Value; a web application
needs Go's html/template: contextual autoescaping across HTML text,
attributes, URLs, JS and CSS, trusted content types, and custom functions.

## Decision

To come: the html mode of std/template, std/html, the FuncMap under
static types, the trusted types as objects keyed by a per-process token
(rt.process_token, a new intrinsic on every backend; the threat model;
why it keeps std/template pure: compared, never rendered), the escaper's
port and its tests.

## Consequences

To come.

## Alternatives

To come.
