# 0249: Public repository, history scrub, and repository layout

Status: Accepted (owner, 2026-10-17: development stays in the private
josephspurrier/xo; a publisher agent prepares and commits a scrubbed
mirror with the full history, the owner reviews it and pushes it to
github.com/xo-lang/xo by hand; issues open, code pull requests not merged directly. Owner,
2026-10-16: MIT, std/ under MIT-0; selfhost/ becomes compiler/;
knowledgebase/ stays; scripts/lead/ stays private for now)

## Context

The owner wants Xo public, with its full git history, because the
history shows what AI agents did to the code. Agents work in the
private github.com/josephspurrier/xo, and every session shares the
owner's SSH key and `gh` login, which can push to and administer
anything the owner owns. An agent must never be able to publish by
accident.

An audit of every blob in the object store (2026-10-16, regex scans plus
gitleaks over all refs) found no credentials: the private keys it flags
are self-signed test fixtures, and the GitHub App key and webhook secret
appear only as SSM parameter names. It did find values that should not
be public:

- the SSH allowlist IP in infra/ci-aws/main.tf and toolchain.md;
- the AWS account ID and the bucket names built from it
  (infra/ci-runners/providers.tf, infra/state/backend.tf, CLAUDE.md, the
  knowledgebase);
- EC2 instance, security group and AMI IDs in the knowledgebase and
  results;
- the Mac's home paths;
- output/finan-logos/, images of an unrelated project;
- bench/perf/llvm/go/llperfgo, a Mach-O binary committed by mistake
  (2026-10-08) that embeds a home path; filter-repo's text replacement
  skips binaries.

The account ID and IP first appear at 792809c0, so a scrub changes the
hash of almost every commit.

