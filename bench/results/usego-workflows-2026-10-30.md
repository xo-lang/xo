# Go workflows suite (`use go`), 2026-10-30

What a backend author does with popular Go modules, written in Xo and run
end to end on the Go backend: scripts/usego-workflows.sh XO OUTDIR runs
each directory of bench/usego-workflows (tidy, check, tidy, build
--backend=go, run; the program prints `PASS NAME`), and
bench/usego-workflows/EXPECTED lists the ones expected to fail with the
work that fixes them (XFAIL; XPASS when one starts passing).

Go go1.26.1 darwin/arm64, xo 0.34.0 (main at 2524383cc, binding format
28).

| workflow | modules | result | what it needed |
|---|---|---|---|
| chi | github.com/go-chi/chi/v5 v5.5.0 | PASS | a Mux as net/http's Handler: the 0416 addendum (e05a3a101, `as_handler`) |
| zap | go.uber.org/zap v1.28.0 | PASS | the constraint interface ObjectMarshalerPtr[T] not bound as a type (Lang 1, 4977319c6) |
| aws | aws-sdk-go-v2 v1.47.5, service/s3 v1.114.6, aws/protocol/eventstream v1.7.25, credentials v1.20.12 | PASS | nested module pins (5bde67d29), functional options (0422, b7e789808), `*string` as `Str?` (0419), the upcast widening (`as_credentials_provider`, 2524383cc) |
| pgx | github.com/jackc/pgx/v5 v5.11.0 | XFAIL | decision 0412 (`...any` arguments, Scan destinations) |
| toml | github.com/BurntSushi/toml v1.6.0 | XFAIL | decision 0412 (go.Any, go.out for Decode) |
| yaml | gopkg.in/yaml.v3 v3.0.1 | XFAIL | decision 0412 (go.Any, go.out for Unmarshal) |

aws runs an S3 put and get against a local stub (path style, a static
credentials provider); chi serves a router with a URL parameter, a mounted
sub router and a 404 through httptest; zap builds a production config and
logs.

Program-side lessons (what an agent writing these had to know):
`resp.body().as_reader()` for io.read_all; `let _ =` on a Result needs
`.ignore_err(...)` (XO0206); functional options are a List of Xo
functions (`s3.new_from_config(cfg, [fn(o: s3.Options) { ... }])`).
