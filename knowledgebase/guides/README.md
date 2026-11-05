# Guides

Short guides for people coming to Xo from another language; the website
copies them as docs pages.

- [option-result-for-go.md](option-result-for-go.md): Go's `(T, error)`, `(T, bool)` and error wrapping habits against Xo's `?`, `??`, `.or_err`, `.map_err`, `.ok()`, `.err()`, `r ?? e =>` and `if let`, and the first errors (XO0204, XO0205, XO0601, XO0406).
- [effectful-closures.md](effectful-closures.md): storing closures that have effects: effect parameters on structs and interfaces, the threaded `send[fx: Effects](req: Request[fx]) uses (..., fx)` form, and the five errors on the way with their fixes (decision 0522).
