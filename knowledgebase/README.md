# Knowledgebase

Durable project knowledge for Xo. Each folder has a `README.md` index with one
line per entry.

| Folder | What goes here |
|---|---|
| [infrastructure/](infrastructure/README.md) | How the project is built, laid out, and run: toolchain versions, repo layout, build and test commands, CI, external namespaces we own or plan to own. |
| [decisions/](decisions/README.md) | Architecture and language design decisions (ADRs): context, decision, consequences, alternatives rejected. |
| [gotchas/](gotchas/README.md) | Surprises and traps: things that broke, behaved unexpectedly, or are easy to get wrong, plus how to avoid them. |
| [guides/](guides/README.md) | Short guides for people coming from another language (Option and Result for Go programmers); the website copies them. |
| [drafts/](drafts/README.md) | Unpublished drafts of posts and documents (the owner and Xo Publish decide what is published). |

[std-performance-guide.md](std-performance-guide.md): how to write fast std code, from the std-against-Go timing audit (measure with a Go twin and quiet --ab, port Go's fast paths, allocation, views, word and byte loops, per-CPU kernels, raw memory, what did not pay), each rule with its measured case; read it before porting a Go package.

[maturity-gaps.md](maturity-gaps.md): what mature languages have that Xo does not yet (stability policy, registry and checksum database, security process, newcomer docs, editor support, platforms, drivers, observability, c-shared, public process), for the owner to revisit.

[positioning.md](positioning.md): why Xo, its measured advantages over and alongside Go, honest costs, and precedents for languages on Go and self-hosting.

The language itself is specified in [../spec/core.md](../spec/core.md). The
spec says *what* Xo is; decisions say *why*.
