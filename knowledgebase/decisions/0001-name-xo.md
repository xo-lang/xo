# 0001: Name the language Xo

Status: Accepted (2026-10-07)

## Context
Wanted a short, Go adjacent name with a free file extension and a free GitHub org.

## Decision
Name: **Xo**. Extension `.xo`. CLI `xo`. Files `xo.mod`, `xo.lock`,
`.xotrace`, `.xocrash`.

## Spelling (2026-10-07)
In prose the language is **Xo** (capitalized). Lowercase `xo` is used only
where it is a literal: the file extension `.xo`, the CLI (`xo build`), files
(`xo.mod`, `xo.lock`), and namespaces (`xo-lang`). Dated results and blind
benchmark notes keep the spelling they were written with.

## Consequences
- Alternating hands to type, visually distinct from "go", pronounced "ex-oh".
- npm `xo` is a JavaScript linter, so search and docs should say "Xo lang".

## Alternatives
- forge: long. jo: Homebrew `jo` JSON tool, GitHub `jo-lang` taken.
- qo: looks like "go" in many fonts, pushes models toward Go syntax; Homebrew `qo` taken.
- zo: `.zo` is Racket compiled output; Homebrew cask `zo` taken.
- ko: ko.build is a well known Go container tool.
- wz: `.wz` used by game archives, both GitHub orgs taken, unclear pronunciation.
- evo, oxa, nyo: viable runners up.
