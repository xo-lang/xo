# Outside registrations

Last verified: 2026-11-09

Every package, pull request and list entry Xo has outside the xo-lang org
(owner, 2026-11-09). Update the row in the same change as anything that
moves it (a publish, a merge, a review, a new pin). Outside actions (a
publish, a fork, a PR, a wiki edit) are the owner's, or the lead's with
the owner's go; agents prepare them locally and say what is ready. The
contribution policy column is what each project asks of AI assisted
work; check it again before each PR or reply.

| Registration | Link | Status | Date | Follow-up owner | Waits on | Pins to bump | Contribution policy |
|---|---|---|---|---|---|---|---|
| npm @xo-lang/tree-sitter-xo 0.1.0 | https://www.npmjs.com/package/@xo-lang/tree-sitter-xo | released (tag v0.1.0, 99b2cfc) | 2026-11-09 | Xo Lang 2 | nothing | none | n/a (a package registry) |
| npm @xo-lang/tree-sitter-xo 0.1.1 | https://www.npmjs.com/package/@xo-lang/tree-sitter-xo | released (tag v0.1.1, 73a7c7e; ABI 15), latest | 2026-11-09 | Xo Lang 2 | nothing | none | n/a (a package registry) |
| crates.io tree-sitter-xo 0.1.0 | https://crates.io/crates/tree-sitter-xo | released (99b2cfc) | 2026-11-09 | Xo Lang 2 | nothing | none | n/a (a package registry) |
| crates.io tree-sitter-xo 0.1.1 | https://crates.io/crates/tree-sitter-xo | released (73a7c7e) | 2026-11-09 | Xo Lang 2 | nothing | none | n/a (a package registry) |
| vim/vim: detect `*.xo` as xo | https://github.com/vim/vim/pull/21850 | open | 2026-11-09 | owner (Vim's AI policy: the owner writes and answers) | Vim's review | none | AI allowed with an Assisted-by trailer, but the commit message and PR description are written by the human, and answers to reviewers in their own words |
| helix-editor/helix: Xo language support | https://github.com/helix-editor/helix/pull/16461 | draft; CI awaits a maintainer's approval of a first-time contributor's run | 2026-11-09 | Xo Lang 2 (fix commits on ~/xo-wt/helix-xo add-xo; the lead pushes to the fork) | maintainer CI approval, then green CI, then the owner marks it ready | languages.toml grammar rev 99b2cfc2ebb6aa6f52751462415573e86fb41a89 (v0.1.0) | none found (docs/CONTRIBUTING.md and the repo root, 2026-11-09) |
| tree-sitter wiki List of parsers: xo row | https://github.com/tree-sitter/tree-sitter/wiki/List-of-parsers (requested in discussions/6114) | listed (a maintainer added the row: ABI 15, queries yes; checked 2026-11-09) | 2026-11-09 | owner | a tree-sitter maintainer adding the row | ABI column (14 in the request; 0.1.1 is 15) | none found (CONTRIBUTING.md, 2026-11-09) |
| Zed extension (zed-industries/extensions) | (planned) | planned: prepared locally by Xo Lang 2 | | Xo Lang 2, owner opens | owner's go | grammar commit in extension.toml | AI_POLICY.md: no autonomous agents; a human who can explain the extension writes the PR body and every reply; CONTRIBUTING: test as a dev extension first |
| nvim-treesitter parser | (planned) | planned | | Xo Lang 2 | vim/vim 21850 merged (Neovim takes filetypes from Vim) | parser revision in nvim-treesitter's lockfile | none found (CONTRIBUTING.md, 2026-11-09); PR template asks for a screenshot and install checks |
| GitHub Linguist | (planned) | planned | | owner | Linguist's usage threshold (repositories using the extension across GitHub) | grammar commit in Linguist's vendor submodule | none found (CONTRIBUTING.md, 2026-11-09) |

## Release steps these touch

- A tree-sitter-xo release (0.1.x) that changes highlights, indents or the
  node types: bump the Helix grammar rev (and the queries under
  runtime/queries/xo if they changed), the Zed extension's grammar commit,
  and nvim-treesitter's revision, each as a follow-up PR; update the rows
  above.
- npm: package.json's publishConfig makes the scoped package public; npm
  placed a "0.0.0-stage" placeholder (staged publishing) next to 0.1.0,
  not dist-tagged.
- When the first editor PR merges: add a public website docs page "Editor
  support" (status per editor, setup from the tree-sitter-xo README,
  package links, `xo lsp`; no figures), and add each later merge to it
  (owner, 2026-11-09).
