# Contributing to Xo

Xo is built mostly by AI agents working under one maintainer. The code is
developed in a private repository and published here as a mirror, with its
full history (decision 0249 in knowledgebase/decisions/). That shapes how
contributions work.

## Issues: yes, please

Issues are the most useful thing you can send:

- **Bugs:** the program, the command you ran (`xo version` included), what
  you expected, and what happened. A small program that reproduces it is
  worth more than a fix.
- **Spec questions and gaps:** where spec/core.md is unclear, contradicts
  itself, or disagrees with the compiler.
- **Use cases:** what you tried to build in Xo and where it got in the way.

A fix for an issue lands in the next publish and names the issue
(`Fixes xo-lang/xo#N`), which closes it.

## Code: as a proposal, not a merge

Pull requests are not merged here: this repository is overwritten by the
next publish. A pull request or patch is still welcome as a precise
proposal. When we use it, one of two things happens:

- the patch is applied to the development repository as is, with you as
  its author, and it appears here in the next publish under your name; or
- the change is made another way, and the commit credits you with a
  `Suggested-by:` or `Co-authored-by:` trailer.

Code you send is licensed under the same terms as the repository (MIT;
MIT-0 for std/), and each commit needs a `Signed-off-by:` line (the
[Developer Certificate of Origin](https://developercertificate.org/),
`git commit -s`).

Before a large change, open an issue first: design decisions are recorded
in knowledgebase/decisions/, and a change to something already decided
starts there.

## Security

Do not open a public issue for a vulnerability; see [SECURITY.md](SECURITY.md).
