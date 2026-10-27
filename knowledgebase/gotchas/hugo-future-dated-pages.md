# Hugo silently drops a page dated in the future

Status: Observed (several times; latest 2026-10-27)

Symptom: a new xolang.dev page (a release page or a blog post) is
missing from the built site. `hugo` exits 0 with no warning, the page's
directory is absent from the output, and links to it 404. On 2026-10-27
the 0.33 release page was dated 17:00 while it was built at 15:50 and was
left out until the date was moved back.

Cause: Hugo skips content whose front matter `date` is later than the
build time (`buildFuture` defaults to false). A release page or a post
written ahead of the publish time, or dated with a planned time or a
timezone offset that puts it in the future, is not rendered, and the
GitHub Pages build does the same.

Fix: date a page at or before the moment it is built and published,
with an explicit offset (for example `2026-10-27T15:50:00-04:00`, the
current local time), never a planned later time. After building, check
the page's output exists: `ls PUBLIC_DIR/releases/X.Y/index.html` or
`ls PUBLIC_DIR/blog/SLUG/index.html`. `hugo --buildFuture` shows whether a
missing page is future dated, but do not publish with it on.