The layout also carries what the Go compiler left behind (decision
0215): internal/check, fix, ssa, parser, edit, cbind and format hold
only testdata the Xo compiler's tests read; internal/ mixes Go support
code with test-only packages; test/ and the internal/*/testdata trees
are two homes for compiler test inputs. There was no LICENSE file, and
std/rt vendors golang.org/x/crypto code without its BSD notice.

## Decision

1. **Development stays private.** Sessions, worktrees, the lead's
   merges, CI (infra/ci-runners, the GitHub App) and the seeds keep
   using josephspurrier/xo exactly as before. It is permanent and must
   never be made public or transferred: it holds the raw values and the
   unscrubbed history. xo-lang/xo is a downstream mirror of main and the
   tags, never a place to develop.
2. **Publishable tree.** Commits on the private main remove the values
   from the code, so the public tree works without them and they do not
   keep coming back:
   - xotool names no seed bucket: `$XO_SEED_URL`, else
     `s3://$XO_SEED_BUCKET`, else the bucket on the first line of
     `<user config dir>/xo/seed-bucket` (outside every checkout, so
     sessions whose environment predates XO_SEED_BUCKET still find
     it), else seed fetching is off (and the build
     fails loud, decision 0216). The owner's machines export
     XO_SEED_BUCKET; CI reads the repository variable XO_SEED_BUCKET;
     scripts/seed-upload.sh requires it; infra/ci-aws/ci.sh requires
     XO_AWS_ACCOUNT.
   - Terraform: no defaults for the account, the seed bucket or the SSH
     address (a git-ignored `terraform.tfvars` supplies them), and the
     S3 backends take their bucket from a git-ignored `backend.hcl`
     (`terraform init -backend-config=backend.hcl`).
   - CLAUDE.md points to the untracked `CLAUDE.local.md`, which holds
     the account ID, the buckets and the address.
   - LICENSE (MIT), std/LICENSE (MIT-0, so programs built with Xo need
     not carry a notice), THIRD_PARTY_NOTICES (Go's BSD-3-Clause license
     for the vendored x/crypto files and the code ported from Go), and
     .gitleaks.toml (allowlist of the known false positives, plus rules
     for account-ID bucket names and EC2 IDs).
3. **The scrub.** `git filter-repo` in ../xo-publish, a separate clone
   that is the only place the public repository's URL is configured,
   with `--replace-text` and `--replace-message` (IP to 203.0.113.10,
   account ID to ACCOUNT_ID, home paths to ~, EC2 IDs to placeholders)
   and `--invert-paths` for output/, scripts/lead/ (the session
   orchestration scripts, private for now) and the stray binary.
   Authors, dates, messages and every other byte stay. The rules and
   tools live in ../xo-publish-tools, outside every repository, because
   the rules contain the raw values.
4. **Incremental publishing.** The first publish rewrites the whole
   history (rehearsed 2026-10-16: 2,075 commits, no leftovers, every
   tree equal to the original after the replacements). Each later
   publish rewrites only the commits since the last one (filter-repo
   `--state-branch`), so a published commit never changes hash, and a
   rule added later applies from then on. A publish that would not
   fast-forward the public main is refused.
5. **Hashes inside files.** Public commits have different hashes from
   private ones. filter-repo rewrites hashes in commit messages; the
   publisher also maps the hashes quoted in files (scripts/seed-chain.txt,
   which the build reads, and the hashes in CLAUDE.md, the knowledgebase,
   ADRs and CHANGELOG) to their public commits, and keeps the private to
   public commit map so either can be looked up. Inside the private
   repository, sessions, memory and the knowledgebase keep using private
   hashes; anything written for outsiders (the website, release notes,
   replies to issues) links public commits.
6. **The agent commits, the owner reviews and pushes.** A publisher
   agent prepares and commits each publish in ../xo-publish: the
   incremental rewrite, gitleaks with .gitleaks.toml,
   a read-through of every new diff and message (a new private value
   becomes a new rule, then the rewrite runs again), the purity check
   (each public tree differs from its private one only by the rules and
   the removed paths), the fast-forward check, and a summary of what
   the publish adds. It stops there. The owner reviews the summary and
   the new commits, then pushes by hand; no agent pushes. Accidental publishing is
   prevented by things an agent cannot do, not by instructions:
   - the push uses a deploy key on xo-lang/xo whose passphrase only the
     owner knows (or a Keychain item that asks the owner on every use);
   - a ruleset on xo-lang/xo lets only that deploy key push or create
     refs; the owner's account is not on the bypass list, so the shared
     SSH key and `gh` login are refused;
   - no private checkout or worktree has xo-lang/xo as a remote, and a
     pre-push hook in the private repository refuses xo-lang URLs (a
     last guard only: `--no-verify` skips it);
   - the first publish goes to xo-lang/xo while it is private; the
     owner checks it and makes it public as a separate step.
7. **Published refs.** main and the tags (v*, go-compiler-final).
   Publishing happens on demand and at each version tag.
8. **Contributions.** Issues are welcome and are where outside work
   matters most: agents turn a good report into a fix. Code pull
   requests are not merged on xo-lang/xo, since the next publish would
   overwrite them. A patch is treated as a proposal: an agent applies
   it to the private repository with `git am`, which keeps the
   contributor as the author through the mirror, or reimplements it
   with a `Suggested-by:` or `Co-authored-by:` trailer. Inbound code is
   MIT (inbound equals outbound) and needs a `Signed-off-by` (the DCO).
   CONTRIBUTING.md and SECURITY.md (private vulnerability reporting)
   say so. A fix for a public issue carries `Fixes xo-lang/xo#N`.
   Agents read public issues with `gh`; they never write to xo-lang/xo.
9. **Layout.** Ordinary commits in the private repository, whenever the
   owner schedules them: a pure move commit (so renames are detected
   and `git log --follow` works), then a commit that fixes paths. The
   mirror carries them like any other commit. Target:

   ```
   README.md  LICENSE  CHANGELOG.md  CONTRIBUTING.md  SECURITY.md  CLAUDE.md
   spec/            unchanged
   compiler/        from selfhost/ (module github.com/xo-lang/xo/compiler)
   std/             unchanged (xo finds std at bin/../std)
   tests/
     compiler/      from internal/{check,fix,ssa,parser,edit,cbind,format}/testdata
     codegen/       from test/codegen
     errorcheck/    from test/errorcheck
     drivers/       Go packages that drive the xo binary: internal/{gen,llvm,
                    arm64,selfhost,repotest,devgoldens}, cmd/xo
   internal/        Go support only: xotool, gotool, gobind, progen, testtmp,
                    mod, benchharness
   cmd/xo-gobind/   unchanged
   scripts/         unchanged (lead/ stays out of the mirror)
   bench/           unchanged; bench/paper/ is never edited or moved
   infra/           unchanged, values in git-ignored files (item 2)
   knowledgebase/   unchanged
   examples/  editors/   unchanged
   ```

   The rename of selfhost/ touches the module path, xobuild's source
   key list, scripts/bootstrap-xo.sh, CLAUDE.md's fixed point path list,
   scripts/ci/suite.sh and ci.yml; the path fix-up commit changes them
   all, and the fixed point proves it.

## Consequences

- No freeze, no re-clone and no change for sessions; the private
  repository, its CI and its seeds stay as they are.
- Two hash spaces: private hashes inside the private repository, public
  hashes for outsiders, joined by the publisher's commit map.
- Publishing needs the owner each time; that is the point.
- A private value that reaches a published commit cannot be removed
  without rewriting public history, so the publisher's review is the
  only gate after a commit is made; gitleaks with .gitleaks.toml in the
  private repository catches the known kinds earlier.
- Contributors' patches arrive through a port, not a merge button; if a
  community grows, a later ADR can move to developing in the public
  repository with a protected main.
- Terraform runs need the git-ignored backend.hcl and terraform.tfvars;
  agents need XO_SEED_BUCKET (CLAUDE.local.md) for seeds.
- Outside contributors cannot read the private seed bucket, so they
  build xo from source (XO_SEED_FALLBACK=chain, 25 to 37 minutes) until
  seeds are published somewhere public, a separate decision.
- Code ported from Go keeps Go's BSD-3-Clause terms; MIT-0 covers only
  the code written for Xo, so a program built with Xo that links those
  parts carries the same notice duty as a Go program.

## Alternatives

- **Move development to the public repository** (this ADR's first
  draft, 2026-10-16): one repository and normal pull requests, but a
  freeze and re-clone, and every session's push would publish at once,
  with no review between a commit and the world.
- **Public repository with a protected main and a sanitizing merge
  agent:** one hash space and real pull requests, but the merge agent's
  review is the only gate before publication, and with one shared
  GitHub identity on the Mac, protection depends on a deploy key the
  agent holds. Kept as the path if outside code contributions grow.
- **Re-run the whole rewrite on every publish:** simpler, but a new rule
  that matches an old commit changes every later public hash.
- **One squashed commit:** leak free, but loses the history the owner
  wants to show.
- **Publish the history unscrubbed and fix forward:** leaves the IP and
  account ID in public history.
- **Restructure inside the rewrite (filter-repo path renames):** old
  commits would no longer build (their scripts and xo.mod files name the
  old paths), and the scrub would no longer be checkable as a pure
  replacement.
