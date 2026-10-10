# GitHub Pages: first deploy fails, then the domain check goes stale

Status: Observed (2026-10-10)

Symptom: the first run of the site's `pages` workflow failed in
`actions/configure-pages` with "Get Pages site failed ... Not Found". After
Pages was enabled and the run re-run, http://xolang.dev served the site, but
no HTTPS certificate was issued, so the `.dev` domain (HTTPS only in every
browser) could not be opened. The Pages health API said "Domain does not
resolve to the GitHub Pages server (NotServedByPagesError)" although public
DNS returned only GitHub's four addresses.

Cause: the repo was pushed before Pages was enabled with "GitHub Actions" as
its source, so the first run had no Pages site. The custom domain check ran
before the Namecheap records propagated and GitHub keeps that result.

Fix: enable Pages (source: GitHub Actions) before the first push, or re-run
the failed run (`gh run rerun <id>`). For the certificate, remove the custom
domain in Settings, Pages, save, add it again, and save; the certificate
then arrives within the hour. Check with
`gh api repos/xo-lang/xolang.dev/pages/health` and
`gh api repos/xo-lang/xolang.dev/pages --jq .https_certificate.state`.
