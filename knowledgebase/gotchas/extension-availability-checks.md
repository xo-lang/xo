# Checking if a file extension is free

Status: Observed (2026-10-07, while naming the language)

GitHub Linguist's `languages.yml` lists only source language extensions. It
did not flag `.so`, `.ko`, `.lo`, `.vo`, `.fo`, or `.kix`, which are all in use.
Combine Linguist with known binary and build artifact extensions before
declaring an extension free.
