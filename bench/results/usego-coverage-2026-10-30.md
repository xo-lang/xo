# `use go` coverage: output buffers as a length in, the filled Bytes out, 2026-10-30

The survey set and method of usego-coverage-2026-10-29b.md (the Windows tester's 47 Go std packages
and 40 third-party modules, one project, the same pins, `go run ./cmd/xo-gobind --dir PROJ --json
PKG...` over the 87 paths, Go 1.26.1, darwin/arm64; the harness is survey.sh and summarize.py of
Lang 1's survey, re-run by Xo lang 5). Before: main at 82a509844 (binding format 31). After: decision
0445 (binding format 32): a `[]byte` Go writes into is an `Int` length in and the bytes Go wrote out.
Percent = functions + methods bound / exported.

Summary: std 90.3% to 92.5% (1898 to 1944 of 2102), third party 98.6% to 98.7% (12477 to 12480 of
12649). The tester's 0.35.0 figures (89.9% std, 98.6% third party) were measured with Go 1.27.0.
"output buffer" in std went from 55 to 9: io.Reader's read on every type, read_at, io.read_full,
io.read_at_least, crypto/rand.read, hex and base64 encode and decode now bind; still skipped are
the four ReadMsg* methods (two buffers) and sort's five in place sorts (the same category, decision
in 0.35.0). The largest std reasons left: pointers to non handle types (66, of which 49 are
sync/atomic and flag functions writing through a pointer, kept skipped by decision 0419), generic
functions (52, Go's slices and maps by decision 0415), context.Context outside the first parameter
(13: context's constructors).

## std: 47 packages: before 1898/2102 = 90.3%, after 1944/2102 = 92.5%

| package | before | after |
|---|---|---|
| io | 51/65 | 65/65 |
| net | 230/247 | 243/247 |
| bytes | 92/95 | 95/95 |
| encoding/hex | 11/13 | 13/13 |
| os | 126/131 | 128/131 |
| encoding/base64 | 12/14 | 14/14 |
| bufio | 60/62 | 62/62 |
| strings | 77/79 | 79/79 |
| fmt | 36/37 | 37/37 |
| crypto/tls | 62/65 | 63/65 |
| net/http | 151/153 | 152/153 |
| math/rand/v2 | 52/54 | 53/54 |
| crypto/rand | 3/4 | 4/4 |
| compress/gzip | 10/11 | 11/11 |
| slices | 0/40 | 0/40 |
| maps | 0/10 | 0/10 |
| errors | 5/6 | 5/6 |
| context | 5/16 | 5/16 |
| sync/atomic | 36/86 | 36/86 |
| log | 34/34 | 34/34 |
| time | 30/37 | 30/37 |
| encoding/json | 35/35 | 35/35 |
| crypto/aes | 2/2 | 2/2 |
| flag | 56/74 | 56/74 |
| container/heap | 10/10 | 10/10 |
| database/sql | 89/89 | 89/89 |
| sync | 36/36 | 36/36 |
| log/slog | 105/108 | 105/108 |
| encoding/binary | 25/25 | 25/25 |
| html/template | 30/30 | 30/30 |
| encoding/xml | 37/37 | 37/37 |
| text/template | 33/33 | 33/33 |
| mime | 8/8 | 8/8 |
| path/filepath | 21/21 | 21/21 |
| sort | 31/36 | 31/36 |
| os/exec | 25/25 | 25/25 |
| archive/zip | 34/34 | 34/34 |
| strconv | 34/36 | 34/36 |
| net/url | 40/40 | 40/40 |
| math | 67/67 | 67/67 |
| regexp | 48/48 | 48/48 |
| unicode/utf8 | 15/15 | 15/15 |
| encoding/csv | 12/12 | 12/12 |
| crypto/sha256 | 4/4 | 4/4 |
| crypto/hmac | 2/2 | 2/2 |
| net/mail | 10/10 | 10/10 |
| hash/crc32 | 6/6 | 6/6 |

skip reasons before (204):
- 66 (32%) pointer to a non handle type
- 55 (27%) output buffer ([]byte Go writes into)
- 52 (25%) generic function or type
- 13 (6%) context.Context outside the first parameter
- 12 (6%) unsupported basic type (complex, uintptr, unsafe.Pointer)
- 3 (1%) channel
- 2 (1%) method of a named basic type (the type maps to its basic type)
- 1 (0%) function result or unmappable callback

skip reasons after (158):
- 66 (42%) pointer to a non handle type
- 52 (33%) generic function or type
- 13 (8%) context.Context outside the first parameter
- 12 (8%) unsupported basic type (complex, uintptr, unsafe.Pointer)
- 9 (6%) output buffer ([]byte Go writes into)
- 3 (2%) channel
- 2 (1%) method of a named basic type (the type maps to its basic type)
- 1 (1%) function result or unmappable callback

## tp: 40 packages: before 12477/12649 = 98.6%, after 12480/12649 = 98.7%

| package | before | after |
|---|---|---|
| github.com/jackc/pgx/v5 | 107/119 | 108/119 |
| github.com/klauspost/compress/zstd | 53/55 | 54/55 |
| golang.org/x/text/cases | 9/10 | 10/10 |
| github.com/hashicorp/golang-lru/v2 | 28/28 | 28/28 |
| github.com/stretchr/testify/assert | 316/316 | 316/316 |
| github.com/samber/lo | 343/414 | 343/414 |
| github.com/pkg/errors | 12/14 | 12/14 |
| github.com/sirupsen/logrus | 247/248 | 247/248 |
| golang.org/x/crypto/bcrypt | 6/6 | 6/6 |
| github.com/BurntSushi/toml | 25/25 | 25/25 |
| go.uber.org/zap | 171/186 | 171/186 |
| github.com/tidwall/sjson | 10/10 | 10/10 |
| gopkg.in/yaml.v3 | 19/19 | 19/19 |
| github.com/fsnotify/fsnotify | 10/12 | 10/12 |
| github.com/gorilla/websocket | 43/43 | 43/43 |
| modernc.org/sqlite | 31/31 | 31/31 |
| github.com/pelletier/go-toml/v2 | 30/32 | 30/32 |
| github.com/robfig/cron/v3 | 31/36 | 31/36 |
| github.com/redis/go-redis/v9 | 9291/9331 | 9291/9331 |
| github.com/google/uuid | 56/59 | 56/59 |
| github.com/expr-lang/expr | 24/25 | 24/25 |
| github.com/golang-jwt/jwt/v5 | 128/130 | 128/130 |
| github.com/rs/zerolog | 248/253 | 248/253 |
| github.com/google/go-cmp/cmp | 48/49 | 48/49 |
| github.com/spf13/cobra | 160/161 | 160/161 |
| golang.org/x/text/language | 65/65 | 65/65 |
| github.com/go-chi/chi/v5 | 73/75 | 73/75 |
| github.com/microcosm-cc/bluemonday | 36/40 | 36/40 |
| github.com/tidwall/gjson | 36/36 | 36/36 |
| github.com/shopspring/decimal | 110/110 | 110/110 |
| github.com/Masterminds/semver/v3 | 37/37 | 37/37 |
| github.com/xuri/excelize/v2 | 206/206 | 206/206 |
| github.com/PuerkitoBio/goquery | 355/355 | 355/355 |
| github.com/cespare/xxhash/v2 | 14/14 | 14/14 |
| github.com/dustin/go-humanize | 27/27 | 27/27 |
| github.com/gofrs/flock | 17/17 | 17/17 |
| github.com/mailru/easyjson | 16/16 | 16/16 |
| github.com/yuin/goldmark | 15/15 | 15/15 |
| golang.org/x/crypto/argon2 | 2/2 | 2/2 |
| golang.org/x/time/rate | 22/22 | 22/22 |

skip reasons before (172):
- 31 (18%) generic function or type
- 24 (14%) function result or unmappable callback
- 21 (12%) pointer to a non handle type
- 18 (10%) unexported type
- 14 (8%) channel
- 12 (7%) name collision after snake_case
- 11 (6%) more than 6 results
- 11 (6%) unsupported basic type (complex, uintptr, unsafe.Pointer)
- 10 (6%) unnamed interface or struct
- 9 (5%) context.Context outside the first parameter
- 3 (2%) output buffer ([]byte Go writes into)
- 3 (2%) type from another package beyond the binding depth
- 3 (2%) pointer receiver on a value type
- 2 (1%) method of a named basic type (the type maps to its basic type)

skip reasons after (169):
- 31 (18%) generic function or type
- 24 (14%) function result or unmappable callback
- 21 (12%) pointer to a non handle type
- 18 (11%) unexported type
- 14 (8%) channel
- 12 (7%) name collision after snake_case
- 11 (7%) more than 6 results
- 11 (7%) unsupported basic type (complex, uintptr, unsafe.Pointer)
- 10 (6%) unnamed interface or struct
- 9 (5%) context.Context outside the first parameter
- 3 (2%) type from another package beyond the binding depth
- 3 (2%) pointer receiver on a value type
- 2 (1%) method of a named basic type (the type maps to its basic type)

